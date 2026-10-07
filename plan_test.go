package csv

import (
	"reflect"
	"testing"
)

type sampleUser struct {
	ID        int64  `csv:"user_id"`
	Name      string `csv:"username,omitempty"`
	Age       int    // fallback to field name
	Ignored   string `csv:"-"`
	unexport  string
}

type embeddedUser struct {
	sampleUser
	Role string `csv:"user_role"`
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		tagStr    string
		wantName  string
		wantIgnore bool
		wantOmit  bool
	}{
		{"", "", false, false},
		{"-", "", true, false},
		{"id", "id", false, false},
		{"id,omitempty", "id", false, true},
		{",omitempty", "", false, true},
		{"col_name", "col_name", false, false},
	}

	for _, tt := range tests {
		tag := parseTag(tt.tagStr)
		if tag.name != tt.wantName || tag.ignore != tt.wantIgnore || tag.omitEmpty != tt.wantOmit {
			t.Errorf("parseTag(%q) = %+v, want name=%q ignore=%v omit=%v",
				tt.tagStr, tag, tt.wantName, tt.wantIgnore, tt.wantOmit)
		}
	}
}

func TestBuildTypePlan(t *testing.T) {
	typ := reflect.TypeOf(sampleUser{})
	headers := []string{"age", "user_id", "username", "extra_col"}

	plan, err := getTypePlan(typ, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.fields) != 3 {
		t.Fatalf("expected 3 mapped fields, got %d", len(plan.fields))
	}

	// Verify fields
	fieldMap := make(map[string]fieldPlan)
	for _, f := range plan.fields {
		fieldMap[f.fieldName] = f
	}

	// ID -> user_id -> col index 1
	if f, ok := fieldMap["ID"]; !ok || f.colIndex != 1 {
		t.Errorf("expected ID at colIndex 1, got %+v", f)
	}
	// Name -> username -> col index 2
	if f, ok := fieldMap["Name"]; !ok || f.colIndex != 2 {
		t.Errorf("expected Name at colIndex 2, got %+v", f)
	}
	// Age -> age (case-insensitive) -> col index 0
	if f, ok := fieldMap["Age"]; !ok || f.colIndex != 0 {
		t.Errorf("expected Age at colIndex 0, got %+v", f)
	}
}

func TestEmbeddedTypePlan(t *testing.T) {
	typ := reflect.TypeOf(embeddedUser{})
	headers := []string{"user_role", "user_id"}

	plan, err := getTypePlan(typ, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.fields) != 2 {
		t.Fatalf("expected 2 mapped fields, got %d", len(plan.fields))
	}
}

func TestTypePlanCache(t *testing.T) {
	typ := reflect.TypeOf(sampleUser{})
	headers := []string{"user_id", "username"}

	plan1, err := getTypePlan(typ, headers)
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := getTypePlan(typ, headers)
	if err != nil {
		t.Fatal(err)
	}

	if plan1 != plan2 {
		t.Fatal("expected cached plan pointer to be identical")
	}
}

type embeddedPtrUser struct {
	*sampleUser
	Role string `csv:"user_role"`
}

func TestEmbeddedPointerTypePlan(t *testing.T) {
	typ := reflect.TypeOf(embeddedPtrUser{})
	headers := []string{"user_role", "user_id"}

	plan, err := getTypePlan(typ, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.fields) != 2 {
		t.Fatalf("expected 2 mapped fields, got %d", len(plan.fields))
	}
}

func TestEmbeddedPointerUnmarshalAndMarshal(t *testing.T) {
	data := "user_role,user_id,username\nadmin,42,alice\n"
	var users []embeddedPtrUser
	if err := Unmarshal([]byte(data), &users); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := users[0]
	if u.Role != "admin" {
		t.Errorf("Role = %q, want admin", u.Role)
	}
	if u.sampleUser == nil {
		t.Fatalf("sampleUser is nil!")
	}
	if u.ID != 42 || u.Name != "alice" {
		t.Errorf("embedded sampleUser = %+v, want ID=42, Name=alice", u.sampleUser)
	}

	// Test Marshal round-trip
	out, err := Marshal(users)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var roundTrip []embeddedPtrUser
	if err := Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("Unmarshal round-trip failed: %v", err)
	}
	if len(roundTrip) != 1 || roundTrip[0].ID != 42 || roundTrip[0].Role != "admin" {
		t.Fatalf("round-trip mismatch: %+v", roundTrip)
	}
}
