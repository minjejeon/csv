package csv

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"time"
	"unsafe"
)

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	timeType            = reflect.TypeOf(time.Time{})
)

func parseSignedInt(b []byte, bitSize int) (int64, error) {
	if len(b) == 0 {
		return 0, strconv.ErrSyntax
	}
	neg := false
	if b[0] == '-' {
		neg = true
		b = b[1:]
	} else if b[0] == '+' {
		b = b[1:]
	}
	if len(b) == 0 {
		return 0, strconv.ErrSyntax
	}

	var maxVal uint64
	if neg {
		maxVal = 1 << (bitSize - 1)
	} else {
		maxVal = (1 << (bitSize - 1)) - 1
	}
	cutoff := maxVal / 10
	maxDigit := maxVal % 10

	var n uint64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, strconv.ErrSyntax
		}
		d := uint64(c - '0')
		if n > cutoff || (n == cutoff && d > maxDigit) {
			return 0, strconv.ErrRange
		}
		n = n*10 + d
	}

	if neg {
		return -int64(n), nil
	}
	return int64(n), nil
}

func parseUnsignedInt(b []byte, bitSize int) (uint64, error) {
	if len(b) == 0 {
		return 0, strconv.ErrSyntax
	}
	if b[0] == '+' {
		b = b[1:]
	}
	if len(b) == 0 {
		return 0, strconv.ErrSyntax
	}

	var maxVal uint64
	if bitSize == 64 {
		maxVal = ^uint64(0)
	} else {
		maxVal = (uint64(1) << bitSize) - 1
	}
	cutoff := maxVal / 10
	maxDigit := maxVal % 10

	var n uint64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, strconv.ErrSyntax
		}
		d := uint64(c - '0')
		if n > cutoff || (n == cutoff && d > maxDigit) {
			return 0, strconv.ErrRange
		}
		n = n*10 + d
	}
	return n, nil
}

func parseBoolFast(b []byte) (bool, error) {
	switch len(b) {
	case 1:
		switch b[0] {
		case '1', 't', 'T':
			return true, nil
		case '0', 'f', 'F':
			return false, nil
		}
	case 4:
		if (b[0] == 't' || b[0] == 'T') &&
			(b[1] == 'r' || b[1] == 'R') &&
			(b[2] == 'u' || b[2] == 'U') &&
			(b[3] == 'e' || b[3] == 'E') {
			return true, nil
		}
	case 5:
		if (b[0] == 'f' || b[0] == 'F') &&
			(b[1] == 'a' || b[1] == 'A') &&
			(b[2] == 'l' || b[2] == 'L') &&
			(b[3] == 's' || b[3] == 'S') &&
			(b[4] == 'e' || b[4] == 'E') {
			return false, nil
		}
	}
	return false, strconv.ErrSyntax
}

func compileSetter(t reflect.Type, offset uintptr, tag csvTag) (fieldSetter, error) {
	// Check TextUnmarshaler on pointer type first
	ptrType := reflect.PointerTo(t)
	if ptrType.Implements(textUnmarshalerType) {
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 && tag.omitEmpty {
				return nil
			}
			target := unsafe.Add(structPtr, offset)
			val := reflect.NewAt(t, target)
			u := val.Interface().(encoding.TextUnmarshaler)
			return u.UnmarshalText(raw)
		}, nil
	}

	if t == timeType {
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					return nil
				}
				return nil
			}
			s := unsafe.String(unsafe.SliceData(raw), len(raw))
			parsed, err := time.Parse(time.RFC3339, s)
			if err != nil {
				// Try date only fallback
				parsed, err = time.Parse("2006-01-02", s)
				if err != nil {
					return err
				}
			}
			*(*time.Time)(unsafe.Add(structPtr, offset)) = parsed
			return nil
		}, nil
	}

	switch t.Kind() {
	case reflect.String:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 && tag.omitEmpty {
				*(*string)(unsafe.Add(structPtr, offset)) = ""
				return nil
			}
			*(*string)(unsafe.Add(structPtr, offset)) = string(raw)
			return nil
		}, nil

	case reflect.Int:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*int)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseSignedInt(raw, strconv.IntSize)
			if err != nil {
				return err
			}
			*(*int)(unsafe.Add(structPtr, offset)) = int(v)
			return nil
		}, nil

	case reflect.Int8:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*int8)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseSignedInt(raw, 8)
			if err != nil {
				return err
			}
			*(*int8)(unsafe.Add(structPtr, offset)) = int8(v)
			return nil
		}, nil

	case reflect.Int16:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*int16)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseSignedInt(raw, 16)
			if err != nil {
				return err
			}
			*(*int16)(unsafe.Add(structPtr, offset)) = int16(v)
			return nil
		}, nil

	case reflect.Int32:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*int32)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseSignedInt(raw, 32)
			if err != nil {
				return err
			}
			*(*int32)(unsafe.Add(structPtr, offset)) = int32(v)
			return nil
		}, nil

	case reflect.Int64:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*int64)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseSignedInt(raw, 64)
			if err != nil {
				return err
			}
			*(*int64)(unsafe.Add(structPtr, offset)) = v
			return nil
		}, nil

	case reflect.Uint:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*uint)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseUnsignedInt(raw, strconv.IntSize)
			if err != nil {
				return err
			}
			*(*uint)(unsafe.Add(structPtr, offset)) = uint(v)
			return nil
		}, nil

	case reflect.Uint8:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*uint8)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseUnsignedInt(raw, 8)
			if err != nil {
				return err
			}
			*(*uint8)(unsafe.Add(structPtr, offset)) = uint8(v)
			return nil
		}, nil

	case reflect.Uint16:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*uint16)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseUnsignedInt(raw, 16)
			if err != nil {
				return err
			}
			*(*uint16)(unsafe.Add(structPtr, offset)) = uint16(v)
			return nil
		}, nil

	case reflect.Uint32:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*uint32)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseUnsignedInt(raw, 32)
			if err != nil {
				return err
			}
			*(*uint32)(unsafe.Add(structPtr, offset)) = uint32(v)
			return nil
		}, nil

	case reflect.Uint64:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*uint64)(unsafe.Add(structPtr, offset)) = 0
					return nil
				}
				return strconv.ErrSyntax
			}
			v, err := parseUnsignedInt(raw, 64)
			if err != nil {
				return err
			}
			*(*uint64)(unsafe.Add(structPtr, offset)) = v
			return nil
		}, nil

	case reflect.Float32:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 && tag.omitEmpty {
				*(*float32)(unsafe.Add(structPtr, offset)) = 0
				return nil
			}
			s := unsafe.String(unsafe.SliceData(raw), len(raw))
			v, err := strconv.ParseFloat(s, 32)
			if err != nil {
				return err
			}
			*(*float32)(unsafe.Add(structPtr, offset)) = float32(v)
			return nil
		}, nil

	case reflect.Float64:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 && tag.omitEmpty {
				*(*float64)(unsafe.Add(structPtr, offset)) = 0
				return nil
			}
			s := unsafe.String(unsafe.SliceData(raw), len(raw))
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*(*float64)(unsafe.Add(structPtr, offset)) = v
			return nil
		}, nil

	case reflect.Bool:
		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 && tag.omitEmpty {
				*(*bool)(unsafe.Add(structPtr, offset)) = false
				return nil
			}
			v, err := parseBoolFast(raw)
			if err != nil {
				return err
			}
			*(*bool)(unsafe.Add(structPtr, offset)) = v
			return nil
		}, nil

	case reflect.Pointer:
		elemType := t.Elem()
		innerSetter, err := compileSetter(elemType, 0, tag)
		if err != nil {
			return nil, err
		}

		return func(structPtr unsafe.Pointer, raw []byte) error {
			ptrLocation := (*unsafe.Pointer)(unsafe.Add(structPtr, offset))
			if len(raw) == 0 {
				*ptrLocation = nil
				return nil
			}

			if *ptrLocation == nil {
				newElem := reflect.New(elemType)
				*ptrLocation = newElem.UnsafePointer()
			}
			return innerSetter(*ptrLocation, raw)
		}, nil

	default:
		return nil, fmt.Errorf("csv: unsupported field type %v", t)
	}
}
