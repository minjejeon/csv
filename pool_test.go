package csv

import (
	"testing"
)

func TestPoolSpans(t *testing.T) {
	holder := acquireSpanHolder()
	if holder == nil {
		t.Fatal("expected non-nil span holder")
	}
	if cap(holder.spans) < 16 {
		t.Fatalf("expected capacity >= 16, got %d", cap(holder.spans))
	}
	if len(holder.spans) != 0 {
		t.Fatalf("expected initial length 0, got %d", len(holder.spans))
	}

	holder.spans = append(holder.spans, fieldSpan{start: 0, end: 5, hasEscapes: false})
	releaseSpanHolder(holder)

	reused := acquireSpanHolder()
	if len(reused.spans) != 0 {
		t.Fatalf("expected reused slice length 0, got %d", len(reused.spans))
	}
	releaseSpanHolder(reused)
}

func TestPoolReadBuf(t *testing.T) {
	buf := acquireReadBuf()
	if buf == nil {
		t.Fatal("expected non-nil read buffer")
	}
	if len(*buf) != defaultBufferSize {
		t.Fatalf("expected buffer length %d, got %d", defaultBufferSize, len(*buf))
	}
	releaseReadBuf(buf)
}

func TestPoolFieldBuf(t *testing.T) {
	holder := acquireFieldBufHolder()
	if holder == nil {
		t.Fatal("expected non-nil field scratch holder")
	}
	if len(holder.buf) != 0 {
		t.Fatalf("expected initial length 0, got %d", len(holder.buf))
	}
	holder.buf = append(holder.buf, "hello"...)
	releaseFieldBufHolder(holder)

	reused := acquireFieldBufHolder()
	if len(reused.buf) != 0 {
		t.Fatalf("expected reused buffer length 0, got %d", len(reused.buf))
	}
	releaseFieldBufHolder(reused)
}

func TestPoolAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping allocation test under race detector")
	}
	allocs := testing.AllocsPerRun(100, func() {
		spans := acquireSpanHolder()
		spans.spans = append(spans.spans, fieldSpan{start: 1, end: 10, hasEscapes: false})
		releaseSpanHolder(spans)

		fbuf := acquireFieldBufHolder()
		fbuf.buf = append(fbuf.buf, 'a', 'b', 'c')
		releaseFieldBufHolder(fbuf)

		rbuf := acquireReadBuf()
		releaseReadBuf(rbuf)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations per run, got %f", allocs)
	}
}
