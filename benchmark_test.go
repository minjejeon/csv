package csv

import (
	"bytes"
	stdcsv "encoding/csv"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jszwec/csvutil"
)

type BenchUser struct {
	ID     int64   `csv:"id"`
	Name   string  `csv:"name"`
	Age    int     `csv:"age"`
	Salary float64 `csv:"salary"`
	Active bool    `csv:"active"`
}

func makeBenchCSV(rows int) []byte {
	var sb strings.Builder
	sb.WriteString("id,name,age,salary,active\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&sb, "%d,User_%d,%d,%.2f,%t\n",
			1000+i, i, 20+(i%50), 50000.0+float64(i)*1.5, i%2 == 0)
	}
	return []byte(sb.String())
}

// 1. Low-level Reader comparison
func BenchmarkReader_Stdlib(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	r := bytes.NewReader(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(data)
		cr := stdcsv.NewReader(r)
		for {
			_, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkReader_OursZeroCopy(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	r := bytes.NewReader(data)
	cr := NewReader(r)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(data)
		cr.Reset(r)
		for {
			_, err := cr.ReadRecord()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// 2. Struct Streaming Decoder comparison
func BenchmarkDecoder_Csvutil(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	r := bytes.NewReader(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(data)
		dec, err := csvutil.NewDecoder(stdcsv.NewReader(r))
		if err != nil {
			b.Fatal(err)
		}
		var user BenchUser
		for {
			if err := dec.Decode(&user); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkDecoder_OursZeroAlloc(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	r := bytes.NewReader(data)
	dec, err := NewDecoder(r)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(data)
		dec.Reset(r)
		var user BenchUser
		for dec.More() {
			if err := dec.Decode(&user); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// 3. Bulk Unmarshal comparison
func BenchmarkUnmarshal_Csvutil(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []BenchUser
		if err := csvutil.Unmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Ours(b *testing.B) {
	data := makeBenchCSV(1000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []BenchUser
		if err := Unmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}
