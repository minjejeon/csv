package csv

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
)

type EmployeeKorean struct {
	ID   int    `csv:"id"`
	Name string `csv:"name"`
	Dept string `csv:"dept"`
}

func TestEncodingEUCKR_Reader(t *testing.T) {
	utf8CSV := "id,name,dept\n1,홍길동,개발팀\n2,이순신,기획팀\n"
	euckrBytes, err := korean.EUCKR.NewEncoder().Bytes([]byte(utf8CSV))
	if err != nil {
		t.Fatalf("failed to encode to EUC-KR: %v", err)
	}

	r := NewReader(bytes.NewReader(euckrBytes), WithCharset("euc-kr"))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[1][1] != "홍길동" || rows[1][2] != "개발팀" {
		t.Errorf("row 1 mismatch: %v", rows[1])
	}
	if rows[2][1] != "이순신" || rows[2][2] != "기획팀" {
		t.Errorf("row 2 mismatch: %v", rows[2])
	}
}

func TestEncodingEUCKR_Unmarshal(t *testing.T) {
	utf8CSV := "id,name,dept\n10,홍길동,인사팀\n20,강감찬,총무팀\n"
	euckrBytes, err := korean.EUCKR.NewEncoder().Bytes([]byte(utf8CSV))
	if err != nil {
		t.Fatalf("failed to encode to EUC-KR: %v", err)
	}

	var emps []EmployeeKorean
	err = Unmarshal(euckrBytes, &emps, WithEncoding(korean.EUCKR))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(emps) != 2 {
		t.Fatalf("expected 2 emps, got %d", len(emps))
	}
	if emps[0].ID != 10 || emps[0].Name != "홍길동" || emps[0].Dept != "인사팀" {
		t.Errorf("emp 0 mismatch: %+v", emps[0])
	}
	if emps[1].ID != 20 || emps[1].Name != "강감찬" || emps[1].Dept != "총무팀" {
		t.Errorf("emp 1 mismatch: %+v", emps[1])
	}
}

func TestEncodingShiftJIS(t *testing.T) {
	utf8CSV := "id,name,dept\n100,田中太郎,営業部\n"
	sjisBytes, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(utf8CSV))
	if err != nil {
		t.Fatalf("failed to encode to Shift_JIS: %v", err)
	}

	var emps []EmployeeKorean
	err = Unmarshal(sjisBytes, &emps, WithCharset("shift_jis"))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(emps) != 1 {
		t.Fatalf("expected 1 emp, got %d", len(emps))
	}
	if emps[0].Name != "田中太郎" || emps[0].Dept != "営業部" {
		t.Errorf("emp mismatch: %+v", emps[0])
	}
}

func TestEncodingParallelUnmarshal(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("id,name,dept\n")
	for i := 0; i < 200; i++ {
		buf.WriteString("1,홍길동,개발팀\n")
	}
	euckrBytes, err := korean.EUCKR.NewEncoder().Bytes(buf.Bytes())
	if err != nil {
		t.Fatalf("failed to encode to EUC-KR: %v", err)
	}

	var emps []EmployeeKorean
	err = ParallelUnmarshal(euckrBytes, &emps, WithCharset("cp949"))
	if err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(emps) != 200 {
		t.Fatalf("expected 200 emps, got %d", len(emps))
	}
	if emps[0].Name != "홍길동" || emps[0].Dept != "개발팀" {
		t.Errorf("emp 0 mismatch: %+v", emps[0])
	}
}

func TestWriterEncodingEUCKR(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, WithCharset("euc-kr"))
	err := w.Write([]string{"이름", "직급"})
	if err != nil {
		t.Fatalf("Write header failed: %v", err)
	}
	err = w.Write([]string{"홍길동", "개발자"})
	if err != nil {
		t.Fatalf("Write record failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	written := buf.Bytes()
	// Read back using EUC-KR Reader
	r := NewReader(bytes.NewReader(written), WithCharset("euc-kr"))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	if len(rows) != 2 || rows[0][0] != "이름" || rows[1][0] != "홍길동" {
		t.Fatalf("unexpected rows: %+v", rows)
	}

	// Verify the written bytes are indeed valid EUC-KR and NOT plain UTF-8
	if bytes.Contains(written, []byte("홍길동")) {
		t.Errorf("expected written bytes to be EUC-KR, but contains raw UTF-8!")
	}
}

func TestMarshalEncodingShiftJIS(t *testing.T) {
	emps := []EmployeeKorean{
		{ID: 1, Name: "田中太郎", Dept: "営業部"},
	}
	data, err := Marshal(emps, WithCharset("shift_jis"))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Read back using Shift_JIS Unmarshal
	var roundTrip []EmployeeKorean
	err = Unmarshal(data, &roundTrip, WithCharset("shift_jis"))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(roundTrip) != 1 || roundTrip[0].Name != "田中太郎" || roundTrip[0].Dept != "営業部" {
		t.Fatalf("roundTrip mismatch: %+v", roundTrip)
	}

	// Verify written bytes are Shift_JIS not UTF-8
	if bytes.Contains(data, []byte("田中太郎")) {
		t.Errorf("expected data to be Shift_JIS, but contains raw UTF-8!")
	}
}

func TestParallelMarshalEncodingEUCKR(t *testing.T) {
	var emps []EmployeeKorean
	for i := 1; i <= 50; i++ {
		emps = append(emps, EmployeeKorean{ID: i, Name: "홍길동", Dept: "개발팀"})
	}
	data, err := ParallelMarshal(emps, WithCharset("euc-kr"))
	if err != nil {
		t.Fatalf("ParallelMarshal failed: %v", err)
	}

	var roundTrip []EmployeeKorean
	err = ParallelUnmarshal(data, &roundTrip, WithCharset("euc-kr"))
	if err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(roundTrip) != 50 {
		t.Fatalf("expected 50 emps, got %d", len(roundTrip))
	}
	if roundTrip[0].Name != "홍길동" || roundTrip[49].Dept != "개발팀" {
		t.Errorf("roundTrip mismatch: %+v", roundTrip[0])
	}
}
