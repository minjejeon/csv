# High-Performance CSV Serialization Pipeline Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Implement zero-allocation, SIMD-accelerated CSV serialization (`Writer`), struct marshaling (`Encoder` / `Marshal`), multithreaded chunked encoding (`ParallelMarshal`), and generic zero-reflection serialization (`MarshalSlice[T]`).

**Architecture:** Use pooled 64KB write buffers and SIMD-accelerated character scanning (`blockScanner.scanSpecial`) to eliminate quoting overhead on tabular data. For struct serialization, precompile reflection metadata into direct `unsafe.Pointer` offset getters that append formatted primitives in-place into the write buffer. Multithreaded chunking enables parallel core scaling.

**Tech Stack:** Go 1.27 (`GOEXPERIMENT=simd`, package `simd`, package `simd/archsimd`), 64-bit SWAR, `sync.Pool`, `unsafe.Pointer`.

---

### Task 1: Streaming Zero-Alloc `Writer` with SIMD Quote Checking

**Files:**
- Create: `writer.go`
- Modify: `pool.go`
- Create: `writer_test.go`

**Step 1: Write the failing test**
In `writer_test.go`:
```go
package csv

import (
    "bytes"
    "testing"
)

func TestWriterBasic(t *testing.T) {
    var buf bytes.Buffer
    w := NewWriter(&buf)
    err := w.Write([]string{"name", "age", "city"})
    if err != nil {
        t.Fatal(err)
    }
    err = w.Write([]string{"Alice", "30", "New York, NY"})
    if err != nil {
        t.Fatal(err)
    }
    if err := w.Flush(); err != nil {
        t.Fatal(err)
    }
    expected := "name,age,city\nAlice,30,\"New York, NY\"\n"
    if buf.String() != expected {
        t.Fatalf("expected %q, got %q", expected, buf.String())
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestWriterBasic .`
Expected: FAIL ("NewWriter not defined")

**Step 3: Write minimal implementation**
- In `pool.go`, add `writeBufPool` pooling 64KB byte slices.
- In `writer.go`, implement `Writer` supporting `Comma`, `Delimiter`, `Quote`, `UseCRLF`.
- Use `w.scanner.scanSpecial(fieldBytes)` to detect whether quotes and escaping are needed.
- Implement `Write([]string)`, `WriteField([]byte)`, `Flush()`, and `Close()`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestWriterBasic .`
Run: `GOEXPERIMENT=simd go test -v -run TestWriterBasic .`
Expected: PASS

**Step 5: Commit**
```bash
git add pool.go writer.go writer_test.go
git commit -m "feat(writer): implement zero-alloc streaming Writer with SIMD quote detection"
```

---

### Task 2: Struct `Encoder` & `Marshal` with Direct Unsafe Getters

**Files:**
- Create: `getter.go`
- Create: `encoder.go`
- Modify: `csv.go`
- Create: `encoder_test.go`
- Create: `marshal_test.go`

**Step 1: Write the failing test**
In `marshal_test.go`:
```go
package csv

import (
    "testing"
)

type TestUser struct {
    ID     int64   `csv:"id"`
    Name   string  `csv:"name"`
    Salary float64 `csv:"salary"`
    Active bool    `csv:"active"`
}

func TestMarshalBasic(t *testing.T) {
    users := []TestUser{
        {ID: 1, Name: "Alice", Salary: 50000.5, Active: true},
        {ID: 2, Name: "Bob, Jr.", Salary: 60000.0, Active: false},
    }
    data, err := Marshal(users)
    if err != nil {
        t.Fatal(err)
    }
    expected := "id,name,salary,active\n1,Alice,50000.5,true\n2,\"Bob, Jr.\",60000,false\n"
    if string(data) != expected {
        t.Fatalf("expected %q, got %q", expected, string(data))
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestMarshalBasic .`
Expected: FAIL ("Marshal not defined")

**Step 3: Write minimal implementation**
- In `getter.go`, implement `typeMarshalPlan` compiling direct `unsafe.Pointer` getters for `int*`, `uint*`, `float*`, `bool`, `string`.
- Format primitives directly into `w.buf` in-place (`strconv.AppendInt`, `strconv.AppendFloat`, boolean literals).
- In `encoder.go`, implement `Encoder` writing header and struct rows.
- In `csv.go`, implement `Marshal(v any, opts ...Option) ([]byte, error)`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestMarshalBasic .`
Run: `GOEXPERIMENT=simd go test -v -run TestMarshalBasic .`
Expected: PASS

**Step 5: Commit**
```bash
git add getter.go encoder.go csv.go encoder_test.go marshal_test.go
git commit -m "feat(marshal): implement high-performance struct Marshal and Encoder with unsafe getters"
```

---

### Task 3: Multithreaded `ParallelMarshal` & Generic `RecordMarshaler` API

**Files:**
- Modify: `parallel.go`
- Modify: `generic.go`
- Create: `parallel_marshal_test.go`
- Create: `generic_marshal_test.go`

**Step 1: Write the failing test**
In `parallel_marshal_test.go`:
```go
func TestParallelMarshalEquivalence(t *testing.T) {
    users := make([]TestUser, 1000)
    for i := range users {
        users[i] = TestUser{ID: int64(i), Name: "User", Salary: float64(i) * 1.5, Active: i%2 == 0}
    }
    seqData, err := Marshal(users)
    if err != nil {
        t.Fatal(err)
    }
    parData, err := ParallelMarshal(users, ParallelOptions{Workers: 4})
    if err != nil {
        t.Fatal(err)
    }
    if string(seqData) != string(parData) {
        t.Fatal("ParallelMarshal output does not match sequential Marshal")
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestParallelMarshalEquivalence .`
Expected: FAIL ("ParallelMarshal not defined")

**Step 3: Write minimal implementation**
- In `parallel.go`, implement `ParallelMarshal(v any, opts ...any) ([]byte, error)`.
- Split slice into chunks; worker 0 writes header; workers write chunk data into pooled buffers; merge into final slice.
- In `generic.go`, define `RecordMarshaler` interface and implement `MarshalSlice[T]`, `ParallelMarshalSlice[T]`.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestParallelMarshalEquivalence .`
Run: `GOEXPERIMENT=simd go test -v -run TestParallelMarshalEquivalence .`
Expected: PASS

**Step 5: Commit**
```bash
git add parallel.go generic.go parallel_marshal_test.go generic_marshal_test.go
git commit -m "feat(parallel): implement ParallelMarshal and generic RecordMarshaler API"
```

---

### Task 4: Round-Trip, Regression & Benchmark Verification

**Files:**
- Create: `marshal_bench_test.go`
- Modify: `docs/plans/task.md`

**Step 1: Write round-trip and benchmark tests**
- In `marshal_bench_test.go`, compare:
  - `BenchmarkWriter_Stdlib` vs `BenchmarkWriter_Ours`
  - `BenchmarkMarshal_Csvutil` vs `BenchmarkMarshal_Ours`
  - `BenchmarkParallelMarshal_16Workers`
- Add round-trip tests: `Unmarshal(Marshal(data)) == data`.

**Step 2: Run all verifications**
Run:
`go test -v ./...`
`GOEXPERIMENT=simd go test -v ./...`
`go test -race ./...`
`GOEXPERIMENT=simd go test -race ./...`
`go test -bench=Benchmark(Writer|Marshal) -benchmem -benchtime=500ms .`

**Step 3: Update documentation and task tracker**
Update `docs/plans/task.md` and commit.
```bash
git add docs/plans/task.md marshal_bench_test.go
git commit -m "docs: complete high-performance serialization implementation tasks"
```
