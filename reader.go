package csv

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
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
	Comma            rune   // Field delimiter (default ',')
	Delimiter        string // Custom delimiter (e.g. "|", "||", "::"). Takes precedence if set.
	Quote            rune   // Quote character (default '"')
	Comment          rune   // Comment character (optional)
	FieldsPerRecord  int    // Number of expected fields per record (<0: no check, 0: first row, >0: fixed)
	LazyQuotes       bool   // Allow bare quotes in unquoted fields
	TrimLeadingSpace bool   // Trim leading whitespace from fields

	delimBytes   []byte
	quoteByte    byte
	isMultiDelim bool
	encoding     encoding.Encoding
	initErr      error
	scanner      blockScanner

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
	recordStart    int
	currFieldStart int
	record         Record
}

func (r *Reader) initDelimAndQuote() {
	if r.Quote == 0 {
		r.Quote = '"'
	}
	r.quoteByte = byte(r.Quote)

	if r.Delimiter == "" {
		if r.Comma == 0 {
			r.Comma = ','
		}
		r.Delimiter = string(r.Comma)
	}
	r.delimBytes = []byte(r.Delimiter)
	if len(r.delimBytes) == 1 {
		r.Comma = rune(r.delimBytes[0])
		r.isMultiDelim = false
	} else {
		r.isMultiDelim = true
	}
	r.scanner = newBlockScanner(r.delimBytes[0], r.quoteByte)
}

// NewReader returns a new Reader reading from r with optional configuration options.
func NewReader(r io.Reader, opts ...Option) *Reader {
	bHolder := acquireReadBuf()
	sHolder := acquireSpanHolder()
	fHolder := acquireFieldBufHolder()

	reader := &Reader{
		Comma:           ',',
		Delimiter:       ",",
		Quote:           '"',
		FieldsPerRecord: -1,
		r:               r,
		bufHolder:       bHolder,
		buf:             *bHolder,
		spanHolder:      sHolder,
		fieldBufHolder:  fHolder,
		bufSize:         defaultBufferSize,
		line:            1,
	}
	for _, opt := range opts {
		opt(reader)
	}
	if reader.encoding != nil && reader.r != nil {
		reader.r = transform.NewReader(reader.r, reader.encoding.NewDecoder())
	}
	reader.initDelimAndQuote()
	reader.record.r = reader
	reader.record.spans = sHolder.spans[:0]
	return reader
}

// Reset resets the Reader to read from r, reusing allocated buffers.
func (r *Reader) Reset(rd io.Reader) {
	if r.bufHolder == nil {
		r.bufHolder = acquireReadBuf()
		r.buf = *r.bufHolder
		r.spanHolder = acquireSpanHolder()
		r.fieldBufHolder = acquireFieldBufHolder()
		r.record.r = r
	}
	if r.encoding != nil && rd != nil {
		rd = transform.NewReader(rd, r.encoding.NewDecoder())
	}
	r.r = rd
	r.pos = 0
	r.end = 0
	r.eof = false
	r.line = 1
	r.numFieldsFirst = 0
	r.recordStart = 0
	r.currFieldStart = 0
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

func (r *Reader) unescapeSpan(span *fieldSpan, b []byte) []byte {
	if span.isUnescaped {
		return r.fieldBufHolder.buf[span.unescapeStart : span.unescapeStart+span.unescapeLen]
	}
	startOffset := len(r.fieldBufHolder.buf)
	q := r.quoteByte
	for i := 0; i < len(b); i++ {
		if b[i] == q && i+1 < len(b) && b[i+1] == q {
			r.fieldBufHolder.buf = append(r.fieldBufHolder.buf, q)
			i++ // skip escaped quote
		} else {
			r.fieldBufHolder.buf = append(r.fieldBufHolder.buf, b[i])
		}
	}
	span.unescapeStart = uint32(startOffset)
	span.unescapeLen = uint32(len(r.fieldBufHolder.buf) - startOffset)
	span.isUnescaped = true
	return r.fieldBufHolder.buf[span.unescapeStart : span.unescapeStart+span.unescapeLen]
}

// ensureMore reads more data from r.r into r.buf when r.pos >= r.end.
// It returns the number of bytes the buffer was shifted (if any), and any read error.
func (r *Reader) ensureMore() (int, error) {
	if r.eof {
		return 0, nil
	}

	shift := 0
	if r.end == len(r.buf) || (r.recordStart > 0 && r.recordStart > len(r.buf)/2) {
		if r.recordStart > 0 {
			shift = r.recordStart
			n := copy(r.buf, r.buf[shift:r.end])
			r.pos -= shift
			r.end = n
			r.recordStart = 0
			if r.currFieldStart >= shift {
				r.currFieldStart -= shift
			}
			for i := range r.record.spans {
				r.record.spans[i].start -= uint32(shift)
				r.record.spans[i].end -= uint32(shift)
			}
		} else if r.end == len(r.buf) {
			newSize := len(r.buf) * 2
			newBuf := make([]byte, newSize)
			copy(newBuf, r.buf)
			r.buf = newBuf
		}
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
			return shift, nil
		}
		return shift, err
	}
	return shift, nil
}

// ReadRecord reads one record and returns a zero-copy Record referencing the internal buffer.
func (r *Reader) ReadRecord() (*Record, error) {
	if r.bufHolder == nil {
		return nil, errors.New("csv: reader is closed")
	}
	if r.initErr != nil {
		return nil, r.initErr
	}
	if r.delimBytes == nil ||
		(len(r.delimBytes) == 1 && r.Comma != 0 && rune(r.delimBytes[0]) != r.Comma) ||
		(r.Delimiter != "" && string(r.delimBytes) != r.Delimiter) ||
		(r.Quote != 0 && r.quoteByte != byte(r.Quote)) {
		if len(r.delimBytes) == 1 && r.Comma != 0 && rune(r.delimBytes[0]) != r.Comma && r.Delimiter == string(r.delimBytes) {
			r.Delimiter = string(r.Comma)
		}
		r.initDelimAndQuote()
	}
	delim := r.delimBytes[0]

	r.record.spans = r.record.spans[:0]
	if r.fieldBufHolder != nil {
		r.fieldBufHolder.buf = r.fieldBufHolder.buf[:0]
	}

	// 1. Skip leading comments and blank lines
	for {
		for r.pos >= r.end && !r.eof {
			if _, err := r.ensureMore(); err != nil {
				return nil, err
			}
		}
		if r.pos >= r.end && r.eof {
			return nil, io.EOF
		}

		// Blank lines
		if r.buf[r.pos] == '\r' {
			if r.pos+1 >= r.end && !r.eof {
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
			if r.pos+1 < r.end && r.buf[r.pos+1] == '\n' {
				r.pos += 2
				r.line++
				continue
			} else if r.pos+1 >= r.end && r.eof {
				r.pos++
				r.line++
				continue
			}
			break
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
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
			continue
		}
		break
	}

	r.recordStart = r.pos
	recordLine := r.line

	// 2. Parse fields
	for {
		for r.pos >= r.end && !r.eof {
			if _, err := r.ensureMore(); err != nil {
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
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
				for r.pos < r.end && (r.buf[r.pos] == ' ' || r.buf[r.pos] == '\t') {
					r.pos++
				}
			}
		}

		// Check if quoted field
		if r.pos < r.end && r.buf[r.pos] == r.quoteByte {
			r.pos++ // consume opening quote
			r.currFieldStart = r.pos
			hasEscapes := false

			for {
				idx := bytes.IndexByte(r.buf[r.pos:r.end], r.quoteByte)
				if idx >= 0 {
					quotePos := r.pos + idx
					for quotePos+1 >= r.end && !r.eof {
						shift, err := r.ensureMore()
						if err != nil {
							return nil, err
						}
						quotePos -= shift
					}

					var fieldEnd int
					if quotePos+1 < r.end && r.buf[quotePos+1] == r.quoteByte {
						// Escaped quote
						hasEscapes = true
						r.pos = quotePos + 2
						continue
					}

					if r.isMultiDelim {
						for quotePos+1+len(r.delimBytes) > r.end && !r.eof {
							shift, err := r.ensureMore()
							if err != nil {
								return nil, err
							}
							quotePos -= shift
						}
					}

					isDelimNext := false
					if !r.isMultiDelim {
						isDelimNext = quotePos+1 < r.end && r.buf[quotePos+1] == delim
					} else {
						isDelimNext = quotePos+1 < r.end && bytes.HasPrefix(r.buf[quotePos+1:r.end], r.delimBytes)
					}

					if isDelimNext ||
						(quotePos+1 < r.end && (r.buf[quotePos+1] == '\r' || r.buf[quotePos+1] == '\n' || r.buf[quotePos+1] == ' ' || r.buf[quotePos+1] == '\t')) ||
						(quotePos+1 >= r.end && r.eof) {
						// Valid closing quote followed by delimiter, newline, whitespace, or EOF
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
						need := 1
						if r.isMultiDelim {
							need = len(r.delimBytes)
						}
						for r.pos+need > r.end && !r.eof {
							shift, err := r.ensureMore()
							if err != nil {
								return nil, err
							}
							fieldEnd -= shift
						}
						if r.pos >= r.end && r.eof {
							break
						}
						if !r.isMultiDelim && r.buf[r.pos] == delim {
							break
						}
						if r.isMultiDelim && bytes.HasPrefix(r.buf[r.pos:r.end], r.delimBytes) {
							break
						}
						b := r.buf[r.pos]
						if b == '\r' || b == '\n' {
							break
						}
						if !r.LazyQuotes && (b != ' ' && b != '\t') {
							return nil, &ParseError{Line: recordLine, Err: ErrQuote}
						}
						r.pos++
					}

					r.record.spans = append(r.record.spans, fieldSpan{
						start:      uint32(r.currFieldStart),
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
						start:      uint32(r.currFieldStart),
						end:        uint32(r.end),
						hasEscapes: hasEscapes,
					})
					r.pos = r.end
					break
				}

				r.pos = r.end
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
		} else {
			// Unquoted field
			r.currFieldStart = r.pos
			foundEnd := false

			for !foundEnd {
				specIdx := r.scanner.scanSpecial(r.buf[r.pos:r.end])
				if specIdx >= 0 {
					targetPos := r.pos + specIdx
					c := r.buf[targetPos]

					if !r.isMultiDelim && c == delim {
						fieldEnd := targetPos
						r.record.spans = append(r.record.spans, fieldSpan{
							start:      uint32(r.currFieldStart),
							end:        uint32(fieldEnd),
							hasEscapes: false,
						})
						r.pos = targetPos
						foundEnd = true
						break
					} else if r.isMultiDelim && c == r.delimBytes[0] {
						for targetPos+len(r.delimBytes) > r.end && !r.eof {
							shift, err := r.ensureMore()
							if err != nil {
								return nil, err
							}
							targetPos -= shift
						}
						if bytes.HasPrefix(r.buf[targetPos:r.end], r.delimBytes) {
							fieldEnd := targetPos
							r.record.spans = append(r.record.spans, fieldSpan{
								start:      uint32(r.currFieldStart),
								end:        uint32(fieldEnd),
								hasEscapes: false,
							})
							r.pos = targetPos
							foundEnd = true
							break
						} else {
							r.pos = targetPos + 1
							continue
						}
					} else if c == '\n' {
						fieldEnd := targetPos
						r.record.spans = append(r.record.spans, fieldSpan{
							start:      uint32(r.currFieldStart),
							end:        uint32(fieldEnd),
							hasEscapes: false,
						})
						r.pos = targetPos
						foundEnd = true
						break
					} else if c == '\r' {
						// Check if followed by \n
						if targetPos+1 >= r.end && !r.eof {
							shift, err := r.ensureMore()
							if err != nil {
								return nil, err
							}
							targetPos -= shift
						}
						if targetPos+1 < r.end && r.buf[targetPos+1] == '\n' {
							// CRLF is end of line
							fieldEnd := targetPos
							r.record.spans = append(r.record.spans, fieldSpan{
								start:      uint32(r.currFieldStart),
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
								start:      uint32(r.currFieldStart),
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
					} else if c == r.quoteByte {
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
						start:      uint32(r.currFieldStart),
						end:        uint32(fieldEnd),
						hasEscapes: false,
					})
					r.pos = r.end
					foundEnd = true
					break
				}

				r.pos = r.end
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
		}

		// Delimiter check
		if !r.isMultiDelim {
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
		} else {
			for r.pos+len(r.delimBytes) > r.end && !r.eof {
				if _, err := r.ensureMore(); err != nil {
					return nil, err
				}
			}
			if r.pos < r.end && bytes.HasPrefix(r.buf[r.pos:r.end], r.delimBytes) {
				r.pos += len(r.delimBytes)
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
		}

		// Newline check
		if r.pos < r.end {
			if r.buf[r.pos] == '\r' {
				if r.pos+1 >= r.end && !r.eof {
					if _, err := r.ensureMore(); err != nil {
						return nil, err
					}
				}
				if r.pos+1 < r.end && r.buf[r.pos+1] == '\n' {
					r.pos += 2
					r.line++
					break
				} else if r.pos+1 >= r.end && r.eof {
					r.pos++
					r.line++
					break
				}
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

// ReadAll reads all the remaining records from r.
func (r *Reader) ReadAll() ([][]string, error) {
	var records [][]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
}

