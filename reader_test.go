package csv

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReaderZeroAllocStreaming(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping zero alloc test under race detector")
	}

	csvData := strings.Repeat("john,doe,30,123.45,true\n", 1000)

	sr := strings.NewReader(csvData)
	r := NewReader(sr)
	allocs := testing.AllocsPerRun(10, func() {
		sr.Reset(csvData)
		r.Reset(sr)
		for {
			rec, err := r.ReadRecord()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.NumFields() != 5 {
				t.Fatalf("expected 5 fields, got %d", rec.NumFields())
			}
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations per streaming pass, got %f", allocs)
	}
}

func TestReaderSlidingBufferAcrossBoundary(t *testing.T) {
	// Generate lines that exceed default buffer or small buffer sizes
	var b bytes.Buffer
	for i := 0; i < 500; i++ {
		b.WriteString("col1_val,col2_val_with_some_longer_content,col3_number_9999\n")
	}

	r := NewReader(&b)
	r.bufSize = 128 // Force very small 128-byte buffer to trigger frequent sliding

	count := 0
	for {
		rec, err := r.ReadRecord()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error at row %d: %v", count, err)
		}
		if rec.NumFields() != 3 {
			t.Fatalf("row %d: expected 3 fields, got %d", count, rec.NumFields())
		}
		if string(rec.Field(0)) != "col1_val" {
			t.Fatalf("row %d: col 0 = %q, want col1_val", count, string(rec.Field(0)))
		}
		count++
	}

	if count != 500 {
		t.Fatalf("expected 500 rows, got %d", count)
	}
}
