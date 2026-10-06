package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

// DefaultParallelWorkers is the default number of worker goroutines for parallel parsing.
const DefaultParallelWorkers = 4

// ParallelOptions configures multithreaded CSV execution.
type ParallelOptions struct {
	Workers int  // Number of worker goroutines (defaults to 4)
	Ordered bool // If true (default), preserves original CSV row ordering
}

// ParallelUnmarshal parses CSV data concurrently across multiple worker goroutines.
func ParallelUnmarshal(data []byte, v any, opts ...ParallelOptions) error {
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

	numWorkers := DefaultParallelWorkers
	if len(opts) > 0 && opts[0].Workers > 0 {
		numWorkers = opts[0].Workers
	}

	chunks, err := splitChunks(data, numWorkers)
	if err != nil {
		return err
	}

	if len(chunks) <= 1 {
		return Unmarshal(data, v)
	}

	// 1. Worker 0 reads headers and compiles typePlan
	r0 := NewReader(bytes.NewReader(data[chunks[0].start:chunks[0].end]))
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
				r = NewReader(bytes.NewReader(data[span.start:span.end]))
			}
			defer r.Close()

			localSlice := reflect.MakeSlice(sliceVal.Type(), 0, 128)

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
					localSlice = reflect.Append(localSlice, newElem)
				} else {
					newElem := reflect.New(structType).Elem()
					structPtr := newElem.Addr().UnsafePointer()

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
					localSlice = reflect.Append(localSlice, newElem)
				}
			}

			chunkResults[chunkIdx] = localSlice
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
