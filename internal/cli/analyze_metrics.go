// internal/cli/analyze_metrics.go
package agon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

type analyzeMetricsOptions struct {
	inputPath          string
	htmlPath           string
	analysisPath       string
	accuracyResultsDir string
	benchmarksDir      string
	metadataDir        string
	hostName           string
	hostNotes          string
	accuracyOnly       bool
}

var analyzeMetricsOpts analyzeMetricsOptions

// analyzeMetricsCmd turns a raw benchmark JSON file into analysis JSON + HTML.
var analyzeMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Generate metric analysis & report from benchmark JSON",
	Long: `Read raw benchmark output (the JSON written by benchmark runs), compute
derived metrics, and emit both the analysis JSON and a self-contained HTML
dashboard for review.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		combined, err := metrics.LoadCombinedMetrics(
			analyzeMetricsOpts.accuracyResultsDir,
			analyzeMetricsOpts.benchmarksDir,
			analyzeMetricsOpts.metadataDir,
		)
		if err != nil {
			return err
		}

		if analyzeMetricsOpts.analysisPath != "" {
			if err := writeAnalysisJSON(analyzeMetricsOpts.analysisPath, combined); err != nil {
				return err
			}
			cmd.Printf("Analysis JSON written to %s\n", analyzeMetricsOpts.analysisPath)
		}

		html, err := metrics.GenerateCombinedReport(combined)
		if err != nil {
			return fmt.Errorf("failed generating HTML report: %w", err)
		}

		if analyzeMetricsOpts.htmlPath == "" {
			analyzeMetricsOpts.htmlPath = "reports/metrics-report.html"
		}

		if err := os.WriteFile(analyzeMetricsOpts.htmlPath, []byte(html), 0o644); err != nil {
			return fmt.Errorf("unable to write HTML report %s: %w", analyzeMetricsOpts.htmlPath, err)
		}

		cmd.Printf("Report written to %s\n", analyzeMetricsOpts.htmlPath)
		return nil
	},
}

func init() {
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.inputPath, "input", "reports/data/model_performance_metrics.json", "Path to benchmark JSON (required)")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.benchmarksDir, "benchmarks-dir", "agonData/modelBenchmarks", "Path to a directory of benchmark JSON files")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.metadataDir, "metadata-dir", "agonData/modelMetadata", "Path to a directory of model metadata JSON files")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.htmlPath, "html-output", "reports/metrics-report.html", "Destination HTML report path")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.analysisPath, "analysis-output", "", "Optional path to write the analysis JSON")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.accuracyResultsDir, "accuracy-results", "agonData/modelAccuracy", "Optional path to accuracy JSONL results directory")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostName, "host-name", "", "Optional cluster/host label to embed in the analysis")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostNotes, "host-notes", "", "Optional host notes to embed in the analysis")
	analyzeMetricsCmd.Flags().BoolVar(&analyzeMetricsOpts.accuracyOnly, "accuracy-only", true, "Build the report from accuracy JSONL data instead of benchmark JSON")

	analyzeCmd.AddCommand(analyzeMetricsCmd)
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

type accuracyLine struct {
	Model              string  `json:"model"`
	Correct            bool    `json:"correct"`
	Difficulty         int     `json:"difficulty"`
	MarginOfError      int     `json:"marginOfError"`
	DeadlineExceeded   bool    `json:"deadlineExceeded"`
	DeadlineTimeoutSec int     `json:"deadlineTimeout"`
	TimeToFirstToken   int     `json:"time_to_first_token"`
	TokensPerSecond    float64 `json:"tokens_per_second"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalDurationMs    int     `json:"total_duration_ms"`
}

type accuracyTotals struct {
	Total          int
	Correct        int
	DifficultySum  int
	MarginSum      int
	Timeouts       int
	TimeoutSeconds int
	ByDifficulty   map[int]accuracyTotals
}

func loadAccuracyStats(dir string) (map[string]metrics.AccuracyStats, error) {
	if dir == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to stat accuracy results dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("accuracy results path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to read accuracy results dir %s: %w", dir, err)
	}

	totals := make(map[string]accuracyTotals)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("unable to open accuracy results file %s: %w", path, err)
		}

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec accuracyLine
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("unable to parse accuracy JSONL %s:%d: %w", path, lineNo, err)
			}
			if rec.Model == "" {
				_ = file.Close()
				return nil, fmt.Errorf("accuracy JSONL missing model field %s:%d", path, lineNo)
			}
			stat := totals[rec.Model]
			if rec.DeadlineTimeoutSec > stat.TimeoutSeconds {
				stat.TimeoutSeconds = rec.DeadlineTimeoutSec
			}
			if rec.DeadlineExceeded {
				stat.Timeouts++
				totals[rec.Model] = stat
				continue
			}
			stat.Total++
			if rec.Correct {
				stat.Correct++
			}
			stat.DifficultySum += rec.Difficulty
			stat.MarginSum += rec.MarginOfError
			if stat.ByDifficulty == nil {
				stat.ByDifficulty = make(map[int]accuracyTotals)
			}
			diffStat := stat.ByDifficulty[rec.Difficulty]
			diffStat.Total++
			if rec.Correct {
				diffStat.Correct++
			}
			stat.ByDifficulty[rec.Difficulty] = diffStat
			totals[rec.Model] = stat
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("error reading accuracy results file %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("error closing accuracy results file %s: %w", path, err)
		}
	}

	stats := make(map[string]metrics.AccuracyStats, len(totals))
	for model, stat := range totals {
		accuracy := 0.0
		avgDifficulty := 0.0
		avgMargin := 0.0
		if stat.Total > 0 {
			accuracy = float64(stat.Correct) / float64(stat.Total)
			avgDifficulty = float64(stat.DifficultySum) / float64(stat.Total)
			avgMargin = float64(stat.MarginSum) / float64(stat.Total)
		}
		byDifficulty := make(map[int]metrics.AccuracyBucket, len(stat.ByDifficulty))
		for difficulty, diffStat := range stat.ByDifficulty {
			diffAccuracy := 0.0
			if diffStat.Total > 0 {
				diffAccuracy = float64(diffStat.Correct) / float64(diffStat.Total)
			}
			byDifficulty[difficulty] = metrics.AccuracyBucket{
				Total:    diffStat.Total,
				Correct:  diffStat.Correct,
				Accuracy: diffAccuracy,
			}
		}
		stats[model] = metrics.AccuracyStats{
			Total:            stat.Total,
			Correct:          stat.Correct,
			Accuracy:         accuracy,
			AvgDifficulty:    avgDifficulty,
			AvgMarginOfError: avgMargin,
			Timeouts:         stat.Timeouts,
			TimeoutSeconds:   stat.TimeoutSeconds,
			ByDifficulty:     byDifficulty,
		}
	}

	return stats, nil
}

func loadAccuracyPerformanceResults(dir string) (metrics.BenchmarkResults, error) {
	if dir == "" {
		return metrics.BenchmarkResults{}, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return metrics.BenchmarkResults{}, nil
		}
		return nil, fmt.Errorf("unable to stat accuracy results dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("accuracy results path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to read accuracy results dir %s: %w", dir, err)
	}

	type perfSample struct {
		tps          float64
		ttftMs       int
		inputTokens  int
		outputTokens int
		totalMs      int
	}

	perfByModel := make(map[string][]perfSample)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("unable to open accuracy results file %s: %w", path, err)
		}

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec accuracyLine
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("unable to parse accuracy JSONL %s:%d: %w", path, lineNo, err)
			}
			if rec.Model == "" {
				_ = file.Close()
				return nil, fmt.Errorf("accuracy JSONL missing model field %s:%d", path, lineNo)
			}
			if rec.DeadlineExceeded {
				continue
			}
			perfByModel[rec.Model] = append(perfByModel[rec.Model], perfSample{
				tps:          rec.TokensPerSecond,
				ttftMs:       rec.TimeToFirstToken,
				inputTokens:  rec.InputTokens,
				outputTokens: rec.OutputTokens,
				totalMs:      rec.TotalDurationMs,
			})
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("error reading accuracy results file %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("error closing accuracy results file %s: %w", path, err)
		}
	}

	results := make(metrics.BenchmarkResults, len(perfByModel))
	for model, samples := range perfByModel {
		if len(samples) == 0 {
			continue
		}

		var (
			sumTPS, sumTTFT, sumInput, sumOutput, sumTotal float64
			minTPS, maxTPS                                 float64
			minTTFT, maxTTFT                               int
			minInput, maxInput                             int
			minOutput, maxOutput                           int
			minTotal, maxTotal                             int
		)

		for i, s := range samples {
			sumTPS += s.tps
			sumTTFT += float64(s.ttftMs)
			sumInput += float64(s.inputTokens)
			sumOutput += float64(s.outputTokens)
			sumTotal += float64(s.totalMs)
			if i == 0 || s.tps < minTPS {
				minTPS = s.tps
			}
			if i == 0 || s.tps > maxTPS {
				maxTPS = s.tps
			}
			if i == 0 || s.ttftMs < minTTFT {
				minTTFT = s.ttftMs
			}
			if i == 0 || s.ttftMs > maxTTFT {
				maxTTFT = s.ttftMs
			}
			if i == 0 || s.inputTokens < minInput {
				minInput = s.inputTokens
			}
			if i == 0 || s.inputTokens > maxInput {
				maxInput = s.inputTokens
			}
			if i == 0 || s.outputTokens < minOutput {
				minOutput = s.outputTokens
			}
			if i == 0 || s.outputTokens > maxOutput {
				maxOutput = s.outputTokens
			}
			if i == 0 || s.totalMs < minTotal {
				minTotal = s.totalMs
			}
			if i == 0 || s.totalMs > maxTotal {
				maxTotal = s.totalMs
			}
		}

		count := float64(len(samples))
		avgTPS := sumTPS / count
		avgTTFT := sumTTFT / count
		avgInput := sumInput / count
		avgOutput := sumOutput / count
		avgTotal := sumTotal / count

		iterations := make([]metrics.Iteration, 0, len(samples))
		for i, s := range samples {
			iterations = append(iterations, metrics.Iteration{
				Iteration: i + 1,
				Stats: metrics.Stats{
					TotalExecutionTime: int64(s.totalMs) * 1e6,
					TimeToFirstToken:   int64(s.ttftMs) * 1e6,
					TokensPerSecond:    s.tps,
					InputTokenCount:    s.inputTokens,
					OutputTokenCount:   s.outputTokens,
				},
			})
		}

		results[model] = metrics.ModelBenchmark{
			ModelName:      model,
			BenchmarkCount: len(samples),
			AverageStats: metrics.Stats{
				TotalExecutionTime: int64(avgTotal * 1e6),
				TimeToFirstToken:   int64(avgTTFT * 1e6),
				TokensPerSecond:    avgTPS,
				InputTokenCount:    int(math.Round(avgInput)),
				OutputTokenCount:   int(math.Round(avgOutput)),
			},
			MinStats: metrics.Stats{
				TotalExecutionTime: int64(minTotal) * 1e6,
				TimeToFirstToken:   int64(minTTFT) * 1e6,
				TokensPerSecond:    minTPS,
				InputTokenCount:    minInput,
				OutputTokenCount:   minOutput,
			},
			MaxStats: metrics.Stats{
				TotalExecutionTime: int64(maxTotal) * 1e6,
				TimeToFirstToken:   int64(maxTTFT) * 1e6,
				TokensPerSecond:    maxTPS,
				InputTokenCount:    maxInput,
				OutputTokenCount:   maxOutput,
			},
			Iterations: iterations,
		}
	}

	return results, nil
}

func loadBenchmarksDir(dir string) (metrics.BenchmarkResults, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to stat benchmarks dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("benchmarks path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to read benchmarks dir %s: %w", dir, err)
	}

	merged := make(metrics.BenchmarkResults)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read benchmark file %s: %w", path, err)
		}
		results, err := parseBenchmarkResults(data)
		if err != nil {
			return nil, fmt.Errorf("unable to parse benchmark file %s: %w", path, err)
		}
		for name, bench := range results {
			if _, exists := merged[name]; exists {
				return nil, fmt.Errorf("duplicate model %q found in %s", name, path)
			}
			merged[name] = bench
		}
	}

	return merged, nil
}
