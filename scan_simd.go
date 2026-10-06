//go:build goexperiment.simd

package csv

import (
	"math/bits"
	"simd"
	"simd/archsimd"
)

func findNextSpecial(data []byte, delim byte, quote byte) int {
	if simd.Emulated() {
		return findNextSpecialFallback(data, delim, quote)
	}

	n := len(data)
	if n == 0 {
		return -1
	}

	vDelim := simd.BroadcastUint8s(delim)
	vQuote := simd.BroadcastUint8s(quote)
	vCR := simd.BroadcastUint8s('\r')
	vLF := simd.BroadcastUint8s('\n')
	vecLen := vDelim.Len()

	i := 0
	for i+vecLen <= n {
		chunk := simd.LoadUint8s(data[i : i+vecLen])
		mDelim := chunk.Equal(vDelim)
		mQuote := chunk.Equal(vQuote)
		mCR := chunk.Equal(vCR)
		mLF := chunk.Equal(vLF)

		mAll := mDelim.Or(mQuote).Or(mCR).Or(mLF)

		switch a := mAll.ToArch().(type) {
		case archsimd.Mask8x32:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros32(maskBits)
			}
		case archsimd.Mask8x16:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros16(maskBits)
			}
		case archsimd.Mask8x64:
			maskBits := a.ToBits()
			if maskBits != 0 {
				return i + bits.TrailingZeros64(maskBits)
			}
		default:
			s := mAll.ToInt8s()
			var buf [64]int8
			s.StorePart(buf[:vecLen])
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
		if b == delim || b == quote || b == '\r' || b == '\n' {
			return i
		}
	}
	return -1
}
