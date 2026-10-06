package csv

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestInternetEdgeCases ports test suites from csv-spectrum, W3C CSVW, and SQLite CSV test suites.
func TestInternetEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		opts     []Option
		expected [][]string
	}{
		{
			name:     "csv-spectrum simple",
			input:    "a,b,c\n1,2,3\n",
			expected: [][]string{{"a", "b", "c"}, {"1", "2", "3"}},
		},
		{
			name:     "csv-spectrum comma in quotes",
			input:    "first,last,address\nJohn,\"Doe, Jr.\",\"123 Main St, Apt 4\"\n",
			expected: [][]string{{"first", "last", "address"}, {"John", "Doe, Jr.", "123 Main St, Apt 4"}},
		},
		{
			name:     "csv-spectrum escaped quotes",
			input:    "a,b\n1,\"ha \"\"ha\"\" ha\"\n3,4\n",
			expected: [][]string{{"a", "b"}, {"1", "ha \"ha\" ha"}, {"3", "4"}},
		},
		{
			name:     "csv-spectrum quotes and new lines",
			input:    "a,b,c\n1,\"line 1\nline 2\",3\n4,5,6\n",
			expected: [][]string{{"a", "b", "c"}, {"1", "line 1\nline 2", "3"}, {"4", "5", "6"}},
		},
		{
			name:     "csv-spectrum json in field",
			input:    "id,json\n1,\"{\"\"key\"\": \"\"value\"\", \"\"arr\"\": [1, 2, 3]}\"\n",
			expected: [][]string{{"id", "json"}, {"1", `{"key": "value", "arr": [1, 2, 3]}`}},
		},
		{
			name:     "csv-spectrum utf-8 multibyte and emojis",
			input:    "language,greeting,emoji\nKorean,안녕하세요,👋\nJapanese,こんにちは,🌸\nArabic,مرحبا,✨\n",
			expected: [][]string{
				{"language", "greeting", "emoji"},
				{"Korean", "안녕하세요", "👋"},
				{"Japanese", "こんにちは", "🌸"},
				{"Arabic", "مرحبا", "✨"},
			},
		},
		{
			name:     "empty fields at start middle end",
			input:    ",,\n,a,\n,,b\n",
			expected: [][]string{
				{"", "", ""},
				{"", "a", ""},
				{"", "", "b"},
			},
		},
		{
			name:     "trailing comma at end of line without newline",
			input:    "a,b,\n1,2,",
			expected: [][]string{{"a", "b", ""}, {"1", "2", ""}},
		},
		{
			name:     "consecutive escaped quotes",
			input:    "\"\"\"\"\"\"\n",
			expected: [][]string{{`""`}},
		},
		{
			name:     "crlf line endings with embedded lf",
			input:    "col1,col2\r\n\"multi\nline\",value\r\n",
			expected: [][]string{{"col1", "col2"}, {"multi\nline", "value"}},
		},
		{
			name:     "single column with quotes",
			input:    "\"row1\"\n\"row2, with comma\"\n\"row3 with \"\"quotes\"\"\"\n",
			expected: [][]string{{"row1"}, {"row2, with comma"}, {`row3 with "quotes"`}},
		},
		{
			name:     "tab delimited with semicolon",
			input:    "name\tage\tcity\nAlice\t30\tParis;France\n",
			opts:     []Option{WithDelimiter("\t")},
			expected: [][]string{{"name", "age", "city"}, {"Alice", "30", "Paris;France"}},
		},
		{
			name:     "pipe delimited with single quote",
			input:    "id|name|note\n1|'O''Reilly'|Engineer\n",
			opts:     []Option{WithDelimiter("|"), WithQuote('\'')},
			expected: [][]string{{"id", "name", "note"}, {"1", "O'Reilly", "Engineer"}},
		},
		{
			name:     "unicode bom prefix stripped or handled cleanly",
			input:    "\xef\xbb\xbfa,b,c\n1,2,3\n",
			expected: [][]string{{"\xef\xbb\xbfa", "b", "c"}, {"1", "2", "3"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader(strings.NewReader(tc.input), tc.opts...)
			records, err := r.ReadAll()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(records, tc.expected) {
				t.Fatalf("expected:\n%#v\ngot:\n%#v", tc.expected, records)
			}
		})
	}
}

// TestInternetLargeCellBoundaries tests very large CSV cells (e.g. 128KB single cell)
// to verify buffer expansion, sliding windows, and zero corruption.
func TestInternetLargeCellBoundaries(t *testing.T) {
	largeStr := strings.Repeat("A-very-long-csv-cell-content-1234567890,", 4000) // ~160KB cell
	input := "id,content,tag\n1,\"" + largeStr + "\",test\n2,normal,end\n"

	r := NewReader(strings.NewReader(input))
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed reading large cell: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(recs))
	}
	if recs[1][1] != largeStr {
		t.Fatalf("large cell corrupted: length expected %d, got %d", len(largeStr), len(recs[1][1]))
	}
	if recs[2][1] != "normal" || recs[2][2] != "end" {
		t.Fatalf("subsequent record corrupted after large cell: %#v", recs[2])
	}
}

// TestInternetFuzzLikeMaliciousPatterns tests strange inputs to ensure no panics or infinite loops.
func TestInternetFuzzLikeMaliciousPatterns(t *testing.T) {
	patterns := []string{
		"",
		"\n",
		"\r",
		"\r\n",
		",,,,,",
		",\n,\n,\n",
		"\"",
		"\"\"",
		"\"\"\"",
		"\"\"\"\"",
		"\"a\"b\"c\"",
		"\"unclosed quote\nnext line\n",
		strings.Repeat("\"", 1000),
		strings.Repeat(",", 10000),
		strings.Repeat("\r\n", 5000),
		"a,b\n\"escaped\x00null\",1\n",
	}

	for i, pat := range patterns {
		r := NewReader(strings.NewReader(pat), WithLazyQuotes(true))
		_, _ = r.ReadAll() // Must not panic or hang

		// Also test Writer on arbitrary patterns
		var buf bytes.Buffer
		w := NewWriter(&buf)
		_ = w.Write([]string{pat, "suffix"})
		_ = w.Flush()
		w.Close()

		if i%5 == 0 {
			// Test streaming record reader
			r2 := NewReader(strings.NewReader(pat))
			for {
				_, err := r2.ReadRecord()
				if err != nil {
					break
				}
			}
		}
	}
}
