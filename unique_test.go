package csv

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

type UniqueUser struct {
	ID    int64  `csv:"id,unique"`
	Email string `csv:"email,unique"`
	Name  string `csv:"name"`
}

func TestUniqueUnmarshalSuccess(t *testing.T) {
	csvData := `id,email,name
1,alice@example.com,Alice
2,bob@example.com,Bob
3,carol@example.com,Carol
`
	var users []UniqueUser
	if err := Unmarshal([]byte(csvData), &users); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
}

func TestUniqueUnmarshalDuplicateID(t *testing.T) {
	csvData := `id,email,name
1,alice@example.com,Alice
2,bob@example.com,Bob
1,charlie@example.com,Charlie
`
	var users []UniqueUser
	err := Unmarshal([]byte(csvData), &users)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected errors.Is(err, ErrDuplicate), got: %v", err)
	}

	var dupErr *DuplicateFieldError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected *DuplicateFieldError, got %T: %v", err, err)
	}
	if dupErr.Field != "ID" && dupErr.Field != "id" {
		t.Errorf("expected field ID/id, got %q", dupErr.Field)
	}
	if dupErr.Value != "1" {
		t.Errorf("expected value '1', got %q", dupErr.Value)
	}
}

func TestUniqueUnmarshalDuplicateEmail(t *testing.T) {
	csvData := `id,email,name
10,alice@example.com,Alice
20,alice@example.com,DuplicateAlice
`
	var users []UniqueUser
	err := Unmarshal([]byte(csvData), &users)
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}

func TestUniqueDecoderStreaming(t *testing.T) {
	csvData := `id,email,name
1,a@example.com,Alice
2,b@example.com,Bob
1,c@example.com,Carol
`
	dec, err := NewDecoder(strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}

	var u UniqueUser
	// Row 1: 1
	if err := dec.Decode(&u); err != nil {
		t.Fatal(err)
	}
	// Row 2: 2
	if err := dec.Decode(&u); err != nil {
		t.Fatal(err)
	}
	// Row 3: 1 (Duplicate)
	err = dec.Decode(&u)
	if err == nil {
		t.Fatal("expected duplicate error on row 3, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}

func TestUniqueParallelUnmarshal(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("id,email,name\n")
	for i := 0; i < 500; i++ {
		// Insert a duplicate at row 450 with row 10
		id := i
		if i == 450 {
			id = 10
		}
		sb.WriteString(strings.Join([]string{
			strings.Repeat("0", 3-len(strings.TrimSpace(string(rune(id))))) + string(rune(id)),
			"user" + string(rune(i)) + "@example.com",
			"User",
		}, ",") + "\n")
	}

	csvLines := []string{"id,email,name"}
	for i := 1; i <= 200; i++ {
		if i == 150 {
			csvLines = append(csvLines, "1,dup@example.com,Dup")
		} else {
			csvLines = append(csvLines, strconv.Itoa(i)+",email"+strconv.Itoa(i)+"@test.com,Name")
		}
	}
	csvContent := strings.Join(csvLines, "\n")

	var users []UniqueUser
	err := ParallelUnmarshal([]byte(csvContent), &users, ParallelOptions{Workers: 4})
	if err == nil {
		t.Fatal("expected duplicate error in ParallelUnmarshal, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}

func TestUniqueMarshal(t *testing.T) {
	users := []UniqueUser{
		{ID: 1, Email: "alice@test.com", Name: "Alice"},
		{ID: 1, Email: "bob@test.com", Name: "Bob"}, // Duplicate ID
	}

	_, err := Marshal(users)
	if err == nil {
		t.Fatal("expected duplicate error in Marshal, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}
