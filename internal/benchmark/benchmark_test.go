package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Model:One":       "model_one",
		"  Model Two  ":   "model-two",
		"Model--Three!!":  "model-three",
		"__Mixed__Case__": "mixed__case",
	}
	for input, expected := range cases {
		if got := Slugify(input); got != expected {
			t.Fatalf("Slugify(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestCalculateAggregates(t *testing.T) {
	result := &BenchmarkResult{
		Iterations: []IterationResult{
			{Stats: IterationStats{TotalExecutionTime: 2 * time.Second, TimeToFirstToken: 200 * time.Millisecond, TokensPerSecond: 5}},
			{Stats: IterationStats{TotalExecutionTime: 1 * time.Second, TimeToFirstToken: 100 * time.Millisecond, TokensPerSecond: 7}},
			{Stats: IterationStats{TotalExecutionTime: 3 * time.Second, TimeToFirstToken: 300 * time.Millisecond, TokensPerSecond: 4}},
		},
	}

	calculateAggregates(result)

	if result.MinStats.TotalExecutionTime != 1*time.Second {
		t.Fatalf("min total execution: %v", result.MinStats.TotalExecutionTime)
	}
	if result.MaxStats.TotalExecutionTime != 3*time.Second {
		t.Fatalf("max total execution: %v", result.MaxStats.TotalExecutionTime)
	}
	if result.AverageStats.TotalExecutionTime != 2*time.Second {
		t.Fatalf("average total execution: %v", result.AverageStats.TotalExecutionTime)
	}
	if result.MinStats.TokensPerSecond != 4 || result.MaxStats.TokensPerSecond != 7 {
		t.Fatalf("tokens per second bounds: %+v", result)
	}
}

func TestWriteResults(t *testing.T) {
	tempDir := t.TempDir()
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	results := map[string]*BenchmarkResult{
		"Model-One": {
			ModelName:      "Model-One",
			BenchmarkCount: 2,
			Iterations: []IterationResult{
				{Iteration: 1},
			},
		},
	}
	if err := writeResults(results, 2); err != nil {
		t.Fatalf("writeResults: %v", err)
	}

	expectedName := filepath.Join("agonData", "modelBenchmarks", "model-one-2.json")
	data, err := os.ReadFile(expectedName)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !strings.Contains(string(data), "Model-One") {
		t.Fatalf("expected model name in output: %s", string(data))
	}
	var decoded map[string]BenchmarkResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
}
