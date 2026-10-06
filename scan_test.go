package csv

import (
	"bytes"
	"math/rand"
	"testing"
)

// naiveFindNextSpecial is our reference oracle for testing
func naiveFindNextSpecial(data []byte, delim byte, quote byte) int {
	for i, b := range data {
		if b == delim || b == quote || b == '\r' || b == '\n' {
			return i
		}
	}
	return -1
}

func TestScanSpecialBasic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		delim byte
		quote byte
		want  int
	}{
		{"empty", "", ',', '"', -1},
		{"no match short", "abcdef", ',', '"', -1},
		{"no match 32", "abcdefghijklmnopqrstuvwxyz123456", ',', '"', -1},
		{"no match 64", "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456", ',', '"', -1},
		{"delim at 0", ",abc", ',', '"', 0},
		{"delim at 5", "abcde,fg", ',', '"', 5},
		{"quote at 0", "\"abc\"", ',', '"', 0},
		{"quote at 10", "1234567890\"hello", ',', '"', 10},
		{"cr at 3", "abc\r\n", ',', '"', 3},
		{"lf at 0", "\nabc", ',', '"', 0},
		{"delim at 31", "1234567890123456789012345678901,", ',', '"', 31},
		{"delim at 32", "12345678901234567890123456789012,", ',', '"', 32},
		{"delim at 33", "123456789012345678901234567890123,", ',', '"', 33},
		{"custom delim tab", "hello\tworld", '\t', '"', 5},
		{"custom delim pipe", "foo|bar", '|', '"', 3},
		{"custom delim semicolon", "foo;bar", ';', '"', 3},
		{"custom single quote at 0", "'abc'", ',', '\'', 0},
		{"custom single quote at 4", "test'val", '|', '\'', 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findNextSpecial([]byte(tt.input), tt.delim, tt.quote)
			if got != tt.want {
				t.Fatalf("findNextSpecial(%q, %q, %q) = %d, want %d", tt.input, tt.delim, tt.quote, got, tt.want)
			}
		})
	}
}

func TestScanSpecialRandomized(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	delims := []byte{',', ';', '\t', '|'}
	quotes := []byte{'"', '\'', '`'}

	// Characters without specials
	safeChars := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-./:;@#")

	for i := 0; i < 200; i++ {
		length := r.Intn(256)
		buf := make([]byte, length)
		for j := range buf {
			buf[j] = safeChars[r.Intn(len(safeChars))]
		}

		delim := delims[r.Intn(len(delims))]
		quote := quotes[r.Intn(len(quotes))]

		// Sometimes inject a special char at random position
		if length > 0 && r.Float64() < 0.7 {
			specials := []byte{delim, quote, '\r', '\n'}
			pos := r.Intn(length)
			buf[pos] = specials[r.Intn(len(specials))]
		}

		want := naiveFindNextSpecial(buf, delim, quote)
		got := findNextSpecial(buf, delim, quote)

		if got != want {
			t.Fatalf("mismatch at run %d (len=%d, delim=%c, quote=%c):\ninput=%q\ngot=%d, want=%d",
				i, length, delim, quote, buf, got, want)
		}
	}
}

func BenchmarkScanner(b *testing.B) {
	delim := byte(',')
	quote := byte('"')
	// 4KB line without specials, ending in newline
	chunk := bytes.Repeat([]byte("0123456789abcdefghijklmnopqrstuv"), 128)
	chunk[len(chunk)-1] = '\n'

	b.SetBytes(int64(len(chunk)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res := findNextSpecial(chunk, delim, quote)
		if res != len(chunk)-1 {
			b.Fatal("unexpected result")
		}
	}
}

func TestScanSpecialPrecomputedVectors(t *testing.T) {
	data := []byte("field1,field2,field3\n")
	idx := findNextSpecial(data, ',', '"')
	if idx != 6 {
		t.Fatalf("expected 6, got %d", idx)
	}
}
