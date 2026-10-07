package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"unsafe"
)

// DefaultParallelWorkers is the default number of worker goroutines for parallel parsing.
const DefaultParallelWorkers = 4

// ParallelOptions configures multithreaded CSV execution.
type ParallelOptions struct {
	Workers int      // Number of worker goroutines (defaults to 4)
	Ordered bool     // If true (default), preserves original CSV row ordering
	Options []Option // CSV Reader options (delimiter, quote, etc.)
}

func parseParallelAndCsvOpts(opts []any) (ParallelOptions, []Option) {
	parOpts := ParallelOptions{Ordered: true}
	var csvOpts []Option
	for _, opt := range opts {
		switch o := opt.(type) {
		case ParallelOptions:
			parOpts = o
			csvOpts = append(csvOpts, o.Options...)
		case *ParallelOptions:
			if o != nil {
				parOpts = *o
				csvOpts = append(csvOpts, o.Options...)
			}
		case Option:
			csvOpts = append(csvOpts, o)
		case []Option:
			csvOpts = append(csvOpts, o...)
		}
	}
	return parOpts, csvOpts
}

// ParallelUnmarshal parses CSV data concurrently across multiple worker goroutines.
func ParallelUnmarshal(data []byte, v any, opts ...any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return fmt.Errorf("csv: ParallelUnmarshal expects a non-nil pointer, got %T", v)
	}

	sliceVal := val.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("csv: ParallelUnmarshal expects a pointer to a slice, got %T", v)
	}

	elemType := sliceVal.Type().Elem()
	isPtrElem := false
	structType := elemType

	if elemType.Kind() == reflect.Pointer {
		isPtrElem = true
		structType = elemType.Elem()
	}

	if structType.Kind() != reflect.Struct {
		return fmt.Errorf("csv: slice elements must be structs or pointers to structs, got %v", elemType)
	}

	parOpts, csvOpts := parseParallelAndCsvOpts(opts)
	numWorkers := DefaultParallelWorkers
	if parOpts.Workers > 0 {
		numWorkers = parOpts.Workers
	}

	dummy := NewReader(nil, csvOpts...)
	if dummy.initErr != nil {
		return dummy.initErr
	}
	if dummy.encoding != nil {
		utf8Data, err := dummy.encoding.NewDecoder().Bytes(data)
		if err != nil {
			return err
		}
		data = utf8Data
		var filteredOpts []any
		if parOpts.Workers > 0 {
			filteredOpts = append(filteredOpts, parOpts)
		}
		for _, opt := range csvOpts {
			d := &Reader{}
			opt(d)
			if d.encoding == nil {
				filteredOpts = append(filteredOpts, opt)
			}
		}
		return ParallelUnmarshal(data, v, filteredOpts...)
	}
	quoteByte := dummy.quoteByte

	chunks, err := splitChunks(data, numWorkers, quoteByte)
	if err != nil {
		return err
	}

	if len(chunks) <= 1 {
		return Unmarshal(data, v, csvOpts...)
	}

	// 1. Worker 0 reads headers and compiles typePlan
	r0 := NewReader(bytes.NewReader(data[chunks[0].start:chunks[0].end]), csvOpts...)
	headers, err := r0.Read()
	if err != nil {
		r0.Close()
		return err
	}

	plan, err := getTypePlan(structType, headers)
	if err != nil {
		r0.Close()
		return err
	}

	numChunks := len(chunks)
	chunkResults := make([]reflect.Value, numChunks)

	var wg sync.WaitGroup
	var once sync.Once
	var workerErr error

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		chunkIdx := i
		span := chunks[i]

		go func() {
			defer wg.Done()

			var r *Reader
			if chunkIdx == 0 {
				r = r0 // already past header
			} else {
				r = NewReader(bytes.NewReader(data[span.start:span.end]), csvOpts...)
			}
			defer r.Close()

			c := countRecords(data[span.start:span.end], quoteByte)
			if chunkIdx == 0 && c > 1 {
				c--
			}
			if c < 16 {
				c = 16
			}

			localSlice := reflect.MakeSlice(sliceVal.Type(), c, c)
			sliceBasePtr := localSlice.Index(0).Addr().UnsafePointer()
			elemSize := structType.Size()
			ptrSize := unsafe.Sizeof(uintptr(0))

			rowIdx := 0
			for {
				rec, err := r.ReadRecord()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					once.Do(func() {
						workerErr = err
					})
					return
				}

				if rowIdx >= localSlice.Len() {
					newCap := localSlice.Len() * 2
					if newCap < 16 {
						newCap = 16
					}
					newSlice := reflect.MakeSlice(sliceVal.Type(), newCap, newCap)
					reflect.Copy(newSlice, localSlice)
					localSlice = newSlice
					sliceBasePtr = localSlice.Index(0).Addr().UnsafePointer()
				}

				numFields := rec.NumFields()

				if isPtrElem {
					newElem := reflect.New(structType)
					structPtr := newElem.UnsafePointer()

					for _, f := range plan.fields {
						if f.colIndex < numFields {
							raw := rec.Field(f.colIndex)
							if err := f.setter(structPtr, raw); err != nil {
								once.Do(func() {
									workerErr = fmt.Errorf("csv: error parsing field %q: %w", f.fieldName, err)
								})
								return
							}
						}
					}
					*(*unsafe.Pointer)(unsafe.Add(sliceBasePtr, uintptr(rowIdx)*ptrSize)) = structPtr
				} else {
					structPtr := unsafe.Add(sliceBasePtr, uintptr(rowIdx)*elemSize)

					for _, f := range plan.fields {
						if f.colIndex < numFields {
							raw := rec.Field(f.colIndex)
							if err := f.setter(structPtr, raw); err != nil {
								once.Do(func() {
									workerErr = fmt.Errorf("csv: error parsing field %q: %w", f.fieldName, err)
								})
								return
							}
						}
					}
				}

				rowIdx++
			}

			chunkResults[chunkIdx] = localSlice.Slice(0, rowIdx)
		}()
	}

	wg.Wait()

	if workerErr != nil {
		return workerErr
	}

	// 2. Concatenate chunkResults in order
	totalRows := 0
	for _, cs := range chunkResults {
		totalRows += cs.Len()
	}

	finalSlice := reflect.MakeSlice(sliceVal.Type(), totalRows, totalRows)
	destIdx := 0
	for _, cs := range chunkResults {
		l := cs.Len()
		if l > 0 {
			reflect.Copy(finalSlice.Slice(destIdx, destIdx+l), cs)
			destIdx += l
		}
	}

	val.Elem().Set(finalSlice)
	return nil
}

func parseParallelAndWriterOpts(opts []any) (ParallelOptions, []any) {
	parOpts := ParallelOptions{Workers: DefaultParallelWorkers, Ordered: true}
	var otherOpts []any
	for _, opt := range opts {
		switch o := opt.(type) {
		case ParallelOptions:
			parOpts = o
			for _, co := range o.Options {
				otherOpts = append(otherOpts, co)
			}
		case *ParallelOptions:
			if o != nil {
				parOpts = *o
				for _, co := range o.Options {
					otherOpts = append(otherOpts, co)
				}
			}
		default:
			otherOpts = append(otherOpts, opt)
		}
	}
	if parOpts.Workers <= 0 {
		parOpts.Workers = DefaultParallelWorkers
	}
	return parOpts, otherOpts
}

// ParallelMarshal encodes a slice of structs or pointers to structs concurrently across multiple workers.
func ParallelMarshal(v any, opts ...any) ([]byte, error) {
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return nil, errors.New("csv: ParallelMarshal(nil)")
	}
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, fmt.Errorf("csv: ParallelMarshal expects a slice or array, got %v", val.Kind())
	}

	elemType := val.Type().Elem()
	isPtr := false
	structType := elemType
	if elemType.Kind() == reflect.Pointer {
		isPtr = true
		structType = elemType.Elem()
	}

	if structType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("csv: slice elements must be structs or pointers to structs, got %v", elemType)
	}

	n := val.Len()
	if n == 0 {
		return nil, nil
	}

	parOpts, otherOpts := parseParallelAndWriterOpts(opts)

	// If single worker or very small dataset, use sequential Marshal
	if parOpts.Workers <= 1 || n < parOpts.Workers {
		return Marshal(v, opts...)
	}

	plan, err := getTypeMarshalPlan(structType)
	if err != nil {
		return nil, err
	}

	workers := parOpts.Workers
	if workers > n {
		workers = n
	}
	chunkSize := (n + workers - 1) / workers
	numChunks := (n + chunkSize - 1) / chunkSize

	type chunkOutput struct {
		buf []byte
		err error
	}
	outputs := make([]chunkOutput, numChunks)
	var wg sync.WaitGroup
	wg.Add(numChunks)

	elemSize := structType.Size()
	ptrSize := unsafe.Sizeof(uintptr(0))

	var sliceBasePtr unsafe.Pointer
	if val.Kind() == reflect.Slice {
		sliceBasePtr = val.UnsafePointer()
	} else {
		if val.CanAddr() {
			sliceBasePtr = val.Addr().UnsafePointer()
		} else {
			newVal := reflect.New(val.Type()).Elem()
			newVal.Set(val)
			sliceBasePtr = newVal.Addr().UnsafePointer()
		}
	}

	for c := 0; c < numChunks; c++ {
		start := c * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}

		go func(chunkIdx, startIdx, endIdx int) {
			defer wg.Done()
			var buf bytes.Buffer
			buf.Grow((endIdx - startIdx) * 64)

			w := NewWriter(&buf, otherOpts...)
			defer w.Close()

			if chunkIdx == 0 {
				if err := w.Write(plan.headerRow); err != nil {
					outputs[chunkIdx].err = err
					return
				}
			}

			for i := startIdx; i < endIdx; i++ {
				var structPtr unsafe.Pointer
				if isPtr {
					structPtr = *(*unsafe.Pointer)(unsafe.Add(sliceBasePtr, uintptr(i)*ptrSize))
					if structPtr == nil {
						for j := range plan.fields {
							if j > 0 {
								w.WriteDelimiter()
							}
						}
						if err := w.WriteNewline(); err != nil {
							outputs[chunkIdx].err = err
							return
						}
						continue
					}
				} else {
					structPtr = unsafe.Add(sliceBasePtr, uintptr(i)*elemSize)
				}

				for j, f := range plan.fields {
					if j > 0 {
						w.WriteDelimiter()
					}
					if err := f.getter(structPtr, w); err != nil {
						outputs[chunkIdx].err = fmt.Errorf("csv: error encoding field %s: %w", f.colName, err)
						return
					}
				}
				if err := w.WriteNewline(); err != nil {
					outputs[chunkIdx].err = err
					return
				}
			}

			if err := w.Flush(); err != nil {
				outputs[chunkIdx].err = err
				return
			}
			outputs[chunkIdx].buf = buf.Bytes()
		}(c, start, end)
	}

	wg.Wait()

	for _, out := range outputs {
		if out.err != nil {
			return nil, out.err
		}
	}

	totalLen := 0
	for _, out := range outputs {
		totalLen += len(out.buf)
	}

	result := make([]byte, totalLen)
	dest := 0
	for _, out := range outputs {
		copy(result[dest:], out.buf)
		dest += len(out.buf)
	}
	return result, nil
}

