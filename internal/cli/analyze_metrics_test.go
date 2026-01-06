// internal/cli/analyze_metrics_test.go
package agon

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/metrics"
)

func TestParseBenchmarkResultsMap(t *testing.T) {
	raw := []byte(`{"m1":{"modelName":"m1","benchmarkCount":1,"averageStats":{"totalExecutionTime":1,"timeToFirstToken":1,"tokensPerSecond":1,"inputTokenCount":1,"outputTokenCount":1},"minStats":{"totalExecutionTime":1,"timeToFirstToken":1,"tokensPerSecond":1,"inputTokenCount":1,"outputTokenCount":1},"maxStats":{"totalExecutionTime":1,"timeToFirstToken":1,"tokensPerSecond":1,"inputTokenCount":1,"outputTokenCount":1},"iterations":[]}}`)
	results, err := parseBenchmarkResults(raw)
	if err != nil {
		t.Fatalf("parseBenchmarkResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestParseBenchmarkResultsLlamaBench(t *testing.T) {
	raw := []byte(`[{"model_filename":"m1.gguf","n_prompt":10,"n_gen":0,"avg_ns":1000},{"model_filename":"m1.gguf","n_prompt":10,"n_gen":5,"avg_ns":2000}]`)
	results, err := parseBenchmarkResults(raw)
	if err != nil {
		t.Fatalf("parseBenchmarkResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestConvertModelMetrics(t *testing.T) {
	models := []metrics.ModelMetrics{
		{
			ModelName: "m1",
			OverallStats: metrics.RunningAggregatedStats{
				TotalRequests: 2,
				TotalDurationMillis: metrics.RunningStat{Mean: 100, Min: 90, Max: 110},
				TTFTMillis:          metrics.RunningStat{Mean: 10, Min: 9, Max: 11},
				TokensPerSecond:     metrics.RunningStat{Mean: 5, Min: 4, Max: 6},
				InputTokens:         metrics.RunningStat{Mean: 7, Min: 6, Max: 8},
				OutputTokens:        metrics.RunningStat{Mean: 3, Min: 2, Max: 4},
			},
		},
	}
	results := convertModelMetrics(models)
	bench, ok := results["m1"]
	if !ok {
		t.Fatal("expected model m1 in results")
	}
	if bench.BenchmarkCount != 2 {
		t.Fatalf("expected benchmark count 2, got %d", bench.BenchmarkCount)
	}
}

func TestMsToNs(t *testing.T) {
	if got := msToNs(1.5); got != 1500000 {
		t.Fatalf("expected 1500000, got %d", got)
	}
}

func TestRoundToInt(t *testing.T) {
	if got := roundToInt(math.NaN()); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := roundToInt(2.4); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestParseLlamaCppBench(t *testing.T) {
	raw := []byte(`[{"model_filename":"path/m1.gguf","n_prompt":10,"n_gen":0,"avg_ns":1000},{"model_filename":"path/m1.gguf","n_prompt":10,"n_gen":5,"avg_ns":2000}]`)
	results, err := parseLlamaCppBench(raw)
	if err != nil {
		t.Fatalf("parseLlamaCppBench error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestModelNameFromFilename(t *testing.T) {
	if got := modelNameFromFilename("/path/model.gguf"); got != "model" {
		t.Fatalf("expected model, got %q", got)
	}
	if got := modelNameFromFilename("model"); got != "model" {
		t.Fatalf("expected model, got %q", got)
	}
}

func TestBuildLlamaBenchIterations(t *testing.T) {
	prompt := &llamaCppBenchEntry{NPrompt: 10, AvgNs: 1000}
	gen := &llamaCppBenchEntry{NGen: 5, AvgNs: 2000}
	iters := buildLlamaBenchIterations(prompt, gen, 10, 5)
	if len(iters) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(iters))
	}
}

func TestBuildStatsFromIterations(t *testing.T) {
	iters := []metrics.Iteration{
		{Iteration: 1, Stats: metrics.Stats{TotalExecutionTime: 10, TimeToFirstToken: 3, TokensPerSecond: 2, InputTokenCount: 1, OutputTokenCount: 2}},
		{Iteration: 2, Stats: metrics.Stats{TotalExecutionTime: 20, TimeToFirstToken: 4, TokensPerSecond: 4, InputTokenCount: 2, OutputTokenCount: 3}},
	}
	avg, min, max := buildStatsFromIterations(iters)
	if min.TotalExecutionTime != 10 || max.TotalExecutionTime != 20 {
		t.Fatalf("unexpected min/max: %#v %#v", min, max)
	}
	if avg.TotalExecutionTime != 15 {
		t.Fatalf("unexpected avg: %#v", avg)
	}
}

func TestLoadAccuracyStatsAndPerformance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m1.jsonl")
	lines := []string{
		`{"model":"m1","correct":true,"difficulty":1,"marginOfError":0,"deadlineExceeded":false,"deadlineTimeout":5,"time_to_first_token":10,"tokens_per_second":5,"input_tokens":3,"output_tokens":4,"total_duration_ms":20}`,
		`{"model":"m1","correct":false,"difficulty":2,"marginOfError":1,"deadlineExceeded":true,"deadlineTimeout":7,"time_to_first_token":10,"tokens_per_second":5,"input_tokens":3,"output_tokens":4,"total_duration_ms":20}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	stats, err := loadAccuracyStats(dir)
	if err != nil {
		t.Fatalf("loadAccuracyStats error: %v", err)
	}
	if stats["m1"].Total != 1 {
		t.Fatalf("expected total 1, got %d", stats["m1"].Total)
	}

	perf, err := loadAccuracyPerformanceResults(dir)
	if err != nil {
		t.Fatalf("loadAccuracyPerformanceResults error: %v", err)
	}
	if len(perf) != 1 {
		t.Fatalf("expected 1 perf entry, got %d", len(perf))
	}
}

func TestLoadBenchmarksDir(t *testing.T) {
	dir := t.TempDir()
	content := metrics.BenchmarkResults{
		"m1": {ModelName: "m1"},
	}
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bench.json"), raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	results, err := loadBenchmarksDir(dir)
	if err != nil {
		t.Fatalf("loadBenchmarksDir error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
