// benchmark/types.go 

package benchmark

import "time"

type BenchmarkResult struct {
	ModelName         string            `json:"modelName"`
	BenchmarkCount    int               `json:"benchmarkCount"`
	AverageStats      IterationStats    `json:"averageStats"`
	MinStats          IterationStats    `json:"minStats"`
	MaxStats          IterationStats    `json:"maxStats"`
	Iterations        []IterationResult `json:"iterations"`
}

type IterationResult struct {
	Iteration        int           `json:"iteration"`
	Stats            IterationStats `json:"stats"`
}

type IterationStats struct {
	TotalExecutionTime time.Duration `json:"totalExecutionTime"`
	TimeToFirstToken   time.Duration `json:"timeToFirstToken"`
	TokensPerSecond    float64       `json:"tokensPerSecond"`
	InputTokenCount    int           `json:"inputTokenCount"`
	OutputTokenCount   int           `json:"outputTokenCount"`
}
