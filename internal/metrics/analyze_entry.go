package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AnalyzeOptions captures the inputs for generating the metrics analysis/report.
type AnalyzeOptions struct {
	InputPath          string
	HTMLPath           string
	AnalysisPath       string
	AccuracyResultsDir string
	BenchmarksDir      string
	MetadataDir        string
	HostName           string
	HostNotes          string
	AccuracyOnly       bool
}

// AnalyzeMetrics builds the metrics analysis JSON and HTML report.
func AnalyzeMetrics(opts AnalyzeOptions, out io.Writer) error {
	combined, err := LoadCombinedMetrics(
		opts.AccuracyResultsDir,
		opts.BenchmarksDir,
		opts.MetadataDir,
		opts.InputPath,
	)
	if err != nil {
		return err
	}

	if opts.AnalysisPath != "" {
		if err := writeAnalysisJSON(opts.AnalysisPath, combined); err != nil {
			return err
		}
		fmt.Fprintf(out, "Analysis JSON written to %s\n", opts.AnalysisPath)
	}

	html, err := GenerateCombinedReport(combined)
	if err != nil {
		return fmt.Errorf("failed generating HTML report: %w", err)
	}

	if opts.HTMLPath == "" {
		opts.HTMLPath = "agonData/reports/metrics-report.html"
	}

	if err := os.WriteFile(opts.HTMLPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("unable to write HTML report %s: %w", opts.HTMLPath, err)
	}

	fmt.Fprintf(out, "Report written to %s\n", opts.HTMLPath)
	return nil
}

func writeAnalysisJSON(path string, analysis CombinedMetrics) error {
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
