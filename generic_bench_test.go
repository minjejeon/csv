package csv

import (
	"testing"
)

func Benchmark500k_Generic_Sequential(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUserStatic
		if err := UnmarshalTo(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Generic_Parallel_Default4Workers(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUserStatic
		if err := ParallelUnmarshalTo(data, &users); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark500k_Generic_Parallel_16Workers(b *testing.B) {
	data := makeLargeCSV(500000)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var users []ParallelUserStatic
		if err := ParallelUnmarshalTo(data, &users, ParallelOptions{Workers: 16}); err != nil {
			b.Fatal(err)
		}
	}
}
