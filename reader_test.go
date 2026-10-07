package csv

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReaderZeroAllocStreaming(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping zero alloc test under race detector")
	}

	csvData := strings.Repeat("john,doe,30,123.45,true\n", 1000)

	sr := strings.NewReader(csvData)
	r := NewReader(sr)
	allocs := testing.AllocsPerRun(10, func() {
		sr.Reset(csvData)
		r.Reset(sr)
		for {
			rec, err := r.ReadRecord()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.NumFields() != 5 {
				t.Fatalf("expected 5 fields, got %d", rec.NumFields())
			}
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations per streaming pass, got %f", allocs)
	}
}

func TestReaderSlidingBufferAcrossBoundary(t *testing.T) {
	// Generate lines that exceed default buffer or small buffer sizes
	var b bytes.Buffer
	for i := 0; i < 500; i++ {
		b.WriteString("col1_val,col2_val_with_some_longer_content,col3_number_9999\n")
	}

	r := NewReader(&b)
	r.bufSize = 128 // Force very small 128-byte buffer to trigger frequent sliding

	count := 0
	for {
		rec, err := r.ReadRecord()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error at row %d: %v", count, err)
		}
		if rec.NumFields() != 3 {
			t.Fatalf("row %d: expected 3 fields, got %d", count, rec.NumFields())
		}
		if string(rec.Field(0)) != "col1_val" {
			t.Fatalf("row %d: col 0 = %q, want col1_val", count, string(rec.Field(0)))
		}
		count++
	}

	if count != 500 {
		t.Fatalf("expected 500 rows, got %d", count)
	}
}

func TestResetRecordStart(t *testing.T) {
	data1 := append(bytes.Repeat([]byte("x"), 40000), []byte("\na,b\n")...)
	r := NewReader(bytes.NewReader(data1))
	_, _ = r.ReadRecord()
	_, _ = r.ReadRecord()

	// Reset with a new reader starting with newline - previously caused slice bounds out of range panic
	r.Reset(bytes.NewReader([]byte("\nhello,world\n")))
	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("read after reset failed: %v", err)
	}
	if rec.NumFields() != 2 || string(rec.Field(0)) != "hello" || string(rec.Field(1)) != "world" {
		t.Fatalf("unexpected record after reset: %v", rec)
	}
}

func TestQuoteBoundaryShift(t *testing.T) {
	// Pad so that recordStart is > 32KB and a quoted field has quote at the end of first 64KB read
	pad := bytes.Repeat([]byte("a,b\n"), 7000)                   // ~28KB
	pad2 := append(pad, bytes.Repeat([]byte("c,d\n"), 1800)...) // ~35KB
	needed := defaultBufferSize - len(pad2) - 1                 // 1 byte before 64KB boundary
	recData := append([]byte("\""), bytes.Repeat([]byte("x"), needed-1)...)
	recData = append(recData, []byte("\"\"\",rest\n")...) // escaped quote across boundary, then closing quote

	allData := append(pad2, recData...)
	r := NewReader(bytes.NewReader(allData))
	for i := 0; i < 7000+1800; i++ {
		_, err := r.ReadRecord()
		if err != nil {
			t.Fatalf("row %d error: %v", i, err)
		}
	}

	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("quote boundary record failed: %v", err)
	}
	if rec.NumFields() != 2 {
		t.Fatalf("expected 2 fields, got %d", rec.NumFields())
	}
	if string(rec.Field(1)) != "rest" {
		t.Fatalf("field 1 = %q, want 'rest'", string(rec.Field(1)))
	}
}

func TestCRLFBoundaryShift(t *testing.T) {
	pad := bytes.Repeat([]byte("a,b\n"), 7000)                   // ~28KB
	pad2 := append(pad, bytes.Repeat([]byte("c,d\n"), 1800)...) // ~35KB
	needed := defaultBufferSize - len(pad2) - 1                 // 1 byte before 64KB boundary
	recData := bytes.Repeat([]byte("x"), needed)                // field fills up to 65534
	recData = append(recData, []byte("\r\nnext_row\n")...)      // \r at 65535, \n at 65536!

	allData := append(pad2, recData...)
	r := NewReader(bytes.NewReader(allData))
	for i := 0; i < 7000+1800; i++ {
		_, err := r.ReadRecord()
		if err != nil {
			t.Fatalf("row %d error: %v", i, err)
		}
	}

	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("CRLF boundary record failed: %v", err)
	}
	if rec.NumFields() != 1 {
		t.Fatalf("expected 1 field, got %d", rec.NumFields())
	}
	if len(rec.Field(0)) != needed {
		t.Fatalf("field len = %d, want %d", len(rec.Field(0)), needed)
	}

	recNext, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("next record failed: %v", err)
	}
	if string(recNext.Field(0)) != "next_row" {
		t.Fatalf("expected next_row, got %q", string(recNext.Field(0)))
	}
}

func TestReaderCustomDelimiterPipe(t *testing.T) {
	data := "a|b|c\n1|2|3\n"
	r := NewReader(strings.NewReader(data), WithDelimiter("|"))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(rec) != 3 || rec[0] != "a" || rec[1] != "b" || rec[2] != "c" {
		t.Fatalf("unexpected record: %v", rec)
	}
}

func TestReaderMultiCharDelimiter(t *testing.T) {
	data := "col1||col2||col3\nval1||val2||val3\n"
	r := NewReader(strings.NewReader(data), WithDelimiter("||"))
	rec1, err := r.Read()
	if err != nil {
		t.Fatalf("read row 1 failed: %v", err)
	}
	if len(rec1) != 3 || rec1[0] != "col1" || rec1[1] != "col2" || rec1[2] != "col3" {
		t.Fatalf("unexpected row 1: %v", rec1)
	}

	rec2, err := r.Read()
	if err != nil {
		t.Fatalf("read row 2 failed: %v", err)
	}
	if len(rec2) != 3 || rec2[0] != "val1" || rec2[1] != "val2" || rec2[2] != "val3" {
		t.Fatalf("unexpected row 2: %v", rec2)
	}
}

func TestReaderCustomQuote(t *testing.T) {
	// Single quote as quote char, with '' escape
	data := "'hello''world',foo,'bar'\n"
	r := NewReader(strings.NewReader(data), WithQuote('\''))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(rec) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(rec), rec)
	}
	if rec[0] != "hello'world" {
		t.Errorf("rec[0] = %q, want \"hello'world\"", rec[0])
	}
	if rec[1] != "foo" {
		t.Errorf("rec[1] = %q, want \"foo\"", rec[1])
	}
	if rec[2] != "bar" {
		t.Errorf("rec[2] = %q, want \"bar\"", rec[2])
	}
}

func TestReaderCustomQuoteAndMultiDelim(t *testing.T) {
	// Multi-char delimiter '::' and single quote '\''
	data := "'first::part'::'second''escaped'::unquoted_val\n"
	r := NewReader(strings.NewReader(data), WithDelimiter("::"), WithQuote('\''))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(rec) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(rec), rec)
	}
	if rec[0] != "first::part" {
		t.Errorf("rec[0] = %q, want 'first::part'", rec[0])
	}
	if rec[1] != "second'escaped" {
		t.Errorf("rec[1] = %q, want \"second'escaped\"", rec[1])
	}
	if rec[2] != "unquoted_val" {
		t.Errorf("rec[2] = %q, want 'unquoted_val'", rec[2])
	}
}

func TestReaderNoQuoteFastPath(t *testing.T) {
	data := "a,b,c,d\n1,2,3,4\n5,6,7,8\n"
	r := NewReader(strings.NewReader(data))
	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatal(err)
	}
	if rec.NumFields() != 4 || string(rec.Field(0)) != "a" {
		t.Fatalf("unexpected record: %v", rec)
	}
	rec2, err := r.ReadRecord()
	if err != nil {
		t.Fatal(err)
	}
	if rec2.NumFields() != 4 || string(rec2.Field(0)) != "1" || string(rec2.Field(3)) != "4" {
		t.Fatalf("unexpected record 2: %v", rec2)
	}
}

func TestCountRecordsQuotedEOF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		quote byte
		want  int
	}{
		{
			name:  "single quoted field no newline",
			input: "\"a\"",
			quote: '"',
			want:  1,
		},
		{
			name:  "single quoted field with newline",
			input: "\"a\"\n",
			quote: '"',
			want:  1,
		},
		{
			name:  "two rows ending with quoted field without newline",
			input: "id,name\n1,\"Alice\"",
			quote: '"',
			want:  2,
		},
		{
			name:  "multiline quoted field without newline",
			input: "id,comment\n1,\"multi\nline\ntext\"",
			quote: '"',
			want:  2,
		},
		{
			name:  "empty input",
			input: "",
			quote: '"',
			want:  0,
		},
		{
			name:  "single newline only",
			input: "\n",
			quote: '"',
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countRecords([]byte(tt.input), tt.quote)
			if got != tt.want {
				t.Fatalf("countRecords(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestReaderFastPathQuoteAfterEOL(t *testing.T) {
	// First row is unquoted: "a,b,c\n"
	// Second row begins with quote: "\"quoted\",val\n"
	// Both rows easily fit within 32 bytes
	data := "a,b,c\n\"quoted\",val\n"
	r := NewReader(strings.NewReader(data))
	rec1, err := r.Read()
	if err != nil {
		t.Fatalf("row 1 read failed: %v", err)
	}
	if len(rec1) != 3 || rec1[0] != "a" || rec1[1] != "b" || rec1[2] != "c" {
		t.Fatalf("row 1 mismatch: %v", rec1)
	}
	rec2, err := r.Read()
	if err != nil {
		t.Fatalf("row 2 read failed: %v", err)
	}
	if len(rec2) != 2 || rec2[0] != "quoted" || rec2[1] != "val" {
		t.Fatalf("row 2 mismatch: %v", rec2)
	}
}





