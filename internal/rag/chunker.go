package rag

import "strings"

// ChunkText splits text into overlapping chunks using word counts as a proxy for tokens.
func ChunkText(text string, chunkSize, overlap int) []chunk {
	if chunkSize <= 0 {
		return nil
	}
	if overlap < 0 {
		overlap = 0
	}
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []chunk
	for i := 0; i < len(words); i += step {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, chunk{
			Offset: i,
			Text:   strings.Join(words[i:end], " "),
			Tokens: end - i,
		})
		if end == len(words) {
			break
		}
	}
	return chunks
}

type chunk struct {
	Offset int
	Text   string
	Tokens int
}
