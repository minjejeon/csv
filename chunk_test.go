package csv

import (
	"bytes"
	"fmt"
	"testing"
)

func TestSplitChunksBasic(t *testing.T) {
	// 100 rows
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&buf, "%d,user_%d,val_%d\n", i, i, i*10)
	}
	data := buf.Bytes()

	chunks, err := splitChunks(data, 4)
	if err != nil {
		t.Fatalf("splitChunks failed: %v", err)
	}

	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}

	// Verify contiguous coverage
	if chunks[0].start != 0 {
		t.Errorf("chunk 0 start = %d, want 0", chunks[0].start)
	}
	for i := 0; i < len(chunks)-1; i++ {
		if chunks[i].end != chunks[i+1].start {
			t.Errorf("gap between chunk %d (end=%d) and %d (start=%d)",
				i, chunks[i].end, i+1, chunks[i+1].start)
		}
		// Each boundary must be at the start of a line (preceding byte must be \n)
		if chunks[i+1].start > 0 && data[chunks[i+1].start-1] != '\n' {
			t.Errorf("chunk %d start (%d) does not follow a newline: char=%q",
				i+1, chunks[i+1].start, data[chunks[i+1].start-1])
		}
	}
	if chunks[len(chunks)-1].end != len(data) {
		t.Errorf("last chunk end = %d, want %d", chunks[len(chunks)-1].end, len(data))
	}
}

func TestSplitChunksMultilineQuote(t *testing.T) {
	// Create CSV where a multiline quote spans across the middle of the dataset
	var buf bytes.Buffer
	buf.WriteString("id,notes,val\n")
	buf.WriteString("1,simple line,10\n")
	// Large multiline field
	buf.WriteString("2,\"multiline\ntext\nspanning\nmultiple\nnewlines\",20\n")
	buf.WriteString("3,another line,30\n")
	data := buf.Bytes()

	// Try splitting with 2 workers
	chunks, err := splitChunks(data, 2)
	if err != nil {
		t.Fatalf("splitChunks failed: %v", err)
	}

	for i := 1; i < len(chunks); i++ {
		start := chunks[i].start
		// The split point must NOT be inside the multiline quotes of row 2!
		// Row 2 starts around byte 31 and ends around byte 90.
		// Check that the chunk starts at a line beginning, and if we read from start,
		// it is a valid row.
		r := NewReader(bytes.NewReader(data[start:chunks[i].end]))
		rec, err := r.Read()
		if err != nil {
			t.Fatalf("chunk %d failed to parse first row: %v", i, err)
		}
		if len(rec) != 3 {
			t.Fatalf("chunk %d first row expected 3 fields, got %d: %v", i, len(rec), rec)
		}
	}
}

func TestSplitChunksEdgeCases(t *testing.T) {
	// Empty data
	chunks, err := splitChunks(nil, 4)
	if err != nil || len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for nil data, got %v, err=%v", chunks, err)
	}

	// 1 worker
	data := []byte("a,b,c\n1,2,3\n")
	chunks, err = splitChunks(data, 1)
	if err != nil || len(chunks) != 1 || chunks[0].start != 0 || chunks[0].end != len(data) {
		t.Fatalf("expected 1 chunk covering entire data, got %+v", chunks)
	}

	// Data smaller than workers
	chunks, err = splitChunks(data, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 || len(chunks) > 2 {
		t.Fatalf("expected 1-2 chunks for tiny data, got %d", len(chunks))
	}
}
