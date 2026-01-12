// benchmark/benchmark.go
package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/models"
	"github.com/mwiater/agon/internal/providerfactory"
	"github.com/mwiater/agon/internal/providers"
)

const userPrompt = "List 3 different fruits in alphabetical order? None of the three can be an apple."

var (
	newChatProvider = providerfactory.NewChatProvider
	unloadModels    = models.UnloadModels
	writeResultsFn  = writeResults
)

// BenchmarkModels runs benchmarks for models defined in the configuration.
func BenchmarkModels(cfg *appconfig.Config) error {
	if len(cfg.Hosts) < 2 {
		return fmt.Errorf("benchmark runs require at least two hosts in the configuration")
	}

	for _, host := range cfg.Hosts {
		if len(host.Models) != 1 {
			return fmt.Errorf("each host in a benchmark run must have exactly one model")
		}
	}

	unloadModels(cfg)

	var modelNames []string
	for _, host := range cfg.Hosts {
		modelNames = append(modelNames, host.Models[0])
	}
	log.Printf("Running benchmark with models: %s", strings.Join(modelNames, ", "))

	results := make(map[string]*BenchmarkResult)
	for _, host := range cfg.Hosts {
		results[host.Models[0]] = &BenchmarkResult{
			ModelName:      host.Models[0],
			BenchmarkCount: cfg.BenchmarkCount,
			Iterations:     make([]IterationResult, 0, cfg.BenchmarkCount),
		}
	}

	var wg sync.WaitGroup
	for _, host := range cfg.Hosts {
		wg.Add(1)
		go func(host appconfig.Host) {
			defer wg.Done()
			provider, err := newChatProvider(cfg)
			if err != nil {
				log.Printf("error creating provider for host %s: %v", host.Name, err)
				return
			}

			log.Printf("Ensuring model %s is loaded on host %s...", host.Models[0], host.Name)
			if err := provider.EnsureModelReady(context.Background(), host, host.Models[0]); err != nil {
				log.Printf("error ensuring model %s is ready on host %s: %v", host.Models[0], host.Name, err)
				return
			}

			for i := 0; i < cfg.BenchmarkCount; i++ {
				log.Printf("Running iteration %d of %d for model %s on host %s...", i+1, cfg.BenchmarkCount, host.Models[0], host.Name)

				startTime := time.Now()
				var timeToFirstToken time.Duration
				firstChunk := true

				var outputTokens int
				var inputTokens int

				req := providers.StreamRequest{
					Host:  host,
					Model: host.Models[0],
					History: []providers.ChatMessage{{
						Role:    "user",
						Content: userPrompt,
					}},
				}

				callbacks := providers.StreamCallbacks{
					OnChunk: func(chunk providers.ChatMessage) error {
						if firstChunk {
							timeToFirstToken = time.Since(startTime)
							firstChunk = false
							log.Printf("First chunk received for model %s on host %s after %s", host.Models[0], host.Name, timeToFirstToken)
						}
						return nil
					},
					OnComplete: func(meta providers.StreamMetadata) error {
						outputTokens = meta.EvalCount
						inputTokens = meta.PromptEvalCount
						return nil
					},
				}

				if err := provider.Stream(context.Background(), req, callbacks); err != nil {
					log.Printf("error during stream with model %s: %v", host.Models[0], err)
					continue
				}

				endTime := time.Now()
				totalExecutionTime := endTime.Sub(startTime)
				tokensPerSecond := float64(outputTokens) / totalExecutionTime.Seconds()

				iterationResult := IterationResult{
					Iteration: i + 1,
					Stats: IterationStats{
						TotalExecutionTime: totalExecutionTime,
						TimeToFirstToken:   timeToFirstToken,
						TokensPerSecond:    tokensPerSecond,
						InputTokenCount:    inputTokens,
						OutputTokenCount:   outputTokens,
					},
				}

				modelResult := results[host.Models[0]]
				modelResult.Iterations = append(modelResult.Iterations, iterationResult)

				log.Printf("Iteration %d for model %s on host %s complete:", i+1, host.Models[0], host.Name)
				log.Printf("  Total Execution Time: %s", totalExecutionTime)
				log.Printf("  Time to First Token: %s", timeToFirstToken)
				log.Printf("  Tokens per Second: %.2f", tokensPerSecond)
				log.Printf("  Input Tokens: %d", inputTokens)
				log.Printf("  Output Tokens: %d", outputTokens)
			}
		}(host)
	}
	wg.Wait()

	for _, result := range results {
		calculateAggregates(result)
	}

	return writeResultsFn(results, cfg.BenchmarkCount)
}

// calculateAggregates calculates the average, min, and max statistics for a benchmark result.
func calculateAggregates(result *BenchmarkResult) {
	if len(result.Iterations) == 0 {
		return
	}

	result.MinStats = result.Iterations[0].Stats
	result.MaxStats = result.Iterations[0].Stats

	var totalExecutionTime time.Duration
	var timeToFirstToken time.Duration
	var tokensPerSecond float64

	for _, iter := range result.Iterations {
		totalExecutionTime += iter.Stats.TotalExecutionTime
		timeToFirstToken += iter.Stats.TimeToFirstToken
		tokensPerSecond += iter.Stats.TokensPerSecond

		if iter.Stats.TotalExecutionTime < result.MinStats.TotalExecutionTime {
			result.MinStats.TotalExecutionTime = iter.Stats.TotalExecutionTime
		}
		if iter.Stats.TotalExecutionTime > result.MaxStats.TotalExecutionTime {
			result.MaxStats.TotalExecutionTime = iter.Stats.TotalExecutionTime
		}

		if iter.Stats.TimeToFirstToken < result.MinStats.TimeToFirstToken {
			result.MinStats.TimeToFirstToken = iter.Stats.TimeToFirstToken
		}
		if iter.Stats.TimeToFirstToken > result.MaxStats.TimeToFirstToken {
			result.MaxStats.TimeToFirstToken = iter.Stats.TimeToFirstToken
		}

		if iter.Stats.TokensPerSecond < result.MinStats.TokensPerSecond {
			result.MinStats.TokensPerSecond = iter.Stats.TokensPerSecond
		}
		if iter.Stats.TokensPerSecond > result.MaxStats.TokensPerSecond {
			result.MaxStats.TokensPerSecond = iter.Stats.TokensPerSecond
		}
	}

	count := float64(len(result.Iterations))
	result.AverageStats.TotalExecutionTime = time.Duration(float64(totalExecutionTime) / count)
	result.AverageStats.TimeToFirstToken = time.Duration(float64(timeToFirstToken) / count)
	result.AverageStats.TokensPerSecond = tokensPerSecond / count
}

// writeResults writes the benchmark results to a JSON file.
func writeResults(results map[string]*BenchmarkResult, benchmarkCount int) error {
	var modelNames []string
	for name := range results {
		modelNames = append(modelNames, name)
	}

	dir := filepath.Join("agonData", "modelBenchmarks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("error creating results directory: %w", err)
	}
	fileName := filepath.Join(dir, fmt.Sprintf("%s-%d.json", Slugify(strings.Join(modelNames, "-")), benchmarkCount))

	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("error creating result file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("error writing results to file: %w", err)
	}

	log.Printf("Benchmark results written to %s", fileName)

	return nil
}

// Slugify converts a string into a "slug" format,
// including replacing colons (:) with underscores (_).
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", "_")
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	s = re.ReplaceAllString(s, "-")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")

	return s
}
