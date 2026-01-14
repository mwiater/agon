package appconfig

import (
	"fmt"
	"io"
)

// ShowConfig prints the current configuration summary.
func ShowConfig(out io.Writer, file string, cfg *Config, fallback Config) {
	if file == "" {
		fmt.Fprintln(out, "No config file loaded (using defaults).")
	} else {
		fmt.Fprintf(out, "Config file: %s\n\n", file)
	}

	fmt.Fprintln(out, "Current configuration:")
	if cfg == nil {
		fmt.Fprintf(out, "  Debug:           %v\n", fallback.Debug)
		fmt.Fprintf(out, "  Multimodel Mode: %v\n", fallback.MultimodelMode)
		fmt.Fprintf(out, "  Pipeline Mode:   %v\n", fallback.PipelineMode)
		fmt.Fprintf(out, "  JSON Mode:       %v\n", fallback.JSONMode)
		fmt.Fprintf(out, "  MCP Mode:        %v\n", fallback.MCPMode)
		fmt.Fprintf(out, "  MCP Binary:      %s\n", fallback.MCPBinary)
		fmt.Fprintf(out, "  MCP Init Timeout: %d seconds\n", fallback.MCPInitTimeout)
		return
	}

	fmt.Fprintf(out, "  Debug:           %v\n", cfg.Debug)
	fmt.Fprintf(out, "  Multimodel Mode: %v\n", cfg.MultimodelMode)
	fmt.Fprintf(out, "  Pipeline Mode:   %v\n", cfg.PipelineMode)
	fmt.Fprintf(out, "  JSON Mode:       %v\n", cfg.JSONMode)
	fmt.Fprintf(out, "  MCP Mode:        %v\n", cfg.MCPMode)
	fmt.Fprintf(out, "  RAG Mode:        %v\n", cfg.RagMode)
	fmt.Fprintf(out, "  MCP Binary:      %s\n", cfg.MCPBinaryPath())
	fmt.Fprintf(out, "  MCP Init Timeout: %s\n", cfg.MCPInitTimeoutDuration())
	if cfg.RagMode {
		fmt.Fprintf(out, "  RAG Corpus Path: %s\n", cfg.RagCorpusPath)
		fmt.Fprintf(out, "  RAG Index Path:  %s\n", cfg.RagIndexPath)
		fmt.Fprintf(out, "  RAG Embedding Model: %s\n", cfg.RagEmbeddingModel)
		fmt.Fprintf(out, "  RAG Embedding Host:  %s\n", cfg.RagEmbeddingHost)
		fmt.Fprintf(out, "  RAG Chunk Size Tokens: %d\n", cfg.RagChunkSizeTokens)
		fmt.Fprintf(out, "  RAG Chunk Overlap Tokens: %d\n", cfg.RagChunkOverlapTokens)
		fmt.Fprintf(out, "  RAG Top K:       %d\n", cfg.RagTopK)
		fmt.Fprintf(out, "  RAG Context Token Limit: %d\n", cfg.RagContextTokenLimit)
		fmt.Fprintf(out, "  RAG Similarity:  %s\n", cfg.RagSimilarity)
		fmt.Fprintf(out, "  RAG Allowed Extensions: %v\n", cfg.RagAllowedExtensions)
		fmt.Fprintf(out, "  RAG Exclude Globs: %v\n", cfg.RagExcludeGlobs)
	}
}
