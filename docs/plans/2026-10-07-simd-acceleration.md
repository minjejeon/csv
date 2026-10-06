# SIMD Acceleration Pipeline Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Accelerate CSV parsing throughput to 2+ GB/s and struct decoding by 2x using block-level SIMD bitmask scanning, DuckDB-style quote-free fast paths, vectorized row counting, and SWAR numeric deserialization.

**Architecture:** Combine Go 1.27 portable SIMD (`simd` / `archsimd`) with direct AMD64 AVX2 / ARM64 Neon bitmasks. The lexer uses a 32-byte block scanner to extract delimiters and newlines via `bits.TrailingZeros` without quote overhead on unquoted records, while the struct decoder utilizes SWAR ASCII integer and boolean parsers to eliminate `strconv` overhead.

**Tech Stack:** Go 1.27 (`GOEXPERIMENT=simd`, package `simd`, package `simd/archsimd`), 64-bit SWAR, AMD64 AVX2 / AVX-512, ARM64 Neon.

---

### Task 1: Vectorized Architecture Scanner & Broadcast Register Cache

**Files:**
- Modify: `scan_simd.go`
- Modify: `reader.go`
- Test: `scan_test.go`

**Step 1: Write the failing test**
Add a benchmark/unit test verifying `findNextSpecial` performance and register reuse in `scan_test.go`:
```go
func TestScanSpecialPrecomputedVectors(t *testing.T) {
    data := []byte("field1,field2,field3\n")
    idx := findNextSpecial(data, ',', '"')
    if idx != 6 {
        t.Fatalf("expected 6, got %d", idx)
    }
}
```

**Step 2: Run test to verify it fails**
Run: `GOEXPERIMENT=simd go test -v -run TestScanSpecialPrecomputedVectors .`
Expected: PASS or verify existing behavior.

**Step 3: Write minimal implementation**
- In `scan_simd.go`, eliminate per-chunk interface assertion `mAll.ToArch().(type)` by specializing for AMD64 (`archsimd.Uint8x32`) or moving type detection out of the inner loop.
- In `reader.go`, pre-broadcast vectors on `Reader` during `initDelimAndQuote()`.

**Step 4: Run test to verify it passes**
Run: `GOEXPERIMENT=simd go test -v -run TestScanSpecial .`
Expected: PASS

**Step 5: Commit**
```bash
git add scan_simd.go reader.go scan_test.go
git commit -m "perf(simd): eliminate interface boxing and pre-broadcast vector registers"
```

---

### Task 2: DuckDB-Style No-Quote Fast-Path Block Scanner

**Files:**
- Modify: `scan_simd.go`
- Modify: `scan_swar.go`
- Modify: `reader.go`
- Test: `reader_test.go`

**Step 1: Write the failing test**
In `reader_test.go`:
```go
func TestReaderNoQuoteFastPath(t *testing.T) {
    data := "a,b,c,d\n1,2,3,4\n5,6,7,8\n"
    r := NewReader(strings.NewReader(data))
    rec, err := r.ReadRecord()
    if err != nil {
        t.Fatal(err)
    }
    if rec.NumFields() != 4 || string(rec.Field(0)) != "a" {
        t.Fatalf("unexpected record: %v", rec)
    }
}
```

**Step 2: Run test to verify it passes or fails**
Run: `go test -v -run TestReaderNoQuoteFastPath .`

**Step 3: Write minimal implementation**
- Implement `scanBlockBitmasks(data []byte, delim, quote byte) (delimMask, quoteMask, eolMask uint32)` in `scan_simd.go` and SWAR fallback in `scan_swar.go`.
- In `Reader.ReadRecord`, when `maskQuote == 0`, extract fields in a tight loop via `bits.TrailingZeros32` without quote branching.
- Seamlessly fallback to the existing RFC 4180 state machine when quotes are present.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestReaderNoQuoteFastPath .`
Run: `GOEXPERIMENT=simd go test -v ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add scan_simd.go scan_swar.go reader.go reader_test.go
git commit -m "feat(reader): add DuckDB-style no-quote vectorized fast path"
```

---

### Task 3: SIMD Vectorized Record & Chunk Counting

**Files:**
- Modify: `csv.go`
- Modify: `chunk.go`
- Test: `chunk_test.go`

**Step 1: Write the failing test**
In `chunk_test.go`:
```go
func TestCountRecordsFastSIMD(t *testing.T) {
    data := []byte("col1,col2\nval1,val2\nval3,val4\n")
    count := countRecords(data)
    if count != 3 {
        t.Fatalf("expected 3 records, got %d", count)
    }
}
```

**Step 2: Run test to verify behavior**
Run: `go test -v -run TestCountRecordsFastSIMD .`

**Step 3: Write minimal implementation**
- Implement `countNewlinesSIMD(data []byte) int` in `csv.go` (and SIMD/SWAR variants):
  - Load 32-byte chunks, compare against `\n` (`chunk.Equal(vLF)`), count bits with `bits.OnesCount32`.
- Use in `countRecords` and chunk boundary detection.

**Step 4: Run test to verify it passes**
Run: `GOEXPERIMENT=simd go test -v -run TestCountRecordsFastSIMD .`
Expected: PASS

**Step 5: Commit**
```bash
git add csv.go chunk.go chunk_test.go
git commit -m "perf(csv): implement SIMD-accelerated newline counting"
```

---

### Task 4: Fast SWAR ASCII Integer & Boolean Parsing in Struct Decoder

**Files:**
- Modify: `setter.go`
- Test: `setter_test.go`
- Test: `decoder_test.go`

**Step 1: Write the failing test**
In `setter_test.go`:
```go
func TestFastParseInteger(t *testing.T) {
    tests := []struct {
        in  string
        exp int64
    }{
        {"0", 0},
        {"123", 123},
        {"987654321", 987654321},
        {"-42", -42},
    }
    for _, tt := range tests {
        v, err := fastParseInt64([]byte(tt.in))
        if err != nil {
            t.Fatalf("unexpected err for %q: %v", tt.in, err)
        }
        if v != tt.exp {
            t.Fatalf("expected %d, got %d", tt.exp, v)
        }
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestFastParseInteger .`
Expected: FAIL ("fastParseInt64 not defined")

**Step 3: Write minimal implementation**
- Implement `fastParseInt64(b []byte) (int64, error)` and `fastParseUint64(b []byte) (uint64, error)` in `setter.go`.
- Fast path: check 1–8 digits, validate ASCII digits, and accumulate using SWAR. Fallback to `strconv.ParseInt` for negative numbers or >18 digits.
- Integrate into integer setter functions in `compileSetter`.
- Implement fast boolean parsing on byte slice in `compileSetter` for bool fields.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestFastParseInteger .`
Run: `go test -v -run TestDecoder .`
Expected: PASS

**Step 5: Commit**
```bash
git add setter.go setter_test.go
git commit -m "perf(decoder): implement fast SWAR integer and boolean setters"
```

---

### Task 5: End-to-End Verification, Benchmarks & Regression Testing

**Files:**
- Test: `benchmark_test.go`
- Test: `fuzz_test.go`
- Modify: `docs/plans/task.md`

**Step 1: Comprehensive Test Execution**
Run standard tests:
`go test -v ./...`
Run SIMD tests:
`GOEXPERIMENT=simd go test -v ./...`
Run Race detector:
`go test -race ./...`
`GOEXPERIMENT=simd go test -race ./...`

**Step 2: Run Benchmarks**
Run:
`go test -bench=. -benchmem -benchtime=500ms ./...`
`GOEXPERIMENT=simd go test -bench=. -benchmem -benchtime=500ms ./...`

**Step 3: Update documentation and task tracker**
Update `docs/plans/task.md` and commit.
```bash
git add docs/plans/task.md
git commit -m "docs: complete SIMD acceleration implementation tasks"
```
