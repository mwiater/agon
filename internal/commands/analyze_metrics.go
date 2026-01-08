// internal/commands/analyze_metrics.go
package agon

import (
	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

var analyzeMetricsOpts metrics.AnalyzeOptions

// analyzeMetricsCmd turns a raw benchmark JSON file into analysis JSON + HTML.
var analyzeMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Generate metric analysis & report from benchmark JSON",
	Long: `Read raw benchmark output (the JSON written by benchmark runs), compute
derived metrics, and emit both the analysis JSON and a self-contained HTML
dashboard for review.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return metrics.AnalyzeMetrics(analyzeMetricsOpts, cmd.OutOrStdout())
	},
}

func init() {
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.InputPath, "input", "internal/reports/data/model_performance_metrics.json", "Path to benchmark JSON (required)")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.BenchmarksDir, "benchmarks-dir", "agonData/modelBenchmarks", "Path to a directory of benchmark JSON files")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.MetadataDir, "metadata-dir", "agonData/modelMetadata", "Path to a directory of model metadata JSON files")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.HTMLPath, "html-output", "agonData/reports/metrics-report.html", "Destination HTML report path")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.AnalysisPath, "analysis-output", "", "Optional path to write the analysis JSON")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.AccuracyResultsDir, "accuracy-results", "agonData/modelAccuracy", "Optional path to accuracy JSONL results directory")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.HostName, "host-name", "", "Optional cluster/host label to embed in the analysis")
	analyzeMetricsCmd.Flags().StringVar(&analyzeMetricsOpts.HostNotes, "host-notes", "", "Optional host notes to embed in the analysis")
	analyzeMetricsCmd.Flags().BoolVar(&analyzeMetricsOpts.AccuracyOnly, "accuracy-only", true, "Build the report from accuracy JSONL data instead of benchmark JSON")

	analyzeCmd.AddCommand(analyzeMetricsCmd)
}
