package csv

import (
	"reflect"
	"testing"
)

type StaticUser struct {
	ID     int
	Name   string
	Age    int
	Active bool
	Score  float64
}

func (u *StaticUser) UnmarshalCSVRecord(rec *Record) error {
	var err error
	u.ID, err = rec.FieldInt(0)
	if err != nil {
		return err
	}
	u.Name = rec.FieldString(1)
	u.Age, err = rec.FieldInt(2)
	if err != nil {
		return err
	}
	u.Active, err = rec.FieldBool(3)
	if err != nil {
		return err
	}
	u.Score, err = rec.FieldFloat64(4)
	return err
}

func TestGenericUnmarshalSlice(t *testing.T) {
	csvData := []byte("id,name,age,active,score\n1,Alice,30,true,95.5\n2,Bob,25,false,88.0\n")

	users, err := UnmarshalSlice[StaticUser, *StaticUser](csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []StaticUser{
		{ID: 1, Name: "Alice", Age: 30, Active: true, Score: 95.5},
		{ID: 2, Name: "Bob", Age: 25, Active: false, Score: 88.0},
	}

	if !reflect.DeepEqual(users, expected) {
		t.Errorf("got %+v, want %+v", users, expected)
	}
}

func TestGenericUnmarshalTo(t *testing.T) {
	csvData := []byte("id,name,age,active,score\n1,Alice,30,true,95.5\n2,Bob,25,false,88.0\n")

	var users []StaticUser
	err := UnmarshalTo(csvData, &users)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []StaticUser{
		{ID: 1, Name: "Alice", Age: 30, Active: true, Score: 95.5},
		{ID: 2, Name: "Bob", Age: 25, Active: false, Score: 88.0},
	}

	if !reflect.DeepEqual(users, expected) {
		t.Errorf("got %+v, want %+v", users, expected)
	}
}

func TestGenericParallelUnmarshalSlice(t *testing.T) {
	csvData := makeLargeCSV(2000)

	users, err := ParallelUnmarshalSlice[ParallelUserStatic, *ParallelUserStatic](csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(users) != 2000 {
		t.Fatalf("expected 2000 users, got %d", len(users))
	}

	if users[0].ID != 1000 || users[0].Name != "user_0" || users[0].Salary != 45000.0 || !users[0].Active {
		t.Errorf("unexpected first user: %+v", users[0])
	}
	if users[1999].ID != 2999 || users[1999].Name != "user_1999" || users[1999].Salary != 49997.5 || users[1999].Active {
		t.Errorf("unexpected last user: %+v", users[1999])
	}
}

type ParallelUserStatic struct {
	ID     int
	Name   string
	Age    int
	Salary float64
	Active bool
}

func (u *ParallelUserStatic) UnmarshalCSVRecord(rec *Record) error {
	var err error
	u.ID, err = rec.FieldInt(0)
	if err != nil {
		return err
	}
	u.Name = rec.FieldString(1)
	u.Age, err = rec.FieldInt(2)
	if err != nil {
		return err
	}
	u.Salary, err = rec.FieldFloat64(3)
	if err != nil {
		return err
	}
	u.Active, err = rec.FieldBool(4)
	return err
}
