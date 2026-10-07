package csv

import (
	"bytes"
	"io"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// Writer writes records to a CSV-encoded output stream.
type Writer struct {
	Comma     rune   // Field delimiter (default ',')
	Delimiter string // Custom delimiter (e.g. "|", "||", "::"). Takes precedence if set.
	Quote     rune   // Quote character (default '"')
	UseCRLF   bool   // True to use \r\n as the line terminator

	w          io.Writer
	transformW *transform.Writer
	encoding   encoding.Encoding
	bufHolder  *[]byte
	buf        []byte
	delimBytes []byte
	quoteByte  byte
	scanner    blockScanner
	hasScanner bool
	err        error
}

func (w *Writer) checkDelimAndQuote() {
	if !w.hasScanner || w.delimBytes == nil ||
		(len(w.delimBytes) == 1 && w.Comma != 0 && rune(w.delimBytes[0]) != w.Comma) ||
		(w.Delimiter != "" && string(w.delimBytes) != w.Delimiter) ||
		(w.Quote != 0 && w.quoteByte != byte(w.Quote)) {
		if len(w.delimBytes) == 1 && w.Comma != 0 && rune(w.delimBytes[0]) != w.Comma && (w.Delimiter == string(w.delimBytes) || w.Delimiter == ",") {
			w.Delimiter = string(w.Comma)
		}
		w.initDelimAndQuote()
	}
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
		w.scanner = newBlockScanner(w.delimBytes[0], w.quoteByte)
	} else {
		// In multi-char delim mode, scanner doesn't scan single delim bytes
		w.scanner = newBlockScanner(0, w.quoteByte)
	}
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
		case encoding.Encoding:
			writer.encoding = fn
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
			if r.encoding != nil {
				writer.encoding = r.encoding
			}
		}
	}

	if writer.encoding != nil && writer.w != nil {
		writer.transformW = transform.NewWriter(writer.w, writer.encoding.NewEncoder())
		writer.w = writer.transformW
	}

	writer.initDelimAndQuote()
	return writer
}

// Error reports any error that occurred during a previous Write or Flush.
func (w *Writer) Error() error {
	return w.err
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	n, err := w.w.Write(w.buf)
	if err != nil {
		w.buf = w.buf[n:]
		if w.err == nil {
			w.err = err
		}
		return err
	}
	w.buf = w.buf[:0]
	return nil
}

// Close flushes buffered data and returns pooled memory buffers.
func (w *Writer) Close() error {
	err := w.Flush()
	if w.transformW != nil {
		if terr := w.transformW.Close(); terr != nil && err == nil {
			err = terr
		}
	}
	if err != nil && w.err == nil {
		w.err = err
	}
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
	if len(w.delimBytes) > 1 {
		if bytes.Contains(field, w.delimBytes) {
			return true
		}
		return bytes.IndexByte(field, w.quoteByte) != -1 ||
			bytes.IndexByte(field, '\r') != -1 ||
			bytes.IndexByte(field, '\n') != -1
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
	w.checkDelimAndQuote()

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
	w.checkDelimAndQuote()
	w.writeFieldBytes(field)
}

// WriteDelimiter writes the field delimiter.
func (w *Writer) WriteDelimiter() {
	w.checkDelimAndQuote()
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

