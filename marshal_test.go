package csv

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

type TestUserMarshal struct {
	ID        int64     `csv:"id"`
	Name      string    `csv:"name"`
	Age       int       `csv:"age"`
	Salary    float64   `csv:"salary"`
	Active    bool      `csv:"active"`
	Bio       string    `csv:"bio,omitempty"`
	CreatedAt time.Time `csv:"created_at"`
}

func TestMarshalBasic(t *testing.T) {
	tm, _ := time.Parse(time.RFC3339, "2026-10-07T00:00:00Z")
	users := []TestUserMarshal{
		{ID: 1, Name: "Alice", Age: 30, Salary: 50000.5, Active: true, Bio: "Engineer", CreatedAt: tm},
		{ID: 2, Name: "Bob, Jr.", Age: 25, Salary: 60000.0, Active: false, Bio: "Quotes \"here\"", CreatedAt: tm},
	}

	data, err := Marshal(users)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), string(data))
	}

	if lines[0] != "id,name,age,salary,active,bio,created_at" {
		t.Fatalf("header mismatch: %s", lines[0])
	}
	if lines[1] != "1,Alice,30,50000.5,true,Engineer,2026-10-07T00:00:00Z" {
		t.Fatalf("row 1 mismatch: %s", lines[1])
	}
	if lines[2] != "2,\"Bob, Jr.\",25,60000,false,\"Quotes \"\"here\"\"\",2026-10-07T00:00:00Z" {
		t.Fatalf("row 2 mismatch: %s", lines[2])
	}
}

func TestMarshalPointersAndOmitEmpty(t *testing.T) {
	type PtrStruct struct {
		ID   int     `csv:"id"`
		Name *string `csv:"name,omitempty"`
		Tag  string  `csv:"tag,omitempty"`
	}

	name := "Alice"
	items := []PtrStruct{
		{ID: 1, Name: &name, Tag: "vip"},
		{ID: 2, Name: nil, Tag: ""},
	}

	data, err := Marshal(items)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	expected := "id,name,tag\n1,Alice,vip\n2,,\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestEncoderStreaming(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	u1 := TestUserMarshal{ID: 1, Name: "Alice", Age: 30, Salary: 100.0, Active: true}
	u2 := TestUserMarshal{ID: 2, Name: "Bob", Age: 40, Salary: 200.0, Active: false}

	if err := enc.Encode(u1); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(&u2); err != nil {
		t.Fatal(err)
	}
	if err := enc.Flush(); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
}

func TestMarshalArray(t *testing.T) {
	arr := [2]TestUserMarshal{
		{ID: 1, Name: "Alice", Age: 30, Salary: 100.0, Active: true},
		{ID: 2, Name: "Bob", Age: 40, Salary: 200.0, Active: false},
	}
	data, err := Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), string(data))
	}
}

func TestMarshalCustomDelimQuote(t *testing.T) {
	type SimpleItem struct {
		ID   int    `csv:"id"`
		Name string `csv:"name"`
	}
	items := []SimpleItem{
		{ID: 1, Name: "Alice, the 'great'"},
	}
	data, err := Marshal(items, WithDelimiter("|"), WithQuote('\''))
	if err != nil {
		t.Fatal(err)
	}
	expected := "id|name\n1|'Alice, the ''great'''\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestByteSliceFieldMarshalAndUnmarshal(t *testing.T) {
	type Document struct {
		ID       int     `csv:"id"`
		RawData  []byte  `csv:"raw_data"`
		Optional []byte  `csv:"optional,omitempty"`
		PtrData  *[]byte `csv:"ptr_data,omitempty"`
	}

	ptr := []byte("pointer-bytes")
	docs := []Document{
		{
			ID:       1,
			RawData:  []byte("hello world"),
			Optional: []byte("extra"),
			PtrData:  &ptr,
		},
		{
			ID:       2,
			RawData:  []byte("second line"),
			Optional: nil,
			PtrData:  nil,
		},
	}

	// 1. Test Marshal
	data, err := Marshal(docs)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	expected := "id,raw_data,optional,ptr_data\n1,hello world,extra,pointer-bytes\n2,second line,,\n"
	if string(data) != expected {
		t.Fatalf("Marshal mismatch:\ngot:  %s\nwant: %s", string(data), expected)
	}

	// 2. Test Unmarshal
	var parsed []Document
	if err := Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed documents, got %d", len(parsed))
	}
	if string(parsed[0].RawData) != "hello world" || string(parsed[0].Optional) != "extra" || parsed[0].PtrData == nil || string(*parsed[0].PtrData) != "pointer-bytes" {
		t.Errorf("doc 0 mismatch: %+v", parsed[0])
	}
	if string(parsed[1].RawData) != "second line" || len(parsed[1].Optional) != 0 || parsed[1].PtrData != nil {
		t.Errorf("doc 1 mismatch: %+v", parsed[1])
	}
}

