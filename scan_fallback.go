//go:build !goexperiment.simd || !amd64

package csv

func findNextSpecial(data []byte, delim byte) int {
	return findNextSpecialFallback(data, delim)
}
