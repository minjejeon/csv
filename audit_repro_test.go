package csv

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
	"unique"
)

// 1. Zero-value unique.Handle crash
func TestRepro_ZeroUniqueHandleCrash(t *testing.T) {
	type Item struct {
		ID   int                   `csv:"id"`
		Code unique.Handle[string] `csv:"code"`
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("FAIL: Marshal panicked on zero unique.Handle: %v", r)
		}
	}()

	items := []Item{{ID: 1}}
	data, err := Marshal(items)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	expected := "id,code\n1,\n"
	if string(data) != expected {
		t.Errorf("got %q, want %q", string(data), expected)
	}
}

// 2. *time.Time serializing to empty string
func TestRepro_PointerTimeSerialization(t *testing.T) {
	type Event struct {
		Name string     `csv:"name"`
		At   *time.Time `csv:"at"`
	}

	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	events := []Event{{Name: "launch", At: &now}}

	data, err := Marshal(events)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	str := string(data)
	t.Logf("Result: %s", str)
	if !strings.Contains(str, "2026-10-07") {
		t.Errorf("FAIL: *time.Time serialized to empty string! got: %q", str)
	}
}

// 3. fastParseSmallUint failing for "82", "83", "92", etc.
func TestRepro_FastParseSmallUintBitwiseFlaw(t *testing.T) {
	inputs := []string{"82", "83", "92", "38", "48", "823", "912"}
	for _, in := range inputs {
		v, ok := fastParseSmallUint([]byte(in))
		if !ok {
			t.Errorf("FAIL: fastParseSmallUint failed for valid decimal %q", in)
		} else {
			t.Logf("fastParseSmallUint(%q) = %d (ok)", in, v)
		}
	}
}

// 4. Recursive struct embedding stack overflow
func TestRepro_RecursiveStructEmbedding(t *testing.T) {
	type Node struct {
		*Node
		Val int `csv:"val"`
	}

	// This should either safely handle or return an error, but NOT crash the process with stack overflow.
	// We run it with a recover.
	done := make(chan bool)
	go func() {
		defer func() {
			recover()
			done <- true
		}()
		var nodes []Node
		_ = Unmarshal([]byte("val\n1\n"), &nodes)
	}()

	select {
	case <-done:
		t.Log("Recovered or completed without stack overflow")
	case <-time.After(2 * time.Second):
		t.Errorf("FAIL: Infinite recursion detected on recursive struct embedding!")
	}
}

// 5. Tag csv:"ts,format:2006-01-02" with colon
func TestRepro_TagFormatWithColon(t *testing.T) {
	tag := parseTag("ts,format:2006-01-02")
	if tag.format != "2006-01-02" {
		t.Errorf("FAIL: parseTag failed to parse format:2006-01-02, got %q", tag.format)
	}

	type Record struct {
		CreatedAt time.Time `csv:"ts,format:2006-01-02"`
	}

	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	data, err := Marshal([]Record{{CreatedAt: now}})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if !strings.Contains(string(data), "2026-10-07") {
		t.Errorf("FAIL: Marshal with colon format failed: %q", string(data))
	}
}

// 6. Multiline quoted field line number tracking
func TestRepro_MultilineQuotedFieldLineNumbers(t *testing.T) {
	csvData := "\"line 1\nline 2\nline 3\",val1\nline 4,val2\n"
	r := NewReader(strings.NewReader(csvData))

	rec1, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("ReadRecord 1 error: %v", err)
	}
	t.Logf("Rec1 line: %d", rec1.Line())

	rec2, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("ReadRecord 2 error: %v", err)
	}
	t.Logf("Rec2 line: %d", rec2.Line())

	// rec2 is on physical line 4 of the file!
	if rec2.Line() != 4 {
		t.Errorf("FAIL: Multiline quoted field desynchronized line number: got %d, want 4", rec2.Line())
	}
}

// 7. ParallelMarshal on empty slice emitting header
func TestRepro_ParallelMarshalEmptySlice(t *testing.T) {
	type User struct {
		ID   int    `csv:"id"`
		Name string `csv:"name"`
	}

	outSeq, err := Marshal([]User{})
	if err != nil {
		t.Fatalf("Marshal err: %v", err)
	}

	outPar, err := ParallelMarshal([]User{})
	if err != nil {
		t.Fatalf("ParallelMarshal err: %v", err)
	}

	if string(outSeq) != string(outPar) {
		t.Errorf("FAIL: Inconsistent empty slice output: Marshal=%q, ParallelMarshal=%q", string(outSeq), string(outPar))
	}
}

// 8. WithCharset error suppression
func TestRepro_InvalidCharsetErrorSuppression(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, WithCharset("completely-invalid-nonexistent-charset"))
	if w.Error() == nil {
		t.Errorf("FAIL: WithCharset('invalid') did not report error in w.Error()")
	}
}

// 9. bitSize == 0 in parseSignedInt
func TestRepro_ParseSignedIntZeroBitSize(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("FAIL: parseSignedInt panicked on bitSize == 0: %v", r)
		}
	}()

	v, err := parseSignedInt([]byte("-42"), 0)
	if err != nil {
		t.Errorf("parseSignedInt error: %v", err)
	}
	if v != -42 {
		t.Errorf("got %d, want -42", v)
	}
}

// 10. scanBlock32 bounds check on AMD64
func TestRepro_ScanBlock32ShortChunk(t *testing.T) {
	s := newBlockScanner(byte(','), byte('"'))
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("FAIL: scanBlock32 panicked on short chunk: %v", r)
		}
	}()
	short := []byte("a,b,c")
	s.scanBlock32(short)
}

// 11. Anonymous time.Time embedding dropped
func TestRepro_AnonymousTimeEmbedding(t *testing.T) {
	type Event struct {
		ID int `csv:"id"`
		time.Time
	}

	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	events := []Event{{ID: 1, Time: now}}
	data, err := Marshal(events)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	t.Logf("Marshal result: %s", string(data))
	if !strings.Contains(string(data), "Time") || !strings.Contains(string(data), "2026-10-07") {
		t.Errorf("FAIL: Anonymous time.Time was dropped from serialization: %q", string(data))
	}
}

// 12. Struct field shadowing inversion
func TestRepro_EmbeddedStructShadowing(t *testing.T) {
	type Inner struct {
		Name string `csv:"name"`
	}
	type Outer struct {
		Inner
		Name string `csv:"name"`
	}

	var out []Outer
	err := Unmarshal([]byte("name\nouter_val\n"), &out)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out) == 0 || out[0].Name != "outer_val" {
		t.Errorf("FAIL: Outer.Name was shadowed by Inner.Name! out=%+v", out)
	}
}

// 13. Pointer to custom TextMarshaler
type auditCustomStatus int

func (s auditCustomStatus) MarshalText() ([]byte, error) {
	return []byte("active"), nil
}

func TestRepro_PointerTextMarshaler(t *testing.T) {
	type Record struct {
		Status *auditCustomStatus `csv:"status"`
	}
	st := auditCustomStatus(1)
	records := []Record{{Status: &st}}
	data, err := Marshal(records)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	t.Logf("Result: %s", string(data))
	if !strings.Contains(string(data), "active") {
		t.Errorf("FAIL: *customStatus was serialized to empty string! got: %q", string(data))
	}
}

// 14. parseTag greedy format consuming subsequent options
func TestRepro_R2_FormatTagWithOmitempty(t *testing.T) {
	tag := parseTag("ts,format:2006-01-02,omitempty")
	if tag.format != "2006-01-02" {
		t.Errorf("FAIL: tag.format was corrupted by subsequent options: got %q, want '2006-01-02'", tag.format)
	}
	if !tag.omitEmpty {
		t.Errorf("FAIL: tag.omitEmpty was lost due to greedy format parsing!")
	}
}

// 15. Field order preservation in embedded struct serialization
func TestRepro_R2_EmbeddedStructFieldOrder(t *testing.T) {
	type Base struct {
		ID int `csv:"id"`
	}
	type Record struct {
		Base
		Name string `csv:"name"`
	}

	data, err := Marshal([]Record{{Base: Base{ID: 1}, Name: "Alice"}})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	expected := "id,name\n1,Alice\n"
	if string(data) != expected {
		t.Errorf("FAIL: Embedded struct reordered fields! got %q, want %q", string(data), expected)
	}
}

// 16. Writer.Close error masking
func TestRepro_R2_WriterCloseErrorMasking(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	// Force sticky error
	w.err = errors.New("sticky write error")
	err := w.Close()
	if err == nil {
		t.Errorf("FAIL: w.Close() masked sticky error and returned nil!")
	}
}

// 17. FieldsPerRecord partial record return on ErrFieldCount
func TestRepro_R2_FieldsPerRecordPartialReturn(t *testing.T) {
	r := NewReader(strings.NewReader("a,b,c\n1,2\n"))
	r.FieldsPerRecord = 3

	// First row (3 fields)
	rec1, err := r.Read()
	if err != nil {
		t.Fatalf("row 1 error: %v", err)
	}
	if len(rec1) != 3 {
		t.Fatalf("row 1 expected 3 fields, got %d", len(rec1))
	}

	// Second row (2 fields) -> should return partial record AND ErrFieldCount
	rec2, err := r.Read()
	if !errors.Is(err, ErrFieldCount) {
		t.Fatalf("expected ErrFieldCount, got %v", err)
	}
	if len(rec2) != 2 {
		t.Errorf("FAIL: Read() returned nil or empty record on ErrFieldCount! got %v", rec2)
	}
}
