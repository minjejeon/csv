package csv

import (
	"bytes"
	"io"
)

// Writer writes records to a CSV-encoded output stream.
type Writer struct {
	Comma     rune   // Field delimiter (default ',')
	Delimiter string // Custom delimiter (e.g. "|", "||", "::"). Takes precedence if set.
	Quote     rune   // Quote character (default '"')
	UseCRLF   bool   // True to use \r\n as the line terminator

	w          io.Writer
	bufHolder  *[]byte
	buf        []byte
	delimBytes []byte
	quoteByte  byte
	scanner    blockScanner
	hasScanner bool
}

func (w *Writer) initDelimAndQuote() {
	if w.Quote == 0 {
		w.Quote = '"'
	}
	w.quoteByte = byte(w.Quote)

	if w.Delimiter == "" {
		if w.Comma == 0 {
			w.Comma = ','
		}
		w.Delimiter = string(w.Comma)
	}
	w.delimBytes = []byte(w.Delimiter)
	if len(w.delimBytes) == 1 {
		w.Comma = rune(w.delimBytes[0])
	}
	w.scanner = newBlockScanner(w.delimBytes[0], w.quoteByte)
	w.hasScanner = true
}

// NewWriter returns a new Writer writing to w with optional configuration options.
func NewWriter(w io.Writer, opts ...any) *Writer {
	bHolder := acquireWriteBuf()
	writer := &Writer{
		Comma:     ',',
		Delimiter: ",",
		Quote:     '"',
		w:         w,
		bufHolder: bHolder,
		buf:       *bHolder,
	}

	for _, opt := range opts {
		switch fn := opt.(type) {
		case func(*Writer):
			fn(writer)
		case Option:
			var r Reader
			fn(&r)
			if r.Delimiter != "" {
				writer.Delimiter = r.Delimiter
			}
			if r.Comma != 0 {
				writer.Comma = r.Comma
			}
			if r.Quote != 0 {
				writer.Quote = r.Quote
			}
		}
	}

	writer.initDelimAndQuote()
	return writer
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	n, err := w.w.Write(w.buf)
	if err != nil {
		w.buf = w.buf[n:]
		return err
	}
	w.buf = w.buf[:0]
	return nil
}

// Close flushes buffered data and returns pooled memory buffers.
func (w *Writer) Close() error {
	err := w.Flush()
	if w.bufHolder != nil {
		*w.bufHolder = w.buf
		releaseWriteBuf(w.bufHolder)
		w.bufHolder = nil
		w.buf = nil
	}
	return err
}

func (w *Writer) fieldNeedsQuotes(field []byte) bool {
	if len(field) == 0 {
		return false
	}
	if len(w.delimBytes) > 1 && bytes.Contains(field, w.delimBytes) {
		return true
	}
	return w.scanner.scanSpecial(field) >= 0
}

func (w *Writer) writeFieldBytes(field []byte) {
	if w.fieldNeedsQuotes(field) {
		w.buf = append(w.buf, w.quoteByte)
		q := w.quoteByte
		for i := 0; i < len(field); i++ {
			b := field[i]
			if b == q {
				w.buf = append(w.buf, q, q)
			} else {
				w.buf = append(w.buf, b)
			}
		}
		w.buf = append(w.buf, w.quoteByte)
	} else {
		w.buf = append(w.buf, field...)
	}
}

// Write writes a single CSV record to w.
func (w *Writer) Write(record []string) error {
	if !w.hasScanner {
		w.initDelimAndQuote()
	}

	for i, field := range record {
		if i > 0 {
			w.buf = append(w.buf, w.delimBytes...)
		}
		w.writeFieldBytes([]byte(field))
	}

	if w.UseCRLF {
		w.buf = append(w.buf, '\r', '\n')
	} else {
		w.buf = append(w.buf, '\n')
	}

	if len(w.buf) >= 60*1024 {
		return w.Flush()
	}
	return nil
}

// WriteAll writes multiple CSV records to w and flushes.
func (w *Writer) WriteAll(records [][]string) error {
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.Flush()
}

// WriteFieldBytes writes a raw byte slice field with SIMD quoting if required.
func (w *Writer) WriteFieldBytes(field []byte) {
	if !w.hasScanner {
		w.initDelimAndQuote()
	}
	w.writeFieldBytes(field)
}

// WriteDelimiter writes the field delimiter.
func (w *Writer) WriteDelimiter() {
	if !w.hasScanner {
		w.initDelimAndQuote()
	}
	w.buf = append(w.buf, w.delimBytes...)
}

// WriteNewline writes the line terminator and auto-flushes if needed.
func (w *Writer) WriteNewline() error {
	if w.UseCRLF {
		w.buf = append(w.buf, '\r', '\n')
	} else {
		w.buf = append(w.buf, '\n')
	}
	if len(w.buf) >= 60*1024 {
		return w.Flush()
	}
	return nil
}

