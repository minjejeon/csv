# Portable SIMD, Custom Encoding, and Project Documentation Design

**Date**: 2026-10-06  
**Status**: Approved  

---

## 1. Overview & Goals

1. **Portable SIMD (`package simd`)**:
   - Replace or complement direct architecture-locked `archsimd` intrinsics with Go 1.27's standard portable SIMD package (`simd.Uint8s`, `simd.Mask8s`).
   - Remove the `&& amd64` build constraint from SIMD vector search (`//go:build goexperiment.simd`) so that vector acceleration runs portably across architectures (AMD64 AVX2/AVX512, ARM64 Neon, WASM, and pure Go emulation).
2. **Custom Encodings (EUC-KR, CP949, Shift_JIS, ISO-8859-1, etc.)**:
   - Support `WithEncoding(enc encoding.Encoding)` from `golang.org/x/text/encoding`.
   - Support `WithCharset(name string)` for convenient string-based selection (e.g. `"euc-kr"`, `"cp949"`, `"shift_jis"`, `"latin1"`).
   - Normalize streams cleanly into UTF-8 via `transform.NewReader` before ingestion, retaining 100% native SIMD scanning speed and zero per-field decoding overhead.
3. **Environment & PATH Verification**:
   - Confirm Go 1.27 (`/home/minje/golang/bin/go`) is the default compiler in PATH and shell configuration.
4. **Documentation & English Coding Standard**:
   - Explicitly document in `AGENTS.md` that all code, comments, commit messages, and documentation must be written in English by default.
   - Create a clean, production-grade `README.md` in English with architecture details, quickstart, functional options, benchmarks, and SIMD activation instructions.

---

## 2. Technical Design

### A. Portable SIMD Vector Scanner

Go 1.27 provides `package simd` with vector types:
- `simd.BroadcastUint8s(b)`
- `simd.LoadUint8s(slice)`
- `chunk.Equal(v).Or(...)` returning `simd.Mask8s`
- Vector length is dynamically determined by `simd.VectorBitSize() / 8` (e.g., 32 bytes on AVX2, 16 bytes on Neon/ARM64, 64 bytes on AVX-512).

`scan_simd.go` will be updated:
- Build tag: `//go:build goexperiment.simd`
- Functions:
  - If hardware is supported, loads vector of length `simd.VectorBitSize() / 8`.
  - Performs vector equality check against `delim`, `quote`, `\r`, `\n`.
  - Dispatches `mAll.ToArch()` to extract bitmask trailing zeros on AMD64 (`archsimd.Mask8x32.ToBits()`) and ARM64 (`archsimd.Mask8x16.ToBits()`).
  - Gracefully falls back to SWAR in case of emulation (`simd.Emulated()`).

`scan_fallback.go`:
- Build tag: `//go:build !goexperiment.simd`
- Runs 8-byte SWAR scanner.

### B. Custom Encoding Architecture

In `option.go`:
```go
// WithEncoding sets a custom character encoding for decoding CSV data into UTF-8.
func WithEncoding(enc encoding.Encoding) Option {
    return func(r *Reader) {
        r.encoding = enc
    }
}

// WithCharset sets a character encoding by name (e.g. "euc-kr", "cp949", "shift_jis", "latin1").
func WithCharset(name string) Option { ... }
```

When `r.encoding != nil`:
- `NewReader(r io.Reader, opts...)`: wraps `r` with `transform.NewReader(r, r.encoding.NewDecoder())`.
- In `Unmarshal(data []byte, v any, opts...)`: wraps `bytes.NewReader(data)` with `transform.NewReader(..., enc.NewDecoder())`.
- In `ParallelUnmarshal` / `ParallelReader`: transforms data before worker distribution or inside worker streams.

### C. AGENTS.md & README.md

- `AGENTS.md`: Add explicit rule at the top:
  `> **Language Policy**: All code, comments, commit messages, and documentation must be written in English by default.`
- `README.md`: English documentation with:
  - Features overview (Zero-alloc, Go 1.27 Portable SIMD, Functional options, Custom delimiter/quote/encodings, csvutil compatible struct tags, RecordUnmarshaler generic API).
  - Quickstart examples.
  - Performance benchmarks table.

---

## 3. Success Criteria & Verification
1. `GOEXPERIMENT=simd go test -v ./...` passes on all test cases including portable SIMD.
2. `go test -v ./...` passes on fallback path.
3. `TestEncodingEUCKR` and `TestEncodingShiftJIS` verify Korean/Japanese legacy encoded CSV is converted cleanly into UTF-8 struct strings.
4. `go test -race ./...` produces 0 race conditions.
5. `README.md` and `AGENTS.md` updated as specified.
