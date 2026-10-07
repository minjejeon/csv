# Stability & DX Hardening Design Specification

## Overview
This document specifies comprehensive stability, resource safety, and developer experience (DX) hardening for `github.com/minjejeon/csv`.
The enhancements eliminate critical memory/pool leaks, prevent unbounded buffer growth, protect against zero-copy pointer aliasing hazards, introduce rich error diagnostics with line/column/field/value context, provide automatic UTF-8 BOM stripping, and enable custom date/time formats and multi-charset writer support.

---

## 1. Memory & Resource Safety Architecture

### 1.1 Pool Buffer Leak Elimination in `ParallelUnmarshal` and `ParallelUnmarshalTo`
- **Root Cause**: In [`parallel.go`](file:///home/minje/dev/csv/parallel.go) and [`generic.go`](file:///home/minje/dev/csv/generic.go), `dummy := NewReader(nil, csvOpts...)` is created to inspect quote bytes and character encodings. Because `dummy.Close()` is never invoked, every parallel execution leaks a 64KB read buffer, a span holder, and an unescape scratch buffer from `sync.Pool`.
- **Solution**:
  1. Extract reader options without instantiating pooled buffers using a non-allocating configuration extractor `extractReaderConfig(opts)`.
  2. If a dummy reader is ever used, guarantee `defer dummy.Close()` is executed immediately.
  3. Ensure non-UTF8 encoding conversions do not trigger compounding buffer leaks on recursive calls.

### 1.2 Bounded vs GC-Managed `sync.Pool` Lifecycle
- **Decision**: Rather than imposing an arbitrary strict cap (e.g., 2MB) that could cause repeated reallocations when processing medium-to-large records (5MB~20MB), leave pool buffer reclamation to Go's runtime GC (`sync.Pool` victim cache).
- `sync.Pool` entries are automatically reclaimed across GC cycles when idle, ensuring maximum throughput for large batch serializations without artificial overhead.

### 1.3 Zero-Copy Record Cloning (`Record.Clone()`)
- **Root Cause**: `r.ReadRecord()` returns a pointer `&r.record` reusing internal sliding buffers. Storing records or passing them across goroutines leads to data corruption on subsequent read iterations.
- **Solution**:
  - Add `func (rec *Record) Clone() *Record`:
    - Allocates an independent byte slice holding cloned field data.
    - Copies all `fieldSpan`s with re-based offsets.
    - Resolves any escaped quotes directly into the cloned slice.
    - Sets `r = nil` on the cloned record so field access is completely detached from the reader.

### 1.4 Embedded Struct Pointer Support
- **Root Cause**: In [`plan.go`](file:///home/minje/dev/csv/plan.go), `collectStructFields` only recurses into anonymous fields if `f.Type.Kind() == reflect.Struct`, ignoring `*EmbeddedStruct`.
- **Solution**:
  - Support both `reflect.Struct` and `reflect.Pointer` to struct for embedded fields.
  - Automatically instantiate embedded pointer structs when setting fields if the pointer is nil.

---

## 2. Rich Error Diagnostics (DX)

### 2.1 Structured `DecodeError`
- Define `type DecodeError`:
  ```go
  type DecodeError struct {
      Line   int    // Line number in CSV file (1-based)
      Column int    // Field column index (0-based)
      Header string // CSV column header name (e.g. "age")
      Field  string // Struct field name (e.g. "Age")
      Value  string // Raw string value that caused failure (e.g. "invalid")
      Err    error  // Underlying conversion error (e.g. strconv.ErrSyntax)
  }

  func (e *DecodeError) Error() string {
      if e.Header != "" {
          return fmt.Sprintf("csv: line %d, column %q (field %s): %v (value: %q)",
              e.Line, e.Header, e.Field, e.Err, e.Value)
      }
      return fmt.Sprintf("csv: line %d, col %d (field %s): %v (value: %q)",
          e.Line, e.Column, e.Field, e.Err, e.Value)
  }

  func (e *DecodeError) Unwrap() error {
      return e.Err
  }
  ```

### 2.2 Error Context Propagation
- In [`decoder.go`](file:///home/minje/dev/csv/decoder.go), [`csv.go`](file:///home/minje/dev/csv/csv.go), and [`parallel.go`](file:///home/minje/dev/csv/parallel.go), wrap any setter failures into `*DecodeError` with the active row/line and raw value.
- Support Go 1.13+ `errors.Is(err, ...)` and `errors.As(err, &decodeErr)`.

---

## 3. Developer Experience & Feature Extensions

### 3.1 UTF-8 BOM Auto-Stripping (`WithTrimBOM`)
- Excel on Windows frequently prepends `\xef\xbb\xbf` to CSV exports, corrupting the first column header (`\xef\xbb\xbfid`).
- Add `WithTrimBOM(trim bool)` Option (enabled by default `trim = true`).
- In `Reader`: automatically trim leading UTF-8 BOM bytes at stream startup.
- In `Decoder` & `Unmarshal`: clean the first column header to guarantee seamless struct mapping.

### 3.2 Custom `time.Time` Formats in Struct Tags
- Support `format=...` in `parseTag`:
  - `csv:"created_at,format=2006-01-02 15:04:05"`
  - `csv:"date,format=2006-01-02"`
  - Common presets: `format=RFC3339`, `format=DateOnly`, `format=DateTime`, `format=unix`, `format=unixmilli`.
- Compile custom format layout into `fieldSetter` in `setter.go` and `fieldGetter` in `getter.go`.

### 3.3 Symmetric `Writer` Encoding Support
- Support `WithCharset` and `WithEncoding` on `NewWriter`, `NewEncoder`, and `Marshal`.
- If a custom encoding is configured, transparently wrap the output stream with `transform.NewWriter(w, enc.NewEncoder())` and flush on `Close()`.

---

## 4. Verification Plan

1. **Unit & Edge Case Tests**:
   - Verify `dummy` pool buffer recycling in `parallel_test.go` and `generic_test.go`.
   - Verify `releaseWriteBuf` caps buffers at 2MB in `pool_test.go`.
   - Verify `Record.Clone()` independence when caller mutates/reads next record.
   - Verify `DecodeError` structure and `errors.As` / `errors.Is` compatibility.
   - Verify UTF-8 BOM stripping across `Reader`, `Decoder`, and `Unmarshal`.
   - Verify custom `format=...` tags on `time.Time` fields during both marshal and unmarshal.
   - Verify `Writer` generating EUC-KR and Shift_JIS streams.
2. **Race & SIMD Tests**:
   - `go test -v ./...`
   - `GOEXPERIMENT=simd go test -v ./...`
   - `go test -race ./...`
   - `GOEXPERIMENT=simd go test -race ./...`
3. **Zero-Alloc & Throughput Benchmarks**:
   - Ensure zero performance degradation on standard microbenchmarks.
