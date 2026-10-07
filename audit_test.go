package csv

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

// smallBufferReader is an io.Reader wrapper that returns at most chunkSize bytes per Read.
type smallBufferReader struct {
	r         io.Reader
	chunkSize int
}

func (s *smallBufferReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	target := len(p)
	if target > s.chunkSize {
		target = s.chunkSize
	}
	return s.r.Read(p[:target])
}

// TestSmallBufferBoundaries tests every possible buffer chunk size from 1 to 32 bytes
// to stress-test sliding buffer shifts, CRLF boundaries, escaped quote boundaries,
// and multi-character delimiters.
func TestSmallBufferBoundaries(t *testing.T) {
	csvData := "id||name||comment||num\r\n" +
		"1||\"Alice \"\"The Boss\"\"\"||\"Line 1\r\nLine 2\"||100\r\n" +
		"2||\"Bob, Jr.\"||\"\"\"Simple Quote\"\"\"||200\r\n" +
		"3||Charlie||||300\r\n"

	expected := [][]string{
		{"id", "name", "comment", "num"},
		{"1", "Alice \"The Boss\"", "Line 1\r\nLine 2", "100"},
		{"2", "Bob, Jr.", "\"Simple Quote\"", "200"},
		{"3", "Charlie", "", "300"},
	}

	for chunkSize := 1; chunkSize <= 32; chunkSize++ {
		t.Run(fmt.Sprintf("ChunkSize_%d", chunkSize), func(t *testing.T) {
			sbr := &smallBufferReader{
				r:         strings.NewReader(csvData),
				chunkSize: chunkSize,
			}
			r := NewReader(sbr, WithDelimiter("||"))
			r.bufSize = chunkSize // also test small internal buffer

			var rows [][]string
			for {
				rec, err := r.Read()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("chunkSize %d failed: %v", chunkSize, err)
				}
				rows = append(rows, rec)
			}

			if len(rows) != len(expected) {
				t.Fatalf("chunkSize %d: expected %d rows, got %d", chunkSize, len(expected), len(rows))
			}
			for i := range expected {
				for j := range expected[i] {
					if rows[i][j] != expected[i][j] {
						t.Errorf("chunkSize %d: row %d col %d mismatch:\ngot:  %q\nwant: %q",
							chunkSize, i, j, rows[i][j], expected[i][j])
					}
				}
			}
		})
	}
}

// TestEscapedQuotesMultipleFieldsNoAliasing tests that accessing multiple fields with
// escaped quotes in the same record does not overwrite or corrupt each other.
func TestEscapedQuotesMultipleFieldsNoAliasing(t *testing.T) {
	csvData := "\"field \"\"one\"\"\",\"field \"\"two\"\"\",\"field \"\"three\"\"\"\n"
	r := NewReader(strings.NewReader(csvData))
	rec, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("failed to read record: %v", err)
	}

	f0 := rec.Field(0)
	f1 := rec.Field(1)
	f2 := rec.Field(2)

	want0 := "field \"one\""
	want1 := "field \"two\""
	want2 := "field \"three\""

	if string(f0) != want0 {
		t.Errorf("f0 corrupted! got %q, want %q", string(f0), want0)
	}
	if string(f1) != want1 {
		t.Errorf("f1 corrupted! got %q, want %q", string(f1), want1)
	}
	if string(f2) != want2 {
		t.Errorf("f2 corrupted! got %q, want %q", string(f2), want2)
	}
}

// TestReaderClosedSafety tests that calling ReadRecord or Read after Close returns an error
// and does not panic.
func TestReaderClosedSafety(t *testing.T) {
	r := NewReader(strings.NewReader("a,b,c\n1,2,3\n"))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(rec) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(rec))
	}

	if err := r.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Calling ReadRecord after Close must return an error and NOT panic
	_, err = r.ReadRecord()
	if err == nil {
		t.Error("expected error reading after Close, got nil")
	}

	_, err = r.Read()
	if err == nil {
		t.Error("expected error reading after Close, got nil")
	}
}

// TestIntegerEdgeCases tests parsing min and max values for int8, int16, int32, int64, and uints.
func TestIntegerEdgeCases(t *testing.T) {
	// MinInt64
	minInt64Str := fmt.Sprintf("%d", math.MinInt64)
	v, err := parseSignedInt([]byte(minInt64Str), 64)
	if err != nil {
		t.Fatalf("parseSignedInt(MinInt64) failed: %v", err)
	}
	if v != math.MinInt64 {
		t.Errorf("MinInt64 mismatch: got %d, want %d", v, int64(math.MinInt64))
	}

	// MaxInt64
	maxInt64Str := fmt.Sprintf("%d", math.MaxInt64)
	v, err = parseSignedInt([]byte(maxInt64Str), 64)
	if err != nil {
		t.Fatalf("parseSignedInt(MaxInt64) failed: %v", err)
	}
	if v != math.MaxInt64 {
		t.Errorf("MaxInt64 mismatch: got %d, want %d", v, int64(math.MaxInt64))
	}

	// MinInt32
	minInt32Str := fmt.Sprintf("%d", math.MinInt32)
	v, err = parseSignedInt([]byte(minInt32Str), 32)
	if err != nil {
		t.Fatalf("parseSignedInt(MinInt32) failed: %v", err)
	}
	if v != math.MinInt32 {
		t.Errorf("MinInt32 mismatch: got %d, want %d", v, int32(math.MinInt32))
	}

	// MaxUint64
	maxUint64Str := fmt.Sprintf("%d", uint64(math.MaxUint64))
	u, err := parseUnsignedInt([]byte(maxUint64Str), 64)
	if err != nil {
		t.Fatalf("parseUnsignedInt(MaxUint64) failed: %v", err)
	}
	if u != math.MaxUint64 {
		t.Errorf("MaxUint64 mismatch: got %d, want %d", u, uint64(math.MaxUint64))
	}
}

// TestTimeOmitEmptyAndEmpty tests time.Time behavior with and without omitempty.
type timeTestStruct struct {
	Req time.Time `csv:"req"`
	Opt time.Time `csv:"opt,omitempty"`
}

func TestTimeOmitEmptyAndEmpty(t *testing.T) {
	data := "req,opt\n2026-01-01T00:00:00Z,\n"
	var out []timeTestStruct
	if err := Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	if out[0].Req.Year() != 2026 {
		t.Errorf("Req year = %d, want 2026", out[0].Req.Year())
	}
	if !out[0].Opt.IsZero() {
		t.Errorf("Opt expected zero time, got %v", out[0].Opt)
	}

	// When required time is empty, it should return an error
	invalidData := "req,opt\n,2026-01-01T00:00:00Z\n"
	var invalidOut []timeTestStruct
	err := Unmarshal([]byte(invalidData), &invalidOut)
	t.Logf("invalidData unmarshal err = %v", err)
	if err == nil {
		t.Error("expected error when required time is empty, got nil")
	}
}

func TestDateOnly(t *testing.T) {
	data := "req,opt\n2026-01-02,2026-05-10\n"
	var out []timeTestStruct
	err := Unmarshal([]byte(data), &out)
	if err != nil {
		t.Fatalf("Unmarshal date-only failed: %v", err)
	}
}

type reuseStruct struct {
	ID   int     `csv:"id"`
	Name string  `csv:"name,omitempty"`
	Dept *string `csv:"dept,omitempty"`
}

func TestDecoderStructReuse(t *testing.T) {
	data := "id,name,dept\n" +
		"1,Alice,Engineering\n" +
		"2,,\n"

	dec, err := NewDecoder(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var row reuseStruct
	if err := dec.Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.Name != "Alice" || row.Dept == nil || *row.Dept != "Engineering" {
		t.Fatalf("row 1 unexpected: %+v", row)
	}

	// Decode row 2 reusing the same row variable
	if err := dec.Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.ID != 2 {
		t.Errorf("row 2 ID = %d, want 2", row.ID)
	}
	if row.Name != "" {
		t.Errorf("row 2 Name should be cleared, got %q", row.Name)
	}
	if row.Dept != nil {
		t.Errorf("row 2 Dept should be nil, got %v (%q)", row.Dept, *row.Dept)
	}
}

func TestGB2312Encoding(t *testing.T) {
	// "你好" in standard GB2312 is 0xC4 0xE3 0xBA 0xC3
	gb2312Data := []byte("name\n\xC4\xE3\xBA\xC3\n")
	r := NewReader(strings.NewReader(string(gb2312Data)), WithCharset("gb2312"))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("GB2312 ReadAll failed: %v", err)
	}
	if len(rows) != 2 || rows[1][0] != "你好" {
		t.Fatalf("GB2312 decoded improperly: got %+v, want '你好'", rows)
	}
}

func TestRecordClone(t *testing.T) {
	csvData := "1,\"Alice \"\"The Boss\"\"\",100\n2,\"Bob\",200\n"
	r := NewReader(strings.NewReader(csvData))

	rec1, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("ReadRecord failed: %v", err)
	}

	cloned1 := rec1.Clone()

	// Read next record, which overwrites r's internal buffers
	rec2, err := r.ReadRecord()
	if err != nil {
		t.Fatalf("ReadRecord row 2 failed: %v", err)
	}

	// Close reader completely to release all pool buffers
	_ = r.Close()

	// rec2 is from closed reader, but cloned1 must still be 100% valid
	if cloned1.NumFields() != 3 {
		t.Fatalf("cloned1 NumFields = %d, want 3", cloned1.NumFields())
	}
	if got := cloned1.FieldString(0); got != "1" {
		t.Errorf("cloned1 field 0 = %q, want '1'", got)
	}
	if got := cloned1.FieldString(1); got != "Alice \"The Boss\"" {
		t.Errorf("cloned1 field 1 = %q, want 'Alice \"The Boss\"'", got)
	}
	v, err := cloned1.FieldInt(2)
	if err != nil || v != 100 {
		t.Errorf("cloned1 field 2 = %d (err: %v), want 100", v, err)
	}

	_ = rec2
}
