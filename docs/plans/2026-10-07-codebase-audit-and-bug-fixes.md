# Codebase Audit and Bug Fixes Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Fix all bugs and hardening issues discovered during comprehensive codebase audit across Writer, Reader, Decoder, Plan, Pool, and Generics.

**Architecture:** Address identified defects systematically with TDD: write reproducing unit tests first, verify failure, implement surgical fixes without breaking existing performance or zero-allocation contracts, and verify against standard and SIMD test suites with race detector.

**Tech Stack:** Go 1.27, unsafe, sync.Pool, encoding/csv compatibility, SIMD/SWAR.

---

### Task 1: Writer Delimiter/Quote Mutation, Quoting Precision, and Error Tracking
**Files:**
- Modify: `writer.go`
- Test: `writer_test.go`

**Step 1: Write the failing test**
In `writer_test.go`:
- Test mutating `w.Comma = ';'` after `NewWriter` produces semicolon-delimited output.
- Test mutating `w.Quote = '\''` after `NewWriter` produces custom-quoted output.
- Test that with multi-character delimiter `"||"`, a field with single pipe `"single|pipe"` is NOT quoted.
- Test `w.Error()` returns the first error on flush/write failure.

**Step 2: Run test to verify it fails**
Run: `go test -v -run "TestWriterMutation|TestMultiCharDelimWriterQuoting"`
Expected: FAIL.

**Step 3: Implement minimal code to make tests pass**
In `writer.go`:
- Add `err error` to `Writer`.
- Implement `checkDelimAndQuote()` to detect when `w.Comma`, `w.Delimiter`, or `w.Quote` changed, updating `w.delimBytes`, `w.quoteByte`, and `w.scanner`.
- In `fieldNeedsQuotes`, if `len(w.delimBytes) > 1`, only quote if `bytes.Contains(field, w.delimBytes)` or field contains `w.quoteByte`, `\r`, or `\n`.
- Track errors in `w.err` and implement `func (w *Writer) Error() error`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run "TestWriterMutation|TestMultiCharDelimWriterQuoting"`
Expected: PASS.

---

### Task 2: Record Counting with Quoted Fields at EOF
**Files:**
- Modify: `csv.go:244-297`
- Test: `reader_test.go`

**Step 1: Write the failing test**
In `reader_test.go`:
- Add `TestCountRecordsQuotedEOF` verifying `countRecords([]byte("\"a\""), '"') == 1`, `countRecords([]byte("id,name\n1,\"Alice\""), '"') == 2`, and multiline quoted cases.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestCountRecordsQuotedEOF`
Expected: FAIL (returns 0 instead of 1).

**Step 3: Implement minimal code to make test pass**
In `csv.go`:
- In `countRecords`, ensure that if `s` does not end with `\n`, the final record is counted: `if s[len(s)-1] != '\n' { n++ }` for both fast path and quoted path.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestCountRecordsQuotedEOF`
Expected: PASS.

---

### Task 3: Struct Decoder Reused Struct Clearing & Duplicate Header Mapping
**Files:**
- Modify: `plan.go`
- Modify: `decoder.go`
- Modify: `getter.go`
- Test: `decoder_test.go`
- Test: `unique_test.go`

**Step 1: Write failing tests**
- `TestDecoderMissingColReuse`: when decoding into a reused struct instance, columns missing from a short record must be reset to zero value.
- `TestDuplicateHeaders`: CSV with duplicate headers `id,id` correctly maps to multiple struct fields tagged with `csv:"id"`.
- `TestUniqueBoolOmitEmpty`: `unique.Handle[bool]` with `omitempty` tag produces empty field when false.

**Step 2: Run tests to verify they fail**
Run: `go test -v -run "TestDecoderMissingColReuse|TestDuplicateHeaders|TestUniqueBoolOmitEmpty"`
Expected: FAIL.

**Step 3: Implement minimal code to make tests pass**
- In `plan.go`: Add `zeroer func(structPtr unsafe.Pointer)` to `fieldPlan`. Implement `compileZeroer(t reflect.Type, offset uintptr)` for primitive and pointer types.
- In `plan.go`: In `buildTypePlan`, check `!usedCols[i]` inside the column matching loop.
- In `decoder.go`: In `Decode`, when `f.colIndex >= numFields`, call `f.zeroer(structPtr)`.
- In `getter.go`: For `unique.Handle[bool]`, check `if !h.Value() && omitEmpty { return nil }`.

**Step 4: Run tests to verify they pass**
Run: `go test -v -run "TestDecoderMissingColReuse|TestDuplicateHeaders|TestUniqueBoolOmitEmpty"`
Expected: PASS.

---

### Task 4: Reader DuckDB Fast-Path Quote Optimization & Pool/API Hardening
**Files:**
- Modify: `reader.go`
- Modify: `pool.go`
- Modify: `generic.go`
- Modify: `record.go`
- Test: `reader_test.go`
- Test: `record.go` (tests in `audit_test.go`)

**Step 1: Write tests for fast-path quote boundary and nil-safety**
- Test reader with unquoted record followed by quoted record in same 32-byte chunk.
- Test `Record.NumFields()`, `Record.Line()`, `Record.Field()` on nil `*Record`.
- Test `UnmarshalTo` and `ParallelUnmarshalTo` with nil `out` returns clean error.
- Test `Record.FieldBool` on empty field returns `false, nil`.

**Step 2: Run tests to verify failure/behavior**
Run: `go test -v -run TestReaderFastPathQuote`
Expected: Verify behavior.

**Step 3: Implement minimal code**
- In `reader.go`: In `readRecordFastNoQuote`, when `mEOL != 0`, check quotes only before `eolOffset`: `if mQuote&((uint32(1)<<eolOffset)-1) != 0 { abort }`.
- In `pool.go`: In `releaseWriteBuf` and `releaseFieldBufHolder`, discard buffers larger than 1MB (`1024*1024`).
- In `generic.go`: Check `if out == nil { return errors.New("csv: out pointer must not be nil") }` in `UnmarshalTo` and `ParallelUnmarshalTo`.
- In `record.go`: Add nil-checks to `NumFields()`, `Line()`, `Field()`, `RawField()`. In `FieldBool`, return `false, nil` for empty byte slice.

**Step 4: Run tests to verify they pass**
Run: `go test -v ./...`
Expected: PASS.

---

### Task 5: End-to-End Verification, Race Detector, and Fuzz Testing
**Files:**
- Test: `fuzz_test.go`
- Run all test suites across standard and SIMD (`GOEXPERIMENT=simd`), with `-race`.
- Run fuzz tests: `FuzzReader`, `FuzzDecoder`, `FuzzStdlibEquivalence`.
- Update `docs/plans/task.md`.
