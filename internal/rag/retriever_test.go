package rag

import "testing"

func TestScoreEntriesOrdersBySimilarity(t *testing.T) {
	entries := []IndexEntry{
		{Doc: "a", Embedding: []float64{1, 0}},
		{Doc: "b", Embedding: []float64{0, 1}},
		{Doc: "c", Embedding: []float64{1, 1}},
	}
	query := []float64{1, 0}

	chunks := scoreEntries(entries, query)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Entry.Doc != "a" {
		t.Fatalf("expected top doc a, got %s", chunks[0].Entry.Doc)
	}
}
