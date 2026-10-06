//go:build goexperiment.simd && amd64

package csv

import (
	"math/bits"
	"simd/archsimd"
)

func findNextSpecial(data []byte, delim byte, quote byte) int {
	if !archsimd.X86.AVX2() {
		return findNextSpecialFallback(data, delim, quote)
	}

	n := len(data)
	if n == 0 {
		return -1
	}

	vDelim := archsimd.BroadcastUint8x32(delim)
	vQuote := archsimd.BroadcastUint8x32(quote)
	vCR := archsimd.BroadcastUint8x32('\r')
	vLF := archsimd.BroadcastUint8x32('\n')

	i := 0
	for i+32 <= n {
		chunk := archsimd.LoadUint8x32(data[i : i+32])
		mDelim := chunk.Equal(vDelim)
		mQuote := chunk.Equal(vQuote)
		mCR := chunk.Equal(vCR)
		mLF := chunk.Equal(vLF)

		mAll := mDelim.Or(mQuote).Or(mCR).Or(mLF)
		maskBits := mAll.ToBits()
		if maskBits != 0 {
			tz := bits.TrailingZeros32(maskBits)
			return i + tz
		}
		i += 32
	}

	for ; i < n; i++ {
		b := data[i]
		if b == delim || b == quote || b == '\r' || b == '\n' {
			return i
		}
	}
	return -1
}
