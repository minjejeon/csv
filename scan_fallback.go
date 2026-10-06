//go:build !goexperiment.simd

package csv

type blockScanner struct {
	delim byte
	quote byte
}

func newBlockScanner(delim, quote byte) blockScanner {
	return blockScanner{delim: delim, quote: quote}
}

func (s *blockScanner) scanSpecial(data []byte) int {
	return findNextSpecialFallback(data, s.delim, s.quote)
}

func (s *blockScanner) scanBlock32(chunk []byte) (maskDelim, maskQuote, maskEOL uint32) {
	for i := 0; i < len(chunk) && i < 32; i++ {
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

func findNextSpecial(data []byte, delim byte, quote byte) int {
	return findNextSpecialFallback(data, delim, quote)
}
