package csv

import (
	"errors"
	"fmt"
	"strings"
)

// ErrDuplicate indicates that a duplicate value was found for a field marked with `unique`.
var ErrDuplicate = errors.New("csv: duplicate value for unique field")

// DuplicateFieldError provides detailed context about the unique constraint violation.
type DuplicateFieldError struct {
	Field string
	Value string
	Row   int
}

func (e *DuplicateFieldError) Error() string {
	if e.Row > 0 {
		return fmt.Sprintf("csv: duplicate value %q for unique field %q at row %d", e.Value, e.Field, e.Row)
	}
	return fmt.Sprintf("csv: duplicate value %q for unique field %q", e.Value, e.Field)
}

func (e *DuplicateFieldError) Unwrap() error {
	return ErrDuplicate
}

type csvTag struct {
	name      string
	ignore    bool
	omitEmpty bool
	inline    bool
	unique    bool
}

func parseTag(tagStr string) csvTag {
	if tagStr == "-" {
		return csvTag{ignore: true}
	}
	parts := strings.Split(tagStr, ",")
	tag := csvTag{
		name: strings.TrimSpace(parts[0]),
	}
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		switch opt {
		case "omitempty":
			tag.omitEmpty = true
		case "inline":
			tag.inline = true
		case "unique", "intern":
			tag.unique = true
		}
	}
	return tag
}

