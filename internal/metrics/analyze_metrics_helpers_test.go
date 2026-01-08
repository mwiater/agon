// internal/metrics/analyze_metrics_helpers_test.go
package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

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

func loadAccuracyStats(dir string) (map[string]AccuracyStats, error) {
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

	stats := make(map[string]AccuracyStats, len(totals))
	for model, stat := range totals {
		accuracy := 0.0
		avgDifficulty := 0.0
		avgMargin := 0.0
		if stat.Total > 0 {
			accuracy = float64(stat.Correct) / float64(stat.Total)
			avgDifficulty = float64(stat.DifficultySum) / float64(stat.Total)
			avgMargin = float64(stat.MarginSum) / float64(stat.Total)
		}
		byDifficulty := make(map[int]AccuracyBucket, len(stat.ByDifficulty))
		for difficulty, diffStat := range stat.ByDifficulty {
			diffAccuracy := 0.0
			if diffStat.Total > 0 {
				diffAccuracy = float64(diffStat.Correct) / float64(diffStat.Total)
			}
			byDifficulty[difficulty] = AccuracyBucket{
				Total:    diffStat.Total,
				Correct:  diffStat.Correct,
				Accuracy: diffAccuracy,
			}
		}
		stats[model] = AccuracyStats{
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

func loadAccuracyPerformanceResults(dir string) (BenchmarkResults, error) {
	if dir == "" {
		return BenchmarkResults{}, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return BenchmarkResults{}, nil
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

	results := make(BenchmarkResults, len(perfByModel))
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

		iterations := make([]Iteration, 0, len(samples))
		for i, s := range samples {
			iterations = append(iterations, Iteration{
				Iteration: i + 1,
				Stats: Stats{
					TotalExecutionTime: int64(s.totalMs) * 1e6,
					TimeToFirstToken:   int64(s.ttftMs) * 1e6,
					TokensPerSecond:    s.tps,
					InputTokenCount:    s.inputTokens,
					OutputTokenCount:   s.outputTokens,
				},
			})
		}

		results[model] = ModelBenchmark{
			ModelName:      model,
			BenchmarkCount: len(samples),
			AverageStats: Stats{
				TotalExecutionTime: int64(avgTotal * 1e6),
				TimeToFirstToken:   int64(avgTTFT * 1e6),
				TokensPerSecond:    avgTPS,
				InputTokenCount:    int(math.Round(avgInput)),
				OutputTokenCount:   int(math.Round(avgOutput)),
			},
			MinStats: Stats{
				TotalExecutionTime: int64(minTotal) * 1e6,
				TimeToFirstToken:   int64(minTTFT) * 1e6,
				TokensPerSecond:    minTPS,
				InputTokenCount:    minInput,
				OutputTokenCount:   minOutput,
			},
			MaxStats: Stats{
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

func loadBenchmarksDirForTest(dir string) (BenchmarkResults, error) {
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

	merged := make(BenchmarkResults)
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
