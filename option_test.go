package csv

import (
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	r := NewReader(strings.NewReader(""),
		WithDelimiter("||"),
		WithQuote('\''),
		WithComment('#'),
		WithLazyQuotes(true),
		WithTrimLeadingSpace(true),
		WithFieldsPerRecord(5),
	)

	if r.Delimiter != "||" {
		t.Errorf("Delimiter = %q, want '||'", r.Delimiter)
	}
	if r.Quote != '\'' {
		t.Errorf("Quote = %q, want \"'\"", r.Quote)
	}
	if r.Comment != '#' {
		t.Errorf("Comment = %q, want '#'", r.Comment)
	}
	if !r.LazyQuotes {
		t.Error("LazyQuotes = false, want true")
	}
	if !r.TrimLeadingSpace {
		t.Error("TrimLeadingSpace = false, want true")
	}
	if r.FieldsPerRecord != 5 {
		t.Errorf("FieldsPerRecord = %d, want 5", r.FieldsPerRecord)
	}

	// Test WithComma sets Delimiter as well
	r2 := NewReader(strings.NewReader(""), WithComma(';'))
	if r2.Comma != ';' || r2.Delimiter != ";" {
		t.Errorf("Comma = %q, Delimiter = %q, want ';'", r2.Comma, r2.Delimiter)
	}
}
