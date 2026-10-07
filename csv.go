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
func Unmarshal(data []byte, v any, opts ...Option) error {
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

	dec, err := NewDecoder(bytes.NewReader(data), opts...)
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

	c := countRecords(data, dec.r.quoteByte)
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
					if err := f.setter(structPtr, raw); err != nil {
						headerName := ""
						if f.colIndex < len(dec.headers) {
							headerName = dec.headers[f.colIndex]
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
				}
			}

			*(*unsafe.Pointer)(unsafe.Add(sliceBasePtr, uintptr(rowIdx)*ptrSize)) = structPtr
		} else {
			structPtr := unsafe.Add(sliceBasePtr, uintptr(rowIdx)*elemSize)

			for _, f := range plan.fields {
				if f.colIndex < numFields {
					raw := rec.Field(f.colIndex)
					if err := f.setter(structPtr, raw); err != nil {
						headerName := ""
						if f.colIndex < len(dec.headers) {
							headerName = dec.headers[f.colIndex]
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
				}
			}
		}

		rowIdx++
	}

	val.Elem().Set(slice.Slice(0, rowIdx))
	return nil
}

// Marshal encodes a slice of structs or slice of struct pointers into CSV bytes.
func Marshal(v any, opts ...any) ([]byte, error) {
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return nil, errors.New("csv: Marshal(nil)")
	}
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, fmt.Errorf("csv: Marshal expects a slice or array, got %v", val.Kind())
	}

	elemType := val.Type().Elem()
	isPtr := false
	structType := elemType
	if elemType.Kind() == reflect.Pointer {
		isPtr = true
		structType = elemType.Elem()
	}

	if structType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("csv: slice elements must be structs or pointers to structs, got %v", elemType)
	}

	plan, err := getTypeMarshalPlan(structType)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := NewWriter(&buf, opts...)

	if err := w.Write(plan.headerRow); err != nil {
		return nil, err
	}

	n := val.Len()
	elemSize := structType.Size()
	ptrSize := unsafe.Sizeof(uintptr(0))

	if n > 0 {
		var sliceBasePtr unsafe.Pointer
		if val.Kind() == reflect.Slice {
			sliceBasePtr = val.UnsafePointer()
		} else {
			if val.CanAddr() {
				sliceBasePtr = val.Addr().UnsafePointer()
			} else {
				newVal := reflect.New(val.Type()).Elem()
				newVal.Set(val)
				sliceBasePtr = newVal.Addr().UnsafePointer()
			}
		}
		for i := 0; i < n; i++ {
			var structPtr unsafe.Pointer
			if isPtr {
				structPtr = *(*unsafe.Pointer)(unsafe.Add(sliceBasePtr, uintptr(i)*ptrSize))
				if structPtr == nil {
					for j := range plan.fields {
						if j > 0 {
							w.WriteDelimiter()
						}
					}
					if err := w.WriteNewline(); err != nil {
						return nil, err
					}
					continue
				}
			} else {
				structPtr = unsafe.Add(sliceBasePtr, uintptr(i)*elemSize)
			}

			for j, f := range plan.fields {
				if j > 0 {
					w.WriteDelimiter()
				}
				if err := f.getter(structPtr, w); err != nil {
					return nil, fmt.Errorf("csv: error encoding field %s: %w", f.colName, err)
				}
			}
			if err := w.WriteNewline(); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Flush(); err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

func countRecords(s []byte, quote ...byte) int {
	if len(s) == 0 {
		return 0
	}

	q := byte('"')
	if len(quote) > 0 && quote[0] != 0 {
		q = quote[0]
	}

	// Fast path: if there are no quotes in the data, count newlines directly
	if bytes.IndexByte(s, q) == -1 {
		n := bytes.Count(s, []byte{'\n'})
		if s[len(s)-1] != '\n' {
			n++
		}
		return n
	}

	cutset := string([]byte{'\n', q})
	hasTrailingRecord := s[len(s)-1] != '\n'

	var n int
	inQuote := false
	for len(s) > 0 {
		i := bytes.IndexAny(s, cutset)
		if i == -1 {
			break
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
	if hasTrailingRecord {
		n++
	}
	return n
}
