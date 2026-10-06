package csv

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

type ParallelUser struct {
	ID     int64   `csv:"id"`
	Name   string  `csv:"name"`
	Age    int     `csv:"age"`
	Salary float64 `csv:"salary"`
	Active bool    `csv:"active"`
}

func makeLargeCSV(rows int) []byte {
	var buf bytes.Buffer
	buf.WriteString("id,name,age,salary,active\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&buf, "%d,user_%d,%d,%.2f,%t\n",
			1000+i, i, 20+(i%45), 45000.0+float64(i)*2.5, i%2 == 0)
	}
	return buf.Bytes()
}

func TestParallelUnmarshalEquivalence(t *testing.T) {
	data := makeLargeCSV(5000)

	// Sequential Unmarshal
	var seqUsers []ParallelUser
	if err := Unmarshal(data, &seqUsers); err != nil {
		t.Fatalf("sequential Unmarshal failed: %v", err)
	}

	// Parallel Unmarshal with 4 workers
	var parUsers []ParallelUser
	if err := ParallelUnmarshal(data, &parUsers, ParallelOptions{Workers: 4}); err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(seqUsers) != len(parUsers) {
		t.Fatalf("row count mismatch: seq=%d, par=%d", len(seqUsers), len(parUsers))
	}

	if !reflect.DeepEqual(seqUsers, parUsers) {
		t.Fatal("ParallelUnmarshal output does not match sequential Unmarshal")
	}
}

func TestParallelUnmarshalPointers(t *testing.T) {
	data := makeLargeCSV(2000)

	var parUsers []*ParallelUser
	if err := ParallelUnmarshal(data, &parUsers, ParallelOptions{Workers: 3}); err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}

	if len(parUsers) != 2000 {
		t.Fatalf("expected 2000 rows, got %d", len(parUsers))
	}
	if parUsers[0].ID != 1000 || parUsers[1999].ID != 2999 {
		t.Errorf("unexpected IDs: first=%d, last=%d", parUsers[0].ID, parUsers[1999].ID)
	}
}

func TestParallelUnmarshalSmall(t *testing.T) {
	data := []byte("id,name,age,salary,active\n1,Alice,30,50000,true\n2,Bob,25,40000,false\n")

	var users []ParallelUser
	if err := ParallelUnmarshal(data, &users, ParallelOptions{Workers: 8}); err != nil {
		t.Fatalf("ParallelUnmarshal small data failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Name != "Alice" || users[1].Name != "Bob" {
		t.Errorf("unexpected users: %+v", users)
	}
}

func TestParallelUnmarshalDefaultWorkers(t *testing.T) {
	data := makeLargeCSV(1000)

	var users []ParallelUser
	// No options passed -> should use DefaultParallelWorkers = 4
	if err := ParallelUnmarshal(data, &users); err != nil {
		t.Fatalf("ParallelUnmarshal with default workers failed: %v", err)
	}

	if len(users) != 1000 {
		t.Fatalf("expected 1000 users, got %d", len(users))
	}
	if DefaultParallelWorkers != 4 {
		t.Fatalf("expected DefaultParallelWorkers to be 4, got %d", DefaultParallelWorkers)
	}
}
