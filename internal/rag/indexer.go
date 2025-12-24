package rag

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

// BuildIndex builds a JSONL index from the configured RAG corpus.
func BuildIndex(ctx context.Context, cfg *appconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if !cfg.RagMode {
		return fmt.Errorf("ragMode is disabled in the configuration")
	}
	if strings.TrimSpace(cfg.RagCorpusPath) == "" {
		return fmt.Errorf("ragCorpusPath is required when ragMode is enabled")
	}
	if strings.TrimSpace(cfg.RagIndexPath) == "" {
		return fmt.Errorf("ragIndexPath is required when ragMode is enabled")
	}
	if strings.TrimSpace(cfg.RagEmbeddingModel) == "" {
		return fmt.Errorf("ragEmbeddingModel is required when ragMode is enabled")
	}
	if cfg.RagChunkSizeTokens <= 0 {
		return fmt.Errorf("ragChunkSizeTokens must be greater than zero")
	}
	if cfg.RagChunkOverlapTokens < 0 {
		return fmt.Errorf("ragChunkOverlapTokens must be zero or greater")
	}
	if cfg.RagChunkOverlapTokens >= cfg.RagChunkSizeTokens {
		return fmt.Errorf("ragChunkOverlapTokens must be smaller than ragChunkSizeTokens")
	}

	host, err := embeddingHost(cfg)
	if err != nil {
		return err
	}

	start := time.Now()
	status := func(format string, args ...any) {
		elapsed := time.Since(start).Truncate(time.Millisecond)
		msg := fmt.Sprintf("[%s] %s", elapsed, fmt.Sprintf(format, args...))
		log.Print(msg)
		fmt.Println(msg)
	}
	status("[RAG] Indexing corpus: %s", cfg.RagCorpusPath)
	status("[RAG] Index output: %s", cfg.RagIndexPath)
	status("[RAG] Embedding model: %s (host: %s)", cfg.RagEmbeddingModel, host.Name)
	status("[RAG] Chunk size: %d tokens, overlap: %d tokens", cfg.RagChunkSizeTokens, cfg.RagChunkOverlapTokens)

	files, err := discoverCorpusFiles(cfg.RagCorpusPath, cfg.RagAllowedExtensions, cfg.RagExcludeGlobs)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no corpus files found under %s", cfg.RagCorpusPath)
	}
	status("[RAG] Discovered %d corpus files", len(files))

	if err := os.MkdirAll(filepath.Dir(cfg.RagIndexPath), 0755); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	out, err := os.Create(cfg.RagIndexPath)
	if err != nil {
		return fmt.Errorf("create index file: %w", err)
	}
	defer out.Close()

	writer := bufio.NewWriter(out)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	client := &http.Client{Timeout: cfg.RequestTimeout()}
	timeout := cfg.RequestTimeout()

	for _, path := range files {
		fileStart := time.Now()
		status("[RAG] Reading file: %s", path)
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read corpus file %s: %w", path, err)
		}
		text := strings.TrimSpace(string(raw))
		if text == "" {
			status("[RAG] Skipping empty file: %s", path)
			continue
		}
		docName := filepath.Base(path)
		chunks := ChunkText(text, cfg.RagChunkSizeTokens, cfg.RagChunkOverlapTokens)
		status("[RAG] Chunked %s into %d chunks", docName, len(chunks))
		for idx, c := range chunks {
			chunkStart := time.Now()
			status("[RAG] Embedding %s chunk %d/%d", docName, idx+1, len(chunks))
			vector, err := EmbedText(ctx, client, host, cfg.RagEmbeddingModel, c.Text, timeout)
			if err != nil {
				return fmt.Errorf("embed %s chunk %d: %w", docName, idx, err)
			}
			status("[RAG] Embedded %s chunk %d/%d in %s", docName, idx+1, len(chunks), time.Since(chunkStart).Truncate(time.Millisecond))
			entry := IndexEntry{
				ChunkID:    fmt.Sprintf("%s:%d", docName, idx),
				Doc:        docName,
				Offset:     c.Offset,
				Text:       c.Text,
				Embedding:  vector,
				TokenCount: c.Tokens,
			}
			if err := encoder.Encode(entry); err != nil {
				return fmt.Errorf("write index entry: %w", err)
			}
		}
		status("[RAG] Finished %s in %s", docName, time.Since(fileStart).Truncate(time.Millisecond))
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush index: %w", err)
	}

	status("[RAG] Index complete in %s", time.Since(start).Truncate(time.Millisecond))
	return nil
}

func discoverCorpusFiles(root string, allowed []string, exclude []string) ([]string, error) {
	var files []string
	allowedMap := make(map[string]struct{}, len(allowed))
	for _, ext := range allowed {
		allowedMap[strings.ToLower(ext)] = struct{}{}
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldExclude(path, exclude) && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldExclude(path, exclude) {
			return nil
		}

		if len(allowedMap) > 0 {
			ext := strings.ToLower(filepath.Ext(path))
			if _, ok := allowedMap[ext]; !ok {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func shouldExclude(path string, patterns []string) bool {
	normalized := filepath.ToSlash(path)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		pattern = filepath.ToSlash(pattern)
		if strings.Contains(pattern, "**") {
			trimmed := strings.ReplaceAll(pattern, "**", "")
			if trimmed != "" && strings.Contains(normalized, trimmed) {
				return true
			}
		}
		if ok, _ := filepath.Match(pattern, normalized); ok {
			return true
		}
	}
	return false
}
