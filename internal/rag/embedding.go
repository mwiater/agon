package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// EmbedText requests an embedding vector from the configured embedding model.
func EmbedText(ctx context.Context, client *http.Client, host appconfig.Host, model, text string, timeout time.Duration) ([]float64, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("rag embedding model is empty")
	}
	payload := map[string]any{
		"model":  model,
		"prompt": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host.URL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding request failed: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse embedding response: %w", err)
	}
	if len(parsed.Embedding) == 0 {
		return nil, fmt.Errorf("embedding response returned empty vector")
	}

	return parsed.Embedding, nil
}
