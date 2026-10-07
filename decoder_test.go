package csv

import (
	"errors"
	"io"
	"strconv"
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

func TestDecoderCloseAndReset(t *testing.T) {
	csvData := "a,b\n1,2\n"
	dec, err := NewDecoder(strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}

	var row struct {
		A int `csv:"a"`
		B int `csv:"b"`
	}

	if err := dec.Decode(&row); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if row.A != 1 || row.B != 2 {
		t.Fatalf("unexpected row: %+v", row)
	}

	// Reading past end returns EOF and auto-closes
	err = dec.Decode(&row)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}

	// Double Close is idempotent
	if err := dec.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("unexpected close error on second close: %v", err)
	}

	// Reset after Close re-acquires reader and works
	dec.Reset(strings.NewReader("a,b\n10,20\n"))
	if err := dec.Decode(&row); err != nil {
		t.Fatalf("decode after reset failed: %v", err)
	}
	if row.A != 10 || row.B != 20 {
		t.Fatalf("unexpected row after reset: %+v", row)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestStructuredDecodeError(t *testing.T) {
	csvData := "id,name,age\n1,Alice,30\n2,Bob,not_a_number\n"
	dec, err := NewDecoder(strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}

	var row struct {
		ID   int    `csv:"id"`
		Name string `csv:"name"`
		Age  int    `csv:"age"`
	}

	if err := dec.Decode(&row); err != nil {
		t.Fatalf("row 1 failed: %v", err)
	}

	err = dec.Decode(&row)
	if err == nil {
		t.Fatal("expected error on row 2, got nil")
	}

	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected error to be *DecodeError, got %T: %v", err, err)
	}

	if decErr.Line != 3 {
		t.Errorf("decErr.Line = %d, want 3", decErr.Line)
	}
	if decErr.Column != 2 {
		t.Errorf("decErr.Column = %d, want 2", decErr.Column)
	}
	if decErr.Header != "age" {
		t.Errorf("decErr.Header = %q, want 'age'", decErr.Header)
	}
	if decErr.Field != "Age" {
		t.Errorf("decErr.Field = %q, want 'Age'", decErr.Field)
	}
	if decErr.Value != "not_a_number" {
		t.Errorf("decErr.Value = %q, want 'not_a_number'", decErr.Value)
	}
	if !errors.Is(err, strconv.ErrSyntax) {
		t.Errorf("expected errors.Is(err, strconv.ErrSyntax) to be true, got false")
	}

	wantSubstr := "csv: line 3, column \"age\" (field Age): invalid syntax (value: \"not_a_number\")"
	if decErr.Error() != wantSubstr {
		t.Errorf("decErr.Error() = %q, want %q", decErr.Error(), wantSubstr)
	}

	// Test Unmarshal also returns *DecodeError
	var rows []struct {
		ID  int `csv:"id"`
		Age int `csv:"age"`
	}
	unmarshalErr := Unmarshal([]byte(csvData), &rows)
	var decErr2 *DecodeError
	if !errors.As(unmarshalErr, &decErr2) {
		t.Fatalf("expected Unmarshal error to be *DecodeError, got %T: %v", unmarshalErr, unmarshalErr)
	}
	if decErr2.Line != 3 || decErr2.Value != "not_a_number" {
		t.Errorf("decErr2 mismatch: %+v", decErr2)
	}
}

