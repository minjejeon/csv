package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
)

type fuzzItem struct {
	A string  `csv:"a"`
	B int     `csv:"b,omitempty"`
	C float64 `csv:"c,omitempty"`
	D bool    `csv:"d,omitempty"`
}

func FuzzReader(f *testing.F) {
	// Seed corpus
	f.Add([]byte("a,b,c\n1,2,3\n"))
	f.Add([]byte("\"hello, world\",123\n"))
	f.Add([]byte("\"\"\"escaped\"\"\",test\r\n"))
	f.Add([]byte("field1,field2\r\n\"multiline\r\nvalue\",456\r\n"))
	f.Add([]byte(",,\n\n,,,\n"))
	f.Add([]byte("||delim||test\n"))
	f.Add([]byte("a \"bare quote\" in unquoted\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(bytes.NewReader(data))
		defer r.Close()

		for {
			rec, err := r.ReadRecord()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				// Any other parse error is acceptable, but MUST NOT panic
				break
			}

			// Validate record invariants
			numFields := rec.NumFields()
			for i := 0; i < numFields; i++ {
				raw := rec.RawField(i)
				field := rec.Field(i)
				_ = raw
				_ = field
				_ = rec.FieldString(i)
			}
		}
	})
}

func FuzzDecoder(f *testing.F) {
	f.Add([]byte("a,b,c,d\nfoo,10,20.5,true\nbar,-5,-0.1,false\n"))
	f.Add([]byte("a,b,c,d\n,,,,\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		dec, err := NewDecoder(bytes.NewReader(data))
		if err != nil {
			return
		}
		defer dec.Close()

		for dec.More() {
			var item fuzzItem
			if err := dec.Decode(&item); err != nil {
				break
			}
		}
	})
}

func FuzzStdlibEquivalence(f *testing.F) {
	f.Add([]byte("a,b,c\n1,2,3\n"))
	f.Add([]byte("col1,col2\n\"quoted string\",normal\n"))
	f.Add([]byte("x,y\n\"with \"\"quotes\"\"\",val\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Read with stdlib
		stdReader := csv.NewReader(bytes.NewReader(data))
		stdRows, stdErr := stdReader.ReadAll()

		// Read with our reader
		ourReader := NewReader(bytes.NewReader(data))
		defer ourReader.Close()
		ourRows, ourErr := ourReader.ReadAll()

		if stdErr == nil {
			// If stdlib parsed successfully without errors, we should match it
			if ourErr != nil {
				t.Fatalf("stdlib succeeded but our reader failed: %v", ourErr)
			}
			if len(ourRows) != len(stdRows) {
				t.Fatalf("row count mismatch: got %d, want %d", len(ourRows), len(stdRows))
			}
			for i := range stdRows {
				if len(ourRows[i]) != len(stdRows[i]) {
					t.Fatalf("row %d field count mismatch: got %d, want %d", i, len(ourRows[i]), len(stdRows[i]))
				}
				for j := range stdRows[i] {
					ourVal := strings.ReplaceAll(ourRows[i][j], "\r\n", "\n")
					stdVal := stdRows[i][j]
					if ourVal != stdVal {
						t.Fatalf("row %d col %d mismatch:\ngot:  %q (normalized: %q)\nwant: %q",
							i, j, ourRows[i][j], ourVal, stdVal)
					}
				}
			}
		}
	})
}
