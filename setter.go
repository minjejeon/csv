package csv

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unique"
	"unsafe"
)

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	timeType            = reflect.TypeOf(time.Time{})
)

func fastParseSmallUint(b []byte) (uint64, bool) {
	switch len(b) {
	case 1:
		d0 := b[0] - '0'
		if d0 > 9 {
			return 0, false
		}
		return uint64(d0), true
	case 2:
		d0 := b[0] - '0'
		d1 := b[1] - '0'
		if (d0 | d1) > 9 {
			return 0, false
		}
		return uint64(d0)*10 + uint64(d1), true
	case 3:
		d0 := b[0] - '0'
		d1 := b[1] - '0'
		d2 := b[2] - '0'
		if (d0 | d1 | d2) > 9 {
			return 0, false
		}
		return uint64(d0)*100 + uint64(d1)*10 + uint64(d2), true
	case 4:
		d0 := b[0] - '0'
		d1 := b[1] - '0'
		d2 := b[2] - '0'
		d3 := b[3] - '0'
		if (d0 | d1 | d2 | d3) > 9 {
			return 0, false
		}
		return uint64(d0)*1000 + uint64(d1)*100 + uint64(d2)*10 + uint64(d3), true
	case 5, 6, 7, 8:
		var n uint64
		for _, c := range b {
			d := c - '0'
			if d > 9 {
				return 0, false
			}
			n = n*10 + uint64(d)
		}
		return n, true
	default:
		return 0, false
	}
}

func parseSignedInt(b []byte, bitSize int) (int64, error) {
	if len(b) > 0 {
		switch bitSize {
		case 64, 0:
			if len(b) <= 8 {
				if v, ok := fastParseSmallUint(b); ok {
					return int64(v), nil
				}
			}
		case 32:
			if len(b) <= 8 {
				if v, ok := fastParseSmallUint(b); ok {
					return int64(v), nil
				}
			}
		case 16:
			if len(b) <= 4 {
				if v, ok := fastParseSmallUint(b); ok {
					return int64(v), nil
				}
			}
		case 8:
			if len(b) <= 2 {
				if v, ok := fastParseSmallUint(b); ok {
					return int64(v), nil
				}
			}
		}
	}

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
		maxVal = uint64(1) << (bitSize - 1)
	} else {
		maxVal = (uint64(1) << (bitSize - 1)) - 1
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
		if bitSize == 64 && n == uint64(1)<<63 {
			return math.MinInt64, nil
		}
		return -int64(n), nil
	}
	return int64(n), nil
}

func parseUnsignedInt(b []byte, bitSize int) (uint64, error) {
	if len(b) > 0 {
		switch bitSize {
		case 64, 0:
			if len(b) <= 8 {
				if v, ok := fastParseSmallUint(b); ok {
					return v, nil
				}
			}
		case 32:
			if len(b) <= 8 {
				if v, ok := fastParseSmallUint(b); ok {
					return v, nil
				}
			}
		case 16:
			if len(b) <= 4 {
				if v, ok := fastParseSmallUint(b); ok {
					return v, nil
				}
			}
		case 8:
			if len(b) <= 2 {
				if v, ok := fastParseSmallUint(b); ok {
					return v, nil
				}
			}
		}
	}

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

func resolveTimeFormat(f string) string {
	switch strings.ToLower(f) {
	case "rfc3339":
		return time.RFC3339
	case "dateonly", "date":
		return "2006-01-02"
	case "datetime":
		return "2006-01-02 15:04:05"
	case "rfc822":
		return time.RFC822
	case "rfc1123":
		return time.RFC1123
	default:
		return f
	}
}

func compileSetter(t reflect.Type, offset uintptr, tag csvTag) (fieldSetter, error) {
	if t == timeType {
		format := tag.format
		isUnix := strings.EqualFold(format, "unix")
		isUnixMilli := strings.EqualFold(format, "unixmilli")

		return func(structPtr unsafe.Pointer, raw []byte) error {
			if len(raw) == 0 {
				if tag.omitEmpty {
					*(*time.Time)(unsafe.Add(structPtr, offset)) = time.Time{}
					return nil
				}
				return strconv.ErrSyntax
			}

			if isUnix {
				sec, err := parseSignedInt(raw, 64)
				if err != nil {
					return err
				}
				*(*time.Time)(unsafe.Add(structPtr, offset)) = time.Unix(sec, 0).UTC()
				return nil
			}
			if isUnixMilli {
				milli, err := parseSignedInt(raw, 64)
				if err != nil {
					return err
				}
				*(*time.Time)(unsafe.Add(structPtr, offset)) = time.UnixMilli(milli).UTC()
				return nil
			}

			s := unsafe.String(unsafe.SliceData(raw), len(raw))
			if format != "" {
				parsed, err := time.Parse(resolveTimeFormat(format), s)
				if err != nil {
					return err
				}
				*(*time.Time)(unsafe.Add(structPtr, offset)) = parsed
				return nil
			}

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

	// Check TextUnmarshaler on pointer type
	ptrType := reflect.PointerTo(t)
	if ptrType.Implements(textUnmarshalerType) {
		return func(structPtr unsafe.Pointer, raw []byte) error {
			target := unsafe.Add(structPtr, offset)
			val := reflect.NewAt(t, target)
			if len(raw) == 0 && tag.omitEmpty {
				val.Elem().Set(reflect.Zero(t))
				return nil
			}
			u := val.Interface().(encoding.TextUnmarshaler)
			return u.UnmarshalText(raw)
		}, nil
	}

	if t.PkgPath() == "unique" && strings.HasPrefix(t.Name(), "Handle[") {
		if valMethod, ok := t.MethodByName("Value"); ok {
			elemType := valMethod.Type.Out(0)
			switch elemType.Kind() {
			case reflect.String:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					s := string(raw)
					h := unique.Make(s)
					*(*unique.Handle[string])(unsafe.Add(structPtr, offset)) = h
					return nil
				}, nil
			case reflect.Int:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[int])(unsafe.Add(structPtr, offset)) = unique.Make(0)
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseSignedInt(raw, strconv.IntSize)
					if err != nil {
						return err
					}
					*(*unique.Handle[int])(unsafe.Add(structPtr, offset)) = unique.Make(int(v))
					return nil
				}, nil
			case reflect.Int64:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[int64])(unsafe.Add(structPtr, offset)) = unique.Make(int64(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseSignedInt(raw, 64)
					if err != nil {
						return err
					}
					*(*unique.Handle[int64])(unsafe.Add(structPtr, offset)) = unique.Make(v)
					return nil
				}, nil
			case reflect.Int32:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[int32])(unsafe.Add(structPtr, offset)) = unique.Make(int32(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseSignedInt(raw, 32)
					if err != nil {
						return err
					}
					*(*unique.Handle[int32])(unsafe.Add(structPtr, offset)) = unique.Make(int32(v))
					return nil
				}, nil
			case reflect.Int16:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[int16])(unsafe.Add(structPtr, offset)) = unique.Make(int16(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseSignedInt(raw, 16)
					if err != nil {
						return err
					}
					*(*unique.Handle[int16])(unsafe.Add(structPtr, offset)) = unique.Make(int16(v))
					return nil
				}, nil
			case reflect.Int8:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[int8])(unsafe.Add(structPtr, offset)) = unique.Make(int8(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseSignedInt(raw, 8)
					if err != nil {
						return err
					}
					*(*unique.Handle[int8])(unsafe.Add(structPtr, offset)) = unique.Make(int8(v))
					return nil
				}, nil
			case reflect.Uint:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[uint])(unsafe.Add(structPtr, offset)) = unique.Make(uint(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseUnsignedInt(raw, strconv.IntSize)
					if err != nil {
						return err
					}
					*(*unique.Handle[uint])(unsafe.Add(structPtr, offset)) = unique.Make(uint(v))
					return nil
				}, nil
			case reflect.Uint64:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[uint64])(unsafe.Add(structPtr, offset)) = unique.Make(uint64(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseUnsignedInt(raw, 64)
					if err != nil {
						return err
					}
					*(*unique.Handle[uint64])(unsafe.Add(structPtr, offset)) = unique.Make(v)
					return nil
				}, nil
			case reflect.Uint32:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[uint32])(unsafe.Add(structPtr, offset)) = unique.Make(uint32(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseUnsignedInt(raw, 32)
					if err != nil {
						return err
					}
					*(*unique.Handle[uint32])(unsafe.Add(structPtr, offset)) = unique.Make(uint32(v))
					return nil
				}, nil
			case reflect.Uint16:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[uint16])(unsafe.Add(structPtr, offset)) = unique.Make(uint16(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseUnsignedInt(raw, 16)
					if err != nil {
						return err
					}
					*(*unique.Handle[uint16])(unsafe.Add(structPtr, offset)) = unique.Make(uint16(v))
					return nil
				}, nil
			case reflect.Uint8:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[uint8])(unsafe.Add(structPtr, offset)) = unique.Make(uint8(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					v, err := parseUnsignedInt(raw, 8)
					if err != nil {
						return err
					}
					*(*unique.Handle[uint8])(unsafe.Add(structPtr, offset)) = unique.Make(uint8(v))
					return nil
				}, nil
			case reflect.Float64:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[float64])(unsafe.Add(structPtr, offset)) = unique.Make(0.0)
							return nil
						}
						return strconv.ErrSyntax
					}
					s := unsafe.String(unsafe.SliceData(raw), len(raw))
					v, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return err
					}
					*(*unique.Handle[float64])(unsafe.Add(structPtr, offset)) = unique.Make(v)
					return nil
				}, nil
			case reflect.Float32:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 {
						if tag.omitEmpty {
							*(*unique.Handle[float32])(unsafe.Add(structPtr, offset)) = unique.Make(float32(0))
							return nil
						}
						return strconv.ErrSyntax
					}
					s := unsafe.String(unsafe.SliceData(raw), len(raw))
					v, err := strconv.ParseFloat(s, 32)
					if err != nil {
						return err
					}
					*(*unique.Handle[float32])(unsafe.Add(structPtr, offset)) = unique.Make(float32(v))
					return nil
				}, nil
			case reflect.Bool:
				return func(structPtr unsafe.Pointer, raw []byte) error {
					if len(raw) == 0 && tag.omitEmpty {
						*(*unique.Handle[bool])(unsafe.Add(structPtr, offset)) = unique.Make(false)
						return nil
					}
					v, err := parseBoolFast(raw)
					if err != nil {
						return err
					}
					*(*unique.Handle[bool])(unsafe.Add(structPtr, offset)) = unique.Make(v)
					return nil
				}, nil
			}
		}
	}

	switch t.Kind() {
	case reflect.String:
		if tag.unique {
			return func(structPtr unsafe.Pointer, raw []byte) error {
				if len(raw) == 0 && tag.omitEmpty {
					*(*string)(unsafe.Add(structPtr, offset)) = ""
					return nil
				}
				s := string(raw)
				*(*string)(unsafe.Add(structPtr, offset)) = unique.Make(s).Value()
				return nil
			}, nil
		}
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

	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return func(structPtr unsafe.Pointer, raw []byte) error {
				if len(raw) == 0 {
					if tag.omitEmpty {
						*(*[]byte)(unsafe.Add(structPtr, offset)) = nil
						return nil
					}
					*(*[]byte)(unsafe.Add(structPtr, offset)) = []byte{}
					return nil
				}
				*(*[]byte)(unsafe.Add(structPtr, offset)) = bytes.Clone(raw)
				return nil
			}, nil
		}
		return nil, fmt.Errorf("csv: unsupported field type %v", t)

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
