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
	analysisPath        string
	accuracyResultsDir string
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
		var results metrics.BenchmarkResults
		if analyzeMetricsOpts.accuracyOnly {
			var err error
			results, err = loadAccuracyPerformanceResults(analyzeMetricsOpts.accuracyResultsDir)
			if err != nil {
				return err
			}
		} else {
			if analyzeMetricsOpts.inputPath == "" {
				return fmt.Errorf("input benchmark file is required (pass --input)")
			}
			data, err := os.ReadFile(analyzeMetricsOpts.inputPath)
			if err != nil {
				return fmt.Errorf("unable to read benchmark file %s: %w", analyzeMetricsOpts.inputPath, err)
			}

			results, err = parseBenchmarkResults(data)
			if err != nil {
				return fmt.Errorf("unable to parse benchmark JSON %s: %w", analyzeMetricsOpts.inputPath, err)
			}
		}

		host := metrics.HostInfo{
			ClusterName: analyzeMetricsOpts.hostName,
			Notes:       analyzeMetricsOpts.hostNotes,
		}

		accuracyStats, err := loadAccuracyStats(analyzeMetricsOpts.accuracyResultsDir)
		if err != nil {
			return err
		}

		analysis := metrics.AnalyzeMetrics(results, host, accuracyStats)

		if analyzeMetricsOpts.analysisPath != "" {
			if err := writeAnalysisJSON(analyzeMetricsOpts.analysisPath, analysis); err != nil {
				return err
			}
			cmd.Printf("Analysis JSON written to %s\n", analyzeMetricsOpts.analysisPath)
		}

		html, err := metrics.GenerateReport(analysis)
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
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.htmlPath, "html-output", "reports/metrics-report.html", "Destination HTML report path")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.analysisPath, "analysis-output", "", "Optional path to write the analysis JSON")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.accuracyResultsDir, "accuracy-results", "accuracy/results", "Optional path to accuracy JSONL results directory")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostName, "host-name", "", "Optional cluster/host label to embed in the analysis")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostNotes, "host-notes", "", "Optional host notes to embed in the analysis")
	analyzeMetricsCmd.Flags().BoolVar(&analyzeMetricsOpts.accuracyOnly, "accuracy-only", true, "Build the report from accuracy JSONL data instead of benchmark JSON")

	analyzeCmd.AddCommand(analyzeMetricsCmd)
}

func writeAnalysisJSON(path string, analysis metrics.Analysis) error {
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

	var modelMetrics []metrics.ModelMetrics
	if err := json.Unmarshal(raw, &modelMetrics); err == nil && len(modelMetrics) > 0 {
		return convertModelMetrics(modelMetrics), nil
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

type accuracyLine struct {
	Model         string `json:"model"`
	Correct       bool   `json:"correct"`
	Difficulty    int    `json:"difficulty"`
	MarginOfError int    `json:"marginOfError"`
	DeadlineExceeded bool `json:"deadlineExceeded"`
	DeadlineTimeoutSec int `json:"deadlineTimeout"`
	TimeToFirstToken int     `json:"time_to_first_token"`
	TokensPerSecond  float64 `json:"tokens_per_second"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalDurationMs  int     `json:"total_duration_ms"`
}

type accuracyTotals struct {
	Total         int
	Correct       int
	DifficultySum int
	MarginSum     int
	Timeouts      int
	TimeoutSeconds int
	ByDifficulty  map[int]accuracyTotals
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
