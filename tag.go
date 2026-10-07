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
	format    string
}

func parseTag(tagStr string) csvTag {
	tagStr = strings.TrimSpace(tagStr)
	if tagStr == "" {
		return csvTag{}
	}
	if tagStr == "-" {
		return csvTag{ignore: true}
	}

	var parts []string
	var current strings.Builder
	inFormat := false
	inQuote := byte(0)

	for i := 0; i < len(tagStr); i++ {
		c := tagStr[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			current.WriteByte(c)
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			current.WriteByte(c)
			continue
		}

		if c == ',' {
			if inFormat {
				rest := strings.TrimSpace(tagStr[i+1:])
				if rest == "omitempty" || strings.HasPrefix(rest, "omitempty,") ||
					rest == "inline" || strings.HasPrefix(rest, "inline,") ||
					rest == "unique" || strings.HasPrefix(rest, "unique,") ||
					rest == "intern" || strings.HasPrefix(rest, "intern,") ||
					strings.HasPrefix(rest, "format:") || strings.HasPrefix(rest, "format=") {
					parts = append(parts, current.String())
					current.Reset()
					inFormat = false
					continue
				}
				current.WriteByte(c)
				continue
			}
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(c)
		s := current.String()
		trimmed := strings.TrimSpace(s)
		if !inFormat && (strings.HasPrefix(trimmed, "format:") || strings.HasPrefix(trimmed, "format=")) {
			inFormat = true
		}
	}
	if current.Len() > 0 || len(parts) > 0 {
		parts = append(parts, current.String())
	}

	tag := csvTag{
		name: strings.TrimSpace(parts[0]),
	}
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		switch {
		case opt == "omitempty":
			tag.omitEmpty = true
		case opt == "inline":
			tag.inline = true
		case opt == "unique" || opt == "intern":
			tag.unique = true
		case strings.HasPrefix(opt, "format="):
			tag.format = strings.Trim(strings.TrimPrefix(opt, "format="), "\"'")
		case strings.HasPrefix(opt, "format:"):
			tag.format = strings.Trim(strings.TrimPrefix(opt, "format:"), "\"'")
		}
	}
	return tag
}

