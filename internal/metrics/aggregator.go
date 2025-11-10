// internal/metrics/aggregator.go
package metrics

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
)

// Aggregator collects and manages performance metrics for models.
type Aggregator struct {
	mutex    sync.Mutex
	metrics  map[string]*ModelMetrics
	filePath string
	ticker   *time.Ticker
}

var (
	instance *Aggregator
	once     sync.Once
)

// GetInstance returns the singleton instance of the Aggregator.
func GetInstance() *Aggregator {
	once.Do(func() {
		instance = NewAggregator()
	})
	return instance
}

// NewAggregator creates and initializes a new Aggregator.
func NewAggregator() *Aggregator {
	agg := &Aggregator{
		metrics:  make(map[string]*ModelMetrics),
		filePath: "reports/data/model_performance_metrics.json",
	}

	agg.load()

	agg.ticker = time.NewTicker(1 * time.Minute)
	go func() {
		for range agg.ticker.C {
			agg.save()
		}
	}()

	return agg
}

// load reads metrics from the JSON file into memory.
func (a *Aggregator) load() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return
	}

	var metricsSlice []*ModelMetrics
	if err := json.Unmarshal(data, &metricsSlice); err != nil {
		return
	}

	for _, m := range metricsSlice {
		a.metrics[m.ModelName] = m
	}
}

// save writes the current metrics from memory to the JSON file.
func (a *Aggregator) save() {
	logging.LogEvent("[METRICS] Saving metrics to %s", a.filePath)
	a.mutex.Lock()
	defer a.mutex.Unlock()

	var metricsSlice []*ModelMetrics
	for _, m := range a.metrics {
		metricsSlice = append(metricsSlice, m)
	}

	data, err := json.MarshalIndent(metricsSlice, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(a.filePath, data, 0644)
}

// Record updates the metrics for a given model with new data.
func (a *Aggregator) Record(meta providers.StreamMetadata, ttft int64) {
	logging.LogEvent("[METRICS] Record called for model %s", meta.Model)
	a.mutex.Lock()
	defer a.mutex.Unlock()

	modelMetrics, exists := a.metrics[meta.Model]
	if !exists {
		modelMetrics = &ModelMetrics{
			ModelName: meta.Model,
		}
		a.metrics[meta.Model] = modelMetrics
	}

	modelMetrics.LastUpdatedUTC = time.Now().UTC()

	updateStats(&modelMetrics.OverallStats, meta, ttft)

	bucket := getBucket(meta.PromptEvalCount)
	found := false
	for i := range modelMetrics.PerformanceBuckets {
		if modelMetrics.PerformanceBuckets[i].Dimension == "input_tokens" && modelMetrics.PerformanceBuckets[i].Bucket == bucket {
			updateStats(&modelMetrics.PerformanceBuckets[i].Stats, meta, ttft)
			found = true
			break
		}
	}
	if !found {
		newBucket := PerformanceBucket{
			Dimension: "input_tokens",
			Bucket:    bucket,
			Stats:     RunningAggregatedStats{},
		}
		updateStats(&newBucket.Stats, meta, ttft)
		modelMetrics.PerformanceBuckets = append(modelMetrics.PerformanceBuckets, newBucket)
	}
}

// updateStats updates the running statistics with new metadata.
func updateStats(stats *RunningAggregatedStats, meta providers.StreamMetadata, ttft int64) {
	stats.TotalRequests++
	updateRunningStat(&stats.TTFTMillis, float64(ttft))

	var tokensPerSecond float64
	if meta.EvalDuration > 0 {
		tokensPerSecond = float64(meta.EvalCount) / (float64(meta.EvalDuration) / 1e9)
	}
	updateRunningStat(&stats.TokensPerSecond, tokensPerSecond)

	updateRunningStat(&stats.InputTokens, float64(meta.PromptEvalCount))
	updateRunningStat(&stats.OutputTokens, float64(meta.EvalCount))
	updateRunningStat(&stats.TotalDurationMillis, float64(meta.TotalDuration/1e6))
}

// updateRunningStat updates a single running statistic using Welford's online algorithm.
func updateRunningStat(rs *RunningStat, value float64) {
	rs.Count++
	if rs.Count == 1 {
		rs.Min = value
		rs.Max = value
	} else {
		if value < rs.Min {
			rs.Min = value
		}
		if value > rs.Max {
			rs.Max = value
		}
	}

	delta := value - rs.Mean
	rs.Mean += delta / float64(rs.Count)
	delta2 := value - rs.Mean
	rs.M2 += delta * delta2
}

// getBucket determines the appropriate performance bucket for a given number of input tokens.
func getBucket(inputTokens int) string {
	switch {
	case inputTokens <= 256:
		return "0-256"
	case inputTokens <= 1024:
		return "257-1024"
	case inputTokens <= 4096:
		return "1025-4096"
	case inputTokens <= 8192:
		return "4097-8192"
	default:
		return "8192+"
	}
}

// Close stops the ticker and saves the metrics.
func (a *Aggregator) Close() {
	a.ticker.Stop()
	a.save()
}

// Close gracefully shuts down the singleton aggregator instance.
func Close() {
	if instance != nil {
		instance.Close()
	}
}