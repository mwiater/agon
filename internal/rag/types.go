package rag

// IndexEntry is a single JSONL record in the RAG index.
type IndexEntry struct {
	ChunkID    string    `json:"chunk_id"`
	Doc        string    `json:"doc"`
	Offset     int       `json:"offset"`
	Text       string    `json:"text"`
	Embedding  []float64 `json:"embedding"`
	TokenCount int       `json:"token_count"`
}
