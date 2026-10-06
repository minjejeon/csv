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
