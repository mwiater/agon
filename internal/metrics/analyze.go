// internal/metrics/analyze.go
package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"sort"
	"time"
)

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

// AggregatedStats stores derived aggregate values for a model.
type AggregatedStats struct {
	TokensPerSecond           float64 `json:"tokensPerSecond"`
	TimeToFirstTokenSeconds   float64 `json:"timeToFirstTokenSeconds"`
	TotalExecutionTimeSeconds float64 `json:"totalExecutionTimeSeconds"`
	InputTokens               float64 `json:"inputTokens"`
	OutputTokens              float64 `json:"outputTokens"`
}

// VarianceStats stores standard deviation metrics for key values.
type VarianceStats struct {
	TokensPerSecondStdDev         float64 `json:"tokensPerSecondStdDev"`
	TimeToFirstTokenStdDevSeconds float64 `json:"timeToFirstTokenStdDevSeconds"`
	OutputTokensStdDev            float64 `json:"outputTokensStdDev"`
}

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

// AccuracyBucket stores aggregated accuracy for a difficulty bucket.
type AccuracyBucket struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

// ScoreStats contains normalized scores for each model.
type ScoreStats struct {
	ThroughputScore float64 `json:"throughputScore"`
	LatencyScore    float64 `json:"latencyScore"`
	EfficiencyScore float64 `json:"efficiencyScore"`
	CompositeScore  float64 `json:"compositeScore"`
}

// LabelStats captures qualitative assessments for a model.
type LabelStats struct {
	RelativeSpeedTier      string `json:"relativeSpeedTier"`
	LatencyProfile         string `json:"latencyProfile"`
	Stability              string `json:"stability"`
	InteractiveSuitability string `json:"interactiveSuitability"`
}

// DerivedRatios stores ratio-based comparisons for a model.
type DerivedRatios struct {
	LatencyShareOfTotal float64 `json:"latencyShareOfTotal"`
	RelativeToFastest   float64 `json:"relativeToFastest"`
}

// ModelAnalysis is the top-level entry for each model in the analysis.
type ModelAnalysis struct {
	ModelName      string          `json:"modelName"`
	BenchmarkCount int             `json:"benchmarkCount"`
	Avg            AggregatedStats `json:"avg"`
	Min            AggregatedStats `json:"min"`
	Max            AggregatedStats `json:"max"`
	Variance       VarianceStats   `json:"variance"`
	Accuracy       AccuracyStats   `json:"accuracy"`
	Scores         ScoreStats      `json:"scores"`
	Labels         LabelStats      `json:"labels"`
	DerivedRatios  DerivedRatios   `json:"derivedRatios"`
	ParetoFront    bool            `json:"paretoFront"`
	Notes          []string        `json:"notes"`
	Iterations     []Iteration     `json:"iterations,omitempty"`
}

// ThroughputRankingEntry captures ordering by throughput.
type ThroughputRankingEntry struct {
	ModelName          string  `json:"modelName"`
	AvgTokensPerSecond float64 `json:"avgTokensPerSecond"`
}

// LatencyRankingEntry captures ordering by latency.
type LatencyRankingEntry struct {
	ModelName                  string  `json:"modelName"`
	AvgTimeToFirstTokenSeconds float64 `json:"avgTimeToFirstTokenSeconds"`
}

// EfficiencyRankingEntry captures ordering by efficiency score.
type EfficiencyRankingEntry struct {
	ModelName       string  `json:"modelName"`
	EfficiencyScore float64 `json:"efficiencyScore"`
}

// AccuracyRankingEntry captures ordering by accuracy.
type AccuracyRankingEntry struct {
	ModelName string  `json:"modelName"`
	Accuracy  float64 `json:"accuracy"`
}

// Rankings groups the sorted ranking lists.
type Rankings struct {
	ByThroughput      []ThroughputRankingEntry `json:"byThroughput"`
	ByLatency         []LatencyRankingEntry    `json:"byLatency"`
	ByEfficiencyScore []EfficiencyRankingEntry `json:"byEfficiencyScore"`
	ByAccuracy        []AccuracyRankingEntry   `json:"byAccuracy"`
}

// Anomaly describes any notable outlier detected in the analysis.
type Anomaly struct {
	Type      string `json:"type"`
	ModelName string `json:"modelName"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
}

// OverallSummary provides a concise overview for the report header.
type OverallSummary struct {
	FastestModel       string   `json:"fastestModel"`
	BestLatencyModel   string   `json:"bestLatencyModel"`
	MostEfficientModel string   `json:"mostEfficientModel"`
	MostAccurateModel  string   `json:"mostAccurateModel"`
	BestTradeoffModel  string   `json:"bestTradeoffModel"`
	SummaryNotes       []string `json:"summaryNotes"`
}

// HostInfo is optional metadata describing the environment.
type HostInfo struct {
	ClusterName string `json:"clusterName"`
	Notes       string `json:"notes"`
}

// Analysis is the root document returned by AnalyzeMetrics and consumed by GenerateReport.
type Analysis struct {
	GeneratedAt     time.Time       `json:"generatedAt"`
	HostInfo        HostInfo        `json:"hostInfo"`
	Overall         OverallSummary  `json:"overall"`
	Models          []ModelAnalysis `json:"models"`
	Rankings        Rankings        `json:"rankings"`
	Anomalies       []Anomaly       `json:"anomalies"`
	Recommendations []string        `json:"recommendations"`
}

// ReportTemplateData feeds the HTML template for metric reports.
type ReportTemplateData struct {
	Title        string
	AnalysisJSON template.JS
}

// AnalyzeMetrics transforms raw benchmark results into a structured Analysis object.
func AnalyzeMetrics(results BenchmarkResults, host HostInfo, accuracy map[string]AccuracyStats) Analysis {
	analysis := Analysis{
		GeneratedAt: time.Now().UTC(),
		HostInfo:    host,
	}

	if len(results) == 0 {
		analysis.Models = []ModelAnalysis{}
		analysis.Recommendations = []string{"Consider keeping sessions warm and reducing context size to minimize TTFT across all models."}
		return analysis
	}

	modelNames := make([]string, 0, len(results))
	for name := range results {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	globalMaxAvgTPS := 0.0
	globalMinAvgTTFT := math.MaxFloat64

	modelAnalyses := make([]*ModelAnalysis, 0, len(modelNames))

	for _, name := range modelNames {
		bench := results[name]
		ma := &ModelAnalysis{
			ModelName:      name,
			BenchmarkCount: bench.BenchmarkCount,
		}
		if ma.BenchmarkCount == 0 {
			ma.BenchmarkCount = len(bench.Iterations)
		}

		iterTPS := make([]float64, 0, len(bench.Iterations))
		iterTTFT := make([]float64, 0, len(bench.Iterations))
		iterOutputTokens := make([]float64, 0, len(bench.Iterations))
		iterInputTokens := make([]float64, 0, len(bench.Iterations))
		iterTotalExec := make([]float64, 0, len(bench.Iterations))

		for _, iter := range bench.Iterations {
			iterTPS = append(iterTPS, iter.Stats.TokensPerSecond)
			iterTTFT = append(iterTTFT, nsToSeconds(iter.Stats.TimeToFirstToken))
			iterOutputTokens = append(iterOutputTokens, float64(iter.Stats.OutputTokenCount))
			iterInputTokens = append(iterInputTokens, float64(iter.Stats.InputTokenCount))
			iterTotalExec = append(iterTotalExec, nsToSeconds(iter.Stats.TotalExecutionTime))
		}

		avgTPS := bench.AverageStats.TokensPerSecond
		if avgTPS == 0 {
			avgTPS = meanFloat64(iterTPS)
		}

		avgTTFT := nsToSeconds(bench.AverageStats.TimeToFirstToken)
		if avgTTFT == 0 {
			avgTTFT = meanFloat64(iterTTFT)
		}

		avgTotalExec := nsToSeconds(bench.AverageStats.TotalExecutionTime)
		if avgTotalExec == 0 {
			avgTotalExec = meanFloat64(iterTotalExec)
		}

		avgInputTokens := fallbackAverage(float64(bench.AverageStats.InputTokenCount), iterInputTokens)
		avgOutputTokens := fallbackAverage(float64(bench.AverageStats.OutputTokenCount), iterOutputTokens)

		ma.Avg = AggregatedStats{
			TokensPerSecond:           avgTPS,
			TimeToFirstTokenSeconds:   avgTTFT,
			TotalExecutionTimeSeconds: avgTotalExec,
			InputTokens:               avgInputTokens,
			OutputTokens:              avgOutputTokens,
		}

		ma.Min = AggregatedStats{
			TokensPerSecond:           bench.MinStats.TokensPerSecond,
			TimeToFirstTokenSeconds:   nsToSeconds(bench.MinStats.TimeToFirstToken),
			TotalExecutionTimeSeconds: nsToSeconds(bench.MinStats.TotalExecutionTime),
			InputTokens:               float64(bench.MinStats.InputTokenCount),
			OutputTokens:              float64(bench.MinStats.OutputTokenCount),
		}

		ma.Max = AggregatedStats{
			TokensPerSecond:           bench.MaxStats.TokensPerSecond,
			TimeToFirstTokenSeconds:   nsToSeconds(bench.MaxStats.TimeToFirstToken),
			TotalExecutionTimeSeconds: nsToSeconds(bench.MaxStats.TotalExecutionTime),
			InputTokens:               float64(bench.MaxStats.InputTokenCount),
			OutputTokens:              float64(bench.MaxStats.OutputTokenCount),
		}

		ma.Variance = VarianceStats{
			TokensPerSecondStdDev:         stddevFromValues(iterTPS, ma.Avg.TokensPerSecond),
			TimeToFirstTokenStdDevSeconds: stddevFromValues(iterTTFT, ma.Avg.TimeToFirstTokenSeconds),
			OutputTokensStdDev:            stddevFromValues(iterOutputTokens, ma.Avg.OutputTokens),
		}

		if len(bench.Iterations) > 0 {
			ma.Iterations = bench.Iterations
		}

		if ma.Avg.TokensPerSecond > globalMaxAvgTPS {
			globalMaxAvgTPS = ma.Avg.TokensPerSecond
		}
		if ma.Avg.TimeToFirstTokenSeconds > 0 && ma.Avg.TimeToFirstTokenSeconds < globalMinAvgTTFT {
			globalMinAvgTTFT = ma.Avg.TimeToFirstTokenSeconds
		}

		modelAnalyses = append(modelAnalyses, ma)
	}

	if globalMinAvgTTFT == math.MaxFloat64 {
		globalMinAvgTTFT = 0
	}

	rankThroughput := make([]ThroughputRankingEntry, 0, len(modelAnalyses))
	rankLatency := make([]LatencyRankingEntry, 0, len(modelAnalyses))
	rankEfficiency := make([]EfficiencyRankingEntry, 0, len(modelAnalyses))
	rankAccuracy := make([]AccuracyRankingEntry, 0, len(modelAnalyses))

	multiModel := len(modelAnalyses) > 1

	for _, ma := range modelAnalyses {
		if accuracy != nil {
			if stats, ok := accuracy[ma.ModelName]; ok {
				ma.Accuracy = stats
			}
		}

		if multiModel && globalMaxAvgTPS > 0 {
			ma.Scores.ThroughputScore = clampFloat((ma.Avg.TokensPerSecond/globalMaxAvgTPS)*100, 0, 100)
		} else if !multiModel {
			ma.Scores.ThroughputScore = 100
		}

		if multiModel && ma.Avg.TimeToFirstTokenSeconds > 0 && globalMinAvgTTFT > 0 {
			ma.Scores.LatencyScore = clampFloat((globalMinAvgTTFT/ma.Avg.TimeToFirstTokenSeconds)*100, 0, 100)
		} else if !multiModel {
			ma.Scores.LatencyScore = 100
		} else if ma.Avg.TimeToFirstTokenSeconds == 0 {
			ma.Scores.LatencyScore = 100
		}

		ma.Scores.EfficiencyScore = 0.6*ma.Scores.ThroughputScore + 0.4*ma.Scores.LatencyScore
		if ma.Accuracy.Total > 0 {
			accuracyPct := ma.Accuracy.Accuracy * 100
			ma.Scores.CompositeScore = 0.6*accuracyPct + 0.4*ma.Scores.ThroughputScore
		}

		if globalMaxAvgTPS > 0 {
			ma.DerivedRatios.RelativeToFastest = ma.Avg.TokensPerSecond / globalMaxAvgTPS
		}

		if ma.Avg.TotalExecutionTimeSeconds > 0 {
			ratio := ma.Avg.TimeToFirstTokenSeconds / ma.Avg.TotalExecutionTimeSeconds
			ma.DerivedRatios.LatencyShareOfTotal = clampFloat(ratio, 0, 1)
		}

		ma.Labels.RelativeSpeedTier = classifySpeedTier(ma.DerivedRatios.RelativeToFastest)
		ma.Labels.LatencyProfile = classifyLatencyProfile(ma.Avg.TimeToFirstTokenSeconds)
		ma.Labels.Stability = classifyStability(ma.Variance.TokensPerSecondStdDev, ma.Avg.TokensPerSecond)
		ma.Labels.InteractiveSuitability = classifyInteractiveSuitability(ma.Avg.TimeToFirstTokenSeconds, ma.Avg.TokensPerSecond)

		ma.Notes = buildModelNotes(*ma)

		rankThroughput = append(rankThroughput, ThroughputRankingEntry{
			ModelName:          ma.ModelName,
			AvgTokensPerSecond: ma.Avg.TokensPerSecond,
		})
		rankLatency = append(rankLatency, LatencyRankingEntry{
			ModelName:                  ma.ModelName,
			AvgTimeToFirstTokenSeconds: ma.Avg.TimeToFirstTokenSeconds,
		})
		rankEfficiency = append(rankEfficiency, EfficiencyRankingEntry{
			ModelName:       ma.ModelName,
			EfficiencyScore: ma.Scores.EfficiencyScore,
		})
		if ma.Accuracy.Total > 0 {
			rankAccuracy = append(rankAccuracy, AccuracyRankingEntry{
				ModelName: ma.ModelName,
				Accuracy:  ma.Accuracy.Accuracy,
			})
		}
	}

	sort.Slice(rankThroughput, func(i, j int) bool {
		return rankThroughput[i].AvgTokensPerSecond > rankThroughput[j].AvgTokensPerSecond
	})
	sort.Slice(rankLatency, func(i, j int) bool {
		return rankLatency[i].AvgTimeToFirstTokenSeconds < rankLatency[j].AvgTimeToFirstTokenSeconds
	})
	sort.Slice(rankEfficiency, func(i, j int) bool {
		return rankEfficiency[i].EfficiencyScore > rankEfficiency[j].EfficiencyScore
	})
	sort.Slice(rankAccuracy, func(i, j int) bool {
		return rankAccuracy[i].Accuracy > rankAccuracy[j].Accuracy
	})

	bestTradeoffModel := applyParetoFront(modelAnalyses)

	finalModels := make([]ModelAnalysis, len(modelAnalyses))
	for i, ma := range modelAnalyses {
		finalModels[i] = *ma
	}

	analysis.Models = finalModels
	analysis.Rankings = Rankings{
		ByThroughput:      rankThroughput,
		ByLatency:         rankLatency,
		ByEfficiencyScore: rankEfficiency,
		ByAccuracy:        rankAccuracy,
	}

	analysis.Overall = buildOverallSummary(analysis.Rankings)
	if bestTradeoffModel != "" {
		analysis.Overall.BestTradeoffModel = bestTradeoffModel
		analysis.Overall.SummaryNotes = append(analysis.Overall.SummaryNotes, fmt.Sprintf("Best trade-off model is %s based on the Pareto front and composite score tie-break.", bestTradeoffModel))
	}
	analysis.Anomalies = detectAnomalies(analysis.Models)
	analysis.Recommendations = buildRecommendations(analysis.Models)

	return analysis
}

// GenerateReport renders a standalone HTML dashboard powered by the Analysis payload.
func GenerateReport(analysis Analysis) (string, error) {
	data, err := json.Marshal(analysis)
	if err != nil {
		return "", err
	}

	viewModel := ReportTemplateData{
		Title:        "agon: LLM Benchmark Report",
		AnalysisJSON: template.JS(data),
	}

	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, viewModel); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// buildOverallSummary creates a summary of the benchmark results.
func buildOverallSummary(rankings Rankings) OverallSummary {
	var summary OverallSummary
	if len(rankings.ByThroughput) > 0 {
		entry := rankings.ByThroughput[0]
		summary.FastestModel = entry.ModelName
		summary.SummaryNotes = append(summary.SummaryNotes, fmt.Sprintf("Fastest model by throughput is %s with %.2f tokens/sec.", entry.ModelName, entry.AvgTokensPerSecond))
	}
	if len(rankings.ByLatency) > 0 {
		entry := rankings.ByLatency[0]
		summary.BestLatencyModel = entry.ModelName
		summary.SummaryNotes = append(summary.SummaryNotes, fmt.Sprintf("Best latency model is %s with time to first token â‰ˆ %.2fs.", entry.ModelName, entry.AvgTimeToFirstTokenSeconds))
	}
	if len(rankings.ByEfficiencyScore) > 0 {
		entry := rankings.ByEfficiencyScore[0]
		summary.MostEfficientModel = entry.ModelName
		summary.SummaryNotes = append(summary.SummaryNotes, fmt.Sprintf("Most efficient model is %s with an efficiency score of %.1f.", entry.ModelName, entry.EfficiencyScore))
	}
	if len(rankings.ByAccuracy) > 0 {
		entry := rankings.ByAccuracy[0]
		summary.MostAccurateModel = entry.ModelName
		summary.SummaryNotes = append(summary.SummaryNotes, fmt.Sprintf("Most accurate model is %s with %.1f%% accuracy.", entry.ModelName, entry.Accuracy*100))
	}
	return summary
}

func applyParetoFront(models []*ModelAnalysis) string {
	candidates := make([]*ModelAnalysis, 0, len(models))
	for _, model := range models {
		if model.Accuracy.Total > 0 && model.Avg.TokensPerSecond > 0 {
			candidates = append(candidates, model)
		}
	}
	const epsilon = 1e-9
	for _, model := range candidates {
		model.ParetoFront = true
		for _, other := range candidates {
			if model == other {
				continue
			}
			accuracyBetter := other.Accuracy.Accuracy >= model.Accuracy.Accuracy-epsilon
			tpsBetter := other.Avg.TokensPerSecond >= model.Avg.TokensPerSecond-epsilon
			strictBetter := other.Accuracy.Accuracy > model.Accuracy.Accuracy+epsilon || other.Avg.TokensPerSecond > model.Avg.TokensPerSecond+epsilon
			if accuracyBetter && tpsBetter && strictBetter {
				model.ParetoFront = false
				break
			}
		}
	}

	best := ""
	var bestScore float64
	var bestAccuracy float64
	var bestTPS float64
	for _, model := range candidates {
		if !model.ParetoFront {
			continue
		}
		score := model.Scores.CompositeScore
		if best == "" || score > bestScore+epsilon {
			best = model.ModelName
			bestScore = score
			bestAccuracy = model.Accuracy.Accuracy
			bestTPS = model.Avg.TokensPerSecond
			continue
		}
		if math.Abs(score-bestScore) <= epsilon {
			if model.Accuracy.Accuracy > bestAccuracy+epsilon {
				best = model.ModelName
				bestAccuracy = model.Accuracy.Accuracy
				bestTPS = model.Avg.TokensPerSecond
			} else if math.Abs(model.Accuracy.Accuracy-bestAccuracy) <= epsilon && model.Avg.TokensPerSecond > bestTPS+epsilon {
				best = model.ModelName
				bestTPS = model.Avg.TokensPerSecond
			}
		}
	}
	return best
}

// detectAnomalies identifies any notable outliers in the analysis.
func detectAnomalies(models []ModelAnalysis) []Anomaly {
	anomalies := make([]Anomaly, 0)
	for _, model := range models {
		if model.Avg.TimeToFirstTokenSeconds > 120 {
			anomalies = append(anomalies, Anomaly{
				Type:      "very_high_latency",
				ModelName: model.ModelName,
				Severity:  "critical",
				Message:   fmt.Sprintf("%s has extremely high time to first token, making it unsuitable for interactive workloads.", model.ModelName),
			})
		}
		if model.Labels.RelativeSpeedTier == "slow" && hasFasterModelWithSimilarOutputs(model, models) {
			anomalies = append(anomalies, Anomaly{
				Type:      "slow_small_model",
				ModelName: model.ModelName,
				Severity:  "warning",
				Message:   fmt.Sprintf("%s is significantly slower than other models despite similar or smaller outputs; fixed overhead may dominate on this hardware.", model.ModelName),
			})
		}
		if model.Labels.Stability == "unstable" {
			anomalies = append(anomalies, Anomaly{
				Type:      "high_variance",
				ModelName: model.ModelName,
				Severity:  "warning",
				Message:   fmt.Sprintf("%s shows high variability across runs; may indicate contention or thermal throttling.", model.ModelName),
			})
		}
	}
	return anomalies
}

// buildRecommendations generates a list of recommendations based on the analysis.
func buildRecommendations(models []ModelAnalysis) []string {
	recs := make([]string, 0)
	for _, model := range models {
		if model.Labels.InteractiveSuitability == "good" && model.Labels.RelativeSpeedTier == "top" {
			recs = append(recs, fmt.Sprintf("Use %s as the default interactive model on this hardware.", model.ModelName))
		}
		if model.Labels.InteractiveSuitability == "unusable" {
			recs = append(recs, fmt.Sprintf("Avoid %s for interactive usage; reserve it for batch-style or very small outputs.", model.ModelName))
		}
	}
	recs = append(recs, "Consider keeping sessions warm and reducing context size to minimize TTFT across all models.")
	return recs
}

// classifySpeedTier categorizes a model's speed based on its performance relative to the fastest model.
func classifySpeedTier(relative float64) string {
	switch {
	case relative >= 0.75:
		return "top"
	case relative >= 0.4:
		return "mid"
	default:
		return "slow"
	}
}

// classifyLatencyProfile categorizes a model's latency profile based on its time to first token.
func classifyLatencyProfile(seconds float64) string {
	switch {
	case seconds < 10:
		return "low"
	case seconds <= 60:
		return "medium"
	default:
		return "high"
	}
}

// classifyStability categorizes a model's performance stability based on its coefficient of variation.
func classifyStability(stddev, avg float64) string {
	if avg <= 0 {
		if stddev == 0 {
			return "stable"
		}
		return "unstable"
	}
	cv := stddev / avg
	switch {
	case cv < 0.1:
		return "stable"
	case cv < 0.25:
		return "moderate"
	default:
		return "unstable"
	}
}

// classifyInteractiveSuitability determines a model's suitability for interactive use cases.
func classifyInteractiveSuitability(ttftSeconds, tokensPerSecond float64) string {
	switch {
	case ttftSeconds > 120:
		return "unusable"
	case ttftSeconds > 60:
		return "borderline"
	case tokensPerSecond < 2.0:
		return "borderline"
	default:
		return "good"
	}
}

// buildModelNotes creates a list of human-readable notes for a model based on its performance characteristics.
func buildModelNotes(model ModelAnalysis) []string {
	notes := make([]string, 0, 3)
	if model.Labels.LatencyProfile == "high" {
		notes = append(notes, "Very high time to first token; most of the time is spent before streaming.")
	}
	if model.Labels.RelativeSpeedTier == "top" {
		notes = append(notes, "This is one of the fastest models by throughput.")
	}
	if model.Labels.Stability == "unstable" {
		notes = append(notes, "Performance is highly variable across runs.")
	}
	return notes
}

// hasFasterModelWithSimilarOutputs checks if there is a faster model with a similar or smaller output size.
func hasFasterModelWithSimilarOutputs(target ModelAnalysis, models []ModelAnalysis) bool {
	if target.Avg.OutputTokens <= 0 {
		return false
	}
	const tolerance = 1.1
	for _, other := range models {
		if other.ModelName == target.ModelName {
			continue
		}
		if other.Avg.TokensPerSecond <= target.Avg.TokensPerSecond {
			continue
		}
		if other.Avg.OutputTokens <= 0 {
			continue
		}
		if target.Avg.OutputTokens <= other.Avg.OutputTokens*tolerance {
			return true
		}
	}
	return false
}

// fallbackAverage returns the primary value if it's positive, otherwise it calculates the mean of the fallback values.
func fallbackAverage(primary float64, fallback []float64) float64 {
	if primary > 0 {
		return primary
	}
	return meanFloat64(fallback)
}

// meanFloat64 calculates the mean of a slice of float64 values.
func meanFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

// stddevFromValues calculates the standard deviation of a slice of float64 values given their mean.
func stddevFromValues(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		diff := v - mean
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(values)))
}

// nsToSeconds converts nanoseconds to seconds.
func nsToSeconds(ns int64) float64 {
	return float64(ns) / 1e9
}

// clampFloat restricts a float64 value to a given range.
func clampFloat(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

var reportTemplate = template.Must(template.New("metrics-report").Parse(reportTemplateHTML))

const reportTemplateHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css">
  <script src="https://kit.fontawesome.com/517f4f7a2b.js" crossorigin="anonymous"></script>
	<link href="https://fonts.googleapis.com/icon?family=Material+Icons+Two+Tone" rel="stylesheet">
  <style>
    :root {
      --primary: #334155;
      --secondary: #64748B;
      --accent: #3B82F6;
      --light: #F1F5F9;
      --background: #FFFFFF;
      --text: #0F172A;
      --success: #10B981;
      --warning: #F59E0B;
      --border: #E2E8F0;
    }
    [data-theme="dark"] {
      --primary: #0F172A;
      --secondary: #94A3B8;
      --accent: #60A5FA;
      --light: #0B1220;
      --background: #0F172A;
      --text: #E2E8F0;
      --success: #34D399;
      --warning: #FBBF24;
      --border: rgba(148, 163, 184, 0.25);
    }
    body {
      background-color: var(--light);
      color: var(--text);
    }
    .navbar-dark {
      background-color: var(--primary) !important;
    }
    .bg-dark {
      background-color: var(--primary) !important;
    }
    .navbar-dark .navbar-brand,
    .navbar-dark .text-light {
      color: var(--light) !important;
    }
    .card {
      border: 1px solid var(--border);
      background-color: var(--background);
    }
    .table thead th { cursor: pointer; }
    .table thead th,
    .table thead td {
      background-color: var(--light);
      color: var(--text);
      border-color: var(--border);
    }
    .table-striped>tbody>tr:nth-of-type(odd)>* {
      --bs-table-accent-bg: var(--light);
    }
    .table-bordered>:not(caption)>* {
      border-color: var(--border);
    }
    .sort-icon { font-size: 0.8rem; margin-left: 0.25rem; }
    .accordion-button .badge { margin-left: 0.5rem; }
    .accordion-button {
      background-color: var(--light);
      color: var(--text);
    }
    .accordion-button:not(.collapsed) {
      background-color: var(--accent);
      color: var(--background);
    }
    .list-group-item {
      display: flex;
      align-items: center;
      justify-content: space-between;
      background-color: var(--background);
      border-color: var(--border);
      color: var(--text);
    }
    .notes-list li { margin-bottom: 0.25rem; }
    .table#modelsTable>tbody>tr>td.top-performer {
      background-color: #DBEAFE;
      font-weight: 600;
      color: var(--text);
    }
    .chart-card {
      background: var(--background);
      border-radius: 16px;
      padding: 1.5rem;
      box-shadow: 0 1px 3px rgba(15, 23, 42, 0.1);
      border: 1px solid var(--border);
    }
    .chart-title {
      font-size: 1.5rem;
      font-weight: 700;
      color: var(--text);
      margin-bottom: 0.25rem;
    }
    .chart-subtitle {
      color: var(--secondary);
      margin-bottom: 1.5rem;
    }
    .chart-canvas {
      position: relative;
      height: 420px;
    }
    .legend-container {
      display: flex;
      gap: 1.5rem;
      justify-content: center;
      flex-wrap: wrap;
      margin-top: 1.25rem;
      padding-top: 1.25rem;
      border-top: 2px solid var(--border);
    }
    .legend-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .legend-color {
      width: 14px;
      height: 14px;
      border-radius: 50%;
    }
    .legend-text {
      font-size: 0.9rem;
      color: var(--secondary);
    }
    .filter-row {
      display: flex;
      align-items: center;
      gap: 0.75rem;
      flex-wrap: wrap;
      margin-bottom: 1rem;
    }
    .filter-label {
      font-weight: 600;
      color: var(--text);
    }
    .badge.bg-primary {
      background-color: var(--accent) !important;
    }
    .badge.bg-success {
      background-color: var(--success) !important;
    }
    .badge.bg-warning {
      background-color: var(--warning) !important;
      color: var(--background) !important;
    }
    .badge.bg-danger {
      background-color: #DC2626 !important;
    }
    .badge.bg-secondary {
      background-color: var(--secondary) !important;
    }
    .theme-toggle {
      border: 1px solid var(--border);
      color: var(--light);
    }
    [data-theme="dark"] .theme-toggle {
      color: var(--text);
      background-color: rgba(148, 163, 184, 0.15);
    }
    [data-theme="dark"] .table#modelsTable>tbody>tr>td.top-performer {
      background-color: rgba(96, 165, 250, 0.25);
    }
    [data-theme="dark"] .chart-card {
      box-shadow: 0 10px 28px rgba(2, 6, 23, 0.6);
    }
    [data-theme="dark"] .accordion-button:not(.collapsed) {
      background-color: var(--accent);
      color: #0B1220;
    }
    [data-theme="dark"] .badge.bg-warning {
      color: #0B1220 !important;
    }
  </style>
</head>
<body>
  <nav class="navbar navbar-dark bg-dark">
    <div class="container-fluid">
      <span class="navbar-brand mb-0 h1">{{ .Title }}</span>
      <div class="d-flex align-items-center gap-3">
        <button class="btn btn-sm theme-toggle" id="themeToggle" type="button" aria-label="Toggle dark mode">
          <span class="material-icons-two-tone" aria-hidden="true">dark_mode</span>
        </button>
        <span class="text-light">Generated: <span id="generatedAt">-</span></span>
      </div>
    </div>
  </nav>
  <main class="container-fluid my-4">
    <div class="row g-3">
      <div class="col-sm-6 col-lg-2">
        <div class="card shadow-sm h-100">
          <div class="card-body">
            <p style="font-size: 1.5em;" class="text-muted mb-1"><i class="fa-duotone fa-regular fa-rabbit-running fa-fw"></i> Fastest Model</p>
            <h5 class="card-title" id="fastestModel">â€”</h5>
          </div>
        </div>
      </div>
      <div class="col-sm-6 col-lg-2">
        <div class="card shadow-sm h-100">
          <div class="card-body">
            <p style="font-size: 1.5em;" class="text-muted mb-1"><i class="fa-duotone fa-regular fa-gauge-low"></i> Best Latency</p>
            <h5 class="card-title" id="bestLatencyModel">â€”</h5>
          </div>
        </div>
      </div>
      <div class="col-sm-6 col-lg-2">
        <div class="card shadow-sm h-100">
          <div class="card-body">
            <p style="font-size: 1.5em;" class="text-muted mb-1"><i class="fa-duotone fa-regular fa-gauge-high"></i> Most Efficient</p>
            <h5 class="card-title" id="mostEfficientModel">-</h5>
          </div>
        </div>
      </div>
        <div class="col-sm-6 col-lg-2">
          <div class="card shadow-sm h-100">
            <div class="card-body">
              <p style="font-size: 1.5em;" class="text-muted mb-1"><i class="fa-duotone fa-solid fa-bullseye-arrow"></i> Most Accurate</p>
              <h5 class="card-title" id="mostAccurateModel">-</h5>
            </div>
          </div>
        </div>
        <div class="col-sm-6 col-lg-2">
          <div class="card shadow-sm h-100">
            <div class="card-body">
              <p style="font-size: 1.5em;" class="text-muted mb-1"><i class="fa-duotone fa-solid fa-code-compare"></i> Best Trade-off</p>
              <h5 class="card-title" id="bestTradeoffModel">-</h5>
            </div>
          </div>
        </div>
      </div>
    </div>

    <section class="mt-4">
      <div class="card shadow-sm">
        <div class="card-header bg-white">
          <h5 class="mb-0">Model Comparison</h5>
        </div>
        <div class="card-body">
          <div class="table-responsive">
            <table class="table table-striped table-hover table-bordered table-sm" id="modelsTable">
              <thead class="table-light">
                <tr>
                  <th class="sortable" data-type="text">Model <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Avg TPS <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Avg TTFT (s) <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Avg Output Tokens <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Accuracy (%) <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">No. of Questions <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number" id="timeoutsHeader">Timeouts <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Avg Difficulty <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Throughput Score <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Latency Score <span class="material-icons-two-tone sort">import_export</span></th>
                  <th class="sortable" data-type="number">Efficiency Score <span class="material-icons-two-tone sort">import_export</span></th>
                </tr>
              </thead>
              <tbody></tbody>
            </table>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm chart-card">
        <div class="card-body">
          <div class="chart-title">LLM Performance Analysis</div>
          <div class="chart-subtitle">Accuracy vs throughput trade-offs - higher is better</div>
          <div class="chart-canvas">
            <canvas id="accuracyThroughputChart" aria-label="Accuracy vs throughput chart" role="img"></canvas>
          </div>
          <div id="accuracyThroughputEmpty" class="text-muted small mt-2"></div>
          <div class="legend-container">
            <div class="legend-item">
              <div class="legend-color" style="background: #334155;"></div>
              <span class="legend-text"><strong>Excellent</strong> (70%+ accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #64748B;"></div>
              <span class="legend-text"><strong>Good</strong> (50-70% accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #94A3B8;"></div>
              <span class="legend-text"><strong>Fair</strong> (35-50% accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #CBD5E1;"></div>
              <span class="legend-text"><strong>Poor</strong> (&lt;35% accuracy)</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm chart-card">
        <div class="card-body">
          <div class="chart-title">Input Tokens vs Processing</div>
          <div class="chart-subtitle">Visualize how input length affects TTFT and throughput.</div>
          <div class="filter-row">
            <span class="filter-label">Model filter:</span>
            <select class="form-select form-select-sm w-auto" id="inputTokenModelFilter"></select>
          </div>
          <div class="row g-3">
            <div class="col-lg-6">
              <div class="chart-canvas">
                <canvas id="inputTokensTtftChart" aria-label="Input tokens vs time to first token" role="img"></canvas>
              </div>
              <div id="inputTokensTtftEmpty" class="text-muted small mt-2"></div>
            </div>
            <div class="col-lg-6">
              <div class="chart-canvas">
                <canvas id="inputTokensTpsChart" aria-label="Input tokens vs tokens per second" role="img"></canvas>
              </div>
              <div id="inputTokensTpsEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm">
        <div class="card-header bg-white">
          <h5 class="mb-0">Per-Model Details</h5>
        </div>
        <div class="card-body">
          <div class="accordion" id="modelAccordion"></div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="row g-3">
        <div class="col-md-6">
          <div class="card shadow-sm h-100">
            <div class="card-header bg-white">
              <h5 class="mb-0">Anomalies</h5>
            </div>
            <div class="card-body">
              <div class="list-group" id="anomaliesList"></div>
            </div>
          </div>
        </div>
        <div class="col-md-6">
          <div class="card shadow-sm h-100">
            <div class="card-header bg-white">
              <h5 class="mb-0">Recommendations</h5>
            </div>
            <div class="card-body">
              <ol class="list-group list-group-numbered" id="recommendationsList"></ol>
            </div>
          </div>
        </div>
      </div>
    </section>
  </main>

  <script src="https://code.jquery.com/jquery-3.7.1.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js"></script>
  <script>
    var analysis = {{ .AnalysisJSON }};
  </script>
  <script>
    (function($) {
      function formatNumber(value, decimals) {
        if (value === null || value === undefined || isNaN(value)) {
          return 'â€”';
        }
        return Number(value).toFixed(decimals);
      }

      function createNumericCell(value, decimals) {
        var display = formatNumber(value, decimals);
        var $td = $('<td></td>').text(display);
        if (!isNaN(value)) {
          $td.attr('data-value', value);
        }
        return $td;
      }

      function updateSortIcons($header, direction) {
				$header.closest('tr').find('.sort').each(function() {
					$(this)[0].innerHTML = 'import_export'
				});

        if (direction === 'asc') {
					$header.find('.sort')[0].innerHTML = 'keyboard_double_arrow_up'
        } else if (direction === 'desc') {
          $header.find('.sort')[0].innerHTML = 'keyboard_double_arrow_down'
        }
      }

      function populateTable(models) {
        var $tbody = $('#modelsTable tbody').empty();
        models.forEach(function(model) {
          var $row = $('<tr></tr>');
          var paretoBadge = model.paretoFront ? ' <span class="badge bg-success text-uppercase ms-1">Pareto</span>' : '';
          $row.append($('<td><i class="fa-duotone fa-regular fa-user-robot"></i> '+model.modelName+paretoBadge+'</td>'))
          $row.append(createNumericCell(model.avg.tokensPerSecond, 2));
          $row.append(createNumericCell(model.avg.timeToFirstTokenSeconds, 2));
          $row.append(createNumericCell(model.avg.outputTokens, 1));
          var accuracyPct = null;
          if (model.accuracy && typeof model.accuracy.accuracy === 'number') {
            accuracyPct = model.accuracy.accuracy * 100;
          }
          $row.append(createNumericCell(accuracyPct, 1));
          var questionCount = null;
          if (model.accuracy && typeof model.accuracy.total === 'number') {
            questionCount = model.accuracy.total;
          }
          $row.append(createNumericCell(questionCount, 0));
          var timeoutCount = null;
          if (model.accuracy && typeof model.accuracy.timeouts === 'number') {
            timeoutCount = model.accuracy.timeouts;
          }
          $row.append(createNumericCell(timeoutCount, 0));
          var avgDifficulty = null;
          if (model.accuracy && typeof model.accuracy.avgDifficulty === 'number') {
            avgDifficulty = model.accuracy.avgDifficulty;
          }
          $row.append(createNumericCell(avgDifficulty, 2));
          $row.append(createNumericCell(model.scores.throughputScore, 1));
          $row.append(createNumericCell(model.scores.latencyScore, 1));
          $row.append(createNumericCell(model.scores.efficiencyScore, 1));
          $tbody.append($row);
        });
        highlightTopPerformers($tbody);
      }

      function highlightTopPerformers($tbody) {
        var columns = [
          { index: 1, mode: 'max' },
          { index: 2, mode: 'min' },
          { index: 3, mode: 'max' },
          { index: 4, mode: 'max' },
          { index: 5, mode: 'max' },
          { index: 6, mode: 'min' },
          { index: 7, mode: 'max' },
          { index: 8, mode: 'max' },
          { index: 9, mode: 'max' },
          { index: 10, mode: 'max' }
        ];
        columns.forEach(function(column) {
          var best = null;
          $tbody.find('tr').each(function() {
            var $cell = $(this).children().eq(column.index);
            var value = parseFloat($cell.attr('data-value'));
            if (isNaN(value)) {
              return;
            }
            if (best === null) {
              best = value;
              return;
            }
            if (column.mode === 'min' && value < best) {
              best = value;
            } else if (column.mode === 'max' && value > best) {
              best = value;
            }
          });
          if (best === null) {
            return;
          }
          $tbody.find('tr').each(function() {
            var $cell = $(this).children().eq(column.index);
            var value = parseFloat($cell.attr('data-value'));
            if (isNaN(value)) {
              return;
            }
            if (value === best) {
              $cell.addClass('top-performer');
            }
          });
        });
      }

      function buildAccordion(models) {
        var $accordion = $('#modelAccordion').empty();
        models.forEach(function(model, index) {
          var collapseID = 'model-details-' + index;
          var headerID = 'heading-' + index;
          var $item = $('<div class="accordion-item"></div>');
          var badges = '';
          if (model.labels.relativeSpeedTier) {
            badges += '<span class="badge bg-primary text-uppercase">' + model.labels.relativeSpeedTier + '</span>';
          }
          if (model.labels.interactiveSuitability) {
            var badgeClass = 'bg-success';
            if (model.labels.interactiveSuitability === 'borderline') {
              badgeClass = 'bg-warning text-dark';
            } else if (model.labels.interactiveSuitability === 'unusable') {
              badgeClass = 'bg-danger';
            }
            badges += '<span class="badge ' + badgeClass + ' text-uppercase">' + model.labels.interactiveSuitability + '</span>';
          }
          if (model.paretoFront) {
            badges += '<span class="badge bg-success text-uppercase">pareto</span>';
          }
          var header = ''
            + '<h2 class="accordion-header" id="' + headerID + '">'
            + '<button class="accordion-button ' + (index !== 0 ? 'collapsed' : '') + '" type="button" data-bs-toggle="collapse"'
            + ' data-bs-target="#' + collapseID + '" aria-expanded="' + (index === 0 ? 'true' : 'false') + '"'
            + ' aria-controls="' + collapseID + '">'
            + model.modelName + ' ' + badges
            + '</button>'
            + '</h2>';
          var notes = (model.notes || []).map(function(note) {
            return '<li>' + note + '</li>';
          }).join('');
          if (!notes) {
            notes = '<li>No significant notes for this model.</li>';
          }
          var accuracyPct = null;
          var accuracyLine = '-';
          if (model.accuracy && model.accuracy.total > 0) {
            accuracyPct = model.accuracy.accuracy * 100;
            accuracyLine = formatNumber(accuracyPct, 1) + '% (' + model.accuracy.correct + '/' + model.accuracy.total + ')';
          }
          var avgDifficultyLine = '-';
          var avgMarginLine = '-';
          if (model.accuracy && typeof model.accuracy.avgDifficulty === 'number') {
            avgDifficultyLine = formatNumber(model.accuracy.avgDifficulty, 2);
          }
          if (model.accuracy && typeof model.accuracy.avgMarginOfError === 'number') {
            avgMarginLine = formatNumber(model.accuracy.avgMarginOfError, 2);
          }
          var difficultyBreakdown = '-';
          if (model.accuracy && model.accuracy.byDifficulty) {
            var buckets = [];
            Object.keys(model.accuracy.byDifficulty).sort(function(a, b) {
              return Number(a) - Number(b);
            }).forEach(function(key) {
              var bucket = model.accuracy.byDifficulty[key];
              if (!bucket || bucket.total <= 0) {
                return;
              }
              var pct = bucket.accuracy * 100;
              buckets.push('d' + key + ' ' + formatNumber(pct, 1) + '% (' + bucket.correct + '/' + bucket.total + ')');
            });
            if (buckets.length > 0) {
              difficultyBreakdown = buckets.join(', ');
            }
          }
          var bodyParts = [];
          bodyParts.push('<div id="' + collapseID + '" class="accordion-collapse collapse ' + (index === 0 ? 'show' : '') + '" aria-labelledby="' + headerID + '" data-bs-parent="#modelAccordion">');
          bodyParts.push('<div class="accordion-body"><div class="row g-3">');
          bodyParts.push('<div class="col-md-6">');
          bodyParts.push('<h6>Average Stats</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Tokens/sec:</strong> ' + formatNumber(model.avg.tokensPerSecond, 2) + '</li>');
          bodyParts.push('<li><strong>TTFT (s):</strong> ' + formatNumber(model.avg.timeToFirstTokenSeconds, 2) + '</li>');
          bodyParts.push('<li><strong>Total (s):</strong> ' + formatNumber(model.avg.totalExecutionTimeSeconds, 2) + '</li>');
          bodyParts.push('<li><strong>Output tokens:</strong> ' + formatNumber(model.avg.outputTokens, 1) + '</li>');
          bodyParts.push('<li><strong>Accuracy:</strong> ' + accuracyLine + '</li>');
          bodyParts.push('<li><strong>Avg difficulty:</strong> ' + avgDifficultyLine + '</li>');
          bodyParts.push('<li><strong>Avg margin:</strong> ' + avgMarginLine + '</li>');
          bodyParts.push('<li><strong>By difficulty:</strong> ' + difficultyBreakdown + '</li>');
          bodyParts.push('</ul><h6>Variance</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>TPS Ïƒ:</strong> ' + formatNumber(model.variance.tokensPerSecondStdDev, 2) + '</li>');
          bodyParts.push('<li><strong>TTFT Ïƒ (s):</strong> ' + formatNumber(model.variance.timeToFirstTokenStdDevSeconds, 2) + '</li>');
          bodyParts.push('<li><strong>Output Ïƒ:</strong> ' + formatNumber(model.variance.outputTokensStdDev, 2) + '</li>');
          bodyParts.push('</ul></div>');
          bodyParts.push('<div class="col-md-6">');
          bodyParts.push('<h6>Extremes</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Min TPS:</strong> ' + formatNumber(model.min.tokensPerSecond, 2) + '</li>');
          bodyParts.push('<li><strong>Max TPS:</strong> ' + formatNumber(model.max.tokensPerSecond, 2) + '</li>');
          bodyParts.push('<li><strong>Min TTFT (s):</strong> ' + formatNumber(model.min.timeToFirstTokenSeconds, 2) + '</li>');
          bodyParts.push('<li><strong>Max TTFT (s):</strong> ' + formatNumber(model.max.timeToFirstTokenSeconds, 2) + '</li>');
          bodyParts.push('</ul><h6>Ratios &amp; Notes</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Latency share:</strong> ' + formatNumber((model.derivedRatios.latencyShareOfTotal || 0) * 100, 1) + '%</li>');
          bodyParts.push('<li><strong>Relative to fastest:</strong> ' + formatNumber((model.derivedRatios.relativeToFastest || 0) * 100, 1) + '%</li>');
          bodyParts.push('</ul><ul class="notes-list">' + notes + '</ul>');
          bodyParts.push('</div></div></div></div>');
          var body = bodyParts.join('');
          $item.append(header);
          $item.append(body);
          $accordion.append($item);
        });
      }

      function populateAnomalies(anomalies) {
        var $container = $('#anomaliesList').empty();
        if (!anomalies || anomalies.length === 0) {
          $container.append('<div class="list-group-item text-muted">No anomalies detected.</div>');
          return;
        }
        anomalies.forEach(function(anomaly) {
          var badgeClass = 'bg-secondary';
          if (anomaly.severity === 'warning') {
            badgeClass = 'bg-warning text-dark';
          } else if (anomaly.severity === 'critical') {
            badgeClass = 'bg-danger';
          }
          var item = ''
            + '<div class="list-group-item">'
            + '<div>'
            + '<span class="badge ' + badgeClass + ' text-uppercase me-2">' + (anomaly.severity || 'info') + '</span>'
            + '<strong>' + (anomaly.modelName || 'â€”') + '</strong>'
            + '</div>'
            + '<p class="mb-0 small">' + (anomaly.message || '') + '</p>'
            + '</div>';
          $container.append(item);
        });
      }

      function populateRecommendations(recommendations) {
        var $list = $('#recommendationsList').empty();
        if (!recommendations || recommendations.length === 0) {
          $list.append('<li class="list-group-item">No recommendations generated.</li>');
          return;
        }
        recommendations.forEach(function(rec) {
          $list.append('<li class="list-group-item">' + rec + '</li>');
        });
      }

      function buildAccuracyThroughputChart(models) {
        var canvas = document.getElementById('accuracyThroughputChart');
        if (!canvas) {
          return;
        }
        function getColorForAccuracy(accuracy) {
          if (accuracy >= 70) return '#334155';
          if (accuracy >= 50) return '#64748B';
          if (accuracy >= 35) return '#94A3B8';
          return '#CBD5E1';
        }

        var points = [];
        models.forEach(function(model) {
          if (!model.accuracy || model.accuracy.total <= 0) {
            return;
          }
          if (!model.avg || typeof model.avg.tokensPerSecond !== 'number') {
            return;
          }
          points.push({
            x: model.avg.tokensPerSecond,
            y: model.accuracy.accuracy * 100,
            modelName: model.modelName
          });
        });
        if (points.length === 0) {
          $('#accuracyThroughputEmpty').text('No accuracy data available for this report.');
          return;
        }

        var chartData = points.map(function(point) {
          return {
            x: point.x,
            y: point.y,
            modelName: point.modelName,
            backgroundColor: getColorForAccuracy(point.y)
          };
        });

        var labelPlugin = {
          id: 'modelLabels',
          afterDatasetsDraw: function(chart) {
            var ctx = chart.ctx;
            chart.data.datasets.forEach(function(dataset, datasetIndex) {
              var meta = chart.getDatasetMeta(datasetIndex);
              meta.data.forEach(function(element, index) {
                var data = chartData[index];
                if (!data) {
                  return;
                }
                var modelName = (data.modelName || '').split(':')[0];
                if (!modelName) {
                  return;
                }
                ctx.fillStyle = '#0F172A';
                ctx.font = 'bold 11px sans-serif';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'bottom';
                ctx.fillText(modelName, element.x, element.y - 12);
              });
            });
          }
        };

        new Chart(canvas, {
          type: 'scatter',
          data: {
            datasets: [{
              data: chartData,
              pointRadius: 8,
              pointHoverRadius: 12,
              pointBackgroundColor: chartData.map(function(d) { return d.backgroundColor; }),
              pointBorderColor: '#ffffff',
              pointBorderWidth: 2,
              pointHoverBorderWidth: 3
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: {
                title: {
                  display: true,
                  text: 'Throughput (tokens/second)',
                  font: {
                    size: 14,
                    weight: 'bold'
                  },
                  color: '#64748B'
                },
                grid: {
                  color: 'rgba(0, 0, 0, 0.05)'
                },
                ticks: {
                  color: '#64748B'
                }
              },
              y: {
                title: {
                  display: true,
                  text: 'Accuracy (%)',
                  font: {
                    size: 14,
                    weight: 'bold'
                  },
                  color: '#64748B'
                },
                suggestedMin: 0,
                suggestedMax: 100,
                grid: {
                  color: 'rgba(0, 0, 0, 0.05)'
                },
                ticks: {
                  color: '#64748B',
                  callback: function(value) {
                    return value + '%';
                  }
                }
              }
            },
            plugins: {
              legend: {
                display: false
              },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    var point = context.raw || {};
                    var tps = typeof point.x === 'number' ? point.x.toFixed(2) : 'n/a';
                    var acc = typeof point.y === 'number' ? point.y.toFixed(1) : 'n/a';
                    return [
                      'Throughput: ' + tps + ' tokens/sec',
                      'Accuracy: ' + acc + '%'
                    ];
                  },
                  title: function(items) {
                    if (!items.length) {
                      return 'model';
                    }
                    var data = chartData[items[0].dataIndex] || {};
                    return data.modelName || 'model';
                  }
                }
              }
            }
          },
          plugins: [labelPlugin]
        });
      }

      function attachSorting() {
        $('#modelsTable thead th.sortable').each(function(index) {
          var direction = 'none';
          $(this).on('click', function() {
            var type = $(this).data('type');
            direction = direction === 'asc' ? 'desc' : 'asc';
            sortTable(index, type, direction);
            updateSortIcons($(this), direction);
          });
        });
      }

      function buildInputTokenCharts(models) {
        var ttftCanvas = document.getElementById('inputTokensTtftChart');
        var tpsCanvas = document.getElementById('inputTokensTpsChart');
        if (!ttftCanvas || !tpsCanvas) {
          return;
        }

        var palette = [
          '#334155', '#64748B', '#94A3B8', '#CBD5E1', '#3B82F6',
          '#1D4ED8', '#0EA5E9', '#38BDF8', '#14B8A6', '#10B981'
        ];
        var modelColors = {};
        models.forEach(function(model, index) {
          modelColors[model.modelName] = palette[index % palette.length];
        });

        function collectPoints(selectedModel) {
          var datasets = [];
          models.forEach(function(model) {
            if (selectedModel && selectedModel !== 'all' && model.modelName !== selectedModel) {
              return;
            }
            var iterations = model.iterations || [];
            if (!iterations.length) {
              return;
            }
            var color = modelColors[model.modelName] || '#64748B';
            var ttftPoints = [];
            var tpsPoints = [];
            iterations.forEach(function(iter) {
              if (!iter || !iter.stats) {
                return;
              }
              var inputTokens = Number(iter.stats.inputTokenCount);
              if (isNaN(inputTokens)) {
                return;
              }
              var ttftMs = Number(iter.stats.timeToFirstToken) / 1e6;
              if (!isNaN(ttftMs)) {
                ttftPoints.push({ x: inputTokens, y: ttftMs, modelName: model.modelName });
              }
              var tps = Number(iter.stats.tokensPerSecond);
              if (!isNaN(tps)) {
                tpsPoints.push({ x: inputTokens, y: tps, modelName: model.modelName });
              }
            });
            if (ttftPoints.length) {
              datasets.push({
                model: model.modelName,
                color: color,
                ttft: ttftPoints,
                tps: tpsPoints
              });
            }
          });
          return datasets;
        }

        function buildChart(canvas, datasets, yLabel, yFormatter) {
          return new Chart(canvas, {
            type: 'scatter',
            data: {
              datasets: datasets.map(function(dataset) {
                return {
                  label: dataset.model,
                  data: yLabel.indexOf('TTFT') !== -1 ? dataset.ttft : dataset.tps,
                  backgroundColor: dataset.color,
                  borderColor: '#ffffff',
                  borderWidth: 1,
                  pointRadius: 6,
                  pointHoverRadius: 9
                };
              })
            },
            options: {
              responsive: true,
              maintainAspectRatio: false,
              animation: false,
              scales: {
                x: {
                  title: {
                    display: true,
                    text: 'Input tokens',
                    font: { size: 13, weight: 'bold' },
                    color: '#64748B'
                  },
                  grid: { color: 'rgba(0, 0, 0, 0.05)' },
                  ticks: { color: '#64748B' }
                },
                y: {
                  title: {
                    display: true,
                    text: yLabel,
                    font: { size: 13, weight: 'bold' },
                    color: '#64748B'
                  },
                  grid: { color: 'rgba(0, 0, 0, 0.05)' },
                  ticks: {
                    color: '#64748B',
                    callback: yFormatter
                  }
                }
              },
              plugins: {
                legend: {
                  position: 'bottom',
                  labels: {
                    usePointStyle: true,
                    boxWidth: 8,
                    color: '#64748B'
                  }
                },
                tooltip: {
                  callbacks: {
                    title: function(items) {
                      if (!items.length) {
                        return 'model';
                      }
                      var point = items[0].raw || {};
                      return point.modelName || items[0].dataset.label || 'model';
                    },
                    label: function(context) {
                      var point = context.raw || {};
                      var x = typeof point.x === 'number' ? point.x.toFixed(0) : 'n/a';
                      var y = typeof point.y === 'number' ? point.y.toFixed(2) : 'n/a';
                      return 'Input tokens: ' + x + ', ' + yLabel + ': ' + y;
                    }
                  }
                }
              }
            }
          });
        }

        var modelsWithIterations = models.filter(function(model) {
          return model.iterations && model.iterations.length;
        });
        var filter = $('#inputTokenModelFilter').empty();
        filter.append('<option value="all">All models</option>');
        modelsWithIterations.forEach(function(model) {
          filter.append('<option value="' + model.modelName + '">' + model.modelName + '</option>');
        });

        var ttftChart = null;
        var tpsChart = null;

        function renderCharts() {
          if (ttftChart) {
            ttftChart.destroy();
          }
          if (tpsChart) {
            tpsChart.destroy();
          }
          $('#inputTokensTtftEmpty').text('');
          $('#inputTokensTpsEmpty').text('');

          var selected = filter.val();
          var datasets = collectPoints(selected);
          if (!datasets.length) {
            $('#inputTokensTtftEmpty').text('No input token data available for this selection.');
            $('#inputTokensTpsEmpty').text('No input token data available for this selection.');
            return;
          }
          ttftChart = buildChart(ttftCanvas, datasets, 'TTFT (ms)', function(value) { return Math.round(value) + ' ms'; });
          tpsChart = buildChart(tpsCanvas, datasets, 'Tokens per second', function(value) { return Math.round(value); });
        }

        filter.on('change', renderCharts);
        renderCharts();
      }

      function sortTable(columnIndex, type, direction) {
        var $tbody = $('#modelsTable tbody');
        var rows = $tbody.find('tr').get();
        rows.sort(function(a, b) {
          var A = $(a).children().eq(columnIndex).text();
          var B = $(b).children().eq(columnIndex).text();
          if (type === 'number') {
            A = parseFloat($(a).children().eq(columnIndex).attr('data-value')) || 0;
            B = parseFloat($(b).children().eq(columnIndex).attr('data-value')) || 0;
          }
          if (A < B) {
            return direction === 'asc' ? -1 : 1;
          }
          if (A > B) {
            return direction === 'asc' ? 1 : -1;
          }
          return 0;
        });
        $.each(rows, function(_, row) {
          $tbody.append(row);
        });
      }

      
      function applyTheme(theme) {
        var selected = theme === 'dark' ? 'dark' : 'light';
        document.documentElement.setAttribute('data-theme', selected);
        var toggle = document.getElementById('themeToggle');
        if (toggle) {
          var icon = toggle.querySelector('.material-icons-two-tone');
          var label = selected === 'dark' ? 'Switch to light mode' : 'Switch to dark mode';
          toggle.setAttribute('aria-label', label);
          if (icon) {
            icon.textContent = selected === 'dark' ? 'light_mode' : 'dark_mode';
          }
        }
        try {
          localStorage.setItem('agon-theme', selected);
        } catch (e) {}
      }

      function initThemeToggle() {
        var saved = null;
        try {
          saved = localStorage.getItem('agon-theme');
        } catch (e) {}
        applyTheme(saved || 'light');
        var toggle = document.getElementById('themeToggle');
        if (!toggle) {
          return;
        }
        toggle.addEventListener('click', function() {
          var current = document.documentElement.getAttribute('data-theme');
          applyTheme(current === 'dark' ? 'light' : 'dark');
        });
      }$(function() {
        initThemeToggle();
        if (!analysis) {
          return;
        }
        var generatedAt = analysis.generatedAt ? new Date(analysis.generatedAt) : null;
        if (generatedAt) {
          $('#generatedAt').text(generatedAt.toLocaleString());
        }

        var summary = analysis.overall || {};
        $('#fastestModel').text(summary.fastestModel || '-');
        $('#bestLatencyModel').text(summary.bestLatencyModel || '-');
        $('#mostEfficientModel').text(summary.mostEfficientModel || '-');
        $('#mostAccurateModel').text(summary.mostAccurateModel || '-');
        $('#bestTradeoffModel').text(summary.bestTradeoffModel || '-');

        var models = analysis.models || [];
        var timeoutSeconds = null;
        models.forEach(function(model) {
          if (model.accuracy && typeof model.accuracy.timeoutSeconds === 'number') {
            if (timeoutSeconds === null || model.accuracy.timeoutSeconds > timeoutSeconds) {
              timeoutSeconds = model.accuracy.timeoutSeconds;
            }
          }
        });
        if (timeoutSeconds !== null && timeoutSeconds > 0) {
          var headerText = 'Timeouts (' + timeoutSeconds + 's) ';
          var $header = $('#timeoutsHeader');
          var textNode = $header.contents().filter(function() {
            return this.nodeType === 3;
          }).first();
          if (textNode.length) {
            textNode[0].textContent = headerText;
          } else {
            $header.prepend(document.createTextNode(headerText));
          }
        }
        var interactiveCount = models.filter(function(model) {
          return model.labels && model.labels.interactiveSuitability === 'good';
        }).length;
        $('#interactiveCount').text(interactiveCount);

        populateTable(models);
        attachSorting();
        buildAccuracyThroughputChart(models);
        buildInputTokenCharts(models);
        buildAccordion(models);
        populateAnomalies(analysis.anomalies || []);
        populateRecommendations(analysis.recommendations || []);
      });
    })(jQuery);
  </script>
</body>
</html>`
