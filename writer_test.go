package csv

import (
	"bytes"
	"errors"
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

func TestWriterCommaMutation(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Comma = ';'
	if err := w.Write([]string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "a;b\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterQuoteMutation(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Quote = '\''
	if err := w.Write([]string{"has,comma", "plain"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "'has,comma',plain\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriterMultiCharDelimQuotingPrecision(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, WithDelimiter("||"))
	// Single pipe should not trigger quotes when delimiter is "||"
	if err := w.Write([]string{"single|pipe", "has||delim"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	expected := "single|pipe||\"has||delim\"\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

type errWriter struct{}

func (ew *errWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write disk failure")
}

func TestWriterErrorMethod(t *testing.T) {
	ew := &errWriter{}
	w := NewWriter(ew)
	_ = w.Write([]string{"foo", "bar"})
	err := w.Flush()
	if err == nil {
		t.Fatal("expected flush error")
	}
	if w.Error() == nil || w.Error().Error() != "write disk failure" {
		t.Fatalf("w.Error() expected 'write disk failure', got %v", w.Error())
	}
}

