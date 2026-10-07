# csv

[![Go Reference](https://pkg.go.dev/badge/github.com/minjejeon/csv.svg)](https://pkg.go.dev/github.com/minjejeon/csv)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A high-performance, zero-allocation CSV reader and struct unmarshaler for Go.

It combines **Go 1.27 Portable SIMD** hardware acceleration, **zero-copy sliding buffers**, **pre-compiled struct mapping**, and **multithreaded parsing** to deliver blazing fast throughput with minimal memory footprint.

---

## Highlights

- **⚡ Zero-Allocation Lexer**: Achieve `0 B/op` and `0 allocs/op` during row-by-row streaming reading with reused buffer pools.
- **🚀 Go 1.27 Portable SIMD**: Accelerated delimiter, quote, and newline scanning via the standard `simd` package (AVX2/AVX-512 on x86_64, Neon on ARM64) reaching up to **38+ GB/s** raw scanner throughput, with a pure Go SWAR fallback.
- **🧩 Custom & Multi-Character Delimiters**: Seamlessly parse single-character (`|`, `;`, `\t`) and multi-character delimiters (`||`, `::`, `###`).
- **🔤 Custom Quote Characters**: Full RFC 4180 quote escaping support for custom quote marks (e.g. single quote `'` with `''` escaping).
- **🌐 Legacy Character Encodings**: Built-in support for decoding legacy character sets like **EUC-KR**, **CP949**, **Shift_JIS**, **GBK**, **ISO-8859-1**, and **Windows-1252** into clean UTF-8 Go strings.
- **🏷️ Struct Tag Mapping (`csvutil`-compatible)**: Map CSV headers directly to Go struct fields (`csv:"column_name,omitempty"`), pre-compiling reflection metadata into direct pointer offset writes.
- **⚡ Generic Zero-Reflection Unmarshaler**: Implement `RecordUnmarshaler` to unmarshal CSV slices with zero reflection and zero interface boxing (`UnmarshalSlice[T]`).
- **🧵 Multithreaded Parallel Engine**: Split datasets along clean quote-safe record boundaries for parallel parsing (`ParallelUnmarshal`, `ParallelReader`), achieving **4x+ speedups** over conventional libraries.

---

## Installation

```bash
go get github.com/minjejeon/csv
```

*Requires Go 1.25+ (Go 1.27+ recommended for hardware SIMD vector intrinsics).*

---

## Quickstart

### 1. High-Performance Streaming Reader (Zero-Alloc)

```go
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/minjejeon/csv"
)

func main() {
	data := "id,name,role\n1,Alice,Engineer\n2,Bob,Designer\n"
	r := csv.NewReader(strings.NewReader(data))
	defer r.Close()

	for {
		rec, err := r.ReadRecord()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		// rec.Field() returns zero-copy byte slices referencing the internal buffer
		fmt.Printf("Row: %s, %s, %s\n", rec.Field(0), rec.Field(1), rec.Field(2))
	}
}
```

### 2. Struct Unmarshaling (`Unmarshal`)

```go
package main

import (
	"fmt"

	"github.com/minjejeon/csv"
)

type User struct {
	ID    int    `csv:"id"`
	Name  string `csv:"name"`
	Email string `csv:"email,omitempty"`
}

func main() {
	data := []byte("id,name,email\n1,Alice,alice@example.com\n2,Bob,\n")

	var users []User
	if err := csv.Unmarshal(data, &users); err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", users)
}
```

### 3. Custom Delimiters & Quotes (Functional Options)

```go
package main

import (
	"fmt"

	"github.com/minjejeon/csv"
)

type LogEntry struct {
	Timestamp string `csv:"timestamp"`
	Level     string `csv:"level"`
	Message   string `csv:"message"`
}

func main() {
	// Multi-character delimiter "||" and single quote "'"
	data := []byte("timestamp||level||message\n2026-10-06||'INFO'||'System || Ready'\n")

	var logs []LogEntry
	err := csv.Unmarshal(data, &logs,
		csv.WithDelimiter("||"),
		csv.WithQuote('\''),
	)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", logs)
}
```

### 4. Legacy Character Encodings (EUC-KR, Shift_JIS, CP949)

Legacy CSV exports in East Asian or Western European encodings are decoded into UTF-8 automatically:

```go
package main

import (
	"fmt"

	"github.com/minjejeon/csv"
)

type Employee struct {
	ID   int    `csv:"id"`
	Name string `csv:"name"`
	Dept string `csv:"dept"`
}

func main() {
	// EUC-KR or CP949 encoded data
	euckrBytes := []byte{ ... }

	var emps []Employee
	err := csv.Unmarshal(euckrBytes, &emps, csv.WithCharset("euc-kr"))
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", emps)
}
```

You can also pass standard `encoding.Encoding` types directly:
```go
import "golang.org/x/text/encoding/korean"

csv.Unmarshal(data, &emps, csv.WithEncoding(korean.EUCKR))
```

### 5. Compile-Time Zero-Reflection Generic Unmarshaler

Implement `RecordUnmarshaler` on your type for maximum unmarshaling performance without reflection:

```go
type FastRecord struct {
	ID   int
	Name string
}

func (r *FastRecord) UnmarshalCSVRecord(rec *csv.Record) error {
	r.ID = ...
	r.Name = string(rec.Field(1))
	return nil
}

// Unmarshal directly with static allocation:
records, err := csv.UnmarshalSlice[FastRecord](data)
```

### 7. High-Performance Struct Marshaling & Streaming Writer

Serialize Go structs to CSV bytes with zero per-field allocations:

```go
// Bulk marshaling with pre-compiled unsafe getters:
bytes, err := csv.Marshal(users)

// Multithreaded chunked marshaling (scales to 900+ MB/s across CPU cores):
bytes, err := csv.ParallelMarshal(users, csv.ParallelOptions{Workers: 16})

// Streaming struct encoder:
enc := csv.NewEncoder(os.Stdout)
for _, u := range users {
    if err := enc.Encode(u); err != nil {
        panic(err)
    }
}
enc.Flush()
```

### 8. Memory Optimization with Go `unique` Package (`unique.Handle[T]` & Interning)

In massive CSV datasets, repetitive values (such as `status`, `country_code`, `category`, `currency`) often cause millions of duplicate heap allocations.
`csv` integrates seamlessly with Go 1.23+'s standard library `unique` package to deduplicate memory and dramatically reduce GC pressure:

#### A. Automatic String Interning (`csv:"col,unique"` or `csv:"col,intern"`)
Tagging a `string` field with `unique` (or `intern`) automatically canonicalizes strings via `unique.Make(s).Value()`. All rows sharing the same value point to the exact same underlying byte slice (`unsafe.StringData(a) == unsafe.StringData(b)`):

```go
type Order struct {
    ID       int64  `csv:"id"`
    Status   string `csv:"status,unique"`   // Interned: shares canonical memory
    Category string `csv:"category,intern"` // Same as unique
    Region   string `csv:"region,unique"`   // Interned
    Notes    string `csv:"notes"`           // Plain: normal allocation
}
```

#### B. Direct `unique.Handle[T]` Struct Fields
For maximum memory efficiency and $O(1)$ pointer comparison, struct fields can directly use `unique.Handle[T]`. `unique.Handle[string]` takes only 8 bytes (1 pointer word) per struct field compared to 16 bytes for standard `string`:

```go
import "unique"

type FastRecord struct {
    ID     unique.Handle[int]    `csv:"id"`
    Status unique.Handle[string] `csv:"status"` // 8-byte pointer, O(1) comparison
    Active unique.Handle[bool]   `csv:"active"`
}

var records []FastRecord
err := csv.Unmarshal(data, &records)

// O(1) pointer-level equality check without string comparison:
if records[0].Status == records[1].Status {
    fmt.Println("Exact same status handle!")
}
```

#### Benchmark: Memory Savings on Repetitive Data (10,000 Rows)
| Mode | Memory Allocated | Heap Allocations | Allocation Reduction | Memory Reduction |
|---|---|---|---|---|
| **Plain `string`** | 876 KB/op | 30,015 allocs/op | Baseline | Baseline |
| **`string` + `csv:",unique"`** | 567 KB/op | **14 allocs/op** | **-99.95%** | **-35.3%** |
| **`unique.Handle[string]`** | **329 KB/op** | **13 allocs/op** | **-99.96%** | **-62.5%** |

### 9. Reliability & Developer Experience (DX)

#### A. Rich Error Diagnostics (`DecodeError`)
When field conversion fails, `Decoder` and `Unmarshal` return a structured `*DecodeError` providing exact location and context:
```go
if err := dec.Decode(&user); err != nil {
    var decErr *csv.DecodeError
    if errors.As(err, &decErr) {
        // e.g. csv: line 142, column "age" (field Age): invalid syntax (value: "N/A")
        fmt.Printf("Line: %d, Column: %q, Field: %s, Bad Value: %q\n",
            decErr.Line, decErr.Header, decErr.Field, decErr.Value)
    }
}
```
Underlying errors are unwrapped for seamless `errors.Is(err, strconv.ErrSyntax)` checks.

#### B. Automatic Excel UTF-8 BOM Stripping (`WithTrimBOM`)
CSV files generated by Microsoft Excel on Windows often prepend a UTF-8 Byte Order Mark (`\xef\xbb\xbf`), which typically corrupts the first column header. `csv` enables automatic BOM detection and stripping by default:
```go
// Automatically strips \xef\xbb\xbf so header "id" matches User.ID
err := csv.Unmarshal(excelData, &users)
```
You can disable this behavior with `csv.WithTrimBOM(false)`.

#### C. Custom Date & Time Struct Tags (`format=...`)
Customize date and timestamp serialization directly in struct tags:
```go
type AuditLog struct {
    CreatedAt time.Time `csv:"created_at,format=2006-01-02 15:04:05"`
    Date      time.Time `csv:"event_date,format=2006/01/02"`
    UnixSec   time.Time `csv:"epoch_sec,format=unix"`       // Unix timestamp in seconds
    UnixMilli time.Time `csv:"epoch_milli,format=unixmilli"` // Unix timestamp in milliseconds
}
```

#### D. Zero-Copy Record Cloning (`Record.Clone()`)
For zero-copy streaming, `ReadRecord()` returns a pointer referencing internal sliding buffers. When a record needs to be stored across iterations or sent across goroutines, call `.Clone()` to obtain a standalone detached copy:
```go
rec, err := r.ReadRecord()
cloned := rec.Clone() // Safe to store or use after r.Close() or next ReadRecord()
```

#### E. Symmetric Multi-Charset Writer & Marshal
Write legacy encodings (EUC-KR, Shift_JIS, CP949, etc.) just as easily as reading them:
```go
// Direct struct marshal into EUC-KR bytes:
euckrBytes, err := csv.Marshal(koreanRecords, csv.WithCharset("euc-kr"))

// Streaming writer into Shift_JIS:
w := csv.NewWriter(file, csv.WithCharset("shift_jis"))
w.Write([]string{"名前", "役職"})
w.Close()
```

---

## Architectural Guide: Struct Tags vs Generic Zero-Reflection

Which code style should you choose?

| Criteria | Struct Tags (`csv:"..."`) | Generic (`RecordUnmarshaler` / `RecordMarshaler`) |
|---|---|---|
| **Code Style** | Idiomatic, declarative Go struct tags | Explicit `UnmarshalCSVRecord` / `MarshalCSVRecord` methods |
| **Unmarshaling Speed** | **204 MB/s** (Sequential), **500 MB/s** (Parallel) | **220 MB/s** (Sequential), **497 MB/s** (Parallel) |
| **Speed Difference** | Baseline (90~95% of generic speed) | **+7.6% faster** (Sequential), **+12.2% faster** (500k rows) |
| **Marshaling Speed** | **266 MB/s** (Sequential), **804 MB/s** (Parallel) | **244 MB/s** (Sequential), **679 MB/s** (Parallel) |
| **Marshaling Difference**| **+5~9% faster** (Direct stack scratch formatting) | Baseline |
| **Recommended For** | **95% of applications**: Clean, maintainable code with near-maximum performance | **Extreme latency-critical pipelines** (e.g. market data, massive 10GB+ CSV ingest) |

> **Recommendation**: Start with standard **Struct Tags**. Thanks to `csv`'s precompiled `unsafe.Pointer` offset engine, reflection overhead is completely eliminated from the hot parsing loop. Only implement `RecordUnmarshaler` when profiling indicates that saving the remaining 7~12% CPU time is critical for your workload.

---

## Performance Benchmarks

Benchmarks measured on **AMD Ryzen 7 7735HS (16 vCPUs)** under Go 1.27.

### SWAR vs Portable SIMD Comparison (500,000 Rows, ~33.5MB Dataset)

| Benchmark Task | Pure Go SWAR Fallback | Go 1.27 Portable SIMD | SIMD Speedup |
|---|---|---|---|
| **Reader (Zero-Copy Streaming)** | 441.45 MB/s | **675.32 MB/s** | **+53.0%** |
| **Special-Byte Vector Scanner** | 18,200 MB/s | **38,382 MB/s** | **+110.9% (2.1x)** |
| **Parallel Unmarshal (Struct, 16W)** | 524.97 MB/s | **644.02 MB/s** | **+22.7%** |
| **Parallel Unmarshal (Generic, 16W)**| 637.21 MB/s | **723.02 MB/s** | **+13.5%** |
| **Parallel Marshal (Struct, 16W)** | 811.02 MB/s | **889.41 MB/s (~0.89 GB/s)** | **+9.7%** |
| **Parallel Marshal (Generic, 16W)** | 589.56 MB/s | **625.77 MB/s** | **+6.2%** |

### Comparison Against Standard Library and `csvutil`

| Benchmark | Library | Throughput | Latency (500k rows) | Allocations / Op |
|---|---|---|---|---|
| **Struct Marshaling (Sequential)** | `csvutil` | 135.12 MB/s | 247.8 ms | 1,500,024 allocs |
| | **`csv` (Ours)** | **224.83 MB/s** | **148.9 ms (1.7x faster)** | **16 allocs (99.999% reduction)** |
| **Struct Marshaling (Parallel 16W)** | `csvutil` | N/A (single-threaded) | 247.8 ms | 1,500,024 allocs |
| | **`csv` (Ours)** | **889.41 MB/s** | **37.6 ms (6.6x faster)** | **137 allocs** |
| **Struct Unmarshaling (Parallel 16W)**| `csvutil` | 110.79 MB/s | 168.1 ms | 500,028 allocs |
| | **`csv` (Ours)** | **644.02 MB/s** | **28.9 ms (5.8x faster)** | 500,207 allocs |
| **Generic Zero-Reflection (16W)** | **`csv` (Ours)** | **723.02 MB/s** | **25.7 ms (6.5x faster)** | 500,157 allocs |

---

## Enabling Go 1.27 SIMD Vector Acceleration

To compile with hardware-accelerated SIMD instructions:

```bash
GOEXPERIMENT=simd go build ./...
GOEXPERIMENT=simd go test -bench=. ./...
```

When built without `GOEXPERIMENT=simd`, the package automatically falls back to an optimized 8-byte SWAR (SIMD Within A Register) pure Go scanner with zero external dependencies.

---

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
