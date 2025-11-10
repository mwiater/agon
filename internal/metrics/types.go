// internal/metrics/types.go
package metrics

import "time"

// ModelMetrics is the top-level document for a single model's aggregated data.
type ModelMetrics struct {
	ModelName          string                 `json:"model_name"`
	LastUpdatedUTC     time.Time              `json:"last_updated_utc"`
	OverallStats       RunningAggregatedStats `json:"overall_stats"`
	PerformanceBuckets []PerformanceBucket    `json:"performance_buckets"`
}

// PerformanceBucket holds aggregated stats for a specific dimension, like input token count.
type PerformanceBucket struct {
	Dimension string                 `json:"dimension"`
	Bucket    string                 `json:"bucket"`
	Stats     RunningAggregatedStats `json:"stats"`
}

// RunningAggregatedStats stores the running statistical values for a set of metrics.
// It uses Welford's online algorithm for calculating mean and standard deviation.
type RunningAggregatedStats struct {
	TotalRequests int64 `json:"total_requests"`

	TTFTMillis          RunningStat `json:"ttft_ms"`
	TokensPerSecond     RunningStat `json:"tokens_per_second"`
	InputTokens         RunningStat `json:"input_tokens"`
	OutputTokens        RunningStat `json:"output_tokens"`
	TotalDurationMillis RunningStat `json:"total_duration_ms"`
}

// RunningStat holds the necessary values for online calculation of mean, variance, and stddev.
type RunningStat struct {
	Count int64   `json:"-"`
	Mean  float64 `json:"mean"`
	M2    float64 `json:"-"` // Sum of squares of differences from the current mean
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}