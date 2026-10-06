package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

type CustomRecord struct {
	ID    int    `csv:"id"`
	Name  string `csv:"name"`
	Notes string `csv:"notes"`
}

func (cr *CustomRecord) UnmarshalCSVRecord(rec *Record) error {
	if rec.NumFields() < 3 {
		return fmt.Errorf("expected 3 fields, got %d", rec.NumFields())
	}
	id, err := strconv.Atoi(string(rec.Field(0)))
	if err != nil {
		return err
	}
	cr.ID = id
	cr.Name = string(rec.Field(1))
	cr.Notes = string(rec.Field(2))
	return nil
}

func TestCustomDelimDecoder(t *testing.T) {
	data := "id||name||notes\n1||'Alice'||'Hello || World'\n2||'Bob'||'Multi\nLine'\n"
	dec, err := NewDecoder(strings.NewReader(data), WithDelimiter("||"), WithQuote('\''))
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close()

	var rows []CustomRecord
	for {
		var row CustomRecord
		err := dec.Decode(&row)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("Decode failed: %v", err)
		}
		rows = append(rows, row)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Name != "Alice" || rows[0].Notes != "Hello || World" {
		t.Errorf("row 0 mismatch: %+v", rows[0])
	}
	if rows[1].Name != "Bob" || rows[1].Notes != "Multi\nLine" {
		t.Errorf("row 1 mismatch: %+v", rows[1])
	}
}

func TestCustomDelimUnmarshal(t *testing.T) {
	data := []byte("id::name::notes\n10::'Charlie'::'Note::1'\n20::'Dave'::'Note''s'\n")
	var rows []CustomRecord
	err := Unmarshal(data, &rows, WithDelimiter("::"), WithQuote('\''))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID != 10 || rows[0].Name != "Charlie" || rows[0].Notes != "Note::1" {
		t.Errorf("row 0 mismatch: %+v", rows[0])
	}
	if rows[1].ID != 20 || rows[1].Name != "Dave" || rows[1].Notes != "Note's" {
		t.Errorf("row 1 mismatch: %+v", rows[1])
	}
}

func TestCustomDelimGenericUnmarshal(t *testing.T) {
	data := []byte("id||name||notes\n100||Eve||Great\n200||Frank||Awesome\n")
	rows, err := UnmarshalSlice[CustomRecord](data, WithDelimiter("||"))
	if err != nil {
		t.Fatalf("UnmarshalSlice failed: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID != 100 || rows[0].Name != "Eve" {
		t.Errorf("row 0 mismatch: %+v", rows[0])
	}
	if rows[1].ID != 200 || rows[1].Name != "Frank" {
		t.Errorf("row 1 mismatch: %+v", rows[1])
	}
}

func TestCustomDelimParallelUnmarshal(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("id|name|notes\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&buf, "%d|'User_%d'|'Note | %d'\n", i, i, i)
	}
	data := buf.Bytes()

	var rows []CustomRecord
	err := ParallelUnmarshal(data, &rows, WithDelimiter("|"), WithQuote('\''))
	if err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(rows) != 300 {
		t.Fatalf("expected 300 rows, got %d", len(rows))
	}
	for i := 0; i < 300; i++ {
		if rows[i].ID != i || rows[i].Name != fmt.Sprintf("User_%d", i) || rows[i].Notes != fmt.Sprintf("Note | %d", i) {
			t.Fatalf("row %d mismatch: %+v", i, rows[i])
		}
	}
}

func TestCustomDelimParallelGenericUnmarshal(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("id||name||notes\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&buf, "%d||'User_%d'||'Note || %d'\n", i, i, i)
	}
	data := buf.Bytes()

	rows, err := ParallelUnmarshalSlice[CustomRecord](data, WithDelimiter("||"), WithQuote('\''))
	if err != nil {
		t.Fatalf("ParallelUnmarshalSlice failed: %v", err)
	}

	if len(rows) != 300 {
		t.Fatalf("expected 300 rows, got %d", len(rows))
	}
	for i := 0; i < 300; i++ {
		if rows[i].ID != i || rows[i].Name != fmt.Sprintf("User_%d", i) || rows[i].Notes != fmt.Sprintf("Note || %d", i) {
			t.Fatalf("row %d mismatch: %+v", i, rows[i])
		}
	}
}
