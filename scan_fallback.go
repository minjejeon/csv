//go:build !goexperiment.simd

package csv

func findNextSpecial(data []byte, delim byte, quote byte) int {
	return findNextSpecialFallback(data, delim, quote)
}
