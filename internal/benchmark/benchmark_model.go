package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// RunBenchmarkModel is the CLI entry point for a single-model benchmark request.
func RunBenchmarkModel(modelName, gpuName, endpoint string) error {
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(gpuName) == "" {
		return fmt.Errorf("gpu name is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("benchmark endpoint is required")
	}

	fileBase := Slugify(fmt.Sprintf("%s_%s", gpuName, modelName))
	fileName := filepath.Join("agonData", "modelBenchmarks", fileBase+".json")
	if _, err := os.Stat(fileName); err == nil {
		log.Printf("Benchmark already exists, skipping: %s", fileName)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check benchmark file: %w", err)
	}

	if err := os.MkdirAll(filepath.Join("agonData", "modelBenchmarks"), 0o755); err != nil {
		return fmt.Errorf("create benchmarks directory: %w", err)
	}

	payload, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return fmt.Errorf("marshal benchmark payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("benchmark request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("benchmark request failed with status %d: %s", resp.StatusCode, string(bytes.TrimSpace(body)))
	}

	if !json.Valid(body) {
		return fmt.Errorf("benchmark response is not valid JSON")
	}

	if err := os.WriteFile(fileName, body, 0o644); err != nil {
		return fmt.Errorf("write benchmark results: %w", err)
	}

	log.Printf("Benchmark results written to %s", fileName)
	return nil
}
