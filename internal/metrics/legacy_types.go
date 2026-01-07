// internal/metrics/legacy_types.go
package metrics

// Stats mirrors the per-iteration statistics captured in benchmark JSON.
type Stats struct {
	TotalExecutionTime int64   `json:"totalExecutionTime"`
	TimeToFirstToken   int64   `json:"timeToFirstToken"`
	TokensPerSecond    float64 `json:"tokensPerSecond"`
	InputTokenCount    int     `json:"inputTokenCount"`
	OutputTokenCount   int     `json:"outputTokenCount"`
}

// Iteration captures a single benchmark run for a model.
type Iteration struct {
	Iteration int   `json:"iteration"`
	Stats     Stats `json:"stats"`
}

// ModelBenchmark is the root payload for a model's benchmark record.
type ModelBenchmark struct {
	ModelName      string      `json:"modelName"`
	BenchmarkCount int         `json:"benchmarkCount"`
	AverageStats   Stats       `json:"averageStats"`
	MinStats       Stats       `json:"minStats"`
	MaxStats       Stats       `json:"maxStats"`
	Iterations     []Iteration `json:"iterations"`
}

// BenchmarkResults stores the entire benchmark document keyed by model name.
type BenchmarkResults map[string]ModelBenchmark

// AccuracyStats stores aggregated correctness information for a model.
type AccuracyStats struct {
	Total            int                    `json:"total"`
	Correct          int                    `json:"correct"`
	Accuracy         float64                `json:"accuracy"`
	AvgDifficulty    float64                `json:"avgDifficulty"`
	AvgMarginOfError float64                `json:"avgMarginOfError"`
	Timeouts         int                    `json:"timeouts"`
	TimeoutSeconds   int                    `json:"timeoutSeconds"`
	ByDifficulty     map[int]AccuracyBucket `json:"byDifficulty"`
}
