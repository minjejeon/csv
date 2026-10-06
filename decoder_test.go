package csv

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type Employee struct {
	ID        int64     `csv:"id"`
	Name      string    `csv:"name"`
	Salary    float64   `csv:"salary"`
	IsActive  bool      `csv:"active"`
	Dept      *string   `csv:"department,omitempty"`
	CreatedAt time.Time `csv:"created_at"`
}

func TestDecoderBasic(t *testing.T) {
	csvData := `id,name,salary,active,department,created_at
101,Alice,75000.50,true,Engineering,2026-01-15T09:00:00Z
102,Bob,62000.00,false,,2026-02-01T10:30:00Z
`

	dec, err := NewDecoder(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}

	var emp1 Employee
	if err := dec.Decode(&emp1); err != nil {
		t.Fatalf("failed to decode row 1: %v", err)
	}

	if emp1.ID != 101 {
		t.Errorf("ID = %d, want 101", emp1.ID)
	}
	if emp1.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", emp1.Name)
	}
	if emp1.Salary != 75000.50 {
		t.Errorf("Salary = %f, want 75000.50", emp1.Salary)
	}
	if !emp1.IsActive {
		t.Errorf("IsActive = %v, want true", emp1.IsActive)
	}
	if emp1.Dept == nil || *emp1.Dept != "Engineering" {
		t.Errorf("Dept = %v, want Engineering", emp1.Dept)
	}
	if emp1.CreatedAt.Year() != 2026 {
		t.Errorf("CreatedAt = %v, want year 2026", emp1.CreatedAt)
	}

	var emp2 Employee
	if err := dec.Decode(&emp2); err != nil {
		t.Fatalf("failed to decode row 2: %v", err)
	}
	if emp2.ID != 102 || emp2.Name != "Bob" || emp2.IsActive {
		t.Errorf("unexpected emp2: %+v", emp2)
	}
	if emp2.Dept != nil {
		t.Errorf("expected nil Dept for empty omitempty, got %v", *emp2.Dept)
	}

	// Next decode should return EOF
	var emp3 Employee
	err = dec.Decode(&emp3)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF on finished stream, got %v", err)
	}
}

type PrimitiveRow struct {
	A int     `csv:"a"`
	B int64   `csv:"b"`
	C float64 `csv:"c"`
	D bool    `csv:"d"`
	E string  `csv:"e"`
}

func TestDecoderZeroAllocRow(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping zero alloc test under race detector")
	}

	// Repeated CSV data
	rowLine := "123,456789,99.95,true,hello_world\n"
	csvData := "a,b,c,d,e\n" + strings.Repeat(rowLine, 1000)

	sr := strings.NewReader(csvData)
	dec, err := NewDecoder(sr)
	if err != nil {
		t.Fatal(err)
	}

	var row PrimitiveRow
	allocs := testing.AllocsPerRun(10, func() {
		sr.Reset(csvData)
		dec.Reset(sr)
		// Decode header
		for dec.More() {
			if err := dec.Decode(&row); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatalf("unexpected decode error: %v", err)
			}
		}
	})

	// Notice: row has a string field "hello_world". In Go, converting []byte to string
	// allocates 1 string object per row unless reusing string intern.
	// For 1000 rows, each row has at most 1 string allocation (and 0 for numeric fields).
	// If string allocation is counted, allocs per 1000 rows is <= 1000.
	// That's <= 1 alloc/row!
	if allocs > 1005 {
		t.Fatalf("expected <= 1 alloc/row (1000 for string creation), got %f", allocs)
	}
}
