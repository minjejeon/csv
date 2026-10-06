package csv

import (
	"bytes"
)

type chunkSpan struct {
	start int
	end   int
}

// splitChunks divides data into approximately numWorkers chunks such that every chunk
// boundary (except 0 and len(data)) begins cleanly at the start of a record outside any quotes.
func splitChunks(data []byte, numWorkers int, quote ...byte) ([]chunkSpan, error) {
	n := len(data)
	if n == 0 {
		return nil, nil
	}
	if numWorkers <= 1 || n < 1024 {
		return []chunkSpan{{start: 0, end: n}}, nil
	}

	q := byte('"')
	if len(quote) > 0 && quote[0] != 0 {
		q = quote[0]
	}
	cutset := string([]byte{'\n', q})

	targetChunkSize := n / numWorkers
	if targetChunkSize < 256 {
		targetChunkSize = 256
	}

	boundaries := []int{0}

	// Fast path: if there are no quotes in data, jump straight to target positions using IndexByte
	if bytes.IndexByte(data, q) == -1 {
		for len(boundaries) < numWorkers {
			nextTarget := boundaries[len(boundaries)-1] + targetChunkSize
			if nextTarget >= n {
				break
			}
			idx := bytes.IndexByte(data[nextTarget:], '\n')
			if idx < 0 {
				break
			}
			boundary := nextTarget + idx + 1
			boundaries = append(boundaries, boundary)
		}
		if boundaries[len(boundaries)-1] != n {
			boundaries = append(boundaries, n)
		}
		spans := make([]chunkSpan, len(boundaries)-1)
		for i := 0; i < len(boundaries)-1; i++ {
			spans[i] = chunkSpan{start: boundaries[i], end: boundaries[i+1]}
		}
		return spans, nil
	}

	currentPos := 0
	inQuote := false
	currentTarget := targetChunkSize

	for currentPos < n {
		// Look for next quote or newline
		idx := bytes.IndexAny(data[currentPos:], cutset)
		if idx < 0 {
			break
		}

		matchPos := currentPos + idx
		c := data[matchPos]

		if c == q {
			if !inQuote {
				inQuote = true
				currentPos = matchPos + 1
			} else {
				// Check for escaped quote qq inside quoted field
				if matchPos+1 < n && data[matchPos+1] == q {
					currentPos = matchPos + 2
					continue
				}
				inQuote = false
				currentPos = matchPos + 1
			}
		} else if c == '\n' {
			currentPos = matchPos + 1
			if !inQuote {
				// We are outside quotes at a valid line ending!
				if currentPos >= currentTarget && len(boundaries) < numWorkers {
					boundaries = append(boundaries, currentPos)
					currentTarget = currentPos + targetChunkSize
				}
			}
		}
	}

	// If the last boundary is not at EOF, EOF is the end
	if boundaries[len(boundaries)-1] != n {
		boundaries = append(boundaries, n)
	}

	// Build chunk spans
	var chunks []chunkSpan
	for i := 0; i < len(boundaries)-1; i++ {
		start := boundaries[i]
		end := boundaries[i+1]
		if start < end {
			chunks = append(chunks, chunkSpan{start: start, end: end})
		}
	}

	return chunks, nil
}
