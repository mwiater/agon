package agon

import (
	"fmt"

	"github.com/spf13/viper"
)

func runShowConfig() {
	file := viper.ConfigFileUsed()
	if file == "" {
		fmt.Println("No config file loaded (using defaults).")
	} else {
		fmt.Printf("Config file: %s\n\n", file)
	}

	cfg := GetConfig()
	fmt.Println("Current configuration:")
	if cfg == nil {
		fmt.Printf("  Debug:           %v\n", viper.GetBool("debug"))
		fmt.Printf("  Multimodel Mode: %v\n", viper.GetBool("multimodelMode"))
		fmt.Printf("  Pipeline Mode:   %v\n", viper.GetBool("pipelineMode"))
		fmt.Printf("  JSON Mode:       %v\n", viper.GetBool("jsonMode"))
		fmt.Printf("  MCP Mode:        %v\n", viper.GetBool("mcpMode"))
		fmt.Printf("  MCP Binary:      %s\n", viper.GetString("mcpBinary"))
		fmt.Printf("  MCP Init Timeout: %d seconds\n", viper.GetInt("mcpInitTimeout"))
		fmt.Printf("  Export JSON:     %s\n", viper.GetString("export"))
		fmt.Printf("  Export Markdown: %s\n", viper.GetString("exportMarkdown"))
		return
	}

	fmt.Printf("  Debug:           %v\n", cfg.Debug)
	fmt.Printf("  Multimodel Mode: %v\n", cfg.MultimodelMode)
	fmt.Printf("  Pipeline Mode:   %v\n", cfg.PipelineMode)
	fmt.Printf("  JSON Mode:       %v\n", cfg.JSONMode)
	fmt.Printf("  MCP Mode:        %v\n", cfg.MCPMode)
	fmt.Printf("  RAG Mode:        %v\n", cfg.RagMode)
	fmt.Printf("  MCP Binary:      %s\n", cfg.MCPBinaryPath())
	fmt.Printf("  MCP Init Timeout: %s\n", cfg.MCPInitTimeoutDuration())
	fmt.Printf("  Export JSON:     %s\n", cfg.ExportPath)
	fmt.Printf("  Export Markdown: %s\n", cfg.ExportMarkdownPath)
	if cfg.RagMode {
		fmt.Printf("  RAG Corpus Path: %s\n", cfg.RagCorpusPath)
		fmt.Printf("  RAG Index Path:  %s\n", cfg.RagIndexPath)
		fmt.Printf("  RAG Embedding Model: %s\n", cfg.RagEmbeddingModel)
		fmt.Printf("  RAG Embedding Host:  %s\n", cfg.RagEmbeddingHost)
		fmt.Printf("  RAG Chunk Size Tokens: %d\n", cfg.RagChunkSizeTokens)
		fmt.Printf("  RAG Chunk Overlap Tokens: %d\n", cfg.RagChunkOverlapTokens)
		fmt.Printf("  RAG Top K:       %d\n", cfg.RagTopK)
		fmt.Printf("  RAG Context Token Limit: %d\n", cfg.RagContextTokenLimit)
		fmt.Printf("  RAG Similarity:  %s\n", cfg.RagSimilarity)
		fmt.Printf("  RAG Allowed Extensions: %v\n", cfg.RagAllowedExtensions)
		fmt.Printf("  RAG Exclude Globs: %v\n", cfg.RagExcludeGlobs)
	}
}
