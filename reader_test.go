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

func TestResetRecordStart(t *testing.T) {
	data1 := append(bytes.Repeat([]byte("x"), 40000), []byte("\na,b\n")...)
	r := NewReader(bytes.NewReader(data1))
	_, _ = r.ReadRecord()
	_, _ = r.ReadRecord()

	// Reset with a new reader starting with newline - previously caused slice bounds out of range panic
	r.Reset(bytes.NewReader([]byte("\nhello,world\n")))
	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("read after reset failed: %v", err)
	}
	if rec.NumFields() != 2 || string(rec.Field(0)) != "hello" || string(rec.Field(1)) != "world" {
		t.Fatalf("unexpected record after reset: %v", rec)
	}
}

func TestQuoteBoundaryShift(t *testing.T) {
	// Pad so that recordStart is > 32KB and a quoted field has quote at the end of first 64KB read
	pad := bytes.Repeat([]byte("a,b\n"), 7000)                   // ~28KB
	pad2 := append(pad, bytes.Repeat([]byte("c,d\n"), 1800)...) // ~35KB
	needed := defaultBufferSize - len(pad2) - 1                 // 1 byte before 64KB boundary
	recData := append([]byte("\""), bytes.Repeat([]byte("x"), needed-1)...)
	recData = append(recData, []byte("\"\"\",rest\n")...) // escaped quote across boundary, then closing quote

	allData := append(pad2, recData...)
	r := NewReader(bytes.NewReader(allData))
	for i := 0; i < 7000+1800; i++ {
		_, err := r.ReadRecord()
		if err != nil {
			t.Fatalf("row %d error: %v", i, err)
		}
	}

	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("quote boundary record failed: %v", err)
	}
	if rec.NumFields() != 2 {
		t.Fatalf("expected 2 fields, got %d", rec.NumFields())
	}
	if string(rec.Field(1)) != "rest" {
		t.Fatalf("field 1 = %q, want 'rest'", string(rec.Field(1)))
	}
}

func TestCRLFBoundaryShift(t *testing.T) {
	pad := bytes.Repeat([]byte("a,b\n"), 7000)                   // ~28KB
	pad2 := append(pad, bytes.Repeat([]byte("c,d\n"), 1800)...) // ~35KB
	needed := defaultBufferSize - len(pad2) - 1                 // 1 byte before 64KB boundary
	recData := bytes.Repeat([]byte("x"), needed)                // field fills up to 65534
	recData = append(recData, []byte("\r\nnext_row\n")...)      // \r at 65535, \n at 65536!

	allData := append(pad2, recData...)
	r := NewReader(bytes.NewReader(allData))
	for i := 0; i < 7000+1800; i++ {
		_, err := r.ReadRecord()
		if err != nil {
			t.Fatalf("row %d error: %v", i, err)
		}
	}

	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("CRLF boundary record failed: %v", err)
	}
	if rec.NumFields() != 1 {
		t.Fatalf("expected 1 field, got %d", rec.NumFields())
	}
	if len(rec.Field(0)) != needed {
		t.Fatalf("field len = %d, want %d", len(rec.Field(0)), needed)
	}

	recNext, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("next record failed: %v", err)
	}
	if string(recNext.Field(0)) != "next_row" {
		t.Fatalf("expected next_row, got %q", string(recNext.Field(0)))
	}
}


