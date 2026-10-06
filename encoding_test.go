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
