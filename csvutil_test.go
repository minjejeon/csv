package csv

import (
	"reflect"
	"testing"
	"time"
)

type CsvutilItem struct {
	ID          int       `csv:"id"`
	Title       string    `csv:"title"`
	Price       float64   `csv:"price"`
	InStock     bool      `csv:"in_stock"`
	Description *string   `csv:"description,omitempty"`
	Tag         string    `csv:"-"`
	Date        time.Time `csv:"date"`
}

func TestUnmarshalValueSlice(t *testing.T) {
	data := []byte(`id,title,price,in_stock,description,date
1,Book,15.99,true,Good book,2026-05-01T00:00:00Z
2,Pen,1.50,false,,2026-05-02T00:00:00Z
`)

	var items []CsvutilItem
	if err := Unmarshal(data, &items); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Item 1
	if items[0].ID != 1 || items[0].Title != "Book" || items[0].Price != 15.99 || !items[0].InStock {
		t.Errorf("item 0 mismatch: %+v", items[0])
	}
	if items[0].Description == nil || *items[0].Description != "Good book" {
		t.Errorf("item 0 Description mismatch: %v", items[0].Description)
	}

	// Item 2
	if items[1].ID != 2 || items[1].Title != "Pen" || items[1].Price != 1.50 || items[1].InStock {
		t.Errorf("item 1 mismatch: %+v", items[1])
	}
	if items[1].Description != nil {
		t.Errorf("item 1 expected nil Description, got %v", *items[1].Description)
	}
}

func TestUnmarshalPointerSlice(t *testing.T) {
	data := []byte(`id,title,price,in_stock,date
10,Laptop,999.00,true,2026-03-10T12:00:00Z
20,Mouse,25.50,true,2026-03-11T12:00:00Z
`)

	var items []*CsvutilItem
	if err := Unmarshal(data, &items); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != 10 || items[1].ID != 20 {
		t.Errorf("items mismatch: %+v, %+v", items[0], items[1])
	}
}

type TagMismatchItem struct {
	A string `csv:"col_a"`
	B string `csv:"col_b"`
}

func TestUnmarshalReorderedHeaders(t *testing.T) {
	data := []byte(`col_b,col_a
valB1,valA1
valB2,valA2
`)

	var items []TagMismatchItem
	if err := Unmarshal(data, &items); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expected := []TagMismatchItem{
		{A: "valA1", B: "valB1"},
		{A: "valA2", B: "valB2"},
	}

	if !reflect.DeepEqual(items, expected) {
		t.Fatalf("got %+v, want %+v", items, expected)
	}
}

func TestUnmarshalInvalidTarget(t *testing.T) {
	data := []byte("a,b\n1,2\n")

	var notSlice int
	if err := Unmarshal(data, &notSlice); err == nil {
		t.Fatal("expected error when unmarshaling into int")
	}

	var notPtr []TagMismatchItem
	if err := Unmarshal(data, notPtr); err == nil {
		t.Fatal("expected error when unmarshaling into non-pointer")
	}
}
