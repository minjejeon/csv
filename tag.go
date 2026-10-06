package csv

import "strings"

type csvTag struct {
	name      string
	ignore    bool
	omitEmpty bool
	inline    bool
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
		}
	}
	return tag
}
