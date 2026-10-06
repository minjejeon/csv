package csv

import (
	stdcsv "encoding/csv"
	"io"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/jszwec/csvutil"
)

type MarshalBenchUser struct {
	ID     int64     `csv:"id"`
	Name   string    `csv:"name"`
	Age    int       `csv:"age"`
	Salary float64   `csv:"salary"`
	Active bool      `csv:"active"`
	Bio    string    `csv:"bio,omitempty"`
	Date   time.Time `csv:"date"`
}

type BenchUserGeneric struct {
	ID     int64
	Name   string
	Age    int
	Salary float64
	Active bool
}

func (u *BenchUserGeneric) CSVHeader() []string {
	return []string{"id", "name", "age", "salary", "active"}
}

func (u *BenchUserGeneric) MarshalCSVRecord(w *Writer) error {
	w.WriteFieldBytes(strconv.AppendInt(nil, u.ID, 10))
	w.WriteDelimiter()
	w.WriteFieldBytes([]byte(u.Name))
	w.WriteDelimiter()
	w.WriteFieldBytes(strconv.AppendInt(nil, int64(u.Age), 10))
	w.WriteDelimiter()
	w.WriteFieldBytes(strconv.AppendFloat(nil, u.Salary, 'f', 2, 64))
	w.WriteDelimiter()
	if u.Active {
		w.WriteFieldBytes([]byte("true"))
	} else {
		w.WriteFieldBytes([]byte("false"))
	}
	return nil
}

func makeBenchUsers(n int) []MarshalBenchUser {
	tm, _ := time.Parse(time.RFC3339, "2026-10-07T12:00:00Z")
	users := make([]MarshalBenchUser, n)
	for i := 0; i < n; i++ {
		bio := ""
		if i%3 == 0 {
			bio = "Software Engineer, Go & SIMD"
		}
		users[i] = MarshalBenchUser{
			ID:     int64(i + 1),
			Name:   "User " + strconv.Itoa(i),
			Age:    20 + (i % 60),
			Salary: 50000.0 + float64(i)*1.5,
			Active: i%2 == 0,
			Bio:    bio,
			Date:   tm,
		}
	}
	return users
}

func makeBenchUsersGeneric(n int) []BenchUserGeneric {
	users := make([]BenchUserGeneric, n)
	for i := 0; i < n; i++ {
		users[i] = BenchUserGeneric{
			ID:     int64(i + 1),
			Name:   "User " + strconv.Itoa(i),
			Age:    20 + (i % 60),
			Salary: 50000.0 + float64(i)*1.5,
			Active: i%2 == 0,
		}
	}
	return users
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	users := makeBenchUsers(100)
	data, err := Marshal(users)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed []MarshalBenchUser
	if err := Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(parsed) != len(users) {
		t.Fatalf("expected %d users, got %d", len(users), len(parsed))
	}

	for i := range users {
		if users[i].ID != parsed[i].ID ||
			users[i].Name != parsed[i].Name ||
			users[i].Age != parsed[i].Age ||
			users[i].Salary != parsed[i].Salary ||
			users[i].Active != parsed[i].Active ||
			users[i].Bio != parsed[i].Bio ||
			!users[i].Date.Equal(parsed[i].Date) {
			t.Fatalf("mismatch at row %d:\nexpected: %+v\ngot:      %+v", i, users[i], parsed[i])
		}
	}
}

func TestParallelMarshalUnmarshalRoundTrip(t *testing.T) {
	users := makeBenchUsers(5000)
	data, err := ParallelMarshal(users, ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatalf("ParallelMarshal failed: %v", err)
	}

	var parsed []MarshalBenchUser
	if err := ParallelUnmarshal(data, &parsed, ParallelOptions{Workers: 4}); err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(parsed) != len(users) {
		t.Fatalf("expected %d users, got %d", len(users), len(parsed))
	}

	for i := 0; i < 20; i++ {
		if !reflect.DeepEqual(users[i], parsed[i]) {
			t.Fatalf("mismatch at row %d:\nexpected: %+v\ngot:      %+v", i, users[i], parsed[i])
		}
	}
}

// -------------------------------------------------------------
// Writer Benchmarks (50,000 Rows raw string records)
// -------------------------------------------------------------

func BenchmarkWriter_50k_Stdlib(b *testing.B) {
	records := make([][]string, 50000)
	for i := range records {
		records[i] = []string{"1001", "Alice Smith", "30", "75000.50", "true"}
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := stdcsv.NewWriter(io.Discard)
		if err := w.WriteAll(records); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriter_50k_Ours(b *testing.B) {
	records := make([][]string, 50000)
	for i := range records {
		records[i] = []string{"1001", "Alice Smith", "30", "75000.50", "true"}
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := NewWriter(io.Discard)
		if err := w.WriteAll(records); err != nil {
			b.Fatal(err)
		}
		w.Close()
	}
}

// -------------------------------------------------------------
// Struct Marshal Benchmarks (50,000 Rows)
// -------------------------------------------------------------

func BenchmarkMarshal_50k_Csvutil(b *testing.B) {
	users := makeBenchUsers(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := csvutil.Marshal(users)
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkMarshal_50k_Ours(b *testing.B) {
	users := makeBenchUsers(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := Marshal(users)
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkMarshal_50k_Parallel_4Workers(b *testing.B) {
	users := makeBenchUsers(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshal(users, ParallelOptions{Workers: 4})
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkMarshal_50k_Parallel_16Workers(b *testing.B) {
	users := makeBenchUsers(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshal(users, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkMarshal_50k_Generic_Sequential(b *testing.B) {
	users := makeBenchUsersGeneric(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := MarshalSlice[BenchUserGeneric, *BenchUserGeneric](users)
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkMarshal_50k_Generic_Parallel_16Workers(b *testing.B) {
	users := makeBenchUsersGeneric(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshalSlice[BenchUserGeneric, *BenchUserGeneric](users, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

// -------------------------------------------------------------
// Large Scale Struct Marshal Benchmarks (500,000 Rows)
// -------------------------------------------------------------

func BenchmarkMarshal_500k_Csvutil(b *testing.B) {
	users := makeBenchUsers(500000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := csvutil.Marshal(users)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMarshal_500k_Ours_Sequential(b *testing.B) {
	users := makeBenchUsers(500000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := Marshal(users)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMarshal_500k_Ours_Parallel_16Workers(b *testing.B) {
	users := makeBenchUsers(500000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshal(users, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMarshal_500k_Generic_Parallel_16Workers(b *testing.B) {
	users := makeBenchUsersGeneric(500000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshalSlice[BenchUserGeneric, *BenchUserGeneric](users, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}
