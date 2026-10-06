package csv

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

type customStatus int

const (
	statusUnknown customStatus = iota
	statusActive
	statusInactive
)

func (s *customStatus) UnmarshalText(text []byte) error {
	str := strings.ToLower(string(text))
	switch str {
	case "active":
		*s = statusActive
	case "inactive":
		*s = statusInactive
	default:
		return errors.New("invalid status")
	}
	return nil
}

type customStruct struct {
	Status customStatus `csv:"status"`
	Count  uint         `csv:"count"`
}

func TestSetterTextUnmarshaler(t *testing.T) {
	typ := reflect.TypeOf(customStruct{})
	f, _ := typ.FieldByName("Status")
	setter, err := compileSetter(f.Type, f.Offset, parseTag(""))
	if err != nil {
		t.Fatalf("compileSetter failed: %v", err)
	}

	var cs customStruct
	ptr := unsafe.Pointer(&cs)

	if err := setter(ptr, []byte("ACTIVE")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.Status != statusActive {
		t.Fatalf("expected statusActive, got %v", cs.Status)
	}

	if err := setter(ptr, []byte("invalid_val")); err == nil {
		t.Fatal("expected error on invalid status")
	}
}

func TestSetterUint(t *testing.T) {
	typ := reflect.TypeOf(customStruct{})
	f, _ := typ.FieldByName("Count")
	setter, err := compileSetter(f.Type, f.Offset, parseTag(""))
	if err != nil {
		t.Fatalf("compileSetter failed: %v", err)
	}

	var cs customStruct
	ptr := unsafe.Pointer(&cs)

	if err := setter(ptr, []byte("12345")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.Count != 12345 {
		t.Fatalf("expected 12345, got %d", cs.Count)
	}
}

type overflowStruct struct {
	I8  int8  `csv:"i8"`
	I64 int64 `csv:"i64"`
	U8  uint8 `csv:"u8"`
	Opt int   `csv:"opt,omitempty"`
	Req int   `csv:"req"`
}

func TestSetterOverflowAndOmitEmpty(t *testing.T) {
	typ := reflect.TypeOf(overflowStruct{})

	// Test int8 overflow (128 exceeds math.MaxInt8)
	fI8, _ := typ.FieldByName("I8")
	sI8, err := compileSetter(fI8.Type, fI8.Offset, parseTag(""))
	if err != nil {
		t.Fatalf("compileSetter failed: %v", err)
	}
	var s overflowStruct
	ptr := unsafe.Pointer(&s)
	if err := sI8(ptr, []byte("128")); err == nil {
		t.Fatal("expected ErrRange for int8 with 128")
	}
	if err := sI8(ptr, []byte("-129")); err == nil {
		t.Fatal("expected ErrRange for int8 with -129")
	}
	if err := sI8(ptr, []byte("127")); err != nil {
		t.Fatalf("unexpected error for 127: %v", err)
	}
	if s.I8 != 127 {
		t.Fatalf("expected 127, got %d", s.I8)
	}

	// Test uint8 overflow (256 exceeds math.MaxUint8)
	fU8, _ := typ.FieldByName("U8")
	sU8, _ := compileSetter(fU8.Type, fU8.Offset, parseTag(""))
	if err := sU8(ptr, []byte("256")); err == nil {
		t.Fatal("expected ErrRange for uint8 with 256")
	}
	if err := sU8(ptr, []byte("-1")); err == nil {
		t.Fatal("expected ErrSyntax for uint8 with -1")
	}

	// Test empty string with omitempty
	fOpt, _ := typ.FieldByName("Opt")
	sOpt, _ := compileSetter(fOpt.Type, fOpt.Offset, parseTag("opt,omitempty"))
	s.Opt = 42
	if err := sOpt(ptr, []byte("")); err != nil {
		t.Fatalf("unexpected error on empty opt: %v", err)
	}
	if s.Opt != 0 {
		t.Fatalf("expected 0 for empty opt, got %d", s.Opt)
	}

	// Test empty string WITHOUT omitempty
	fReq, _ := typ.FieldByName("Req")
	sReq, _ := compileSetter(fReq.Type, fReq.Offset, parseTag("req"))
	if err := sReq(ptr, []byte("")); err == nil {
		t.Fatal("expected error on empty req without omitempty")
	}
}

