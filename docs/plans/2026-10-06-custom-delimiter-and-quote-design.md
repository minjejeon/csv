# Custom Delimiter & Custom Quote Design

## 1. Overview
This design extends `github.com/minjejeon/csv` to support:
1. **Custom Delimiters**: Single-character delimiters (`|`, `;`, `\t`) and multi-character delimiters (`||`, `::`, `-->`).
2. **Custom Quote Character**: Custom quote runes (e.g., `'`, `"`, or other runes), with doubled-quote escaping (e.g. `''` when quote is `'`).
3. **Functional Options Pattern**: Clean, idiomatic configuration (`WithDelimiter`, `WithQuote`, `WithComment`, etc.) across `NewReader`, `NewDecoder`, `Unmarshal`, `UnmarshalSlice`, and `ParallelUnmarshal`.

---

## 2. API Design

### 2.1 Option Definition (`option.go`)
```go
type Option func(*Reader)

func WithDelimiter(delim string) Option
func WithComma(comma rune) Option
func WithQuote(quote rune) Option
func WithComment(comment rune) Option
func WithLazyQuotes(lazy bool) Option
func WithTrimLeadingSpace(trim bool) Option
func WithFieldsPerRecord(n int) Option
```

### 2.2 Entry Points
- `NewReader(r io.Reader, opts ...Option) *Reader`
- `NewDecoder(r io.Reader, opts ...Option) (*Decoder, error)`
- `Unmarshal(data []byte, v any, opts ...Option) error`
- `UnmarshalSlice[T any, PT interface{ *T; RecordUnmarshaler }](data []byte, opts ...Option) ([]T, error)`
- `UnmarshalTo[T any, PT interface{ *T; RecordUnmarshaler }](data []byte, out *[]T, opts ...Option) error`
- `ParallelUnmarshal(data []byte, v any, opts ...Option) error`

---

## 3. Scanner Architecture

### 3.1 Reader Internal Fields
- `Comma rune` (default `','` for backward compatibility)
- `Quote rune` (default `'"'`)
- `Delimiter string` (if non-empty, takes precedence over `Comma`)
- Pre-compiled internal fields:
  - `delimBytes []byte`
  - `quoteByte byte`
  - `singleDelim byte` (if single-byte delimiter)
  - `isMultiDelim bool`

### 3.2 Scanning Strategy
1. **Single-Byte Delimiter & Single-Byte Quote (Fast Path)**:
   - Uses AVX2 SIMD (`scan_simd.go`) with `archsimd.BroadcastUint8x32(delim)` and `archsimd.BroadcastUint8x32(quote)`.
   - Fallback uses 64-bit SWAR (`scan_swar.go`).
   - Retains 700+ MB/s zero-allocation performance.

2. **Multi-Byte Delimiter**:
   - `findNextSpecial` searches for `delimBytes[0]`, `quoteByte`, `\r`, `\n`.
   - When `delimBytes[0]` matches, the reader checks `bytes.HasPrefix(r.buf[pos:], r.delimBytes)`.
   - If true, matches delimiter and skips `len(delimBytes)` bytes.
   - If false, continues scanning.

3. **Quoting & Escaping**:
   - Quoted field starts with `quoteByte`.
   - Inside quoted field, `quoteByte` followed by another `quoteByte` is an escaped quote.
   - Closing quote must be followed by `delimBytes`, `\r`, `\n`, whitespace, or EOF.
   - Scratch unescaping replaces doubled quotes with a single quote character.

4. **Chunk Splitter & Record Counter**:
   - `splitChunks(data, numWorkers, opts...)` and `countRecords(data, quoteByte)` are updated to respect custom quotes and delimiters.

---

## 4. Verification & Testing Plan
- Test single-char custom delimiters: pipe (`|`), semicolon (`;`), tab (`\t`).
- Test multi-char delimiters: `"||"`, `"::"`, `"sep"`.
- Test custom quote: single quote (`'`), backtick or custom rune.
- Test quotes with multi-character delimiters: `'val1'||'val2'||'val3'`.
- Test escaped quotes with custom quote: `'val''1'||'val''2'`.
- Test functional options on `NewReader`, `NewDecoder`, `Unmarshal`, `UnmarshalSlice`, and `ParallelUnmarshal`.
- Run full test suite under both standard fallback and `GOEXPERIMENT=simd`.
- Verify zero-race with `go test -race ./...`.
- Verify benchmarks to ensure no performance regression on standard CSV.
