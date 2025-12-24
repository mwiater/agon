package rag

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

// RetrievedChunk is a chunk plus similarity score.
type RetrievedChunk struct {
	Entry IndexEntry
	Score float64
}

// RetrievalResult includes context text and telemetry.
type RetrievalResult struct {
	Context        string
	Chunks         []RetrievedChunk
	RetrievalMs    int
	ContextTokens  int
	SourceCoverage int
}

// Retrieve loads the JSONL index, embeds the query, and returns the top-k context.
func Retrieve(ctx context.Context, cfg *appconfig.Config, query string) (RetrievalResult, error) {
	start := time.Now()
	if cfg == nil {
		return RetrievalResult{}, fmt.Errorf("config is nil")
	}
	if !cfg.RagMode {
		return RetrievalResult{}, fmt.Errorf("ragMode is disabled in the configuration")
	}
	if strings.TrimSpace(cfg.RagIndexPath) == "" {
		return RetrievalResult{}, fmt.Errorf("ragIndexPath is required when ragMode is enabled")
	}
	if strings.TrimSpace(cfg.RagEmbeddingModel) == "" {
		return RetrievalResult{}, fmt.Errorf("ragEmbeddingModel is required when ragMode is enabled")
	}
	if strings.TrimSpace(query) == "" {
		return RetrievalResult{}, fmt.Errorf("query is empty")
	}

	host, err := embeddingHost(cfg)
	if err != nil {
		return RetrievalResult{}, err
	}
	entries, err := loadIndex(cfg.RagIndexPath)
	if err != nil {
		return RetrievalResult{}, err
	}
	if len(entries) == 0 {
		return RetrievalResult{}, fmt.Errorf("rag index contains no entries")
	}

	client := &http.Client{Timeout: cfg.RequestTimeout()}
	queryVec, err := EmbedText(ctx, client, host, cfg.RagEmbeddingModel, query, cfg.RequestTimeout())
	if err != nil {
		return RetrievalResult{}, err
	}

	chunks := scoreEntries(entries, queryVec)
	topK := cfg.RagTopK
	if topK <= 0 {
		topK = 4
	}
	if topK > len(chunks) {
		topK = len(chunks)
	}
	selected := chunks[:topK]

	context, contextTokens, sourceCoverage := FormatContext(selected, cfg.RagContextTokenLimit)

	return RetrievalResult{
		Context:        context,
		Chunks:         selected,
		RetrievalMs:    int(time.Since(start) / time.Millisecond),
		ContextTokens:  contextTokens,
		SourceCoverage: sourceCoverage,
	}, nil
}

func loadIndex(path string) ([]IndexEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open rag index: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 8*1024*1024)

	var entries []IndexEntry
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry IndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse rag index line %d: %w", lineNo, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read rag index: %w", err)
	}

	return entries, nil
}

func scoreEntries(entries []IndexEntry, queryVec []float64) []RetrievedChunk {
	chunks := make([]RetrievedChunk, 0, len(entries))
	queryNorm := vectorNorm(queryVec)
	for _, entry := range entries {
		if len(entry.Embedding) != len(queryVec) {
			continue
		}
		score := cosineSimilarity(queryVec, entry.Embedding, queryNorm)
		chunks = append(chunks, RetrievedChunk{
			Entry: entry,
			Score: score,
		})
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})

	return chunks
}

func cosineSimilarity(a, b []float64, normA float64) float64 {
	if normA == 0 {
		return 0
	}
	normB := vectorNorm(b)
	if normB == 0 {
		return 0
	}
	dot := 0.0
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot / (normA * normB)
}

func vectorNorm(v []float64) float64 {
	sum := 0.0
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}
