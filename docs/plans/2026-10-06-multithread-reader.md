# Multithreaded Parallel CSV Reader Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Implement a high-performance multithreaded CSV parser and unmarshaler (`ParallelUnmarshal`, `ParallelReader`) utilizing SIMD boundary scanning and multi-core worker parallelism with strict row ordering.

**Architecture:** (1) `ChunkSplitter` partitions byte buffers into $N$ safe chunks using SIMD quote parity to avoid splitting inside multiline quoted fields; (2) Worker pool parses chunks concurrently with independent zero-alloc reader instances; (3) Ordered result aggregator combines chunks into final slices with 1 allocation; (4) Streaming `ParallelReader` pipeline for `io.Reader`.

**Tech Stack:** Go 1.27 (`simd/archsimd`), `sync.WaitGroup`, `golang.org/x/sync/errgroup` or standard concurrency primitives, `unsafe.Pointer`.

---

### Task 1: Chunk Splitter & Quote-Aware Boundary Finder

**Files:**
- Create: `chunk.go`
- Test: `chunk_test.go`

**Step 1: Write the failing test**
Create `chunk_test.go` testing `splitChunks(data []byte, numWorkers int) ([]chunkSpan, error)`:
- Evenly splits basic CSV.
- Correctly aligns split when target falls inside a field.
- Correctly avoids splitting across a newline within a multiline quoted field (`"line1\nline2"`).
- Handles CRLF and trailing commas.
- Edge cases: data size smaller than worker count, single row, empty data.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestSplitChunks ./...`
Expected: FAIL with undefined `splitChunks`.

**Step 3: Write minimal implementation**
Implement `chunk.go`:
- `type chunkSpan struct { start, end int }`
- `splitChunks(data []byte, numWorkers int) ([]chunkSpan, error)`
- SIMD quote parity scanner to find safe row boundary.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestSplitChunks ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestSplitChunks ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add chunk.go chunk_test.go
git commit -m "feat(chunk): implement quote-aware chunk splitter"
```

---

### Task 2: Parallel Unmarshaler (`ParallelUnmarshal`)

**Files:**
- Create: `parallel.go`
- Test: `parallel_test.go`

**Step 1: Write the failing test**
Create `parallel_test.go` verifying:
- `ParallelUnmarshal` produces identical output to sequential `Unmarshal` across large datasets (10,000+ rows).
- Supports slices of structs and slices of struct pointers.
- Correct row ordering preserved.
- Worker error propagation.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestParallelUnmarshal ./...`
Expected: FAIL with undefined `ParallelUnmarshal`.

**Step 3: Write minimal implementation**
Implement `parallel.go`:
- `type ParallelOptions struct { Workers int; Ordered bool }`
- `ParallelUnmarshal(data []byte, v any, opts ...ParallelOptions) error`
- Worker goroutine loop parsing individual chunk spans and concatenating results in order.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestParallelUnmarshal ./...`
Run: `GOEXPERIMENT=simd go test -v -run TestParallelUnmarshal ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add parallel.go parallel_test.go
git commit -m "feat(parallel): implement ParallelUnmarshal with multi-core worker execution"
```

---

### Task 3: Streaming `ParallelReader` Pipeline

**Files:**
- Create: `parallel_reader.go`
- Test: `parallel_reader_test.go`

**Step 1: Write the failing test**
Create `parallel_reader_test.go` testing `NewParallelReader` streaming records from an `io.Reader` using worker pool batches. Verify record count, data integrity, and EOF handling.

**Step 2: Run test to verify it fails**
Run: `go test -v -run TestParallelReader ./...`
Expected: FAIL with undefined `NewParallelReader`.

**Step 3: Write minimal implementation**
Implement `parallel_reader.go`:
- `ParallelReader` with worker pool and batch channels.
- Batch producer and worker pipeline.

**Step 4: Run test to verify it passes**
Run: `go test -v -run TestParallelReader ./...`
Expected: PASS.

**Step 5: Commit**
```bash
git add parallel_reader.go parallel_reader_test.go
git commit -m "feat(stream): implement streaming ParallelReader pipeline"
```

---

### Task 4: Parallel Scalability Benchmarks

**Files:**
- Create: `parallel_bench_test.go`

**Step 1: Write scalability benchmarks**
Benchmark 100,000 rows comparing:
- Sequential `Unmarshal`
- `ParallelUnmarshal` with 2, 4, 8, 16 workers

**Step 2: Run benchmarks**
Run: `go test -bench=BenchmarkParallel -benchmem ./...`
Run: `GOEXPERIMENT=simd go test -bench=BenchmarkParallel -benchmem ./...`
Verify speedup scaling with core count.

**Step 3: Commit**
```bash
git add parallel_bench_test.go
git commit -m "test(bench): add multi-core scalability benchmarks"
```
