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
							if f.tag.omitEmpty && len(raw) == 0 {
								continue
							}
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
							if f.tag.omitEmpty && len(raw) == 0 {
								continue
							}
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
