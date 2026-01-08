package agon

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

func runAnalyzeMetrics(cmd *cobra.Command, opts analyzeMetricsOptions) error {
	combined, err := metrics.LoadCombinedMetrics(
		opts.accuracyResultsDir,
		opts.benchmarksDir,
		opts.metadataDir,
	)
	if err != nil {
		return err
	}

	if opts.analysisPath != "" {
		if err := writeAnalysisJSON(opts.analysisPath, combined); err != nil {
			return err
		}
		cmd.Printf("Analysis JSON written to %s\n", opts.analysisPath)
	}

	html, err := metrics.GenerateCombinedReport(combined)
	if err != nil {
		return fmt.Errorf("failed generating HTML report: %w", err)
	}

	if opts.htmlPath == "" {
		opts.htmlPath = "reports/metrics-report.html"
	}

	if err := os.WriteFile(opts.htmlPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("unable to write HTML report %s: %w", opts.htmlPath, err)
	}

	cmd.Printf("Report written to %s\n", opts.htmlPath)
	return nil
}

func writeAnalysisJSON(path string, analysis metrics.CombinedMetrics) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("unable to create directory for %s: %w", path, err)
		}
	}

	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal analysis JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("unable to write analysis JSON %s: %w", path, err)
	}
	return nil
}

func parseBenchmarkResults(raw []byte) (metrics.BenchmarkResults, error) {
	var results metrics.BenchmarkResults
	if err := json.Unmarshal(raw, &results); err == nil && len(results) > 0 {
		return results, nil
	}

	llamaBench, err := parseLlamaCppBench(raw)
	if err != nil {
		return nil, err
	}
	if len(llamaBench) > 0 {
		return llamaBench, nil
	}

	var modelMetrics []metrics.ModelMetrics
	if err := json.Unmarshal(raw, &modelMetrics); err == nil && len(modelMetrics) > 0 {
		filtered := make([]metrics.ModelMetrics, 0, len(modelMetrics))
		for _, model := range modelMetrics {
			if model.ModelName != "" {
				filtered = append(filtered, model)
			}
		}
		if len(filtered) > 0 {
			return convertModelMetrics(filtered), nil
		}
	}

	// Final attempt: allow empty payload that still unmarshals into map.
	if results != nil {
		return results, nil
	}

	return nil, fmt.Errorf("json did not match benchmark results schema or aggregator metrics array")
}

func convertModelMetrics(models []metrics.ModelMetrics) metrics.BenchmarkResults {
	out := make(metrics.BenchmarkResults, len(models))
	for _, m := range models {
		overall := m.OverallStats
		bench := metrics.ModelBenchmark{
			ModelName:      m.ModelName,
			BenchmarkCount: int(overall.TotalRequests),
			AverageStats: metrics.Stats{
				TotalExecutionTime: msToNs(overall.TotalDurationMillis.Mean),
				TimeToFirstToken:   msToNs(overall.TTFTMillis.Mean),
				TokensPerSecond:    overall.TokensPerSecond.Mean,
				InputTokenCount:    roundToInt(overall.InputTokens.Mean),
				OutputTokenCount:   roundToInt(overall.OutputTokens.Mean),
			},
			MinStats: metrics.Stats{
				TotalExecutionTime: msToNs(overall.TotalDurationMillis.Min),
				TimeToFirstToken:   msToNs(overall.TTFTMillis.Min),
				TokensPerSecond:    overall.TokensPerSecond.Min,
				InputTokenCount:    roundToInt(overall.InputTokens.Min),
				OutputTokenCount:   roundToInt(overall.OutputTokens.Min),
			},
			MaxStats: metrics.Stats{
				TotalExecutionTime: msToNs(overall.TotalDurationMillis.Max),
				TimeToFirstToken:   msToNs(overall.TTFTMillis.Max),
				TokensPerSecond:    overall.TokensPerSecond.Max,
				InputTokenCount:    roundToInt(overall.InputTokens.Max),
				OutputTokenCount:   roundToInt(overall.OutputTokens.Max),
			},
			Iterations: nil,
		}
		out[m.ModelName] = bench
	}
	return out
}

func msToNs(ms float64) int64 {
	return int64(ms * 1e6)
}

func roundToInt(val float64) int {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	return int(math.Round(val))
}

type llamaCppBenchEntry struct {
	ModelFilename string    `json:"model_filename"`
	NPrompt       int       `json:"n_prompt"`
	NGen          int       `json:"n_gen"`
	AvgNs         float64   `json:"avg_ns"`
	SamplesNs     []float64 `json:"samples_ns"`
}

func parseLlamaCppBench(raw []byte) (metrics.BenchmarkResults, error) {
	var entries []llamaCppBenchEntry
	if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
		return nil, nil
	}

	type benchParts struct {
		prompt *llamaCppBenchEntry
		gen    *llamaCppBenchEntry
	}

	partsByModel := make(map[string]benchParts)
	for _, entry := range entries {
		if entry.ModelFilename == "" {
			continue
		}
		name := modelNameFromFilename(entry.ModelFilename)
		if name == "" {
			continue
		}
		parts := partsByModel[name]
		if entry.NGen > 0 {
			if parts.gen == nil || entry.NGen > parts.gen.NGen {
				copyEntry := entry
				parts.gen = &copyEntry
			}
		} else if entry.NPrompt > 0 {
			if parts.prompt == nil || entry.NPrompt > parts.prompt.NPrompt {
				copyEntry := entry
				parts.prompt = &copyEntry
			}
		}
		partsByModel[name] = parts
	}

	if len(partsByModel) == 0 {
		return nil, nil
	}

	results := make(metrics.BenchmarkResults, len(partsByModel))
	for name, parts := range partsByModel {
		inputTokens := 0
		outputTokens := 0
		if parts.prompt != nil {
			inputTokens = parts.prompt.NPrompt
		}
		if parts.gen != nil {
			outputTokens = parts.gen.NGen
			if inputTokens == 0 {
				inputTokens = parts.gen.NPrompt
			}
		}

		iterations := buildLlamaBenchIterations(parts.prompt, parts.gen, inputTokens, outputTokens)
		avg, min, max := buildStatsFromIterations(iterations)

		results[name] = metrics.ModelBenchmark{
			ModelName:      name,
			BenchmarkCount: len(iterations),
			AverageStats:   avg,
			MinStats:       min,
			MaxStats:       max,
			Iterations:     iterations,
		}
	}

	return results, nil
}

func modelNameFromFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func buildLlamaBenchIterations(prompt, gen *llamaCppBenchEntry, inputTokens, outputTokens int) []metrics.Iteration {
	promptSamples := []float64{}
	genSamples := []float64{}
	promptAvg := 0.0
	genAvg := 0.0

	if prompt != nil {
		promptSamples = prompt.SamplesNs
		promptAvg = prompt.AvgNs
	}
	if gen != nil {
		genSamples = gen.SamplesNs
		genAvg = gen.AvgNs
	}

	count := 0
	switch {
	case len(promptSamples) > 0 && len(genSamples) > 0:
		if len(promptSamples) < len(genSamples) {
			count = len(promptSamples)
		} else {
			count = len(genSamples)
		}
	case len(promptSamples) > 0:
		count = len(promptSamples)
	case len(genSamples) > 0:
		count = len(genSamples)
	default:
		if promptAvg > 0 || genAvg > 0 {
			count = 1
		}
	}

	if count == 0 {
		return nil
	}

	iterations := make([]metrics.Iteration, 0, count)
	for i := 0; i < count; i++ {
		promptNs := promptAvg
		genNs := genAvg
		if i < len(promptSamples) {
			promptNs = promptSamples[i]
		}
		if i < len(genSamples) {
			genNs = genSamples[i]
		}

		totalNs := promptNs + genNs
		tps := 0.0
		if totalNs > 0 && outputTokens > 0 {
			tps = float64(outputTokens) / (totalNs / 1e9)
		}

		iterations = append(iterations, metrics.Iteration{
			Iteration: i + 1,
			Stats: metrics.Stats{
				TotalExecutionTime: int64(totalNs),
				TimeToFirstToken:   int64(promptNs),
				TokensPerSecond:    tps,
				InputTokenCount:    inputTokens,
				OutputTokenCount:   outputTokens,
			},
		})
	}

	return iterations
}

func buildStatsFromIterations(iterations []metrics.Iteration) (metrics.Stats, metrics.Stats, metrics.Stats) {
	if len(iterations) == 0 {
		return metrics.Stats{}, metrics.Stats{}, metrics.Stats{}
	}

	min := iterations[0].Stats
	max := iterations[0].Stats

	var (
		sumTotal  int64
		sumTTFT   int64
		sumTPS    float64
		sumInput  int
		sumOutput int
	)

	for _, iter := range iterations {
		stats := iter.Stats
		sumTotal += stats.TotalExecutionTime
		sumTTFT += stats.TimeToFirstToken
		sumTPS += stats.TokensPerSecond
		sumInput += stats.InputTokenCount
		sumOutput += stats.OutputTokenCount

		if stats.TotalExecutionTime < min.TotalExecutionTime {
			min.TotalExecutionTime = stats.TotalExecutionTime
		}
		if stats.TotalExecutionTime > max.TotalExecutionTime {
			max.TotalExecutionTime = stats.TotalExecutionTime
		}
		if stats.TimeToFirstToken < min.TimeToFirstToken {
			min.TimeToFirstToken = stats.TimeToFirstToken
		}
		if stats.TimeToFirstToken > max.TimeToFirstToken {
			max.TimeToFirstToken = stats.TimeToFirstToken
		}
		if stats.TokensPerSecond < min.TokensPerSecond {
			min.TokensPerSecond = stats.TokensPerSecond
		}
		if stats.TokensPerSecond > max.TokensPerSecond {
			max.TokensPerSecond = stats.TokensPerSecond
		}
		if stats.InputTokenCount < min.InputTokenCount {
			min.InputTokenCount = stats.InputTokenCount
		}
		if stats.InputTokenCount > max.InputTokenCount {
			max.InputTokenCount = stats.InputTokenCount
		}
		if stats.OutputTokenCount < min.OutputTokenCount {
			min.OutputTokenCount = stats.OutputTokenCount
		}
		if stats.OutputTokenCount > max.OutputTokenCount {
			max.OutputTokenCount = stats.OutputTokenCount
		}
	}

	count := float64(len(iterations))
	avg := metrics.Stats{
		TotalExecutionTime: int64(float64(sumTotal) / count),
		TimeToFirstToken:   int64(float64(sumTTFT) / count),
		TokensPerSecond:    sumTPS / count,
		InputTokenCount:    int(math.Round(float64(sumInput) / count)),
		OutputTokenCount:   int(math.Round(float64(sumOutput) / count)),
	}

	return avg, min, max
}
