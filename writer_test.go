package csv

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriterBasic(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	err := w.Write([]string{"name", "age", "city"})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Write([]string{"Alice", "30", "New York, NY"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "name,age,city\nAlice,30,\"New York, NY\"\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterEscapedQuotes(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	err := w.Write([]string{"quote", "val"})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Write([]string{"he said \"hello\"", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "quote,val\n\"he said \"\"hello\"\"\",ok\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterCustomDelimiterAndQuote(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, WithDelimiter("|"), WithQuote('\''))
	err := w.Write([]string{"a|b", "c'd", "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "'a|b'|'c''d'|plain\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterCRLF(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.UseCRLF = true
	err := w.Write([]string{"x", "y"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "x,y\r\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterAutoFlush(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for i := 0; i < 2000; i++ {
		err := w.Write([]string{"field1", "field2", "field3"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2000 {
		t.Fatalf("expected 2000 lines, got %d", len(lines))
	}
}
