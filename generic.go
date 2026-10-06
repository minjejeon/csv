package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
)

// RecordUnmarshaler is implemented by types that unmarshal a CSV record into themselves.
type RecordUnmarshaler interface {
	UnmarshalCSVRecord(rec *Record) error
}

// UnmarshalSlice unmarshals CSV data directly into a slice of T with zero reflection overhead.
// T's pointer (*T) must implement RecordUnmarshaler.
func UnmarshalSlice[T any, PT interface {
	*T
	RecordUnmarshaler
}](data []byte, opts ...Option) ([]T, error) {
	var out []T
	if err := UnmarshalTo[T, PT](data, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// UnmarshalTo unmarshals CSV data directly into the provided slice pointer with zero reflection overhead.
// T's pointer (*T) must implement RecordUnmarshaler.
func UnmarshalTo[T any, PT interface {
	*T
	RecordUnmarshaler
}](data []byte, out *[]T, opts ...Option) error {
	r := NewReader(bytes.NewReader(data), opts...)
	defer r.Close()

	c := countRecords(data, r.quoteByte)
	if c > 1 {
		c-- // exclude header
	}
	if c < 16 {
		c = 16
	}

	// 100% static compile-time allocation without reflection
	slice := make([]T, 0, c)

	// Read and discard header
	if _, err := r.Read(); err != nil {
		if errors.Is(err, io.EOF) {
			*out = slice
			return nil
		}
		return err
	}

	for {
		rec, err := r.ReadRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		idx := len(slice)
		if idx >= cap(slice) {
			newCap := cap(slice) * 2
			if newCap < 16 {
				newCap = 16
			}
			newSlice := make([]T, len(slice), newCap)
			copy(newSlice, slice)
			slice = newSlice
		}

		slice = slice[:idx+1]
		ptr := PT(&slice[idx])
		if err := ptr.UnmarshalCSVRecord(rec); err != nil {
			return fmt.Errorf("csv: error unmarshaling record %d: %w", idx, err)
		}
	}

	*out = slice
	return nil
}

// ParallelUnmarshalSlice concurrently unmarshals CSV data into a slice of T with zero reflection.
func ParallelUnmarshalSlice[T any, PT interface {
	*T
	RecordUnmarshaler
}](data []byte, opts ...any) ([]T, error) {
	var out []T
	if err := ParallelUnmarshalTo[T, PT](data, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// ParallelUnmarshalTo concurrently unmarshals CSV data into the provided slice pointer with zero reflection.
func ParallelUnmarshalTo[T any, PT interface {
	*T
	RecordUnmarshaler
}](data []byte, out *[]T, opts ...any) error {
	parOpts, csvOpts := parseParallelAndCsvOpts(opts)
	numWorkers := DefaultParallelWorkers
	if parOpts.Workers > 0 {
		numWorkers = parOpts.Workers
	}

	dummy := NewReader(nil, csvOpts...)
	quoteByte := dummy.quoteByte

	chunks, err := splitChunks(data, numWorkers, quoteByte)
	if err != nil {
		return err
	}

	if len(chunks) <= 1 {
		return UnmarshalTo[T, PT](data, out, csvOpts...)
	}

	numChunks := len(chunks)
	chunkResults := make([][]T, numChunks)

	var (
		wg        sync.WaitGroup
		once      sync.Once
		workerErr error
	)

	// Worker 0 reads header first
	r0 := NewReader(bytes.NewReader(data[chunks[0].start:chunks[0].end]), csvOpts...)
	if _, err := r0.Read(); err != nil {
		r0.Close()
		return err
	}

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		chunkIdx := i
		span := chunks[i]

		go func() {
			defer wg.Done()

			var r *Reader
			if chunkIdx == 0 {
				r = r0
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

			localSlice := make([]T, 0, c)

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

				idx := len(localSlice)
				if idx >= cap(localSlice) {
					newCap := cap(localSlice) * 2
					if newCap < 16 {
						newCap = 16
					}
					newSlice := make([]T, len(localSlice), newCap)
					copy(newSlice, localSlice)
					localSlice = newSlice
				}

				localSlice = localSlice[:idx+1]
				ptr := PT(&localSlice[idx])
				if err := ptr.UnmarshalCSVRecord(rec); err != nil {
					once.Do(func() {
						workerErr = fmt.Errorf("csv: error unmarshaling record in chunk %d: %w", chunkIdx, err)
					})
					return
				}
			}

			chunkResults[chunkIdx] = localSlice
		}()
	}

	wg.Wait()

	if workerErr != nil {
		return workerErr
	}

	totalRows := 0
	for _, cs := range chunkResults {
		totalRows += len(cs)
	}

	finalSlice := make([]T, totalRows)
	destIdx := 0
	for _, cs := range chunkResults {
		copy(finalSlice[destIdx:destIdx+len(cs)], cs)
		destIdx += len(cs)
	}

	*out = finalSlice
	return nil
}
