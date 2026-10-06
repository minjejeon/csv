# csv

[![Go Reference](https://pkg.go.dev/badge/github.com/minjejeon/csv.svg)](https://pkg.go.dev/github.com/minjejeon/csv)
[![Go Report Card](https://goreportcard.com/badge/github.com/minjejeon/csv)](https://goreportcard.com/report/github.com/minjejeon/csv)
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

### 6. Parallel Multithreaded Unmarshaling

Concurrently parse large CSV files (e.g. hundreds of megabytes or gigabytes) across multiple worker goroutines:

```go
var users []User
// Concurrently chunks and parses data across 16 CPU workers:
err := csv.ParallelUnmarshal(largeData, &users,
	csv.ParallelOptions{Workers: 16},
	csv.WithDelimiter(","),
)
```

---

## Functional Options Reference

| Option | Description |
|---|---|
| `WithDelimiter(string)` | Sets a custom single or multi-character delimiter (e.g. `"\|"`, `";"`, `"\|\|"`, `"::"`). |
| `WithComma(rune)` | Sets a single-character delimiter (backward-compatible with standard `encoding/csv`). |
| `WithQuote(rune)` | Sets a custom quotation mark (default: `'"'`, e.g. `'\''`). |
| `WithCharset(string)` | Decodes CSV data from a named encoding into UTF-8 (`"euc-kr"`, `"cp949"`, `"shift_jis"`, `"latin1"`, etc.). |
| `WithEncoding(encoding.Encoding)` | Decodes CSV data using a `golang.org/x/text/encoding` instance. |
| `WithComment(rune)` | Sets the comment character (e.g. `'#'`). Lines starting with this character are ignored. |
| `WithTrimLeadingSpace(bool)` | If true, leading whitespace in unquoted fields is ignored. |
| `WithLazyQuotes(bool)` | If true, quotes are permitted to appear in unquoted fields. |
| `WithFieldsPerRecord(int)` | Enforces expected field count per record (`-1`: no check, `0`: match first row, `>0`: fixed count). |

---

## Performance Benchmarks

Benchmarks run on **AMD Ryzen 7 7735HS (16 vCPUs)** under Go 1.27 with `GOEXPERIMENT=simd`:

| Benchmark | Speed / Throughput | Memory / Op | Allocs / Op |
|---|---|---|---|
| **Reader (Zero-Copy Streaming)** | **675.32 MB/s** | **0 B/op** | **0 allocs/op** |
| Standard `encoding/csv.Reader` | 229.78 MB/s | 116,672 B/op | 2,015 allocs/op |
| **SIMD Special-Byte Scanner** | **38,382 MB/s (38.3 GB/s)** | **0 B/op** | **0 allocs/op** |
| **Decoder (Row Struct Unmarshaler)** | **150.01 MB/s** | **8,496 B/op** | **1,010 allocs/op** |
| `jszwec/csvutil.Decoder` | 106.56 MB/s | 118,272 B/op | 2,023 allocs/op |
| **Parallel Unmarshal (16 Workers, 500k rows)** | **455.27 MB/s** | 56.5 MB / op | 500k allocs/op |
| `csvutil` (Single-Threaded, 500k rows) | 113.65 MB/s | 44.1 MB / op | 500k allocs/op |
| **Generic RecordUnmarshaler (16 Workers)** | **504.91 MB/s** | 56.4 MB / op | 500k allocs/op |

> **Key takeaway**: In streaming read mode, `github.com/minjejeon/csv` consumes **0 memory allocations**, and in multithreaded mode it is **over 4x faster** than `csvutil`.

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
