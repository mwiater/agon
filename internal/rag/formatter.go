package rag

import (
	"fmt"
	"strings"
)

// FormatContext builds the CONTEXT block and returns context text, token count, and source coverage.
func FormatContext(chunks []RetrievedChunk, maxTokens int) (string, int, int) {
	if len(chunks) == 0 {
		return "", 0, 0
	}
	if maxTokens < 0 {
		maxTokens = 0
	}

	var b strings.Builder
	b.WriteString("CONTEXT\n")

	contextTokens := 0
	remaining := maxTokens
	sourceSet := make(map[string]struct{})

	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Entry.Text)
		if text == "" {
			continue
		}
		doc := chunk.Entry.Doc

		if maxTokens > 0 {
			if remaining <= 0 {
				break
			}
			if tokens := estimateTokens(text); tokens > remaining {
				text = truncateToTokens(text, remaining)
			}
		}

		usedTokens := estimateTokens(text)
		if usedTokens == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf("[doc:%s] %s\n", doc, text))
		contextTokens += usedTokens
		if maxTokens > 0 {
			remaining -= usedTokens
		}
		if _, ok := sourceSet[doc]; !ok {
			sourceSet[doc] = struct{}{}
		}
	}

	return strings.TrimRight(b.String(), "\n"), contextTokens, len(sourceSet)
}

func estimateTokens(text string) int {
	return len(strings.Fields(text))
}

func truncateToTokens(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	parts := strings.Fields(text)
	if len(parts) <= maxTokens {
		return text
	}
	return strings.Join(parts[:maxTokens], " ")
}
