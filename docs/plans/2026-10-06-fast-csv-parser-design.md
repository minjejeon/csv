# Design Document: Fast SIMD-Accelerated Zero-Allocation CSV Parser

- **Module**: `github.com/minjejeon/csv`
- **Date**: 2026-10-06
- **Status**: Approved

---

## 1. Objectives & Requirements

1. **High Throughput & Low Latency**: Maximize CSV parsing throughput utilizing Go 1.27 SIMD (`archsimd` AVX2) vector scanning with automatic pure Go fallback.
2. **Minimal Heap Allocations**: Leverage `sync.Pool` across all layers (read buffers, record spans, unescape buffers) to achieve near-zero heap allocations in streaming decoding loops.
3. **`csvutil`-Compatible Struct Mapping**: Support struct tags (`csv:"field_name,omitempty"`), custom types, `encoding.TextUnmarshaler`, and pointer fields.
4. **Zero-Reflection Hot Path**: Pre-compile struct type metadata into direct `unsafe.Pointer` memory offsets to eliminate reflection overhead and interface boxing during row parsing.
5. **Standard Compatibility & Testing**: Import and pass test suites from Go standard `encoding/csv` (RFC 4180 parsing semantics) and `csvutil` (struct unmarshaling semantics).

---

## 2. Architecture & Components

```
                   +----------------------------------+
                   |          User Code               |
                   +----------------------------------+
                             |              |
                      dec.Decode(&row)    Unmarshal(data, &rows)
                             |              |
                             v              v
                   +----------------------------------+
                   |       csv.Decoder / Unmarshal    |
                   +----------------------------------+
                             |
         +-------------------+-------------------+
         |                                       |
         v                                       v
+-----------------------+              +-----------------------+
|   Struct Plan Cache   |              |      csv.Reader       |
|  (unsafe.Pointer,     |              |  (Buffer Sliding,     |
|   field offsets,      |              |   Record Spans)       |
|   type setters)       |              +-----------------------+
+-----------------------+                         |
                                                  v
                                       +-----------------------+
                                       |      Fast Scanner     |
                                       +-----------------------+
                                       | SIMD (archsimd AVX2)  |  (if GOEXPERIMENT=simd && AVX2)
                                       | Fallback (SWAR / pure)|  (default / non-AVX2)
                                       +-----------------------+
                                                  |
                                                  v
                                       +-----------------------+
                                       |      sync.Pool        |
                                       | - readBufferPool      |
                                       | - recordSpanPool      |
                                       | - fieldBufPool        |
                                       +-----------------------+
```

---

## 3. Low-Level Scanner & Lexer

### 3.1 Field Span Model
Instead of allocating strings or byte slices per cell during lexing, the lexer identifies boundaries inside the active read buffer:
```go
type fieldSpan struct {
    start      uint32
    end        uint32
    hasEscapes bool
}
```

### 3.2 SIMD Scanner (`scan_simd.go`)
- **Build Tag**: `//go:build goexperiment.simd && amd64`
- Loads 32 bytes into `archsimd.Int8x32`.
- Performs 4 vector comparisons in parallel:
  - `chunk.Equal(vComma)`
  - `chunk.Equal(vQuote)`
  - `chunk.Equal(vCR)`
  - `chunk.Equal(vLF)`
- Combines masks using `mask.Or(...)`.
- If the bitmask is zero, skips 32 bytes in 1-2 CPU cycles.
- If non-zero, uses `bits.TrailingZeros32` to find the exact character index and transition states.
- Runtime safety check: calls `archsimd.X86.AVX2()`; falls back to pure Go scanning if AVX2 is absent.

### 3.3 Fallback Scanner (`scan_fallback.go`)
- **Build Tag**: `//go:build !goexperiment.simd || !amd64`
- Uses 8-byte SWAR (SIMD Within A Register) bitwise operations and `bytes.IndexAny` for fast character search.

---

## 4. Memory Management & `sync.Pool`

1. **`readBufferPool` (`sync.Pool`)**:
   - Reuses 64KB byte slices for reading from `io.Reader`.
   - Handles multi-buffer spanning records via sliding window (moves trailing incomplete line to index 0, reads next chunk).
2. **`recordSpanPool` (`sync.Pool`)**:
   - Reuses `[]fieldSpan` slices (initial capacity 32 fields).
   - Released back to pool upon completion of record processing or iterator advance.
3. **`fieldBufPool` (`sync.Pool`)**:
   - Reuses byte buffers needed when unescaping quotes (`""` -> `"`).
   - Unescaped fields take the fast zero-copy path pointing directly to the read buffer.

---

## 5. Struct Mapping Engine (`Decoder`)

### 5.1 Pre-compiled Type Plan (`typePlan`)
- Inspection via `reflect.Type` is done **once** per struct type and cached in a thread-safe `sync.Map`.
- Headers in the CSV are matched with struct tags (`csv:"header_name"` or field name case-sensitive/insensitive fallback).
- Each matched column produces a `fieldPlan`:
  ```go
  type fieldPlan struct {
      colIndex uint32
      offset   uintptr
      setter   func(structPtr unsafe.Pointer, raw []byte) error
  }
  ```

### 5.2 Direct Unsafe Setter Fast-Path
Setters operate directly on `unsafe.Add(structPtr, plan.offset)`:
- `string`: string copy or string conversion from `raw`.
- `int / int8 / int16 / int32 / int64`: `strconv.ParseInt` directly into `*(*int)(target)`.
- `uint / uint8 / uint16 / uint32 / uint64`: `strconv.ParseUint`.
- `float32 / float64`: `strconv.ParseFloat`.
- `bool`: `strconv.ParseBool` or fast ASCII check ('1', 't', 'T', '0', 'f', 'F').
- `time.Time`: RFC3339 or custom format specified in `csv:",format:2006-01-02"`.
- `encoding.TextUnmarshaler`: Reflection/safe fallback to instantiate and invoke `UnmarshalText`.
- Pointers (`*T`): Allocate underlying element if nil, then parse into element.

---

## 6. Public API

```go
package csv

// Reader reads CSV records at low level with zero-alloc option
type Reader struct { ... }
func NewReader(r io.Reader) *Reader
func (r *Reader) Read() ([]string, error)
func (r *Reader) ReadRecord() (*Record, error) // zero-copy record view

// Decoder streams CSV records directly into Go structs
type Decoder struct { ... }
func NewDecoder(r io.Reader) (*Decoder, error)
func (d *Decoder) Decode(v any) error
func (d *Decoder) More() bool

// Unmarshal parses CSV data directly into a slice of structs
func Unmarshal(data []byte, v any) error
```

---

## 7. Testing Strategy

1. **Standard `encoding/csv` Test Suite**:
   - Port all test cases from Go standard library `src/encoding/csv/reader_test.go`:
     - Normal CSV, trailing commas, empty fields, quoted fields, quotes with newlines, RFC 4180 edge cases, CRLF handling, comment handling, lazy quotes.
2. **`csvutil` Test Suite**:
   - Port key test cases from `github.com/jszwec/csvutil`:
     - Struct tag mapping, omitempty, custom field names, primitive types, pointer types, unmarshaler interface, struct embedded fields.
3. **SIMD vs Fallback Parity Tests**:
   - Run complete test suite under both standard build and `GOEXPERIMENT=simd`.
   - Property-based testing / randomized CSV inputs to ensure SIMD and fallback yield identical outputs.
4. **Benchmark Suite**:
   - Measure throughput (MB/s) and allocations (B/op, allocs/op) comparing:
     - Standard `encoding/csv`
     - `csvutil`
     - `github.com/minjejeon/csv` (Fallback)
     - `github.com/minjejeon/csv` (SIMD AVX2)
