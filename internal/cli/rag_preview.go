package agon

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mwiater/agon/internal/rag"
	"github.com/spf13/cobra"
)

// ragPreviewCmd previews RAG retrieval and context assembly for a query.
var ragPreviewCmd = &cobra.Command{
	Use:   "preview <query>",
	Short: "Preview RAG retrieval and context assembly",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return fmt.Errorf("query is required")
		}

		cfg := GetConfig()
		if cfg == nil {
			return fmt.Errorf("config is nil")
		}

		status := func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			log.Print(msg)
			fmt.Println(msg)
		}

		status("[RAG] Preview query: %s", query)
		status("[RAG] ragMode: %v", cfg.RagMode)
		status("[RAG] corpus: %s", cfg.RagCorpusPath)
		status("[RAG] index: %s", cfg.RagIndexPath)
		status("[RAG] embedding model: %s", cfg.RagEmbeddingModel)
		status("[RAG] embedding host: %s", cfg.RagEmbeddingHost)
		status("[RAG] chunk size: %d tokens, overlap: %d tokens", cfg.RagChunkSizeTokens, cfg.RagChunkOverlapTokens)
		status("[RAG] topK: %d", cfg.RagTopK)
		status("[RAG] context token limit: %d", cfg.RagContextTokenLimit)
		status("[RAG] similarity: %s", cfg.RagSimilarity)

		result, err := rag.Retrieve(context.Background(), cfg, query)
		if err != nil {
			return err
		}

		status("[RAG] retrieval_ms: %d", result.RetrievalMs)
		status("[RAG] context_tokens: %d", result.ContextTokens)
		status("[RAG] source_coverage: %d", result.SourceCoverage)
		status("[RAG] chunks: %d", len(result.Chunks))

		for i, chunk := range result.Chunks {
			status("[RAG] chunk %d score=%.6f doc=%s offset=%d tokens=%d", i+1, chunk.Score, chunk.Entry.Doc, chunk.Entry.Offset, chunk.Entry.TokenCount)
			status("[RAG] chunk %d text: %s", i+1, chunk.Entry.Text)
		}

		if result.Context != "" {
			status("[RAG] context:\n%s", result.Context)
		}

		return nil
	},
}

func init() {
	ragCmd.AddCommand(ragPreviewCmd)
}
