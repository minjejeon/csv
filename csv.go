package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"unsafe"
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
	defer dec.Close()

	if err := dec.readHeaders(); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty input produces empty slice
			val.Elem().Set(reflect.MakeSlice(sliceVal.Type(), 0, 0))
			return nil
		}
		return err
	}

	plan, err := getTypePlan(structType, dec.headers)
	if err != nil {
		return err
	}

	c := countRecords(data)
	if c > 1 {
		c-- // exclude header
	}
	if c < 16 {
		c = 16
	}

	slice := reflect.MakeSlice(sliceVal.Type(), c, c)
	sliceBasePtr := slice.Index(0).Addr().UnsafePointer()
	elemSize := structType.Size()
	ptrSize := unsafe.Sizeof(uintptr(0))

	rowIdx := 0
	for {
		rec, err := dec.r.ReadRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if rowIdx >= slice.Len() {
			newCap := slice.Len() * 2
			if newCap < 16 {
				newCap = 16
			}
			newSlice := reflect.MakeSlice(sliceVal.Type(), newCap, newCap)
			reflect.Copy(newSlice, slice)
			slice = newSlice
			sliceBasePtr = slice.Index(0).Addr().UnsafePointer()
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

			*(*unsafe.Pointer)(unsafe.Add(sliceBasePtr, uintptr(rowIdx)*ptrSize)) = structPtr
		} else {
			structPtr := unsafe.Add(sliceBasePtr, uintptr(rowIdx)*elemSize)

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
		}

		rowIdx++
	}

	val.Elem().Set(slice.Slice(0, rowIdx))
	return nil
}

func countRecords(s []byte, quote ...byte) int {
	q := byte('"')
	if len(quote) > 0 && quote[0] != 0 {
		q = quote[0]
	}
	cutset := string([]byte{'\n', q})

	var n int
	inQuote := false
	for len(s) > 0 {
		i := bytes.IndexAny(s, cutset)
		if i == -1 {
			if len(s) > 0 {
				n++
			}
			return n
		}

		c := s[i]
		if c == '\n' {
			if !inQuote {
				n++
			}
			s = s[i+1:]
		} else if c == q {
			if !inQuote {
				inQuote = true
				s = s[i+1:]
			} else {
				if len(s) > i+1 && s[i+1] == q {
					// Escaped quote qq inside quoted field
					s = s[i+2:]
					continue
				}
				inQuote = false
				s = s[i+1:]
			}
		}
	}
	return n
}
