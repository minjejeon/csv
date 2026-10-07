package csv

import (
	"strconv"
	"strings"
	"testing"
)

type TestGenericItem struct {
	ID   int
	Name string
}

func (item *TestGenericItem) CSVHeader() []string {
	return []string{"id", "name"}
}

func (item *TestGenericItem) MarshalCSVRecord(w *Writer) error {
	w.WriteFieldBytes(strconv.AppendInt(nil, int64(item.ID), 10))
	w.WriteDelimiter()
	w.WriteFieldBytes([]byte(item.Name))
	return nil
}

func (item *TestGenericItem) UnmarshalCSVRecord(rec *Record) error {
	id, err := rec.FieldInt(0)
	if err != nil {
		return err
	}
	item.ID = id
	item.Name = rec.FieldString(1)
	return nil
}

func TestGenericMarshalSlice(t *testing.T) {
	items := []TestGenericItem{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}

	data, err := MarshalSlice[TestGenericItem, *TestGenericItem](items)
	if err != nil {
		t.Fatal(err)
	}

	expected := "id,name\n1,Alice\n2,Bob\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}

	// Round-trip verification with UnmarshalSlice
	parsed, err := UnmarshalSlice[TestGenericItem, *TestGenericItem](data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 || parsed[0].Name != "Alice" || parsed[1].Name != "Bob" {
		t.Fatalf("round-trip failed: %+v", parsed)
	}
}

func TestGenericParallelMarshalSlice(t *testing.T) {
	items := make([]TestGenericItem, 1000)
	for i := range items {
		items[i] = TestGenericItem{ID: i, Name: "Item"}
	}

	seqData, err := MarshalSlice[TestGenericItem, *TestGenericItem](items)
	if err != nil {
		t.Fatal(err)
	}

	parData, err := ParallelMarshalSlice[TestGenericItem, *TestGenericItem](items, ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}

	if string(seqData) != string(parData) {
		t.Fatal("ParallelMarshalSlice output does not match MarshalSlice")
	}

	lines := strings.Split(strings.TrimSpace(string(parData)), "\n")
	if len(lines) != 1001 {
		t.Fatalf("expected 1001 lines, got %d", len(lines))
	}
}

func TestGenericMarshalSliceEmptyWithHeaderProvider(t *testing.T) {
	var empty []TestGenericItem
	data, err := MarshalSlice[TestGenericItem, *TestGenericItem](empty)
	if err != nil {
		t.Fatalf("MarshalSlice failed: %v", err)
	}
	expected := "id,name\n"
	if string(data) != expected {
		t.Errorf("MarshalSlice(empty) = %q; want %q", string(data), expected)
	}

	parData, err := ParallelMarshalSlice[TestGenericItem, *TestGenericItem](empty)
	if err != nil {
		t.Fatalf("ParallelMarshalSlice failed: %v", err)
	}
	if string(parData) != expected {
		t.Errorf("ParallelMarshalSlice(empty) = %q; want %q", string(parData), expected)
	}
}
