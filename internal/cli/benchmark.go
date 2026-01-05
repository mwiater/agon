// internal/cli/benchmark.go
package agon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mwiater/agon/benchmark"
	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

// benchmarkCmd represents the benchmark command.
var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run benchmarks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("benchmark command called")
		metrics.GetInstance().SetMetricsEnabled(true) // Enable metrics for benchmark mode
		cfg := GetConfig()
		if cfg == nil {
			log.Println("config is nil")
			return nil
		}
		log.Printf("benchmark mode: %v", cfg.BenchmarkMode)
		return benchmark.BenchmarkModels(GetConfig())
	},
}

var benchmarkModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Run a single benchmark against a benchmark server endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName, _ := cmd.Flags().GetString("model")
		gpuName, _ := cmd.Flags().GetString("gpu")
		endpoint, _ := cmd.Flags().GetString("benchmark-endpoint")

		fileBase := benchmark.Slugify(fmt.Sprintf("%s_%s", gpuName, modelName))
		fileName := filepath.Join("benchmark", "benchmarks", fileBase+".json")
		if _, err := os.Stat(fileName); err == nil {
			log.Printf("Benchmark already exists, skipping: %s", fileName)
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check benchmark file: %w", err)
		}

		if err := os.MkdirAll(filepath.Join("benchmark", "benchmarks"), 0o755); err != nil {
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
	},
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)
	benchmarkCmd.AddCommand(benchmarkModelCmd)

	benchmarkModelCmd.Flags().StringP("model", "m", "", "model name to benchmark")
	benchmarkModelCmd.Flags().StringP("gpu", "g", "", "GPU name for output filename")
	benchmarkModelCmd.Flags().StringP("benchmark-endpoint", "b", "", "benchmark server endpoint URL")
	_ = benchmarkModelCmd.MarkFlagRequired("model")
	_ = benchmarkModelCmd.MarkFlagRequired("gpu")
	_ = benchmarkModelCmd.MarkFlagRequired("benchmark-endpoint")
}
