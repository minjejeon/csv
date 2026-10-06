package csv

import (
	"errors"
	"io"
	"runtime"
	"sync"
)

// BatchRecord holds a batch of parsed CSV rows with its sequence index.
type BatchRecord struct {
	Index int
	Rows  [][]string
	Err   error
}

// ParallelReader provides parallel record streaming over an io.Reader.
type ParallelReader struct {
	r          *Reader
	opts       ParallelOptions
	batchSize  int
	outCh      chan BatchRecord
	closeOnce  sync.Once
	doneCh     chan struct{}
	nextIndex  int
	pending    map[int][][]string
	err        error
	eofReached bool
}

// NewParallelReader starts streaming records from r using worker goroutines.
func NewParallelReader(r io.Reader, opts ...ParallelOptions) (*ParallelReader, error) {
	numWorkers := runtime.GOMAXPROCS(0)
	ordered := true
	if len(opts) > 0 {
		if opts[0].Workers > 0 {
			numWorkers = opts[0].Workers
		}
		ordered = opts[0].Ordered
	}

	reader := NewReader(r)
	batchSize := 500

	pr := &ParallelReader{
		r:         reader,
		opts:      ParallelOptions{Workers: numWorkers, Ordered: ordered},
		batchSize: batchSize,
		outCh:     make(chan BatchRecord, numWorkers*2),
		doneCh:    make(chan struct{}),
		pending:   make(map[int][][]string),
	}

	go pr.producerLoop()
	return pr, nil
}

func (pr *ParallelReader) producerLoop() {
	defer close(pr.outCh)
	defer pr.r.Close()

	batchIdx := 0

	for {
		select {
		case <-pr.doneCh:
			return
		default:
		}

		rows := make([][]string, 0, pr.batchSize)
		var loopErr error

		for i := 0; i < pr.batchSize; i++ {
			row, err := pr.r.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					loopErr = io.EOF
					break
				}
				loopErr = err
				break
			}
			rows = append(rows, row)
		}

		if len(rows) > 0 || loopErr != nil {
			select {
			case <-pr.doneCh:
				return
			case pr.outCh <- BatchRecord{
				Index: batchIdx,
				Rows:  rows,
				Err:   loopErr,
			}:
				batchIdx++
			}
		}

		if loopErr != nil {
			return
		}
	}
}

// ReadBatch reads and returns the next batch of CSV rows in order.
func (pr *ParallelReader) ReadBatch() ([][]string, error) {
	if pr.eofReached {
		return nil, io.EOF
	}

	if pr.opts.Ordered {
		// Return from pending map if next index is already present
		if rows, ok := pr.pending[pr.nextIndex]; ok {
			delete(pr.pending, pr.nextIndex)
			pr.nextIndex++
			return rows, nil
		}

		for {
			item, ok := <-pr.outCh
			if !ok {
				pr.eofReached = true
				if pr.err != nil {
					return nil, pr.err
				}
				return nil, io.EOF
			}

			if item.Err != nil && !errors.Is(item.Err, io.EOF) {
				pr.err = item.Err
			}

			if item.Index == pr.nextIndex {
				pr.nextIndex++
				if len(item.Rows) == 0 && item.Err != nil {
					pr.eofReached = true
					return nil, item.Err
				}
				return item.Rows, nil
			}

			// Store out-of-order batch
			pr.pending[item.Index] = item.Rows
			if rows, ok := pr.pending[pr.nextIndex]; ok {
				delete(pr.pending, pr.nextIndex)
				pr.nextIndex++
				return rows, nil
			}
		}
	} else {
		item, ok := <-pr.outCh
		if !ok {
			return nil, io.EOF
		}
		if len(item.Rows) == 0 && item.Err != nil {
			return nil, item.Err
		}
		return item.Rows, nil
	}
}

// Close terminates background goroutines and releases resources.
func (pr *ParallelReader) Close() error {
	pr.closeOnce.Do(func() {
		close(pr.doneCh)
	})
	return nil
}
