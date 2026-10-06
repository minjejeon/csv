# Fast SIMD Zero-Allocation CSV Parser Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Build a blazingly fast Go CSV reader and struct unmarshaler (`github.com/minjejeon/csv`) featuring SIMD AVX2 acceleration with pure Go fallback, `sync.Pool` zero-allocation pooling, and `csvutil`-compatible `unsafe.Pointer` struct mapping.

**Architecture:** A layered architecture consisting of: (1) memory pools for 64KB read buffers, field spans, and unescape scratch buffers; (2) dual-path scanner with Go 1.27 `simd/archsimd` AVX2 vector scanning and SWAR pure Go fallback; (3) zero-copy RFC 4180 lexer with sliding window buffering; (4) cached type plan with `unsafe.Pointer` offset writes for zero reflection overhead in hot loops; (5) streaming `Decoder` and bulk `Unmarshal` public API.

**Tech Stack:** Go 1.27 (`simd/archsimd` with `GOEXPERIMENT=simd`), `sync.Pool`, `unsafe.Pointer`, `reflect`, Go standard testing & benchmarking tools.

---

### Task 1: Core Buffer & Span Pool Management

**Files:**
- Create: `pool.go`
- Test: `pool_test.go`

**Step 1: Write the failing test**
Create `pool_test.go` testing that `spanSlicePool`, `readBufPool`, and `fieldBufPool` return zeroed/properly sized slices, can be reused across allocations, and `testing.AllocsPerRun` for pooled operations is zero.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestPool ./...`
Expected: FAIL with undefined identifiers (`getSpanSlice`, `putSpanSlice`, etc.).

**Step 3: Write minimal implementation**
Implement `pool.go` with:
- `type fieldSpan struct { start, end uint32; hasEscapes bool }`
- `var spanSlicePool sync.Pool`
- `var readBufPool sync.Pool`
- `var fieldBufPool sync.Pool`
- Helper functions: `acquireSpans()`, `releaseSpans()`, `acquireReadBuf()`, `releaseReadBuf()`, `acquireFieldBuf()`, `releaseFieldBuf()`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestPool ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add pool.go pool_test.go
git commit -m "feat(pool): implement memory and span slice pools"
```

---

### Task 2: Fast Byte Scanner with SIMD (AVX2) and Fallback (SWAR)

**Files:**
- Create: `scan.go`
- Create: `scan_simd.go` (`//go:build goexperiment.simd && amd64`)
- Create: `scan_fallback.go` (`//go:build !goexperiment.simd || !amd64`)
- Test: `scan_test.go`

**Step 1: Write the failing test**
Create `scan_test.go` testing `findNextSpecial(data []byte, delim byte)` against a variety of inputs (clean text, strings with delimiter, quotes, CRLF, varying chunk lengths 0 to 128 bytes). Verify that SIMD and fallback scan results are 100% identical.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestScanner ./...`
Expected: FAIL with `findNextSpecial` undefined.

**Step 3: Write minimal implementation**
- `scan.go`: Common scanner interface and state tracker.
- `scan_simd.go`: Loads 32 bytes with `archsimd.LoadInt8x32` (or unsafe slice load), compares against delim, quote, `\r`, `\n` using `.Equal()`, combines masks with `.Or()`, and gets bit index with `bits.TrailingZeros32`. Includes runtime `archsimd.X86.AVX2()` guard.
- `scan_fallback.go`: Pure Go SWAR 64-bit integer bit-masking and byte loop searching for `,`, `"`, `\r`, `\n`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestScanner ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestScanner ./...`
Expected: PASS on both.

**Step 5: Commit**
```bash
git add scan.go scan_simd.go scan_fallback.go scan_test.go
git commit -m "feat(scanner): implement SIMD AVX2 and SWAR fallback byte scanners"
```

---

### Task 3: Zero-Allocation Low-Level CSV Reader & Port `encoding/csv` Tests

**Files:**
- Create: `reader.go`
- Create: `record.go`
- Test: `reader_test.go`
- Test: `stdlib_reader_test.go`

**Step 1: Write the failing test**
Port RFC 4180 test cases from Go standard library `src/encoding/csv/reader_test.go` into `stdlib_reader_test.go`. Include checks for commas, quotes, CRLF, multi-line fields, and unescaping `""`.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestStdlibReader ./...`
Expected: FAIL with `NewReader` undefined.

**Step 3: Write minimal implementation**
Implement `reader.go`:
- `Reader` struct wrapping `io.Reader`, with buffer sliding window, delimiter, comment, and lazy quotes options.
- `ReadRecord() (*Record, error)` returning a pooled zero-copy record referencing read buffer.
- `Read() ([]string, error)` for compatibility with `encoding/csv.Reader`.
- Quote unescaping using `acquireFieldBuf()`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestStdlibReader ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestStdlibReader ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add reader.go record.go reader_test.go stdlib_reader_test.go
git commit -m "feat(reader): implement RFC 4180 zero-copy reader passing stdlib tests"
```

---

### Task 4: Struct Tag Parser & Cached Execution Plan

**Files:**
- Create: `tag.go`
- Create: `plan.go`
- Test: `plan_test.go`

**Step 1: Write the failing test**
Create `plan_test.go` verifying struct tags (`csv:"col_name,omitempty"`, `csv:"-"`), matching against CSV headers, and caching compiled `*typePlan` in `sync.Map`.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestPlan ./...`
Expected: FAIL with `buildTypePlan` undefined.

**Step 3: Write minimal implementation**
Implement `tag.go` and `plan.go`:
- Struct tag parser for `csv` tag options.
- `typePlan` compiling struct fields into `[]fieldPlan`.
- Cache `*typePlan` by `(reflect.Type, headerHash)` in `sync.Map`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestPlan ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add tag.go plan.go plan_test.go
git commit -m "feat(plan): implement struct tag parser and type plan caching"
```

---

### Task 5: Zero-Allocation `unsafe.Pointer` Field Setters & Decoder

**Files:**
- Create: `setter.go`
- Create: `decoder.go`
- Test: `setter_test.go`
- Test: `decoder_test.go`

**Step 1: Write the failing test**
Test decoding into struct types with string, int, uint, float, bool, pointers, `time.Time`, and custom `encoding.TextUnmarshaler`. Verify zero heap allocations per row when decoding primitive structs.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestDecoder ./...`
Expected: FAIL with `NewDecoder` undefined.

**Step 3: Write minimal implementation**
- `setter.go`: Implement fast typed setters using `unsafe.Add(ptr, offset)` for all primitive types:
  - `string`: zero-copy or direct string cast.
  - `int*`, `uint*`, `float*`, `bool`: `strconv` parsing directly into memory address.
  - Pointers: nil check, allocate pointer target if nil, call inner setter.
  - Custom types: check `encoding.TextUnmarshaler`.
- `decoder.go`: `Decoder` struct wrapping `Reader`, `Decode(v any) error`, `More() bool`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestDecoder ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestDecoder ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add setter.go decoder.go setter_test.go decoder_test.go
git commit -m "feat(decoder): implement unsafe field setters and streaming Decoder"
```

---

### Task 6: High-Level `Unmarshal` API & `csvutil` Test Suite

**Files:**
- Create: `csv.go`
- Test: `csv_test.go`
- Test: `csvutil_test.go`

**Step 1: Write the failing test**
Port struct unmarshaling test cases from `github.com/jszwec/csvutil` into `csvutil_test.go` testing `Unmarshal(data, &slice)` with slices of structs, custom tags, omitempty, pointers, and embedded fields.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestCsvutil ./...`
Expected: FAIL with `Unmarshal` undefined.

**Step 3: Write minimal implementation**
Implement `csv.go`:
- `Unmarshal(data []byte, v any) error`: handles slices of structs, pointer slices, and headers automatically.
- Wire together `Decoder`, `Reader`, and `typePlan`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestCsvutil ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestCsvutil ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add csv.go csv_test.go csvutil_test.go
git commit -m "feat(api): implement Unmarshal API and pass csvutil compatibility test suite"
```

---

### Task 7: Comprehensive Benchmarks & Allocation Verification

**Files:**
- Create: `benchmark_test.go`

**Step 1: Write benchmarks**
Benchmark reader throughput and decoder throughput against:
- Standard library `encoding/csv`
- `github.com/jszwec/csvutil`
- `github.com/minjejeon/csv` (Fallback mode)
- `github.com/minjejeon/csv` (SIMD AVX2 mode)
Include tests for `ns/op`, `B/op`, `allocs/op`.

**Step 2: Run benchmarks and verify zero-allocation targets**
Run: `go test -bench=. -benchmem benchmark_test.go`
Run: `GOEXPERIMENT=simd go test -bench=. -benchmem benchmark_test.go`
Expected: Zero allocations in `BenchmarkDecoderStream` hot loop, significantly higher throughput (MB/s) than stdlib and `csvutil`.

**Step 3: Commit**
```bash
git add benchmark_test.go
git commit -m "test(bench): add comparative benchmarks against stdlib and csvutil"
```
