package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestParallelReaderStreaming(t *testing.T) {
	// 5000 rows
	var buf bytes.Buffer
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&buf, "id_%d,name_%d,value_%d\n", i, i, i*2)
	}
	data := buf.Bytes()

	pr, err := NewParallelReader(bytes.NewReader(data), ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatalf("NewParallelReader failed: %v", err)
	}
	defer pr.Close()

	totalRows := 0
	expectedID := 0

	for {
		batch, err := pr.ReadBatch()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("ReadBatch error: %v", err)
		}

		for _, row := range batch {
			wantID := fmt.Sprintf("id_%d", expectedID)
			if row[0] != wantID {
				t.Fatalf("row %d id mismatch: got %q, want %q", expectedID, row[0], wantID)
			}
			expectedID++
			totalRows++
		}
	}

	if totalRows != 5000 {
		t.Fatalf("expected 5000 rows, got %d", totalRows)
	}
}

func TestParallelReaderClose(t *testing.T) {
	data := []byte("a,b\n1,2\n3,4\n5,6\n")
	pr, err := NewParallelReader(bytes.NewReader(data), ParallelOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := pr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
