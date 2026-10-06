# AGENTS.md

> **Language Policy**: All code, comments, commit messages, and documentation must be written in English by default.

## Project Overview
`github.com/minjejeon/csv` is a high-performance, zero-allocation CSV reader, writer, and struct unmarshaler for Go.
It combines:
1. **Zero-Alloc Lexer**: Uses `sync.Pool` for byte buffers and record field span slices (`[]fieldSpan`), avoiding per-row and per-field heap allocations.
2. **Portable SIMD Acceleration**: Uses Go 1.27 portable standard vector operations (`GOEXPERIMENT=simd`, package `simd`) supporting AVX2/AVX-512 (AMD64) and Neon (ARM64) scanning up to 38+ GB/s, with pure Go SWAR fallback (`//go:build !goexperiment.simd`).
3. **Struct Mapping Engine**: `csvutil`-compatible struct tag mapping (`csv:"col_name,omitempty"`), pre-compiling reflection metadata into direct `unsafe.Pointer` offset setters with safe reflection fallback.
4. **Rich Features**: Functional options (`WithDelimiter`, `WithQuote`, `WithCharset`, `WithEncoding`), multi-character delimiters (`||`, `::`), custom quote characters (`'`), legacy encodings (`EUC-KR`, `CP949`, `Shift_JIS`, `ISO-8859-1`), multithreaded chunking (`ParallelUnmarshal`), and generic zero-reflection unmarshaler (`UnmarshalSlice[T]`).

---

## Build & Test Instructions

### Environment
- Go version: Go 1.27.x (`/home/minje/golang/bin/go`).
- PATH: `/home/minje/golang/bin` is configured as the local default Go toolchain in `~/.bashrc`.

### Running Tests
- **Standard (Fallback) Tests**:
  ```bash
  go test -v ./...
  ```
- **SIMD Experimental Tests (Portable SIMD / AVX2 / Neon)**:
  ```bash
  GOEXPERIMENT=simd go test -v ./...
  ```
- **Race Detector**:
  ```bash
  go test -race ./...
  GOEXPERIMENT=simd go test -race ./...
  ```

### Running Benchmarks
- Compare with standard `encoding/csv` and `csvutil`:
  ```bash
  go test -bench=. -benchmem ./...
  GOEXPERIMENT=simd go test -bench=. -benchmem ./...
  ```

---

## Architecture & Code Conventions

### 1. Language Standard
- **Always write code, type definitions, function comments, docstrings, error messages, and commit messages in English.**

### 2. Lexer & Scanner (`Reader`)
- `scan_simd.go`: `//go:build goexperiment.simd` - Portable vector scan using Go 1.27 `simd.Uint8s` and architecture mask conversion.
- `scan_fallback.go`: `//go:build !goexperiment.simd` - Fast 8-byte SWAR / pure Go fallback scan.
- Single-byte delimiters use SIMD directly; multi-character delimiters scan prefix via SIMD and verify prefix lookahead.

### 3. Memory Pooling (`sync.Pool`)
- `readBufferPool`: Reuses 64KB read buffers for `io.Reader`.
- `recordSpanPool`: Reuses `[]fieldSpan` slices to avoid slice allocations per record.
- `fieldBufPool`: Reuses scratch buffers for unescaping `""` / `''` into `"` / `'`.
- All pooled objects must be properly reset and returned to the pool (`defer pool.Put(...)` or explicit release).

### 4. Decoder & Unmarshaler
- Pre-compile struct fields into `[]fieldPlan`.
- Cache `*typePlan` keyed by `reflect.Type` in a thread-safe cache (`sync.Map`).
- Use direct `unsafe.Pointer` offset writes for primitive types (`string`, `int*`, `uint*`, `float*`, `bool`) to achieve zero interface boxing and zero reflection in the hot loop.
- Support `encoding.TextUnmarshaler` and custom unmarshaling interfaces.
- `RecordUnmarshaler` interface: Compile-time zero-reflection generic unmarshaler (`UnmarshalSlice[T]`, `UnmarshalTo(data, &slice)`).

### 5. Encoding & Charsets
- Custom encodings (`EUC-KR`, `CP949`, `Shift_JIS`, etc.) are handled via `golang.org/x/text/encoding` and `transform.NewReader`.
- Input streams are normalized to clean UTF-8 before ingestion, keeping internal SIMD scanners and record parsers uniformly fast.

### 6. Test Compatibility
- Must verify test cases ported from Go's standard `encoding/csv` (RFC 4180 compliance, quotes, CRLF/LF, comments, lazy quotes).
- Must verify test cases ported from `jszwec/csvutil` (struct tag mapping, type conversions, text unmarshalers, omitempty, custom types).
