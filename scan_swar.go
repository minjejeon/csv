package csv

import (
	"encoding/binary"
	"math/bits"
)

// findNextSpecialFallback finds the index of the first occurrence of delim, '"', '\r', or '\n'
// using 64-bit SWAR (SIMD Within A Register) operations.
func findNextSpecialFallback(data []byte, delim byte) int {
	n := len(data)
	i := 0

	const (
		repeat01 = 0x0101010101010101
		repeat80 = 0x8080808080808080
	)
	maskDelim := uint64(delim) * repeat01
	maskQuote := uint64('"') * repeat01
	maskCR := uint64('\r') * repeat01
	maskLF := uint64('\n') * repeat01

	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])

		vDelim := w ^ maskDelim
		hasDelim := (vDelim - repeat01) & ^vDelim & repeat80

		vQuote := w ^ maskQuote
		hasQuote := (vQuote - repeat01) & ^vQuote & repeat80

		vCR := w ^ maskCR
		hasCR := (vCR - repeat01) & ^vCR & repeat80

		vLF := w ^ maskLF
		hasLF := (vLF - repeat01) & ^vLF & repeat80

		match := hasDelim | hasQuote | hasCR | hasLF
		if match != 0 {
			// Find the exact matching byte index
			tz := bits.TrailingZeros64(match) / 8
			// Verify in case of non-ASCII byte borrow edge-case
			for idx := i; idx <= i+tz && idx < n; idx++ {
				b := data[idx]
				if b == delim || b == '"' || b == '\r' || b == '\n' {
					return idx
				}
			}
			for idx := i + tz + 1; idx < i+8 && idx < n; idx++ {
				b := data[idx]
				if b == delim || b == '"' || b == '\r' || b == '\n' {
					return idx
				}
			}
		}
		i += 8
	}

	for ; i < n; i++ {
		b := data[i]
		if b == delim || b == '"' || b == '\r' || b == '\n' {
			return i
		}
	}
	return -1
}
