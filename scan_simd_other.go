//go:build goexperiment.simd && !amd64

package csv

import (
	"math/bits"
	"simd"
	"simd/archsimd"
)

type blockScanner struct {
	delim  byte
	quote  byte
	vecLen int
	vDelim simd.Uint8s
	vQuote simd.Uint8s
	vCR    simd.Uint8s
	vLF    simd.Uint8s
}

func newBlockScanner(delim, quote byte) blockScanner {
	return blockScanner{
		delim:  delim,
		quote:  quote,
		vecLen: simd.BroadcastUint8s(delim).Len(),
		vDelim: simd.BroadcastUint8s(delim),
		vQuote: simd.BroadcastUint8s(quote),
		vCR:    simd.BroadcastUint8s('\r'),
		vLF:    simd.BroadcastUint8s('\n'),
	}
}

func (s *blockScanner) scanSpecial(data []byte) int {
	if simd.Emulated() {
		return findNextSpecialFallback(data, s.delim, s.quote)
	}

	n := len(data)
	if n == 0 {
		return -1
	}

	vecLen := s.vecLen
	i := 0
	for i+vecLen <= n {
		chunk := simd.LoadUint8s(data[i : i+vecLen])
		mDelim := chunk.Equal(s.vDelim)
		mQuote := chunk.Equal(s.vQuote)
		mCR := chunk.Equal(s.vCR)
		mLF := chunk.Equal(s.vLF)

		mAll := mDelim.Or(mQuote).Or(mCR).Or(mLF)

		switch a := mAll.ToArch().(type) {
		case archsimd.Mask8x16:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros16(maskBits)
			}
		case archsimd.Mask8x32:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros32(maskBits)
			}
		case archsimd.Mask8x64:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros64(maskBits)
			}
		default:
			s8 := mAll.ToInt8s()
			var buf [64]int8
			s8.StorePart(buf[:vecLen])
			for idx := 0; idx < vecLen; idx++ {
				if buf[idx] != 0 {
					return i + idx
				}
			}
		}
		i += vecLen
	}

	for ; i < n; i++ {
		b := data[i]
		if b == s.delim || b == s.quote || b == '\r' || b == '\n' {
			return i
		}
	}
	return -1
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
	d := s.delim
	q := s.quote
	for i := 0; i < 32; i++ {
		b := chunk[i]
		if b == d {
			maskDelim |= 1 << i
		} else if b == q {
			maskQuote |= 1 << i
		} else if b == '\r' || b == '\n' {
			maskEOL |= 1 << i
		}
	}
	return maskDelim, maskQuote, maskEOL
}

func findNextSpecial(data []byte, delim byte, quote byte) int {
	s := newBlockScanner(delim, quote)
	return s.scanSpecial(data)
}
