package csv

import (
	"testing"
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
