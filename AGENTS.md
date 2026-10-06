# AGENTS.md

## Project Overview
`github.com/minjejeon/csv` is a high-performance, zero-allocation CSV reader and struct unmarshaler for Go.
It combines:
1. **Zero-Alloc Lexer**: Uses `sync.Pool` for byte buffers and record field span slices (`[]fieldSpan`), avoiding per-row and per-field heap allocations.
2. **SIMD Acceleration**: Uses Go 1.27 `simd/archsimd` AVX2 vector operations (`GOEXPERIMENT=simd`) to scan 32 bytes/chunk for delimiters, quotes, and newlines in 1-2 clock cycles, with a pure Go / SWAR fallback (`//go:build !goexperiment.simd || !amd64`).
3. **Struct Mapping Engine**: `csvutil`-compatible struct tag mapping (`csv:"col_name,omitempty"`), pre-compiling reflection metadata into direct `unsafe.Pointer` offset setters with safe reflection fallback.
4. **API**: Streaming `Decoder` (row-by-row zero-allocation) and bulk `Unmarshal([]byte, &slice)`.

---

## Build & Test Instructions

### Environment
- Go version: Go 1.27.x (`/home/minje/golang/bin/go`)

### Running Tests
- **Standard (Fallback) Tests**:
  ```bash
  go test -v ./...
  ```
- **SIMD Experimental Tests (AVX2)**:
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

### 1. Lexer & Scanner (`Reader`)
- `scan_simd.go`: `//go:build goexperiment.simd && amd64` - SIMD vector scan using `archsimd.Int8x32`.
- `scan_fallback.go`: `//go:build !goexperiment.simd || !amd64` - Fast SWAR / standard fallback scan.
- Always check `archsimd.X86.AVX2()` before issuing AVX2 intrinsics.

### 2. Memory Pooling (`sync.Pool`)
- `readBufferPool`: Reuses 64KB read buffers for `io.Reader`.
- `recordSpanPool`: Reuses `[]fieldSpan` slices to avoid slice allocations per record.
- `fieldBufPool`: Reuses scratch buffers for unescaping `""` into `"`.
- All pooled objects must be properly reset and returned to the pool (`defer pool.Put(...)` or explicit release).

### 3. Decoder & Unmarshaler
- Pre-compile struct fields into `[]fieldPlan`.
- Cache `*typePlan` keyed by `reflect.Type` in a thread-safe cache (`sync.Map`).
- Use direct `unsafe.Pointer` offset writes for primitive types (`string`, `int*`, `uint*`, `float*`, `bool`) to achieve zero interface boxing and zero reflection in the hot loop.
- Support `encoding.TextUnmarshaler` and custom unmarshaling interfaces.

### 4. Test Compatibility
- Must verify test cases ported from Go's standard `encoding/csv` (RFC 4180 compliance, quotes, CRLF/LF, comments, lazy quotes).
- Must verify test cases ported from `jszwec/csvutil` (struct tag mapping, type conversions, text unmarshalers, omitempty, custom types).
