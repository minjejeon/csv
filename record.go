package csv

import (
	"strconv"
	"unsafe"
)

// Record represents a single parsed CSV row with zero-copy field access.
type Record struct {
	r     *Reader
	raw   []byte
	spans []fieldSpan
}

// NumFields returns the number of fields in the record.
func (rec *Record) NumFields() int {
	return len(rec.spans)
}

// RawField returns the raw byte slice of field i as it appears in the read buffer.
// Note: If the field was quoted and had escaped quotes (""), RawField will include the escapes.
func (rec *Record) RawField(i int) []byte {
	if i < 0 || i >= len(rec.spans) {
		return nil
	}
	s := rec.spans[i]
	return rec.raw[s.start:s.end]
}

// Field returns the unescaped byte slice for field i.
// If the field has no escaped quotes, this is zero-copy directly from the read buffer.
// If the field has escaped quotes, it returns unescaped bytes using the Reader's scratch buffer.
func (rec *Record) Field(i int) []byte {
	if i < 0 || i >= len(rec.spans) {
		return nil
	}
	s := rec.spans[i]
	raw := rec.raw[s.start:s.end]
	if !s.hasEscapes {
		return raw
	}
	return rec.r.unescape(raw)
}

// FieldString returns the unescaped string of field i.
func (rec *Record) FieldString(i int) string {
	b := rec.Field(i)
	return string(b)
}

// FieldInt parses field i as an int.
func (rec *Record) FieldInt(i int) (int, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	n, err := parseSignedInt(b, strconv.IntSize)
	return int(n), err
}

// FieldInt64 parses field i as an int64.
func (rec *Record) FieldInt64(i int) (int64, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	return parseSignedInt(b, 64)
}

// FieldUint parses field i as a uint.
func (rec *Record) FieldUint(i int) (uint, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	n, err := parseUnsignedInt(b, strconv.IntSize)
	return uint(n), err
}

// FieldUint64 parses field i as a uint64.
func (rec *Record) FieldUint64(i int) (uint64, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	return parseUnsignedInt(b, 64)
}

// FieldBool parses field i as a boolean.
func (rec *Record) FieldBool(i int) (bool, error) {
	return parseBoolFast(rec.Field(i))
}

// FieldFloat64 parses field i as a float64.
func (rec *Record) FieldFloat64(i int) (float64, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	s := unsafe.String(unsafe.SliceData(b), len(b))
	return strconv.ParseFloat(s, 64)
}

// FieldFloat32 parses field i as a float32.
func (rec *Record) FieldFloat32(i int) (float32, error) {
	b := rec.Field(i)
	if len(b) == 0 {
		return 0, nil
	}
	s := unsafe.String(unsafe.SliceData(b), len(b))
	v, err := strconv.ParseFloat(s, 32)
	return float32(v), err
}
