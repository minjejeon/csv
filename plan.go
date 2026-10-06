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

type structFieldInfo struct {
	name      string
	offset    uintptr
	index     []int
	fieldType reflect.Type
	tag       csvTag
}

func collectStructFields(t reflect.Type, baseOffset uintptr, baseIndex []int) []structFieldInfo {
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

		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			embedded := collectStructFields(f.Type, currOffset, currIndex)
			fields = append(fields, embedded...)
			continue
		}

		fields = append(fields, structFieldInfo{
			name:      f.Name,
			offset:    currOffset,
			index:     currIndex,
			fieldType: f.Type,
			tag:       tag,
		})
	}
	return fields
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

func buildTypePlan(t reflect.Type, headers []string) (*typePlan, error) {
	allFields := collectStructFields(t, 0, nil)
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
				if h == targetName {
					colIdx = i
					break
				}
			}
			// 2. Case-insensitive match with tag name
			if colIdx == -1 {
				for i, h := range headers {
					if strings.EqualFold(h, targetName) {
						colIdx = i
						break
					}
				}
			}
		} else {
			// 3. Exact match with field name
			for i, h := range headers {
				if h == sf.name {
					colIdx = i
					break
				}
			}
			// 4. Case-insensitive match with field name
			if colIdx == -1 {
				for i, h := range headers {
					if strings.EqualFold(h, sf.name) {
						colIdx = i
						break
					}
				}
			}
		}

		if colIdx >= 0 && !usedCols[colIdx] {
			usedCols[colIdx] = true
			plannedFields = append(plannedFields, fieldPlan{
				fieldName: sf.name,
				colIndex:  colIdx,
				offset:    sf.offset,
				index:     sf.index,
				fieldType: sf.fieldType,
				tag:       sf.tag,
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
