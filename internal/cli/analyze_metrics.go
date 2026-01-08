// internal/cli/analyze_metrics.go
package agon

import "github.com/spf13/cobra"

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
		return runAnalyzeMetrics(cmd, analyzeMetricsOpts)
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
