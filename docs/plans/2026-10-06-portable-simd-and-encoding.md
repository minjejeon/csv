# Portable SIMD, Custom Encoding, and Project Documentation Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use single-flow mode to execute this plan task-by-task.

**Goal:** Integrate Go 1.27 portable SIMD, custom encoding support (EUC-KR/CP949/Shift_JIS), English coding convention in AGENTS.md, and comprehensive English README.md.

**Architecture:** Utilize Go 1.27's portable standard `simd` package for vector scanning across all supported architectures; integrate `golang.org/x/text/encoding` via functional options wrapping stream decoders; write English documentation and update project configuration.

**Tech Stack:** Go 1.27 (`GOEXPERIMENT=simd`), `simd`, `simd/archsimd`, `golang.org/x/text/encoding/korean`, `golang.org/x/text/encoding/japanese`, `golang.org/x/text/transform`.

---

### Task 1: Portable SIMD Scanner

**Files:**
- Modify: `scan_simd.go`
- Modify: `scan_fallback.go`
- Test: `scan_test.go`

**Step 1: Write test or verify existing SIMD test suite**
Run: `GOEXPERIMENT=simd /home/minje/golang/bin/go test -v -run=TestScanSpecial .`

**Step 2: Update `scan_simd.go` and `scan_fallback.go`**
Change `scan_simd.go` to `//go:build goexperiment.simd` (removing `&& amd64`).
Use `simd.BroadcastUint8s` and `simd.LoadUint8s` with `simd.VectorBitSize() / 8`.
Extract matches via `mAll.ToArch()`.
Update `scan_fallback.go` to `//go:build !goexperiment.simd`.

**Step 3: Run tests in both standard and SIMD modes**
Run: `/home/minje/golang/bin/go test -v -run=TestScanSpecial . && GOEXPERIMENT=simd /home/minje/golang/bin/go test -v -run=TestScanSpecial .`
Expected: PASS

**Step 4: Commit**
`git add scan_simd.go scan_fallback.go scan_test.go && git commit -m "feat(simd): transition to Go 1.27 portable simd package"`

---

### Task 2: Custom Encoding Support (EUC-KR, Shift_JIS, CP949)

**Files:**
- Create: `encoding.go`
- Create: `encoding_test.go`
- Modify: `option.go`
- Modify: `reader.go`
- Modify: `csv.go`

**Step 1: Write failing test in `encoding_test.go`**
Test `WithEncoding(korean.EUCKR)` and `WithCharset("euc-kr")` with EUC-KR encoded bytes.
Test unmarshaling and streaming decoding into UTF-8 struct strings.

**Step 2: Run test to verify it fails**
Run: `/home/minje/golang/bin/go test -v -run=TestEncoding .`
Expected: FAIL (WithEncoding undefined or not working)

**Step 3: Implement custom encoding support**
In `encoding.go`: define `WithEncoding` and `WithCharset`.
In `NewReader`: wrap `io.Reader` with `transform.NewReader(r, enc.NewDecoder())` when encoding is set.
In `Unmarshal`: handle encoded data slices.

**Step 4: Run test to verify it passes**
Run: `/home/minje/golang/bin/go test -v -run=TestEncoding .`
Expected: PASS

**Step 5: Commit**
`git add encoding.go encoding_test.go option.go reader.go csv.go go.mod go.sum && git commit -m "feat(encoding): add custom encoding support for EUC-KR, CP949, and Shift_JIS"`

---

### Task 3: AGENTS.md English Policy & Environment Note

**Files:**
- Modify: `AGENTS.md`

**Step 1: Update `AGENTS.md`**
Add mandatory rule:
`> **Language Policy**: All code, comments, commit messages, and documentation must be written in English by default.`
Document Go 1.27 default PATH and portable SIMD flags.

**Step 2: Commit**
`git add AGENTS.md && git commit -m "docs(agents): mandate English language policy for all code and comments"`

---

### Task 4: Comprehensive README.md Creation

**Files:**
- Create: `README.md`

**Step 1: Write `README.md` in English**
Cover:
- Overview & Highlights (Zero-Alloc Lexer, Portable SIMD, Memory Safety, Custom Delimiters & Quotes, Custom Encodings).
- Installation & Requirements (Go 1.27+).
- Quickstart & Examples (Reader, Decoder, Unmarshal, RecordUnmarshaler Generic API, ParallelUnmarshal).
- Custom Options (`WithDelimiter("||")`, `WithQuote('\'')`, `WithCharset("euc-kr")`).
- Performance Benchmarks table comparing with `encoding/csv` and `csvutil`.

**Step 2: Commit**
`git add README.md && git commit -m "docs: create comprehensive English README"`

---

### Task 5: End-to-End Verification, Tag v0.3.0 & Push

**Files:**
- Verification across test suite, race detector, benchmarks

**Step 1: Run full standard and SIMD test suites**
Run: `/home/minje/golang/bin/go test -count=1 ./... && GOEXPERIMENT=simd /home/minje/golang/bin/go test -count=1 ./...`
Expected: ALL PASS

**Step 2: Run race detector**
Run: `/home/minje/golang/bin/go test -race ./... && GOEXPERIMENT=simd /home/minje/golang/bin/go test -race ./...`
Expected: Clean (0 race)

**Step 3: Create git tag and push**
Create tag `v0.3.0` and push commits & tag to `origin`.
