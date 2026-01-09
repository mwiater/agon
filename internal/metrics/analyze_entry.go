package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

	opts.HTMLPath = appendTemplateSuffix(opts.HTMLPath, detectParameterTemplate(combined))

	if err := os.WriteFile(opts.HTMLPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("unable to write HTML report %s: %w", opts.HTMLPath, err)
	}

	fmt.Fprintf(out, "Report written to %s\n", opts.HTMLPath)
	return nil
}

func appendTemplateSuffix(path, templateName string) string {
	trimmed := strings.TrimSpace(templateName)
	if trimmed == "" {
		return path
	}
	suffix := "." + trimmed + "_profile.html"
	if strings.HasSuffix(path, suffix) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	if base == "" {
		base = path
	}
	return base + suffix
}

func detectParameterTemplate(combined CombinedMetrics) string {
	seen := make(map[string]struct{})
	for _, bundle := range combined.Models {
		for _, record := range bundle.Accuracy {
			trimmed := strings.TrimSpace(record.ParameterTemplate)
			if trimmed == "" {
				continue
			}
			seen[trimmed] = struct{}{}
			if len(seen) > 1 {
				return ""
			}
		}
	}
	for template := range seen {
		return template
	}
	return ""
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
