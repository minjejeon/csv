package csv

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// Decoder streams CSV records directly into Go structs.
type Decoder struct {
	r          *Reader
	opts       []Option
	headers    []string
	headerRead bool
	plan       *typePlan
	targetType reflect.Type
	hasMore    bool
}

// NewDecoder creates a new streaming CSV decoder reading from r.
func NewDecoder(r io.Reader, opts ...Option) (*Decoder, error) {
	reader := NewReader(r, opts...)
	return &Decoder{
		r:       reader,
		opts:    opts,
		hasMore: true,
	}, nil
}

// Header returns the parsed CSV column headers.
func (d *Decoder) Header() []string {
	return d.headers
}

// Reset resets the Decoder to read from r, reusing internal buffers.
func (d *Decoder) Reset(r io.Reader) {
	if d.r != nil {
		d.r.Reset(r)
	} else {
		d.r = NewReader(r, d.opts...)
	}
	d.headers = nil
	d.headerRead = false
	d.plan = nil
	d.targetType = nil
	d.hasMore = true
}

// Close releases the Decoder's pooled buffers back to their respective pools.
func (d *Decoder) Close() error {
	if d.r != nil {
		err := d.r.Close()
		d.r = nil
		return err
	}
	return nil
}

func (d *Decoder) readHeaders() error {
	if d.headerRead {
		return nil
	}
	headers, err := d.r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			d.hasMore = false
			_ = d.Close()
		}
		return err
	}
	d.headers = headers
	d.headerRead = true
	return nil
}

// More reports whether there are more records to read.
func (d *Decoder) More() bool {
	return d.hasMore
}

// DecodeError describes a field-level conversion failure during CSV decoding.
type DecodeError struct {
	Line   int    // Line number in CSV input (1-based)
	Column int    // Field column index in CSV record (0-based)
	Header string // CSV column header name (if available)
	Field  string // Struct field name
	Value  string // Raw string value that failed conversion
	Err    error  // Underlying conversion error
}

func (e *DecodeError) Error() string {
	if e.Header != "" {
		return fmt.Sprintf("csv: line %d, column %q (field %s): %v (value: %q)",
			e.Line, e.Header, e.Field, e.Err, e.Value)
	}
	return fmt.Sprintf("csv: line %d, col %d (field %s): %v (value: %q)",
		e.Line, e.Column, e.Field, e.Err, e.Value)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

// Decode decodes the next CSV record into the struct pointed to by v.
func (d *Decoder) Decode(v any) error {
	if !d.hasMore || d.r == nil {
		return io.EOF
	}

	if !d.headerRead {
		if err := d.readHeaders(); err != nil {
			return err
		}
	}

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return fmt.Errorf("csv: Decode requires a non-nil pointer, got %T", v)
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("csv: Decode target must be a struct pointer, got %T", v)
	}

	elemType := elem.Type()
	if d.plan == nil || d.targetType != elemType {
		plan, err := getTypePlan(elemType, d.headers)
		if err != nil {
			return err
		}
		d.plan = plan
		d.targetType = elemType
	}

	rec, err := d.r.ReadRecord()
	if err != nil {
		if errors.Is(err, io.EOF) {
			d.hasMore = false
			_ = d.Close()
		}
		return err
	}

	structPtr := val.UnsafePointer()
	numFields := rec.NumFields()

	for _, f := range d.plan.fields {
		if f.colIndex < numFields {
			raw := rec.Field(f.colIndex)
			if err := f.setter(structPtr, raw); err != nil {
				headerName := ""
				if f.colIndex < len(d.headers) {
					headerName = d.headers[f.colIndex]
				}
				return &DecodeError{
					Line:   rec.line,
					Column: f.colIndex,
					Header: headerName,
					Field:  f.fieldName,
					Value:  string(raw),
					Err:    err,
				}
			}
		} else {
			_ = f.setter(structPtr, nil)
		}
	}

	return nil
}
