package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	ErrBareQuote  = errors.New("bare \" in non-quoted-field")
	ErrQuote      = errors.New("extraneous or missing \" in quoted-field")
	ErrFieldCount = errors.New("wrong number of fields")
)

type ParseError struct {
	Line   int
	Column int
	Err    error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("record on line %d: %v", e.Line, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// Reader reads records from a CSV-encoded file.
type Reader struct {
	Comma            rune // Field delimiter (default ',')
	Comment          rune // Comment character (optional)
	FieldsPerRecord  int  // Number of expected fields per record (<0: no check, 0: first row, >0: fixed)
	LazyQuotes       bool // Allow bare quotes in unquoted fields
	TrimLeadingSpace bool // Trim leading whitespace from fields

	r   io.Reader
	buf []byte

	bufHolder      *[]byte
	spanHolder     *spanHolder
	fieldBufHolder *byteBufHolder

	bufSize int // Read chunk size (default 64KB)
	pos     int
	end     int
	eof     bool
	line    int

	numFieldsFirst int
	record         Record
}

// NewReader returns a new Reader reading from r.
func NewReader(r io.Reader) *Reader {
	bHolder := acquireReadBuf()
	sHolder := acquireSpanHolder()
	fHolder := acquireFieldBufHolder()

	reader := &Reader{
		Comma:           ',',
		FieldsPerRecord: -1,
		r:               r,
		bufHolder:       bHolder,
		buf:             *bHolder,
		spanHolder:      sHolder,
		fieldBufHolder:  fHolder,
		bufSize:         defaultBufferSize,
		line:            1,
	}
	reader.record.r = reader
	reader.record.spans = sHolder.spans[:0]
	return reader
}

// Reset resets the Reader to read from r, reusing allocated buffers.
func (r *Reader) Reset(rd io.Reader) {
	r.r = rd
	r.pos = 0
	r.end = 0
	r.eof = false
	r.line = 1
	r.numFieldsFirst = 0
	if r.spanHolder != nil {
		r.record.spans = r.spanHolder.spans[:0]
	}
}

// Close releases the pooled buffers back to their respective pools.
func (r *Reader) Close() error {
	if r.bufHolder != nil {
		releaseReadBuf(r.bufHolder)
		r.bufHolder = nil
		r.buf = nil
	}
	if r.spanHolder != nil {
		releaseSpanHolder(r.spanHolder)
		r.spanHolder = nil
	}
	if r.fieldBufHolder != nil {
		releaseFieldBufHolder(r.fieldBufHolder)
		r.fieldBufHolder = nil
	}
	return nil
}

func (r *Reader) unescape(b []byte) []byte {
	r.fieldBufHolder.buf = r.fieldBufHolder.buf[:0]
	for i := 0; i < len(b); i++ {
		if b[i] == '"' && i+1 < len(b) && b[i+1] == '"' {
			r.fieldBufHolder.buf = append(r.fieldBufHolder.buf, '"')
			i++ // skip escaped quote
		} else {
			r.fieldBufHolder.buf = append(r.fieldBufHolder.buf, b[i])
		}
	}
	return r.fieldBufHolder.buf
}

// ensureMore reads more data from r.r into r.buf when r.pos >= r.end.
func (r *Reader) ensureMore() error {
	if r.eof {
		return nil
	}
	if r.end == len(r.buf) {
		newSize := len(r.buf) * 2
		newBuf := make([]byte, newSize)
		copy(newBuf, r.buf)
		r.buf = newBuf
	}

	readTarget := len(r.buf)
	if r.bufSize > 0 && r.end+r.bufSize < readTarget {
		readTarget = r.end + r.bufSize
	}

	n, err := r.r.Read(r.buf[r.end:readTarget])
	r.end += n
	if err != nil {
		if errors.Is(err, io.EOF) {
			r.eof = true
			return nil
		}
		return err
	}
	return nil
}

// ReadRecord reads one record and returns a zero-copy Record referencing the internal buffer.
func (r *Reader) ReadRecord() (*Record, error) {
	delim := byte(r.Comma)
	if r.Comma == 0 {
		delim = ','
	}

	// Shift unread bytes to index 0 so the new record always starts cleanly at 0
	if r.pos > 0 {
		n := copy(r.buf, r.buf[r.pos:r.end])
		r.end = n
		r.pos = 0
	}

	r.record.spans = r.record.spans[:0]

	// 1. Skip leading comments and blank lines
	for {
		for r.pos >= r.end && !r.eof {
			if err := r.ensureMore(); err != nil {
				return nil, err
			}
		}
		if r.pos >= r.end && r.eof {
			return nil, io.EOF
		}

		// Blank lines
		if r.buf[r.pos] == '\r' {
			r.pos++
			if r.pos >= r.end && !r.eof {
				if err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
			if r.pos < r.end && r.buf[r.pos] == '\n' {
				r.pos++
			}
			r.line++
			continue
		}
		if r.buf[r.pos] == '\n' {
			r.pos++
			r.line++
			continue
		}

		// Comment lines
		if r.Comment != 0 && rune(r.buf[r.pos]) == r.Comment {
			for {
				idx := bytes.IndexByte(r.buf[r.pos:r.end], '\n')
				if idx >= 0 {
					r.pos += idx + 1
					r.line++
					break
				}
				if r.eof {
					r.pos = r.end
					break
				}
				if err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
			continue
		}
		break
	}

	// Shift again if comments or blank lines advanced pos
	if r.pos > 0 {
		n := copy(r.buf, r.buf[r.pos:r.end])
		r.end = n
		r.pos = 0
	}

	recordLine := r.line

	// 2. Parse fields
	for {
		for r.pos >= r.end && !r.eof {
			if err := r.ensureMore(); err != nil {
				return nil, err
			}
		}

		// Handle empty field at EOF
		if r.pos >= r.end && r.eof {
			r.record.spans = append(r.record.spans, fieldSpan{
				start:      uint32(r.pos),
				end:        uint32(r.pos),
				hasEscapes: false,
			})
			break
		}

		// Trim leading space if requested
		if r.TrimLeadingSpace {
			for r.pos < r.end && (r.buf[r.pos] == ' ' || r.buf[r.pos] == '\t') {
				r.pos++
			}
			for r.pos >= r.end && !r.eof {
				if err := r.ensureMore(); err != nil {
					return nil, err
				}
				for r.pos < r.end && (r.buf[r.pos] == ' ' || r.buf[r.pos] == '\t') {
					r.pos++
				}
			}
		}

		// Check if quoted field
		if r.pos < r.end && r.buf[r.pos] == '"' {
			r.pos++ // consume opening quote
			fieldStart := r.pos
			hasEscapes := false

			for {
				idx := bytes.IndexByte(r.buf[r.pos:r.end], '"')
				if idx >= 0 {
					quotePos := r.pos + idx
					if quotePos+1 >= r.end && !r.eof {
						if err := r.ensureMore(); err != nil {
							return nil, err
						}
					}

					var fieldEnd int
					if quotePos+1 < r.end && r.buf[quotePos+1] == '"' {
						// Escaped quote ""
						hasEscapes = true
						r.pos = quotePos + 2
						continue
					} else if (quotePos+1 < r.end && (r.buf[quotePos+1] == delim || r.buf[quotePos+1] == '\r' || r.buf[quotePos+1] == '\n')) ||
						(quotePos+1 >= r.end && r.eof) {
						// Valid closing quote followed by delimiter, newline, or EOF
						fieldEnd = quotePos
						r.pos = quotePos + 1
					} else if r.LazyQuotes {
						// Bare quote in quoted field with LazyQuotes enabled
						r.pos = quotePos + 1
						continue
					} else {
						return nil, &ParseError{Line: recordLine, Err: ErrQuote}
					}

					// Scan until delimiter, newline, or EOF
					for {
						if r.pos >= r.end && !r.eof {
							if err := r.ensureMore(); err != nil {
								return nil, err
							}
						}
						if r.pos >= r.end && r.eof {
							break
						}
						b := r.buf[r.pos]
						if b == delim || b == '\r' || b == '\n' {
							break
						}
						if !r.LazyQuotes && (b != ' ' && b != '\t') {
							return nil, &ParseError{Line: recordLine, Err: ErrQuote}
						}
						r.pos++
					}

					r.record.spans = append(r.record.spans, fieldSpan{
						start:      uint32(fieldStart),
						end:        uint32(fieldEnd),
						hasEscapes: hasEscapes,
					})
					break
				}

				// Quote not in current buffer
				if r.eof {
					if !r.LazyQuotes {
						return nil, &ParseError{Line: recordLine, Err: ErrQuote}
					}
					r.record.spans = append(r.record.spans, fieldSpan{
						start:      uint32(fieldStart),
						end:        uint32(r.end),
						hasEscapes: hasEscapes,
					})
					r.pos = r.end
					break
				}

				r.pos = r.end
				if err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
		} else {
			// Unquoted field
			fieldStart := r.pos
			foundEnd := false

			for !foundEnd {
				specIdx := findNextSpecial(r.buf[r.pos:r.end], delim)
				if specIdx >= 0 {
					targetPos := r.pos + specIdx
					c := r.buf[targetPos]

					if c == delim || c == '\n' {
						fieldEnd := targetPos
						r.record.spans = append(r.record.spans, fieldSpan{
							start:      uint32(fieldStart),
							end:        uint32(fieldEnd),
							hasEscapes: false,
						})
						r.pos = targetPos
						foundEnd = true
						break
					} else if c == '\r' {
						// Check if followed by \n
						if targetPos+1 >= r.end && !r.eof {
							if err := r.ensureMore(); err != nil {
								return nil, err
							}
						}
						if targetPos+1 < r.end && r.buf[targetPos+1] == '\n' {
							// CRLF is end of line
							fieldEnd := targetPos
							r.record.spans = append(r.record.spans, fieldSpan{
								start:      uint32(fieldStart),
								end:        uint32(fieldEnd),
								hasEscapes: false,
							})
							r.pos = targetPos
							foundEnd = true
							break
						} else if targetPos+1 >= r.end && r.eof {
							// Trailing CR at EOF
							fieldEnd := targetPos
							r.record.spans = append(r.record.spans, fieldSpan{
								start:      uint32(fieldStart),
								end:        uint32(fieldEnd),
								hasEscapes: false,
							})
							r.pos = targetPos
							foundEnd = true
							break
						} else {
							// Bare \r is part of the field
							r.pos = targetPos + 1
							continue
						}
					} else if c == '"' {
						if !r.LazyQuotes {
							return nil, &ParseError{Line: recordLine, Err: ErrBareQuote}
						}
						r.pos = targetPos + 1
						continue
					}
				}

				if r.eof {
					fieldEnd := r.end
					r.record.spans = append(r.record.spans, fieldSpan{
						start:      uint32(fieldStart),
						end:        uint32(fieldEnd),
						hasEscapes: false,
					})
					r.pos = r.end
					foundEnd = true
					break
				}

				r.pos = r.end
				if err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
		}

		// Delimiter check
		if r.pos < r.end && r.buf[r.pos] == delim {
			r.pos++
			if r.pos >= r.end && r.eof {
				r.record.spans = append(r.record.spans, fieldSpan{
					start:      uint32(r.pos),
					end:        uint32(r.pos),
					hasEscapes: false,
				})
				break
			}
			continue
		}

		// Newline check
		if r.pos < r.end {
			if r.buf[r.pos] == '\r' {
				r.pos++
				if r.pos >= r.end && !r.eof {
					if err := r.ensureMore(); err != nil {
						return nil, err
					}
				}
				if r.pos < r.end && r.buf[r.pos] == '\n' {
					r.pos++
				}
				r.line++
				break
			} else if r.buf[r.pos] == '\n' {
				r.pos++
				r.line++
				break
			}
		}

		if r.pos >= r.end && r.eof {
			break
		}
	}

	numFields := len(r.record.spans)
	if r.FieldsPerRecord > 0 {
		if numFields != r.FieldsPerRecord {
			return nil, &ParseError{Line: recordLine, Err: ErrFieldCount}
		}
	} else if r.FieldsPerRecord == 0 {
		if r.numFieldsFirst == 0 {
			r.numFieldsFirst = numFields
		} else if numFields != r.numFieldsFirst {
			return nil, &ParseError{Line: recordLine, Err: ErrFieldCount}
		}
	}

	r.record.raw = r.buf
	return &r.record, nil
}

// Read reads one record and returns a []string slice.
func (r *Reader) Read() ([]string, error) {
	rec, err := r.ReadRecord()
	if err != nil {
		return nil, err
	}
	n := rec.NumFields()
	res := make([]string, n)
	for i := 0; i < n; i++ {
		res[i] = string(rec.Field(i))
	}
	return res, nil
}
