package csv

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
