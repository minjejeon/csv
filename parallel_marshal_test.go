package csv

import (
	"strings"
	"testing"
)

func TestParallelMarshalEquivalence(t *testing.T) {
	users := make([]TestUserMarshal, 1000)
	for i := range users {
		users[i] = TestUserMarshal{
			ID:     int64(i),
			Name:   "User",
			Age:    20 + (i % 50),
			Salary: float64(i) * 1.5,
			Active: i%2 == 0,
		}
	}

	seqData, err := Marshal(users)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	for _, workers := range []int{1, 2, 4, 8} {
		parData, err := ParallelMarshal(users, ParallelOptions{Workers: workers})
		if err != nil {
			t.Fatalf("ParallelMarshal failed with %d workers: %v", workers, err)
		}
		if string(seqData) != string(parData) {
			t.Fatalf("ParallelMarshal output (%d workers) does not match sequential Marshal:\nlen seq=%d, len par=%d",
				workers, len(seqData), len(parData))
		}
	}
}

func TestParallelMarshalPointers(t *testing.T) {
	users := make([]*TestUserMarshal, 500)
	for i := range users {
		if i%10 == 0 {
			users[i] = nil
		} else {
			users[i] = &TestUserMarshal{
				ID:     int64(i),
				Name:   "PointerUser",
				Age:    30,
				Salary: 99.9,
				Active: true,
			}
		}
	}

	seqData, err := Marshal(users)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	parData, err := ParallelMarshal(users, ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatalf("ParallelMarshal failed: %v", err)
	}

	if string(seqData) != string(parData) {
		t.Fatalf("ParallelMarshal pointers does not match sequential Marshal")
	}
}

func TestParallelMarshalCustomOptions(t *testing.T) {
	users := make([]TestUserMarshal, 100)
	for i := range users {
		users[i] = TestUserMarshal{ID: int64(i), Name: "Name"}
	}

	parData, err := ParallelMarshal(users, ParallelOptions{Workers: 2}, WithDelimiter("|"), WithQuote('\''))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(parData)), "\n")
	if len(lines) != 101 {
		t.Fatalf("expected 101 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "|") {
		t.Fatalf("expected custom delimiter '|' in header: %s", lines[0])
	}
}
