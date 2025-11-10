// benchmark/types.go
package benchmark

import "time"

// BenchmarkResult holds the aggregated results for a single model's benchmark.
type BenchmarkResult struct {
	ModelName      string            `json:"modelName"`
	BenchmarkCount int               `json:"benchmarkCount"`
	AverageStats   IterationStats    `json:"averageStats"`
	MinStats       IterationStats    `json:"minStats"`
	MaxStats       IterationStats    `json:"maxStats"`
	Iterations     []IterationResult `json:"iterations"`
}

// IterationResult holds the statistics for a single benchmark iteration.
type IterationResult struct {
	Iteration int           `json:"iteration"`
	Stats     IterationStats `json:"stats"`
}

// IterationStats contains the detailed performance metrics for one iteration.
type IterationStats struct {
	TotalExecutionTime time.Duration `json:"totalExecutionTime"`
	TimeToFirstToken   time.Duration `json:"timeToFirstToken"`
	TokensPerSecond    float64       `json:"tokensPerSecond"`
	InputTokenCount    int           `json:"inputTokenCount"`
	OutputTokenCount   int           `json:"outputTokenCount"`
}