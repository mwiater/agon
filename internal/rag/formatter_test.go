package rag

import "testing"

func TestFormatContextRespectsTokenLimit(t *testing.T) {
	chunks := []RetrievedChunk{
		{Entry: IndexEntry{Doc: "a.md", Text: "one two three four"}},
		{Entry: IndexEntry{Doc: "b.md", Text: "five six seven"}},
	}

	context, tokens, sources := FormatContext(chunks, 5)
	if tokens != 5 {
		t.Fatalf("expected 5 tokens, got %d", tokens)
	}
	if sources != 2 {
		t.Fatalf("expected 2 sources, got %d", sources)
	}
	if context == "" {
		t.Fatalf("expected context to be non-empty")
	}
}

func TestFormatContextNoChunks(t *testing.T) {
	context, tokens, sources := FormatContext(nil, 10)
	if context != "" || tokens != 0 || sources != 0 {
		t.Fatalf("expected empty result when no chunks")
	}
}
