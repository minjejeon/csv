# High-Performance CSV Serialization Pipeline Design

## 1. Overview & Motivation
Building on the high-performance reader and struct unmarshaler in `github.com/minjejeon/csv`, this document specifies the complete zero-allocation, SIMD-accelerated serialization and marshaling engine.

Key goals:
1. **Low-Level Streaming `Writer`**: Zero heap allocations per record, pooled 64KB write buffers, SIMD-accelerated quote detection and escaping.
2. **High-Level Struct `Encoder` & `Marshal`**: Precompiled reflection type plans with direct `unsafe.Pointer` offset getters, in-place primitive formatting into the active write buffer (zero intermediate string allocations).
3. **Multithreaded `ParallelMarshal`**: Parallel chunked marshaling scaling across multi-core CPUs.
4. **Generic Zero-Reflection API**: `RecordMarshaler` compile-time interface and `MarshalSlice[T]`.

---

## 2. Architecture & Design Specifications

### Component 1: Streaming Zero-Allocation `Writer`
- **Configuration & Pooling**:
  - `Writer` wraps an `io.Writer`.
  - Configurable options: `Comma rune` (default `','`), `Delimiter string` (multi-char support), `Quote rune` (default `'"'`), `UseCRLF bool` (default `false`), `AutoFlush bool`.
  - Reuses pooled 64KB write buffers (`writeBufPool`).
- **SIMD Quote Detection**:
  - Uses `blockScanner` (with AVX2 `archsimd.Uint8x32` on AMD64, Neon on ARM64, and SWAR fallback) to test whether a field contains delimiters, quotes, `\r`, or `\n`.
  - If match is `-1` (>90% of tabular fields), directly copies raw bytes into `w.buf` via `copy`/`memmove`.
  - If match is $\ge 0$, writes opening quote, escapes quotes (`"` $\to$ `""`), and writes closing quote.
- **Flushing & Lifecycle**:
  - Automatic buffer flush when `len(w.buf)` exceeds 60KB.
  - `Flush() error` flushes remaining data to the underlying `io.Writer`.
  - `Close() error` flushes and releases the pooled buffer to `writeBufPool`.

### Component 2: Struct `Encoder` & `Marshal` Engine
- **Precompiled Type Plans (`typeMarshalPlan`)**:
  - Cached in thread-safe `sync.Map` by `reflect.Type`.
  - Precomputes field names from `csv:"col_name,omitempty"` tags.
  - Precalculates `uintptr` field offsets.
  - Generates direct `unsafe.Pointer` getters:
    - Primitive integers (`int*`, `uint*`): in-place append via `strconv.AppendInt` / `strconv.AppendUint`.
    - Floating point (`float32`, `float64`): in-place append via `strconv.AppendFloat`.
    - Boolean (`bool`): appends `"true"` or `"false"` byte literals.
    - String (`string`): direct unsafe string pointer read, SIMD quote check, and buffer append.
    - `time.Time`: direct RFC3339 format into buffer.
    - `encoding.TextMarshaler`: fallback calling `MarshalText()`.
- **`Encoder`**:
  - Streaming struct encoder writing header row on first call (configurable) followed by rows.
- **`Marshal`**:
  - Serializes slice of structs or pointer to slice into contiguous `[]byte`.

### Component 3: Multithreaded `ParallelMarshal`
- Slices partitioned into equal chunks across worker goroutines (`runtime.NumCPU()`).
- Worker 0 writes CSV header followed by its chunk of records into a thread-local buffer.
- Workers $1 \dots N-1$ marshal data rows concurrently into thread-local buffers.
- Sums chunk lengths and merges into a single contiguous byte slice with **1 final allocation**.

### Component 4: Generic Zero-Reflection API
- `RecordMarshaler` interface:
  ```go
  type RecordMarshaler interface {
      MarshalCSVRecord(w *Writer) error
  }
  ```
- `MarshalSlice[T any](slice []T, opts ...Option) ([]byte, error)`.
- `ParallelMarshalSlice[T any](slice []T, opts ...any) ([]byte, error)`.

---

## 3. Data Flow Diagram

```
Struct Slice / Stream
        │
        ▼
[typeMarshalPlan (Cached sync.Map)]
        │
        ├── Direct Unsafe Getters
        │   ├── int64 / int ──► strconv.AppendInt directly to w.buf
        │   ├── float64     ──► strconv.AppendFloat directly to w.buf
        │   ├── bool        ──► "true" / "false" literal to w.buf
        │   └── string      ──► SIMD Quote Check ──► Raw Copy or Quoted Escape
        ▼
[Streaming Writer (Pooled 64KB Buffer)]
        │
        ├── Auto-Flush to io.Writer (Streaming)
        └── Merge Chunks into Single Contiguous Slice (ParallelMarshal)
```

---

## 4. Testing & Verification Plan

1. **RFC 4180 Compatibility Tests**:
   - Quotes containing commas, newlines, quotes, CRLF, and trailing spaces.
2. **`csvutil` Struct Tag Compatibility**:
   - `omitempty`, custom field names, omitted fields (`-`), pointer fields, embedded structs.
3. **Round-Trip Tests**:
   - Verify `Unmarshal(Marshal(data)) == data` across diverse struct types.
4. **Benchmarks**:
   - Compare `Writer` against standard `encoding/csv.Writer`.
   - Compare `Marshal` and `Encoder` against `csvutil.Marshal` and `csvutil.Encoder`.
   - Benchmark `ParallelMarshal` scalability across 1, 2, 4, 8, 16 workers.
   - Confirm 0 allocs/op in hot streaming loop.
