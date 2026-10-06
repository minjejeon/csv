# SIMD Acceleration & High-Performance Pipeline Design

## 1. Overview & Motivation
High-performance analytical engines such as **Apache Arrow**, **DuckDB**, and **Polars** achieve multi-gigabyte-per-second CSV ingestion through a combination of:
1. **Block-level SIMD scanning** (vectorized bitmasks for structural character classification).
2. **Two-tier state machines** (a quote-free fast path that skips quote handling for unquoted tabular data).
3. **Vectorized row counting** (SIMD newline classification with hardware popcount).
4. **Fast numeric parsing** (SWAR-based integer and boolean parsing bypassing standard library generic overhead).

Empirical CPU profiling of `github.com/minjejeon/csv` revealed:
- Under `GOEXPERIMENT=simd`, **48.80% of CPU time** is spent recreating broadcast vector registers (`simd/archsimd.BroadcastUint8x32`) per field.
- In `Reader.ReadRecord`, **36.8%–59.7% of CPU time** is spent in scalar branching and quote verification across every single field.
- In `Unmarshal`, **12.24% of CPU time** is spent in scalar row counting (`bytes.IndexAny`).
- In `Decoder`, **18%–20% of CPU time** is spent in standard `strconv.ParseFloat` and `strconv.ParseInt`.

This document specifies the end-to-end architecture to eliminate these bottlenecks and achieve multi-GB/s single-core throughput.

---

## 2. Architecture & Design Specifications

### Component 1: Zero-Allocation & Zero-Interface SIMD Vector Scanner
- **Eliminate Interface Boxing**:
  - `scan_simd.go` currently executes `mAll.ToArch().(type)` inside the inner vector loop.
  - Implement architecture-specialized scan routines (`scan_simd_amd64.go` using `simd/archsimd.Uint8x32` directly, `scan_simd_other.go` using portable `simd.Uint8s`, and `scan_swar.go` for non-SIMD builds).
- **Pre-computed Vector Broadcasts**:
  - Store pre-broadcast vector registers on `Reader` (or initialize once per record scan) so that `BroadcastUint8s` is never invoked inside the field scan loop.
- **Block Bitmask Generation**:
  - Implement 32-byte block scanning (`scanBlockBitmask32`) returning 32-bit bitmasks for delimiters, quotes, carriage returns, and line feeds:
    ```go
    type blockBitmasks struct {
        delim uint32
        quote uint32
        cr    uint32
        lf    uint32
    }
    ```
  - Use `bits.TrailingZeros32` to jump directly to structural character positions without inspecting individual bytes.

### Component 2: DuckDB-Style Two-Tier State Machine (No-Quote Fast Path)
- **Fast-Path Invariant**:
  - Over 90% of tabular CSV rows contain no quote characters.
  - While scanning 32-byte blocks, if `mask.quote == 0`:
    - Delimiter bits (`mask.delim`) immediately finalize field boundaries:
      ```go
      r.record.spans = append(r.record.spans, fieldSpan{
          start:      uint32(currStart),
          end:        uint32(delimIdx),
          hasEscapes: false,
      })
      ```
    - Newline bits (`mask.lf` or `mask.cr` + `\n`) terminate the record.
    - Bypasses quote state tracking, bare quote checks, and quote unescaping.
- **Quoted / RFC 4180 Fallback**:
  - If `mask.quote != 0` or multi-character delimiters / comments / `LazyQuotes` are active, switch seamlessly to the RFC 4180 state machine.

### Component 3: Vectorized Row Counting (`CountRecordsFast`)
- Accelerate pre-allocation row counting in `csv.go` and chunk splitting in `chunk.go`:
- Scan 32-byte / 64-byte chunks with vector `chunk.Equal(vLF)`:
  - Convert match mask to bits and accumulate count using `bits.OnesCount32` / `bits.OnesCount64`.
  - Achieves 30+ GB/s newline counting on AMD64 AVX2 / ARM64 Neon.

### Component 4: Fast SWAR ASCII Numeric & Boolean Parsing
- In `setter.go`, replace standard `strconv` calls in hot setters:
- **Integers (`int`, `int64`, `uint`, `uint64`)**:
  - Fast SWAR parser for 1–8 ASCII digit numbers:
    - Validate all characters in range `'0'..'9'` using SWAR bitwise mask.
    - Multiply and sum digits in parallel using integer arithmetic.
  - Loop unrolling for 9–18 digit numbers.
  - Safe fallback to `strconv.ParseInt` for negative signs, hexadecimal prefixes, or values > 18 digits.
- **Booleans (`bool`)**:
  - Direct byte comparison on `'1'`, `'0'`, `'t'`, `'f'`, `'T'`, `'F'`, `"true"`, `"false"`.

---

## 3. Data Flow Diagram

```
Raw CSV Byte Stream
        │
        ▼
[Block Scanner (32-byte SIMD / SWAR)]
        │
        ├── Has Quotes? (mask.quote != 0) ──► [RFC 4180 Quoted Fallback Engine]
        │                                             │
        └── No Quotes? (mask.quote == 0)              │
                │                                     │
                ▼                                     │
    [Vectorized No-Quote Fast Path]                   │
    (Bitmask Delim / EOL Extraction via TrailingZeros)│
                │                                     │
                └───────────────┬─────────────────────┘
                                │
                                ▼
                       [Record Spans Produced]
                                │
                                ▼
                 [Struct Decoder / Unmarshaler]
                                │
                ├── String / Bytes (Zero-Copy Slice)
                ├── Fast SWAR Int / Uint / Bool
                └── Float / Custom (TextUnmarshaler Fallback)
```

---

## 4. Testing & Verification Plan

1. **RFC 4180 & Regression Testing**:
   - `go test -v ./...` and `GOEXPERIMENT=simd go test -v ./...` must pass with 0 failures.
   - All standard library reader compatibility tests (`stdlib_reader_test.go`), `csvutil` tag tests (`csvutil_test.go`), and fuzz tests (`fuzz_test.go`) must continue to pass.
2. **Race Detector**:
   - `go test -race ./...` and `GOEXPERIMENT=simd go test -race ./...`.
3. **Benchmarks**:
   - Verify `BenchmarkReader_OursZeroCopy` increases from ~80 MB/s (SIMD) to $\ge 1.5$–$2.5\text{ GB/s}$.
   - Verify `BenchmarkDecoder_OursZeroAlloc` and `BenchmarkUnmarshal_Ours` show $\ge 2\times$ throughput speedup.
   - Ensure 0 heap allocations per operation across all hot paths.
