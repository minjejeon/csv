package csv

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

type fieldSetter func(structPtr unsafe.Pointer, raw []byte) error

type fieldPlan struct {
	fieldName string
	colIndex  int
	offset    uintptr
	index     []int
	fieldType reflect.Type
	tag       csvTag
	setter    fieldSetter
	zeroer    func(structPtr unsafe.Pointer)
}

type typePlan struct {
	targetType reflect.Type
	fields     []fieldPlan
}

type planKey struct {
	typ     reflect.Type
	headers string
}

var planCache sync.Map

type ptrOffset struct {
	offset uintptr
	typ    reflect.Type
}

type structFieldInfo struct {
	name       string
	offset     uintptr
	index      []int
	fieldType  reflect.Type
	tag        csvTag
	ptrOffsets []ptrOffset
}

func collectStructFields(t reflect.Type, baseOffset uintptr, baseIndex []int, ptrOffsets []ptrOffset) []structFieldInfo {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var fields []structFieldInfo
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}

		tag := parseTag(f.Tag.Get("csv"))
		if tag.ignore {
			continue
		}

		currOffset := baseOffset + f.Offset
		currIndex := append(slicesClone(baseIndex), f.Index...)

		if f.Anonymous {
			ft := f.Type
			if ft.Kind() == reflect.Struct {
				embedded := collectStructFields(ft, currOffset, currIndex, ptrOffsets)
				fields = append(fields, embedded...)
				continue
			} else if ft.Kind() == reflect.Pointer && ft.Elem().Kind() == reflect.Struct {
				subType := ft.Elem()
				newPtrOffsets := append(slicesClonePtrOffsets(ptrOffsets), ptrOffset{offset: currOffset, typ: subType})
				embedded := collectStructFields(subType, 0, currIndex, newPtrOffsets)
				fields = append(fields, embedded...)
				continue
			}
		}

		fields = append(fields, structFieldInfo{
			name:       f.Name,
			offset:     currOffset,
			index:      currIndex,
			fieldType:  f.Type,
			tag:        tag,
			ptrOffsets: ptrOffsets,
		})
	}
	return fields
}

func slicesClonePtrOffsets(s []ptrOffset) []ptrOffset {
	if len(s) == 0 {
		return nil
	}
	c := make([]ptrOffset, len(s))
	copy(c, s)
	return c
}

func slicesClone(s []int) []int {
	c := make([]int, len(s))
	copy(c, s)
	return c
}

// getTypePlan returns a compiled execution plan for decoding CSV rows with given headers into type t.
func getTypePlan(t reflect.Type, headers []string) (*typePlan, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("csv: target must be a struct or pointer to struct, got %v", t)
	}

	key := planKey{
		typ:     t,
		headers: strings.Join(headers, "\x00"),
	}

	if val, ok := planCache.Load(key); ok {
		return val.(*typePlan), nil
	}

	plan, err := buildTypePlan(t, headers)
	if err != nil {
		return nil, err
	}

	planCache.Store(key, plan)
	return plan, nil
}

func compileZeroer(t reflect.Type, offset uintptr) func(structPtr unsafe.Pointer) {
	switch t.Kind() {
	case reflect.String:
		return func(structPtr unsafe.Pointer) {
			*(*string)(unsafe.Add(structPtr, offset)) = ""
		}
	case reflect.Int:
		return func(structPtr unsafe.Pointer) {
			*(*int)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Int64:
		return func(structPtr unsafe.Pointer) {
			*(*int64)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Int32:
		return func(structPtr unsafe.Pointer) {
			*(*int32)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Int16:
		return func(structPtr unsafe.Pointer) {
			*(*int16)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Int8:
		return func(structPtr unsafe.Pointer) {
			*(*int8)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Uint:
		return func(structPtr unsafe.Pointer) {
			*(*uint)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Uint64:
		return func(structPtr unsafe.Pointer) {
			*(*uint64)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Uint32:
		return func(structPtr unsafe.Pointer) {
			*(*uint32)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Uint16:
		return func(structPtr unsafe.Pointer) {
			*(*uint16)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Uint8:
		return func(structPtr unsafe.Pointer) {
			*(*uint8)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Float64:
		return func(structPtr unsafe.Pointer) {
			*(*float64)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Float32:
		return func(structPtr unsafe.Pointer) {
			*(*float32)(unsafe.Add(structPtr, offset)) = 0
		}
	case reflect.Bool:
		return func(structPtr unsafe.Pointer) {
			*(*bool)(unsafe.Add(structPtr, offset)) = false
		}
	case reflect.Pointer:
		return func(structPtr unsafe.Pointer) {
			*(*unsafe.Pointer)(unsafe.Add(structPtr, offset)) = nil
		}
	default:
		zeroVal := reflect.Zero(t)
		return func(structPtr unsafe.Pointer) {
			reflect.NewAt(t, unsafe.Add(structPtr, offset)).Elem().Set(zeroVal)
		}
	}
}

func buildTypePlan(t reflect.Type, headers []string) (*typePlan, error) {
	allFields := collectStructFields(t, 0, nil, nil)
	if len(allFields) == 0 {
		return &typePlan{targetType: t}, nil
	}

	var plannedFields []fieldPlan
	usedCols := make(map[int]bool)

	// For each struct field, find best matching column in headers
	for _, sf := range allFields {
		colIdx := -1

		targetName := sf.tag.name
		if targetName != "" {
			// 1. Exact match with tag name
			for i, h := range headers {
				if !usedCols[i] && h == targetName {
					colIdx = i
					break
				}
			}
			// 2. Case-insensitive match with tag name
			if colIdx == -1 {
				for i, h := range headers {
					if !usedCols[i] && strings.EqualFold(h, targetName) {
						colIdx = i
						break
					}
				}
			}
		} else {
			// 3. Exact match with field name
			for i, h := range headers {
				if !usedCols[i] && h == sf.name {
					colIdx = i
					break
				}
			}
			// 4. Case-insensitive match with field name
			if colIdx == -1 {
				for i, h := range headers {
					if !usedCols[i] && strings.EqualFold(h, sf.name) {
						colIdx = i
						break
					}
				}
			}
		}

		if colIdx >= 0 {
			usedCols[colIdx] = true
			setter, err := compileSetter(sf.fieldType, sf.offset, sf.tag)
			if err != nil {
				return nil, err
			}
			zeroer := compileZeroer(sf.fieldType, sf.offset)
			if len(sf.ptrOffsets) > 0 {
				pos := sf.ptrOffsets
				innerSetter := setter
				innerZeroer := zeroer
				setter = func(structPtr unsafe.Pointer, raw []byte) error {
					curr := structPtr
					for _, po := range pos {
						ptrLoc := (*unsafe.Pointer)(unsafe.Add(curr, po.offset))
						if *ptrLoc == nil {
							newVal := reflect.New(po.typ)
							*ptrLoc = newVal.UnsafePointer()
						}
						curr = *ptrLoc
					}
					return innerSetter(curr, raw)
				}
				zeroer = func(structPtr unsafe.Pointer) {
					curr := structPtr
					for _, po := range pos {
						ptrLoc := (*unsafe.Pointer)(unsafe.Add(curr, po.offset))
						if *ptrLoc == nil {
							return
						}
						curr = *ptrLoc
					}
					innerZeroer(curr)
				}
			}
			plannedFields = append(plannedFields, fieldPlan{
				fieldName: sf.name,
				colIndex:  colIdx,
				offset:    sf.offset,
				index:     sf.index,
				fieldType: sf.fieldType,
				tag:       sf.tag,
				setter:    setter,
				zeroer:    zeroer,
			})
		}
	}

	if len(plannedFields) == 0 && len(headers) > 0 {
		// No matched fields
		return nil, errors.New("csv: no matching fields found between headers and struct")
	}

	return &typePlan{
		targetType: t,
		fields:     plannedFields,
	}, nil
}
