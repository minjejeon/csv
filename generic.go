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
	if out == nil {
		return errors.New("csv: out pointer must not be nil")
	}
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
	if out == nil {
		return errors.New("csv: out pointer must not be nil")
	}
	parOpts, csvOpts := parseParallelAndCsvOpts(opts)
	numWorkers := DefaultParallelWorkers
	if parOpts.Workers > 0 {
		numWorkers = parOpts.Workers
	}

	cfg, err := parseReaderConfig(csvOpts)
	if err != nil {
		return err
	}
	if cfg.encoding != nil {
		utf8Data, err := cfg.encoding.NewDecoder().Bytes(data)
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
		return ParallelUnmarshalTo[T, PT](data, out, filteredOpts...)
	}
	quoteByte := cfg.quoteByte

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

// RecordMarshaler is implemented by types that marshal themselves into a CSV record.
type RecordMarshaler interface {
	MarshalCSVRecord(w *Writer) error
}

// HeaderProvider is an optional interface implemented by types that declare their CSV column headers.
type HeaderProvider interface {
	CSVHeader() []string
}

// MarshalSlice encodes a slice of T into CSV bytes with zero reflection overhead.
// T's pointer (*T) must implement RecordMarshaler.
func MarshalSlice[T any, PT interface {
	*T
	RecordMarshaler
}](slice []T, opts ...any) ([]byte, error) {
	n := len(slice)
	var zero T
	var header []string
	if hp, ok := any(PT(&zero)).(HeaderProvider); ok {
		header = hp.CSVHeader()
	}

	if n == 0 {
		if len(header) == 0 {
			return nil, nil
		}
		var buf bytes.Buffer
		w := NewWriter(&buf, opts...)
		if err := w.Write(header); err != nil {
			_ = w.Close()
			return nil, err
		}
		if err := w.Flush(); err != nil {
			_ = w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var buf bytes.Buffer
	buf.Grow(n * 64)
	w := NewWriter(&buf, opts...)

	if len(header) > 0 {
		if err := w.Write(header); err != nil {
			return nil, err
		}
	}

	for i := 0; i < n; i++ {
		ptr := PT(&slice[i])
		if err := ptr.MarshalCSVRecord(w); err != nil {
			return nil, err
		}
		if err := w.WriteNewline(); err != nil {
			return nil, err
		}
	}

	if err := w.Flush(); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ParallelMarshalSlice encodes a slice of T concurrently across multiple workers with zero reflection overhead.
// T's pointer (*T) must implement RecordMarshaler.
func ParallelMarshalSlice[T any, PT interface {
	*T
	RecordMarshaler
}](slice []T, opts ...any) ([]byte, error) {
	n := len(slice)
	if n == 0 {
		return MarshalSlice[T, PT](slice, opts...)
	}

	parOpts, otherOpts := parseParallelAndWriterOpts(opts)
	if parOpts.Workers <= 1 || n < parOpts.Workers {
		return MarshalSlice[T, PT](slice, opts...)
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

	var zero T
	var header []string
	if hp, ok := any(PT(&zero)).(HeaderProvider); ok {
		header = hp.CSVHeader()
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

			if chunkIdx == 0 && len(header) > 0 {
				if err := w.Write(header); err != nil {
					outputs[chunkIdx].err = err
					_ = w.Close()
					return
				}
			}

			for i := startIdx; i < endIdx; i++ {
				ptr := PT(&slice[i])
				if err := ptr.MarshalCSVRecord(w); err != nil {
					outputs[chunkIdx].err = err
					_ = w.Close()
					return
				}
				if err := w.WriteNewline(); err != nil {
					outputs[chunkIdx].err = err
					_ = w.Close()
					return
				}
			}

			if err := w.Flush(); err != nil {
				outputs[chunkIdx].err = err
				_ = w.Close()
				return
			}
			if err := w.Close(); err != nil {
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

