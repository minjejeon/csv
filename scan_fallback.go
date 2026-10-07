//go:build !goexperiment.simd

package csv

import (
	"encoding/binary"
)

type blockScanner struct {
	delim     byte
	quote     byte
	maskDelim uint64
	maskQuote uint64
	maskCR    uint64
	maskLF    uint64
}

const (
	swarRepeat01 = 0x0101010101010101
	swarRepeat80 = 0x8080808080808080
)

func newBlockScanner(delim, quote byte) blockScanner {
	return blockScanner{
		delim:     delim,
		quote:     quote,
		maskDelim: uint64(delim) * swarRepeat01,
		maskQuote: uint64(quote) * swarRepeat01,
		maskCR:    uint64('\r') * swarRepeat01,
		maskLF:    uint64('\n') * swarRepeat01,
	}
}

func (s *blockScanner) scanSpecial(data []byte) int {
	return findNextSpecialFallback(data, s.delim, s.quote)
}

func (s *blockScanner) scanBlock32(chunk []byte) (maskDelim, maskQuote, maskEOL uint32) {
	if len(chunk) < 32 {
		for i := 0; i < len(chunk); i++ {
			b := chunk[i]
			if b == s.delim {
				maskDelim |= 1 << i
			} else if b == s.quote {
				maskQuote |= 1 << i
			} else if b == '\r' || b == '\n' {
				maskEOL |= 1 << i
			}
		}
		return maskDelim, maskQuote, maskEOL
	}

	_ = chunk[31]
	for base := 0; base < 32; base += 8 {
		w := binary.LittleEndian.Uint64(chunk[base:])

		vDelim := w ^ s.maskDelim
		hasDelim := (vDelim - swarRepeat01) & ^vDelim & swarRepeat80

		vQuote := w ^ s.maskQuote
		hasQuote := (vQuote - swarRepeat01) & ^vQuote & swarRepeat80

		vCR := w ^ s.maskCR
		hasCR := (vCR - swarRepeat01) & ^vCR & swarRepeat80

		vLF := w ^ s.maskLF
		hasLF := (vLF - swarRepeat01) & ^vLF & swarRepeat80

		match := hasDelim | hasQuote | hasCR | hasLF
		if match == 0 && (w&swarRepeat80 == 0) {
			continue
		}

		for j := 0; j < 8; j++ {
			b := chunk[base+j]
			if b == s.delim {
				maskDelim |= 1 << (base + j)
			} else if b == s.quote {
				maskQuote |= 1 << (base + j)
			} else if b == '\r' || b == '\n' {
				maskEOL |= 1 << (base + j)
			}
		}
	}
	return maskDelim, maskQuote, maskEOL
}

func findNextSpecial(data []byte, delim byte, quote byte) int {
	return findNextSpecialFallback(data, delim, quote)
}
