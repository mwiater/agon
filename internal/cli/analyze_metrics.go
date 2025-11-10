// internal/cli/analyze_metrics.go
package agon

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

type analyzeMetricsOptions struct {
	inputPath    string
	htmlPath     string
	analysisPath string
	hostName     string
	hostNotes    string
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
		if analyzeMetricsOpts.inputPath == "" {
			return fmt.Errorf("input benchmark file is required (pass --input)")
		}

		data, err := os.ReadFile(analyzeMetricsOpts.inputPath)
		if err != nil {
			return fmt.Errorf("unable to read benchmark file %s: %w", analyzeMetricsOpts.inputPath, err)
		}

		results, err := parseBenchmarkResults(data)
		if err != nil {
			return fmt.Errorf("unable to parse benchmark JSON %s: %w", analyzeMetricsOpts.inputPath, err)
		}

		host := metrics.HostInfo{
			ClusterName: analyzeMetricsOpts.hostName,
			Notes:       analyzeMetricsOpts.hostNotes,
		}

		analysis := metrics.AnalyzeMetrics(results, host)

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
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostName, "host-name", "", "Optional cluster/host label to embed in the analysis")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.hostNotes, "host-notes", "", "Optional host notes to embed in the analysis")

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
