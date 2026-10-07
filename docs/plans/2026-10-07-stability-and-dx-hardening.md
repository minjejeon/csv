# Stability & DX Hardening Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Implement memory safety fixes, pool bounding, Record.Clone, structured DecodeError diagnostics, UTF-8 BOM auto-stripping, custom time.Time format tags, and multi-charset Writer encoding.

**Architecture:** Refactor option inspection in parallel pipelines to eliminate throwaway reader buffer allocations, bound sync.Pool write buffers to 2MB, implement detached Record.Clone, introduce structured DecodeError with row/col/val diagnostics, add transparent BOM handling and custom format struct tags, and enable transform.NewWriter in Writer.

**Tech Stack:** Go 1.27 (`/home/minje/golang/bin/go`), SIMD (`GOEXPERIMENT=simd`), `golang.org/x/text/encoding`, `transform`.

---

### Task 1: Memory & Resource Safety Hardening

**Files:**
- Modify: `parallel.go:70-115, 305-320`
- Modify: `generic.go:105-140`
- Modify: `pool.go:100-120`
- Modify: `record.go:1-50`
- Modify: `plan.go:40-80`
- Test: `pool_test.go`
- Test: `audit_test.go`

**Step 1: Write the failing tests**
- In `pool_test.go`: test that `releaseWriteBuf` drops buffers with capacity > 2MB.
- In `audit_test.go`: test `Record.Clone()` independence when reader advances.
- In `parallel_test.go`: verify `ParallelUnmarshal` runs repeatedly without accumulating pooled memory allocations.

**Step 2: Run tests to verify failures**
Run: `go test -v -run "TestPoolCap|TestRecordClone" ./...`
Expected: FAIL (methods or behaviors not yet implemented)

**Step 3: Write minimal implementation**
1. In `pool.go`: define `const maxPoolBufferSize = 2 * 1024 * 1024`. In `releaseWriteBuf`, check `if cap(*b) > maxPoolBufferSize { return }`.
2. In `parallel.go` & `generic.go`: replace `dummy := NewReader(nil, csvOpts...)` with lightweight option inspection helper or ensure `defer dummy.Close()`.
3. In `record.go`: implement `func (rec *Record) Clone() *Record` that copies spans and raw bytes into a new standalone buffer.
4. In `plan.go`: extend `collectStructFields` to handle `f.Type.Kind() == reflect.Pointer && f.Type.Elem().Kind() == reflect.Struct`.

**Step 4: Run tests to verify they pass**
Run: `go test -v -run "TestPoolCap|TestRecordClone|TestParallel" ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add pool.go parallel.go generic.go record.go plan.go pool_test.go audit_test.go
git commit -m "fix(safety): eliminate pool leaks, bound write pool buffer, and add Record.Clone"
```

---

### Task 2: Rich Error Diagnostics (Structured `DecodeError`)

**Files:**
- Create/Modify: `decoder.go:1-139`
- Modify: `csv.go:70-125`
- Modify: `parallel.go:180-210`
- Test: `decoder_test.go`

**Step 1: Write the failing tests**
- In `decoder_test.go`: test that type conversion errors return `*DecodeError` with matching `Line`, `Column`, `Header`, `Field`, `Value`, and unwrap to underlying syntax/range errors via `errors.As` and `errors.Is`.

**Step 2: Run test to verify failure**
Run: `go test -v -run "TestDecodeError" ./...`
Expected: FAIL (`DecodeError` undefined)

**Step 3: Write minimal implementation**
1. In `decoder.go`: define `type DecodeError struct { Line, Column int; Header, Field, Value string; Err error }` with `Error()` and `Unwrap() error`.
2. Track row / line numbers in `Decoder`, `Unmarshal`, and `ParallelUnmarshal`.
3. Return `*DecodeError` instead of untyped `fmt.Errorf`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run "TestDecodeError" ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add decoder.go csv.go parallel.go decoder_test.go
git commit -m "feat(diagnostics): introduce structured DecodeError with line, column, and raw value context"
```

---

### Task 3: UTF-8 BOM Auto-Stripping & Custom Time Format Tags

**Files:**
- Modify: `tag.go:30-60`
- Modify: `setter.go:230-260`
- Modify: `getter.go:85-110`
- Modify: `reader.go:90-130, 230-260`
- Modify: `option.go:40-60`
- Test: `internet_edge_cases_test.go`
- Test: `tag_test.go`

**Step 1: Write the failing tests**
- In `internet_edge_cases_test.go`: test that a CSV with UTF-8 BOM header `\xef\xbb\xbfid,name` parses column `id` cleanly without BOM artifact.
- In `tag_test.go`: test `time.Time` fields with `format=2006-01-02 15:04:05` round-trip through Unmarshal and Marshal.

**Step 2: Run test to verify failure**
Run: `go test -v -run "TestBOM|TestTimeFormatTag" ./...`
Expected: FAIL

**Step 3: Write minimal implementation**
1. Add `WithTrimBOM(trim bool)` option (default: true).
2. In `Reader`, detect and strip `\xef\xbb\xbf` BOM if present at start.
3. In `tag.go`, parse `format=...` option into `tag.format`.
4. In `setter.go` & `getter.go`, handle custom layouts for `time.Time`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run "TestBOM|TestTimeFormatTag" ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add tag.go setter.go getter.go reader.go option.go internet_edge_cases_test.go tag_test.go
git commit -m "feat(dx): add UTF-8 BOM auto-stripping and custom time.Time format tags"
```

---

### Task 4: Symmetric Multi-Charset Writer Encoding

**Files:**
- Modify: `writer.go:30-100`
- Modify: `encoder.go:15-30`
- Modify: `csv.go:130-180`
- Test: `encoding_test.go`

**Step 1: Write the failing test**
- In `encoding_test.go`: test `Marshal` and `NewWriter` writing EUC-KR and Shift_JIS encoded bytes.

**Step 2: Run test to verify failure**
Run: `go test -v -run "TestWriterEncoding" ./...`
Expected: FAIL

**Step 3: Write minimal implementation**
1. In `writer.go`, recognize `encoding` option and wrap underlying `w` with `transform.NewWriter(w, enc.NewEncoder())`.
2. In `Writer.Close()`, flush transformation pipeline.
3. Update `Marshal` to forward encoding options.

**Step 4: Run test to verify it passes**
Run: `go test -v -run "TestWriterEncoding" ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add writer.go encoder.go csv.go encoding_test.go
git commit -m "feat(encoding): support multi-charset encoding on Writer, Encoder, and Marshal"
```

---

### Task 5: End-to-End Verification, Race Detection, and Benchmarks

**Files:**
- Test: `./...`

**Step 1: Run comprehensive tests**
Run: `go test -v ./...`
Run: `GOEXPERIMENT=simd go test -v ./...`
Run: `go test -race ./...`
Run: `GOEXPERIMENT=simd go test -race ./...`
Expected: PASS across all suites with 0 data races.

**Step 2: Run benchmarks to verify zero degradation**
Run: `go test -bench=. -benchmem ./...`
Run: `GOEXPERIMENT=simd go test -bench=. -benchmem ./...`
Expected: No allocation regressions.

**Step 3: Update documentation & task tracker**
- Update `README.md` with new features (`DecodeError`, `Record.Clone()`, `format=...`, `WithTrimBOM`).
- Update `docs/plans/task.md`.
```bash
git add README.md docs/plans/task.md
git commit -m "docs: document stability hardening and DX features in README"
```
