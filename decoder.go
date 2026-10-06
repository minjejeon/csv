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
	headers    []string
	headerRead bool
	plan       *typePlan
	targetType reflect.Type
	hasMore    bool
}

// NewDecoder creates a new streaming CSV decoder reading from r.
func NewDecoder(r io.Reader) (*Decoder, error) {
	reader := NewReader(r)
	return &Decoder{
		r:       reader,
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
		d.r = NewReader(r)
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
			if f.tag.omitEmpty && len(raw) == 0 {
				continue
			}
			if err := f.setter(structPtr, raw); err != nil {
				return fmt.Errorf("csv: error parsing field %q: %w", f.fieldName, err)
			}
		}
	}

	return nil
}
