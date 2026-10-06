# Design Document: Multithreaded Parallel CSV Reader & Unmarshaler

- **Module**: `github.com/minjejeon/csv`
- **Date**: 2026-10-06
- **Status**: Approved

---

## 1. Objectives

1. **Multi-Core Linear Scaling**: Utilize all CPU cores (`runtime.GOMAXPROCS(0)`) to achieve gigabyte-per-second parallel CSV parsing and struct decoding throughput.
2. **Accurate RFC 4180 Chunk Boundary Resolution**: Rapidly identify genuine row boundaries across chunk splits without being deceived by newlines inside quoted fields (`"...\n..."`).
3. **Deterministic Row Ordering**: Guarantee exact preservation of row order by default (`Ordered: true`), with an optional unordered fast path for bulk insertions.
4. **Hybrid Source Support**: Direct memory chunking for `[]byte` and file inputs, plus worker-pool pipelining for general streaming `io.Reader`.

---

## 2. Architecture & Components

```
                      +---------------------------------------+
                      |         ParallelUnmarshal(data)       |
                      +---------------------------------------+
                                          |
                                          v
                      +---------------------------------------+
                      |             ChunkSplitter             |
                      |  (SIMD quote parity, boundary align)  |
                      +---------------------------------------+
                                          |
                     +--------------------+--------------------+
                     |                    |                    |
                     v                    v                    v
             +---------------+    +---------------+    +---------------+
             | Worker 0      |    | Worker 1      |    | Worker N-1    |
             | [0, chunk1)   |    | [chunk1, c2)  |    | [cN-1, EOF)   |
             | (reads header)|    | (skips header)|    | (skips header)|
             +---------------+    +---------------+    +---------------+
                     |                    |                    |
                     +--------------------+--------------------+
                                          |
                                          v
                      +---------------------------------------+
                      |       Ordered Result Aggregator       |
                      |   (Concatenates chunk slice outputs)  |
                      +---------------------------------------+
```

---

## 3. Chunk Splitting & Boundary Alignment

### 3.1 Target Split Points
For data of length $L$ and $W$ workers:
- Each target split point $T_k \approx k \times (L / W)$.
- $T_0 = 0$, $T_W = L$.

### 3.2 Quote Parity Boundary Alignment
At target split point $T_k$ ($k > 0$):
1. Locate nearest preceding and following newline (`\n`) characters.
2. Verify quote parity:
   - A newline is a valid record boundary if and only if the total count of unescaped quotes from the start of the record (or previous verified boundary) to that newline is even.
   - Using the SIMD scanner, quotes across the boundary neighborhood are counted in nanoseconds.
3. Establish exact boundary offset $B_k$.
4. Chunk $k$ spans `data[B_k : B_{k+1}]`.

---

## 4. Parallel Worker Execution & Aggregation

### 4.1 Header Distribution & Plan Sharing
- Worker 0 parses the first line to extract CSV headers.
- It compiles `*typePlan` and shares it across all workers (read-only thread-safe access).
- Workers $1 \dots W-1$ start decoding immediately from their first complete data row.

### 4.2 Ordered vs Unordered Output
- **Ordered (default)**:
  - Each worker decodes its rows into a local slice chunk `chunks[k]`.
  - Upon completion (`errgroup.Wait()`), calculate total row count $\sum \text{len}(chunks[k])$.
  - Pre-allocate destination slice once and copy or append sequentially.
- **Unordered**:
  - Workers emit parsed record batches to an output channel as soon as ready.

---

## 5. Public API

```go
package csv

// ParallelOptions configures parallel processing.
type ParallelOptions struct {
    Workers int  // Number of worker goroutines (defaults to runtime.GOMAXPROCS(0))
    Ordered bool // Preserve original CSV row order (default: true)
}

// ParallelUnmarshal parses CSV bytes into a struct slice across multiple goroutines.
func ParallelUnmarshal(data []byte, v any, opts ...ParallelOptions) error

// ParallelReader provides parallel record streaming over io.Reader.
type ParallelReader struct { ... }
func NewParallelReader(r io.Reader, opts ...ParallelOptions) (*ParallelReader, error)
```

---

## 6. Testing & Benchmarking Strategy

1. **Boundary Edge Case Tests**:
   - Chunks splitting exactly inside multi-line quoted fields.
   - Chunks splitting across CRLF (`\r\n` where `\r` is in chunk $k$ and `\n` is in chunk $k+1$).
   - CSV with escaped quotes (`""`).
   - Small files (< 4KB) with many workers.
   - Single row CSVs.
2. **Equivalence Tests**:
   - `ParallelUnmarshal` result must be 100% `reflect.DeepEqual` with sequential `Unmarshal`.
3. **Benchmarks**:
   - Measure scalability (1, 2, 4, 8, 16 workers) on 100,000+ rows.
