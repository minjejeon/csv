# Custom Delimiter & Custom Quote Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Add functional options to configure custom single-character delimiters (e.g. `|`), multi-character delimiters (e.g. `||`, `::`), and custom quote runes (e.g. `'`), while maintaining SIMD AVX2 acceleration and zero allocations.

**Architecture:** Functional Options pattern (`Option`) configuring `Reader` settings. Single-byte delimiters and quotes maintain the ultra-fast AVX2 SIMD / SWAR vector scanning path, while multi-byte delimiters use vector search for the first byte followed by prefix matching. Doubled custom quotes are unescaped using the reader's scratch buffer.

**Tech Stack:** Go 1.27 (`simd/archsimd` AVX2 vector operations, SWAR fallback), `sync.Pool`, `unsafe.Pointer`.

---

### Task 1: Functional Options Core (`option.go`)

**Files:**
- Create: `option.go`
- Test: `option_test.go`

**Step 1: Write failing test**
Create `option_test.go` testing that `WithDelimiter`, `WithComma`, `WithQuote`, `WithComment`, `WithLazyQuotes`, `WithTrimLeadingSpace`, `WithFieldsPerRecord` properly apply options to `Reader`.

**Step 2: Run test to verify it fails**
Run: `/home/minje/golang/bin/go test -v -run=TestOptions .`
Expected: FAIL (undefined `Option`, `WithDelimiter`, etc.)

**Step 3: Implement minimal code in `option.go`**
Define `type Option func(*Reader)` and all functional options.

**Step 4: Run test to verify it passes**
Run: `/home/minje/golang/bin/go test -v -run=TestOptions .`
Expected: PASS

**Step 5: Commit**
`git add option.go option_test.go && git commit -m "feat: add functional options for reader configuration"`

---

### Task 2: Scanner Custom Delimiter & Quote Vector Search

**Files:**
- Modify: `scan_simd.go`
- Modify: `scan_swar.go`
- Modify: `scan_fallback.go`
- Modify: `scan_test.go`

**Step 1: Write failing test**
In `scan_test.go`, test scanning with custom quote byte (e.g. `'`) and custom delimiter byte (e.g. `|`).

**Step 2: Run test to verify it fails**
Run: `/home/minje/golang/bin/go test -v -run=TestScanCustomQuote .`
Expected: FAIL

**Step 3: Update `findNextSpecial` signature to accept `(data []byte, delim byte, quote byte)`**
Update `scan_simd.go` to broadcast `quote` instead of hardcoding `'"'`.
Update `scan_swar.go` to mask `quote` instead of hardcoding `'"'`.
Update `scan_fallback.go`.

**Step 4: Run tests to verify they pass**
Run: `/home/minje/golang/bin/go test -v -run=TestScan . && GOEXPERIMENT=simd /home/minje/golang/bin/go test -v -run=TestScan .`
Expected: PASS

**Step 5: Commit**
`git add scan_simd.go scan_swar.go scan_fallback.go scan_test.go && git commit -m "feat(scanner): support custom quote and delimiter in SIMD and SWAR scanners"`

---

### Task 3: Reader Multi-Character Delimiter & Custom Quote Engine

**Files:**
- Modify: `reader.go`
- Modify: `reader_test.go`

**Step 1: Write failing tests in `reader_test.go`**
Test reading with:
- Pipe delimiter `|`
- Multi-char delimiter `||` and `::`
- Custom quote `'` (e.g. `'hello''world'|'foo'`)
- Multi-char delimiter combined with custom quote: `'col1'||'col2'||'col3'`

**Step 2: Run tests to verify they fail**
Run: `/home/minje/golang/bin/go test -v -run='TestReaderCustomDelimiter|TestReaderMultiCharDelimiter|TestReaderCustomQuote' .`
Expected: FAIL

**Step 3: Implement Reader engine updates in `reader.go`**
- Initialize `Quote` (default `'"'`), `Delimiter` (default `string(Comma)`), `delimBytes`, `quoteByte`.
- Apply `opts ...Option` in `NewReader`.
- In `ReadRecord()`:
  - If `isMultiDelim`: match `delimBytes[0]`, check prefix, advance `len(delimBytes)`.
  - In quoted field: scan for `quoteByte`, check doubled `quoteByte` for escaping, check closing quote followed by delimiter, newline, or EOF.
  - In unescape: replace doubled `quoteByte` with single `quoteByte`.

**Step 4: Run tests to verify they pass**
Run: `/home/minje/golang/bin/go test -v -run='TestReaderCustomDelimiter|TestReaderMultiCharDelimiter|TestReaderCustomQuote' .`
Expected: PASS

**Step 5: Commit**
`git add reader.go reader_test.go && git commit -m "feat(reader): support multi-character delimiters and custom quote characters"`

---

### Task 4: Chunk Splitter & Record Counter Support

**Files:**
- Modify: `chunk.go`
- Modify: `chunk_test.go`
- Modify: `csv.go`

**Step 1: Write failing test**
In `chunk_test.go`, test splitting data with multi-character delimiters and custom quotes.

**Step 2: Run test to verify it fails**
Run: `/home/minje/golang/bin/go test -v -run=TestSplitChunksCustomQuoteAndDelim .`
Expected: FAIL

**Step 3: Update `splitChunks` and `countRecords`**
Support `quoteByte byte` in `countRecords`.
Support `quoteByte byte` and `delimBytes []byte` in `splitChunks`.

**Step 4: Run tests to verify they pass**
Run: `/home/minje/golang/bin/go test -v -run=TestSplitChunks .`
Expected: PASS

**Step 5: Commit**
`git add chunk.go chunk_test.go csv.go && git commit -m "feat(chunk): support custom quote and multi-char delimiter in chunk splitter"`

---

### Task 5: Integration Across Decoder, Unmarshal, Generic, and Parallel APIs

**Files:**
- Modify: `decoder.go`
- Modify: `csv.go`
- Modify: `generic.go`
- Modify: `parallel.go`
- Create: `custom_delim_test.go`

**Step 1: Write failing integration test**
In `custom_delim_test.go`, test:
- `NewDecoder(r, WithDelimiter("||"), WithQuote('\''))`
- `Unmarshal(data, &slice, WithDelimiter("::"), WithQuote('\''))`
- `UnmarshalSlice[T](data, WithDelimiter("||"))`
- `ParallelUnmarshal(data, &slice, WithDelimiter("|"), WithQuote('"'))`

**Step 2: Run test to verify it fails**
Run: `/home/minje/golang/bin/go test -v -run=TestCustomDelimIntegration .`
Expected: FAIL

**Step 3: Wire options into Decoder, Unmarshal, Generic, and Parallel APIs**
Accept `opts ...Option` and pass through to `NewReader`.

**Step 4: Run test to verify it passes**
Run: `/home/minje/golang/bin/go test -v -run=TestCustomDelimIntegration .`
Expected: PASS

**Step 5: Commit**
`git add decoder.go csv.go generic.go parallel.go custom_delim_test.go && git commit -m "feat(api): wire functional options across Decoder, Unmarshal, and Parallel APIs"`

---

### Task 6: Verification, Benchmarks, Git Tag & Push

**Files:**
- Verification: all tests, race detector, benchmarks

**Step 1: Run standard & SIMD test suite**
Run: `/home/minje/golang/bin/go test -v ./... && GOEXPERIMENT=simd /home/minje/golang/bin/go test -v ./...`
Expected: ALL PASS

**Step 2: Run race detector**
Run: `/home/minje/golang/bin/go test -race ./... && GOEXPERIMENT=simd /home/minje/golang/bin/go test -race ./...`
Expected: Clean (0 race)

**Step 3: Run benchmarks**
Run: `/home/minje/golang/bin/go test -bench=. -benchmem ./...`
Verify no regressions on standard CSV.

**Step 4: Create git tag and push**
Create tag `v0.2.0` (or update per user) and push commits and tags to GitHub `origin`.
