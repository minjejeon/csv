package csv

import (
	"testing"

	"github.com/jszwec/csvutil"
)

func BenchmarkParallelScale_Sequential(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := Unmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelScale_1Worker(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 1}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelScale_2Workers(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 2}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelScale_4Workers(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 4}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelScale_8Workers(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 8}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelScale_16Workers(b *testing.B) {
	data := makeLargeCSV(50000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 16}); err != nil {
			b.Fatal(err)
		}
	}
}

// 500,000 Rows Benchmarks (~18MB dataset)
func Benchmark500k_Csvutil(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := csvutil.Unmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Sequential(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := Unmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Parallel_Default4Workers(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		// Default uses 4 workers
		if err := ParallelUnmarshal(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Parallel_8Workers(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 8}); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Parallel_16Workers(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUser
		if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 16}); err != nil {
			b.Fatal(err)
		}
	}
}

