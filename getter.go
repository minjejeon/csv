package csv

import (
	"encoding"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unique"
	"unsafe"
)

var (
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

type fieldGetterFunc func(structPtr unsafe.Pointer, w *Writer) error

type fieldGetterPlan struct {
	colName string
	offset  uintptr
	tag     csvTag
	getter  fieldGetterFunc
}

type typeMarshalPlan struct {
	structType reflect.Type
	fields     []fieldGetterPlan
	headerRow  []string
}

var marshalPlanCache sync.Map // reflect.Type -> *typeMarshalPlan

func getTypeMarshalPlan(t reflect.Type) (*typeMarshalPlan, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("csv: expected struct type, got %v", t)
	}

	if val, ok := marshalPlanCache.Load(t); ok {
		return val.(*typeMarshalPlan), nil
	}

	plan, err := buildTypeMarshalPlan(t)
	if err != nil {
		return nil, err
	}
	marshalPlanCache.Store(t, plan)
	return plan, nil
}

func buildTypeMarshalPlan(t reflect.Type) (*typeMarshalPlan, error) {
	fieldInfos := collectStructFields(t, 0, nil, nil)
	sort.SliceStable(fieldInfos, func(i, j int) bool {
		return fieldInfos[i].depth < fieldInfos[j].depth
	})

	usedNames := make(map[string]bool)
	var filtered []structFieldInfo
	for _, fi := range fieldInfos {
		colName := fi.name
		if fi.tag.name != "" {
			colName = fi.tag.name
		}
		if usedNames[colName] {
			continue
		}
		usedNames[colName] = true
		filtered = append(filtered, fi)
	}

	plan := &typeMarshalPlan{
		structType: t,
		fields:     make([]fieldGetterPlan, 0, len(filtered)),
		headerRow:  make([]string, 0, len(filtered)),
	}

	for _, fi := range filtered {
		colName := fi.name
		if fi.tag.name != "" {
			colName = fi.tag.name
		}

		getter, err := compileGetter(fi)
		if err != nil {
			return nil, fmt.Errorf("csv: failed compiling getter for field %s: %w", fi.name, err)
		}
		if len(fi.ptrOffsets) > 0 {
			pos := fi.ptrOffsets
			innerGetter := getter
			getter = func(structPtr unsafe.Pointer, w *Writer) error {
				curr := structPtr
				for _, po := range pos {
					ptrLoc := *(*unsafe.Pointer)(unsafe.Add(curr, po.offset))
					if ptrLoc == nil {
						return nil
					}
					curr = ptrLoc
				}
				return innerGetter(curr, w)
			}
		}

		plan.fields = append(plan.fields, fieldGetterPlan{
			colName: colName,
			offset:  fi.offset,
			tag:     fi.tag,
			getter:  getter,
		})
		plan.headerRow = append(plan.headerRow, colName)
	}

	return plan, nil
}

func compileGetter(f structFieldInfo) (fieldGetterFunc, error) {
	offset := f.offset
	omitEmpty := f.tag.omitEmpty

	if f.fieldType == timeType {
		format := f.tag.format
		isUnix := strings.EqualFold(format, "unix")
		isUnixMilli := strings.EqualFold(format, "unixmilli")

		return func(structPtr unsafe.Pointer, w *Writer) error {
			t := *(*time.Time)(unsafe.Add(structPtr, offset))
			if t.IsZero() && omitEmpty {
				return nil
			}
			if isUnix {
				var scratch [32]byte
				b := strconv.AppendInt(scratch[:0], t.Unix(), 10)
				w.WriteFieldBytes(b)
				return nil
			}
			if isUnixMilli {
				var scratch [32]byte
				b := strconv.AppendInt(scratch[:0], t.UnixMilli(), 10)
				w.WriteFieldBytes(b)
				return nil
			}
			layout := time.RFC3339
			if format != "" {
				layout = resolveTimeFormat(format)
			}
			var scratch [64]byte
			b := t.AppendFormat(scratch[:0], layout)
			w.WriteFieldBytes(b)
			return nil
		}, nil
	}

	if f.fieldType.Kind() == reflect.Pointer {
		elemType := f.fieldType.Elem()
		elemGetter, err := compileGetter(structFieldInfo{
			name:      f.name,
			offset:    0,
			fieldType: elemType,
			tag:       f.tag,
		})
		if err != nil {
			return nil, err
		}
		return func(structPtr unsafe.Pointer, w *Writer) error {
			ptr := *(*unsafe.Pointer)(unsafe.Add(structPtr, offset))
			if ptr == nil {
				return nil
			}
			return elemGetter(ptr, w)
		}, nil
	}

	if f.fieldType.Implements(textMarshalerType) || reflect.PointerTo(f.fieldType).Implements(textMarshalerType) {
		t := f.fieldType
		return func(structPtr unsafe.Pointer, w *Writer) error {
			ptr := unsafe.Add(structPtr, offset)
			val := reflect.NewAt(t, ptr)
			if m, ok := val.Interface().(encoding.TextMarshaler); ok {
				b, err := m.MarshalText()
				if err != nil {
					return err
				}
				if len(b) == 0 && omitEmpty {
					return nil
				}
				w.WriteFieldBytes(b)
				return nil
			}
			return nil
		}, nil
	}

	if f.fieldType.PkgPath() == "unique" && strings.HasPrefix(f.fieldType.Name(), "Handle[") {
		if valMethod, ok := f.fieldType.MethodByName("Value"); ok {
			elemType := valMethod.Type.Out(0)
			switch elemType.Kind() {
			case reflect.String:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[string])(unsafe.Add(structPtr, offset))
					if h == (unique.Handle[string]{}) {
						return nil
					}
					val := h.Value()
					if len(val) == 0 && omitEmpty {
						return nil
					}
					w.WriteFieldBytes(unsafe.Slice(unsafe.StringData(val), len(val)))
					return nil
				}, nil
			case reflect.Int:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[int])(unsafe.Add(structPtr, offset))
					var v int
					if h != (unique.Handle[int]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
					return nil
				}, nil
			case reflect.Int64:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[int64])(unsafe.Add(structPtr, offset))
					var v int64
					if h != (unique.Handle[int64]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendInt(scratch[:0], v, 10))
					return nil
				}, nil
			case reflect.Int32:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[int32])(unsafe.Add(structPtr, offset))
					var v int32
					if h != (unique.Handle[int32]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
					return nil
				}, nil
			case reflect.Int16:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[int16])(unsafe.Add(structPtr, offset))
					var v int16
					if h != (unique.Handle[int16]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
					return nil
				}, nil
			case reflect.Int8:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[int8])(unsafe.Add(structPtr, offset))
					var v int8
					if h != (unique.Handle[int8]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
					return nil
				}, nil
			case reflect.Uint:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[uint])(unsafe.Add(structPtr, offset))
					var v uint
					if h != (unique.Handle[uint]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
					return nil
				}, nil
			case reflect.Uint64:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[uint64])(unsafe.Add(structPtr, offset))
					var v uint64
					if h != (unique.Handle[uint64]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendUint(scratch[:0], v, 10))
					return nil
				}, nil
			case reflect.Uint32:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[uint32])(unsafe.Add(structPtr, offset))
					var v uint32
					if h != (unique.Handle[uint32]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
					return nil
				}, nil
			case reflect.Uint16:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[uint16])(unsafe.Add(structPtr, offset))
					var v uint16
					if h != (unique.Handle[uint16]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
					return nil
				}, nil
			case reflect.Uint8:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[uint8])(unsafe.Add(structPtr, offset))
					var v uint8
					if h != (unique.Handle[uint8]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
					return nil
				}, nil
			case reflect.Float64:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[float64])(unsafe.Add(structPtr, offset))
					var v float64
					if h != (unique.Handle[float64]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendFloat(scratch[:0], v, 'g', -1, 64))
					return nil
				}, nil
			case reflect.Float32:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[float32])(unsafe.Add(structPtr, offset))
					var v float32
					if h != (unique.Handle[float32]{}) {
						v = h.Value()
					}
					if v == 0 && omitEmpty {
						return nil
					}
					var scratch [32]byte
					w.WriteFieldBytes(strconv.AppendFloat(scratch[:0], float64(v), 'g', -1, 32))
					return nil
				}, nil
			case reflect.Bool:
				return func(structPtr unsafe.Pointer, w *Writer) error {
					h := *(*unique.Handle[bool])(unsafe.Add(structPtr, offset))
					if h == (unique.Handle[bool]{}) {
						if omitEmpty {
							return nil
						}
						w.WriteFieldBytes([]byte("false"))
						return nil
					}
					val := h.Value()
					if !val && omitEmpty {
						return nil
					}
					if val {
						w.WriteFieldBytes([]byte("true"))
					} else {
						w.WriteFieldBytes([]byte("false"))
					}
					return nil
				}, nil
			}
		}
	}

	switch f.fieldType.Kind() {
	case reflect.String:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			s := *(*string)(unsafe.Add(structPtr, offset))
			if len(s) == 0 && omitEmpty {
				return nil
			}
			w.WriteFieldBytes(unsafe.Slice(unsafe.StringData(s), len(s)))
			return nil
		}, nil

	case reflect.Int:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*int)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
			return nil
		}, nil

	case reflect.Int64:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*int64)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendInt(scratch[:0], v, 10))
			return nil
		}, nil

	case reflect.Int32:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*int32)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
			return nil
		}, nil

	case reflect.Int16:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*int16)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
			return nil
		}, nil

	case reflect.Int8:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*int8)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendInt(scratch[:0], int64(v), 10))
			return nil
		}, nil

	case reflect.Uint:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uint)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
			return nil
		}, nil

	case reflect.Uint64:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uint64)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], v, 10))
			return nil
		}, nil

	case reflect.Uintptr:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uintptr)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
			return nil
		}, nil

	case reflect.Uint32:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uint32)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
			return nil
		}, nil

	case reflect.Uint16:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uint16)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
			return nil
		}, nil

	case reflect.Uint8:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*uint8)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [32]byte
			w.WriteFieldBytes(strconv.AppendUint(scratch[:0], uint64(v), 10))
			return nil
		}, nil

	case reflect.Float64:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*float64)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [64]byte
			w.WriteFieldBytes(strconv.AppendFloat(scratch[:0], v, 'f', -1, 64))
			return nil
		}, nil

	case reflect.Float32:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*float32)(unsafe.Add(structPtr, offset))
			if v == 0 && omitEmpty {
				return nil
			}
			var scratch [64]byte
			w.WriteFieldBytes(strconv.AppendFloat(scratch[:0], float64(v), 'f', -1, 32))
			return nil
		}, nil

	case reflect.Bool:
		return func(structPtr unsafe.Pointer, w *Writer) error {
			v := *(*bool)(unsafe.Add(structPtr, offset))
			if !v && omitEmpty {
				return nil
			}
			if v {
				w.WriteFieldBytes([]byte("true"))
			} else {
				w.WriteFieldBytes([]byte("false"))
			}
			return nil
		}, nil

	case reflect.Slice:
		if f.fieldType.Elem().Kind() == reflect.Uint8 {
			return func(structPtr unsafe.Pointer, w *Writer) error {
				b := *(*[]byte)(unsafe.Add(structPtr, offset))
				if len(b) == 0 && omitEmpty {
					return nil
				}
				w.WriteFieldBytes(b)
				return nil
			}, nil
		}
	}

	return nil, fmt.Errorf("csv: unsupported field type %v", f.fieldType)
}
