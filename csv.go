package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// Unmarshal parses CSV-encoded data and stores the result in the slice pointed to by v.
// v must be a pointer to a slice of structs or pointer to a slice of struct pointers.
func Unmarshal(data []byte, v any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return fmt.Errorf("csv: Unmarshal expects a non-nil pointer, got %T", v)
	}

	sliceVal := val.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("csv: Unmarshal expects a pointer to a slice, got %T", v)
	}

	elemType := sliceVal.Type().Elem()
	isPtrElem := false
	structType := elemType

	if elemType.Kind() == reflect.Pointer {
		isPtrElem = true
		structType = elemType.Elem()
	}

	if structType.Kind() != reflect.Struct {
		return fmt.Errorf("csv: slice elements must be structs or pointers to structs, got %v", elemType)
	}

	dec, err := NewDecoder(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer dec.r.Close()

	if err := dec.readHeaders(); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty input produces empty slice
			return nil
		}
		return err
	}

	plan, err := getTypePlan(structType, dec.headers)
	if err != nil {
		return err
	}

	for {
		rec, err := dec.r.ReadRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		numFields := rec.NumFields()

		if isPtrElem {
			newElem := reflect.New(structType)
			structPtr := newElem.UnsafePointer()

			for _, f := range plan.fields {
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

			sliceVal = reflect.Append(sliceVal, newElem)
		} else {
			newElem := reflect.New(structType).Elem()
			structPtr := newElem.Addr().UnsafePointer()

			for _, f := range plan.fields {
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

			sliceVal = reflect.Append(sliceVal, newElem)
		}
	}

	val.Elem().Set(sliceVal)
	return nil
}
