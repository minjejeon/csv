package csv

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

// Encoder writes struct values to a CSV stream.
type Encoder struct {
	w             *Writer
	plan          *typeMarshalPlan
	headerWritten bool
	NoHeader      bool
}

// NewEncoder returns a new Encoder writing to w with optional configuration options.
func NewEncoder(w io.Writer, opts ...any) *Encoder {
	return &Encoder{
		w: NewWriter(w, opts...),
	}
}

// Encode writes a single struct or pointer to struct to the CSV stream.
func (enc *Encoder) Encode(v any) error {
	if v == nil {
		return errors.New("csv: Encode(nil)")
	}

	val := reflect.ValueOf(v)
	typ := val.Type()
	if typ.Kind() == reflect.Pointer {
		if val.IsNil() {
			return errors.New("csv: Encode(nil pointer)")
		}
		val = val.Elem()
		typ = val.Type()
	}

	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("csv: expected struct or pointer to struct, got %v", typ)
	}

	if enc.plan == nil || enc.plan.structType != typ {
		plan, err := getTypeMarshalPlan(typ)
		if err != nil {
			return err
		}
		enc.plan = plan
	}

	// Write header if first record
	if !enc.headerWritten && !enc.NoHeader {
		if err := enc.w.Write(enc.plan.headerRow); err != nil {
			return err
		}
		enc.headerWritten = true
	}

	var structPtr unsafe.Pointer
	if val.CanAddr() {
		structPtr = val.Addr().UnsafePointer()
	} else {
		newVal := reflect.New(typ).Elem()
		newVal.Set(val)
		structPtr = newVal.Addr().UnsafePointer()
	}
	for i, f := range enc.plan.fields {
		if i > 0 {
			enc.w.WriteDelimiter()
		}
		if err := f.getter(structPtr, enc.w); err != nil {
			return fmt.Errorf("csv: error encoding field %s: %w", f.colName, err)
		}
	}

	return enc.w.WriteNewline()
}

// Flush flushes buffered data to the underlying io.Writer.
func (enc *Encoder) Flush() error {
	return enc.w.Flush()
}

// Close flushes data and closes the underlying Writer.
func (enc *Encoder) Close() error {
	return enc.w.Close()
}
