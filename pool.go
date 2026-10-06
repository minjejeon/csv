package csv

import (
	"sync"
)

// defaultBufferSize is the default 64KB read buffer size for streaming io.Reader
const defaultBufferSize = 64 * 1024

// initialSpanCap is the initial capacity for field spans per record
const initialSpanCap = 32

// initialFieldBufCap is the initial capacity for unescape field scratch buffers
const initialFieldBufCap = 256

// fieldSpan records the byte slice offsets of a CSV field within the active buffer
type fieldSpan struct {
	start         uint32
	end           uint32
	unescapeStart uint32
	unescapeLen   uint32
	hasEscapes    bool
	isUnescaped   bool
}

// spanHolder pools []fieldSpan without interface allocation on Put
type spanHolder struct {
	spans []fieldSpan
}

var spanPool = sync.Pool{
	New: func() any {
		return &spanHolder{
			spans: make([]fieldSpan, 0, initialSpanCap),
		}
	},
}

func acquireSpanHolder() *spanHolder {
	h := spanPool.Get().(*spanHolder)
	h.spans = h.spans[:0]
	return h
}

func releaseSpanHolder(h *spanHolder) {
	if h == nil {
		return
	}
	h.spans = h.spans[:0]
	spanPool.Put(h)
}

// readBufPool pools *[]byte read buffers
var readBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, defaultBufferSize)
		return &b
	},
}

func acquireReadBuf() *[]byte {
	return readBufPool.Get().(*[]byte)
}

func releaseReadBuf(b *[]byte) {
	if b == nil || cap(*b) < defaultBufferSize {
		return
	}
	*b = (*b)[:defaultBufferSize]
	readBufPool.Put(b)
}

// byteBufHolder pools scratch buffers without interface allocation on Put
type byteBufHolder struct {
	buf []byte
}

var fieldBufPool = sync.Pool{
	New: func() any {
		return &byteBufHolder{
			buf: make([]byte, 0, initialFieldBufCap),
		}
	},
}

func acquireFieldBufHolder() *byteBufHolder {
	h := fieldBufPool.Get().(*byteBufHolder)
	h.buf = h.buf[:0]
	return h
}

func releaseFieldBufHolder(h *byteBufHolder) {
	if h == nil {
		return
	}
	h.buf = h.buf[:0]
	fieldBufPool.Put(h)
}
