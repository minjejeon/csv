package csv

// Option configures Reader behavior.
type Option func(*Reader)

// WithDelimiter sets a custom field delimiter, which can be single or multi-character (e.g. "|", "||", "::").
func WithDelimiter(d string) Option {
	return func(r *Reader) {
		r.Delimiter = d
		if len(d) == 1 {
			r.Comma = rune(d[0])
		}
	}
}

// WithComma sets a single-character field delimiter (standard encoding/csv compatible).
func WithComma(c rune) Option {
	return func(r *Reader) {
		r.Comma = c
		r.Delimiter = string(c)
	}
}

// WithQuote sets a custom quote character (default '"').
func WithQuote(q rune) Option {
	return func(r *Reader) {
		r.Quote = q
	}
}

// WithComment sets the comment character.
func WithComment(c rune) Option {
	return func(r *Reader) {
		r.Comment = c
	}
}

// WithLazyQuotes enables or disables lazy quotes.
func WithLazyQuotes(lazy bool) Option {
	return func(r *Reader) {
		r.LazyQuotes = lazy
	}
}

// WithTrimLeadingSpace enables or disables trimming leading whitespace from fields.
func WithTrimLeadingSpace(trim bool) Option {
	return func(r *Reader) {
		r.TrimLeadingSpace = trim
	}
}

// WithFieldsPerRecord sets the expected number of fields per record.
func WithFieldsPerRecord(n int) Option {
	return func(r *Reader) {
		r.FieldsPerRecord = n
	}
}

// WithTrimBOM specifies whether to automatically detect and strip leading UTF-8 BOM (\xef\xbb\xbf)
// from the input stream. Defaults to true.
func WithTrimBOM(trim bool) Option {
	return func(r *Reader) {
		r.TrimBOM = trim
	}
}
