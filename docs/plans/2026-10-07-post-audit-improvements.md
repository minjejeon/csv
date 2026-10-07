# Post-Audit Quality & Feature Improvements Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Execute this plan in single-flow mode.

**Goal:** Implement all 5 identified improvements across `record.go`, `generic.go`, `setter.go`, `getter.go`, and `scan_fallback.go`:
1. `Record.Field*` out of range bounds check and error return (`ErrFieldIndex`).
2. `MarshalSlice` and `ParallelMarshalSlice` header row emission for empty slices with `HeaderProvider`.
3. Struct field `[]byte` type support for both `Unmarshal` and `Marshal`.
4. `Record.Strings() []string` convenience method.
5. SWAR / bounds-check optimization in `scan_fallback.go` for `scanBlock32`.

---

### Task 1: `Record.Field*` Bounds Checking & `Record.Strings()` Helper
**Files:**
- Modify: `record.go`
- Modify: `reader.go` (use `rec.Strings()`)
- Test: `record_test.go` (or `audit_test.go`)

**Step 1: Write failing tests**
- Test `rec.FieldInt(-1)`, `rec.FieldInt(99)`, `rec.FieldBool(99)`, `rec.FieldFloat64(99)` return `ErrFieldIndex`.
- Test `rec.FieldInt(1)` on empty field `"1,\n"` still returns `0, nil`.
- Test `rec.Strings()` returns `[]string{"a", "b", "c"}` for a 3-field record, and `[]string{}` for empty record.

**Step 2: Implement minimal code**
- Define `var ErrFieldIndex = errors.New("csv: field index out of range")`.
- In `FieldInt`, `FieldInt64`, `FieldUint`, `FieldUint64`, `FieldBool`, `FieldFloat64`, `FieldFloat32`:
  Check `if rec == nil || i < 0 || i >= len(rec.spans) { return 0/false, ErrFieldIndex }`.
- In `record.go`, add `func (rec *Record) Strings() []string`.
- In `reader.go`, update `Read()` to call `rec.Strings()`.

**Step 3: Run tests to verify**
- `go test -v -run "TestRecordField"`

---

### Task 2: Generic `MarshalSlice` Empty Slice Header Emission
**Files:**
- Modify: `generic.go`
- Test: `generic_marshal_test.go`

**Step 1: Write failing tests**
- In `generic_marshal_test.go`, test `MarshalSlice[TestGenericItem, *TestGenericItem](nil)` and `[]TestGenericItem{}` return `"id,name\n"`.
- Test `ParallelMarshalSlice` with empty slice returns `"id,name\n"`.

**Step 2: Implement minimal code**
- In `generic.go`: In `MarshalSlice` and `ParallelMarshalSlice`, check if `n == 0`. If `HeaderProvider` is implemented, write header row and return bytes.

**Step 3: Run tests to verify**
- `go test -v -run "TestGenericMarshal"`

---

### Task 3: Struct Field `[]byte` Support in Setter, Getter & Plan
**Files:**
- Modify: `setter.go`
- Modify: `getter.go`
- Modify: `plan.go`
- Test: `setter_test.go` / `marshal_test.go` / `decoder_test.go`

**Step 1: Write failing tests**
- Struct with `[]byte` field `Data []byte` can be decoded from CSV and encoded to CSV round-trip.
- Struct with `*[]byte` pointer field works.
- `omitempty` on `[]byte` produces empty CSV field when nil/empty.

**Step 2: Implement minimal code**
- In `setter.go`: add `case reflect.Slice:` for `elem.Kind() == reflect.Uint8` using `bytes.Clone(raw)`.
- In `getter.go`: add `case reflect.Slice:` for `elem.Kind() == reflect.Uint8` using `w.WriteFieldBytes(b)`.
- In `plan.go`: in `compileZeroer`, add `case reflect.Slice:` setting to `nil`.

**Step 3: Run tests to verify**
- `go test -v -run "TestByteSliceField"`

---

### Task 4: Fallback `scanBlock32` Bounds-Elimination & SWAR Skip
**Files:**
- Modify: `scan_fallback.go`
- Test: `scan_test.go`

**Step 1: Benchmark baseline**
- Benchmark `readRecordFastNoQuote` or fallback scanner.

**Step 2: Implement optimized `scanBlock32`**
- In `scan_fallback.go`: Eliminate slice bounds checking with `_ = chunk[31]`.
- Implement fast 8-byte ASCII-skip checking using 64-bit word equality or SWAR masks.

**Step 3: Run tests and benchmarks**
- `go test -v -run TestScan`
- Verify against all tests.

---

### Task 5: Full Verification Across Standard & SIMD, Race Detector, Commit & Push
- Run `go test -v ./...`
- Run `GOEXPERIMENT=simd go test -v ./...`
- Run `go test -race ./...`
- Run `GOEXPERIMENT=simd go test -race ./...`
- Update `docs/plans/task.md`.
