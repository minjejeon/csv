//go:build goexperiment.simd && amd64

package csv

import (
	"math/bits"
	"simd"
	"simd/archsimd"
)

type blockScanner struct {
	delim byte
	quote byte

	vDelim archsimd.Uint8x32
	vQuote archsimd.Uint8x32
	vCR    archsimd.Uint8x32
	vLF    archsimd.Uint8x32
}

func newBlockScanner(delim, quote byte) blockScanner {
	return blockScanner{
		delim:  delim,
		quote:  quote,
		vDelim: archsimd.BroadcastUint8x32(delim),
		vQuote: archsimd.BroadcastUint8x32(quote),
		vCR:    archsimd.BroadcastUint8x32('\r'),
		vLF:    archsimd.BroadcastUint8x32('\n'),
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

	i := 0
	for i+32 <= n {
		chunk := archsimd.LoadUint8x32(data[i : i+32])
		mDelim := chunk.Equal(s.vDelim)
		mQuote := chunk.Equal(s.vQuote)
		mCR := chunk.Equal(s.vCR)
		mLF := chunk.Equal(s.vLF)

		mAll := mDelim.Or(mQuote).Or(mCR).Or(mLF)
		maskBits := mAll.ToBits()
		if maskBits != 0 {
			return i + bits.TrailingZeros32(maskBits)
		}
		i += 32
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
		for i, b := range chunk {
			bit := uint32(1) << i
			if b == s.delim {
				maskDelim |= bit
			} else if b == s.quote {
				maskQuote |= bit
			} else if b == '\r' || b == '\n' {
				maskEOL |= bit
			}
		}
		return maskDelim, maskQuote, maskEOL
	}
	v := archsimd.LoadUint8x32(chunk[:32])
	mD := v.Equal(s.vDelim)
	mQ := v.Equal(s.vQuote)
	mEOL := v.Equal(s.vCR).Or(v.Equal(s.vLF))
	return mD.ToBits(), mQ.ToBits(), mEOL.ToBits()
}

func findNextSpecial(data []byte, delim byte, quote byte) int {
	s := newBlockScanner(delim, quote)
	return s.scanSpecial(data)
}
