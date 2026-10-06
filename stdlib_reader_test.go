package csv

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type readTest struct {
	Name               string
	Input              string
	Output             [][]string
	Comma              rune
	Comment            rune
	UseFieldsPerRecord bool
	FieldsPerRecord    int
	LazyQuotes         bool
	TrimLeadingSpace   bool
}

var readTests = []readTest{
	{
		Name:   "Simple",
		Input:  "a,b,c\n",
		Output: [][]string{{"a", "b", "c"}},
	},
	{
		Name:   "CRLF",
		Input:  "a,b\r\nc,d\r\n",
		Output: [][]string{{"a", "b"}, {"c", "d"}},
	},
	{
		Name:   "BareCR",
		Input:  "a,b\rc,d\r\n",
		Output: [][]string{{"a", "b\rc", "d"}},
	},
	{
		Name: "RFC4180test",
		Input: "#field1,field2,field3\n" +
			`"aaa","bb` + "\n" + `b","ccc"` + "\n" +
			`"a,a","b""bb","ccc"` + "\n" +
			"zzz,yyy,xxx\n",
		Output: [][]string{
			{"#field1", "field2", "field3"},
			{"aaa", "bb\nb", "ccc"},
			{"a,a", `b"bb`, "ccc"},
			{"zzz", "yyy", "xxx"},
		},
		UseFieldsPerRecord: true,
		FieldsPerRecord:    0,
	},
	{
		Name:   "NoEOLTest",
		Input:  "a,b,c",
		Output: [][]string{{"a", "b", "c"}},
	},
	{
		Name:   "Semicolon",
		Input:  "a;b;c\n",
		Output: [][]string{{"a", "b", "c"}},
		Comma:  ';',
	},
	{
		Name: "MultiLine",
		Input: `"two` + "\n" + `line","one line","three` + "\n" + `line` + "\n" + `field"`,
		Output: [][]string{{"two\nline", "one line", "three\nline\nfield"}},
	},
	{
		Name:  "BlankLine",
		Input: "a,b,c\n\nd,e,f\n\n",
		Output: [][]string{
			{"a", "b", "c"},
			{"d", "e", "f"},
		},
	},
	{
		Name:             "TrimSpace",
		Input:            " a,  b,   c\n",
		Output:           [][]string{{"a", "b", "c"}},
		TrimLeadingSpace: true,
	},
	{
		Name:   "LeadingSpace",
		Input:  " a,  b,   c\n",
		Output: [][]string{{" a", "  b", "   c"}},
	},
	{
		Name:    "Comment",
		Input:   "#1,2,3\na,b,c\n#comment",
		Output:  [][]string{{"a", "b", "c"}},
		Comment: '#',
	},
	{
		Name:   "NoComment",
		Input:  "#1,2,3\na,b,c",
		Output: [][]string{{"#1", "2", "3"}, {"a", "b", "c"}},
	},
	{
		Name:       "LazyQuotes",
		Input:      `a "word","1"2",a","b`,
		Output:     [][]string{{`a "word"`, `1"2`, `a"`, `b`}},
		LazyQuotes: true,
	},
	{
		Name:       "BareQuotes",
		Input:      `a "word","1"2",a"`,
		Output:     [][]string{{`a "word"`, `1"2`, `a"`}},
		LazyQuotes: true,
	},
	{
		Name:       "BareDoubleQuotes",
		Input:      `a""b,c`,
		Output:     [][]string{{`a""b`, `c`}},
		LazyQuotes: true,
	},
	{
		Name:             "TrimQuote",
		Input:            ` "a"," b",c`,
		Output:           [][]string{{"a", " b", "c"}},
		TrimLeadingSpace: true,
	},
	{
		Name:   "TrailingCommaEOF",
		Input:  "a,b,c,",
		Output: [][]string{{"a", "b", "c", ""}},
	},
	{
		Name:   "EmptyFields",
		Input:  ",,",
		Output: [][]string{{"" , "", ""}},
	},
	{
		Name:   "EscapedQuotes",
		Input:  `"""",hello,"a""b"`,
		Output: [][]string{{`"`, "hello", `a"b`}},
	},
}

func TestStdlibRead(t *testing.T) {
	for _, tt := range readTests {
		t.Run(tt.Name, func(t *testing.T) {
			r := NewReader(strings.NewReader(tt.Input))
			if tt.Comma != 0 {
				r.Comma = tt.Comma
			}
			if tt.Comment != 0 {
				r.Comment = tt.Comment
			}
			if tt.UseFieldsPerRecord {
				r.FieldsPerRecord = tt.FieldsPerRecord
			}
			r.LazyQuotes = tt.LazyQuotes
			r.TrimLeadingSpace = tt.TrimLeadingSpace

			var out [][]string
			for {
				rec, err := r.Read()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("unexpected read error: %v", err)
				}
				out = append(out, rec)
			}

			if !reflect.DeepEqual(out, tt.Output) {
				t.Fatalf("output mismatch:\ngot:  %+v\nwant: %+v", out, tt.Output)
			}
		})
	}
}
