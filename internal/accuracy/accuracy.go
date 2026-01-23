// accuracy/accuracy.go
package accuracy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providerfactory"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/rag"
)

const (
	promptSuitePath = "internal/accuracy/accuracy_prompts.json"
	resultsDir      = "agonData/modelAccuracy"
)

const (
	accuracyAnswerHint      = "Answer with a single integer only. No words, no punctuation."
	accuracyFinalAnswerHint = "Final answer:"
)

// RunAccuracy executes the accuracy suite for each configured host/model pair.
func RunAccuracy(cfg *appconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("accuracy runs require at least one host in the configuration")
	}
	timeoutSeconds := int(cfg.RequestTimeout().Seconds())

	for _, host := range cfg.Hosts {
		if len(host.Models) != 1 {
			return fmt.Errorf("each host in an accuracy run must have exactly one model")
		}
	}

	suite, err := loadPromptSuite(promptSuitePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("error creating results directory: %w", err)
	}

	type hostRunner struct {
		host     appconfig.Host
		model    string
		provider providers.ChatProvider
	}

	runners := make([]hostRunner, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		modelName := host.Models[0]
		log.Printf("Preparing accuracy checks for model %s on host %s...", modelName, host.Name)

		provider, err := providerfactory.NewChatProvider(cfg)
		if err != nil {
			return fmt.Errorf("error creating provider for host %s: %w", host.Name, err)
		}

		log.Printf("Ensuring model %s is loaded on host %s...", modelName, host.Name)
		if err := provider.EnsureModelReady(context.Background(), host, modelName); err != nil {
			_ = provider.Close()
			return fmt.Errorf("error ensuring model %s is ready on host %s: %w", modelName, host.Name, err)
		}

		runners = append(runners, hostRunner{
			host:     host,
			model:    modelName,
			provider: provider,
		})
	}
	defer func() {
		for _, runner := range runners {
			_ = runner.provider.Close()
		}
	}()

	totalPrompts := len(suite.Tests)
	var wg sync.WaitGroup
	for _, runner := range runners {
		wg.Add(1)
		go func(r hostRunner, total int) {
			defer wg.Done()
			for i, t := range suite.Tests {
				iteration := i + 1
				fmt.Printf("[%d/%d] %s / %s - Prompt: %s\n", iteration, total, r.host.Name, r.model, t.Prompt)

				if cfg.RagMode {
					if err := runRagCompare(cfg, r.provider, r.host, r.model, suite.SystemPrompt, t, timeoutSeconds); err != nil {
						log.Printf("error running RAG compare for model %s: %v", r.model, err)
					}
					continue
				}

				systemPrompt := buildAccuracySystemPrompt(suite.SystemPrompt)
				userPrompt := buildAccuracyUserPrompt(t.Prompt)
				rawResponse, response, meta, err := runPrompt(r.provider, r.host, r.model, systemPrompt, userPrompt)
				if err != nil {
					deadlineExceeded := isDeadlineExceeded(err)
					if deadlineExceeded {
						fmt.Printf("[%d/%d] %s / %s - Result: deadlineExceeded=true error=%v\n", iteration, total, r.host.Name, r.model, err)
						ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
						result := AccuracyResult{
							Timestamp:          time.Now().Format(time.RFC3339),
							Host:               r.host.Name,
							Model:              r.model,
							PromptID:           t.ID,
							Prompt:             t.Prompt,
							ExpectedAnswer:     t.ExpectedAnswer,
							Response:           err.Error(),
							LogProbs:           meta.LogProbs,
							Correct:            false,
							MarginOfError:      t.MarginOfError,
							Difficulty:         t.Difficulty,
							TimeToFirstToken:   ttftMs,
							TokensPerSecond:    tokensPerSecond,
							InputTokens:        inputTokens,
							OutputTokens:       outputTokens,
							TotalDurationMs:    totalDurationMs,
							DeadlineExceeded:   true,
							DeadlineTimeoutSec: timeoutSeconds,
							ParameterTemplate:  r.host.ParameterTemplate,
						}
						applyTimingMetrics(&result, meta)

						if err := appendResult(r.model, result); err != nil {
							log.Printf("error writing result for model %s: %v", r.model, err)
						}
					} else {
						fmt.Printf("[%d/%d] %s / %s - Result: error=%v\n", iteration, total, r.host.Name, r.model, err)
					}
					return
				}

				correct := matchesExpected(response, t.ExpectedAnswer, t.MarginOfError)
				fmt.Printf("[%d/%d] %s / %s - Result: correct=%t response=%q expected=%d\n", iteration, total, r.host.Name, r.model, correct, response, t.ExpectedAnswer)
				if response == "" && normalizeResponse(response) == "" {
					fmt.Printf("[%d/%d] %s / %s - Full response: %q\n", iteration, total, r.host.Name, r.model, rawResponse)
				}

				ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
				result := AccuracyResult{
					Timestamp:          time.Now().Format(time.RFC3339),
					Host:               r.host.Name,
					Model:              r.model,
					PromptID:           t.ID,
					Prompt:             t.Prompt,
					ExpectedAnswer:     t.ExpectedAnswer,
					Response:           response,
					EvaluatedResponse:  normalizeResponse(response),
					LogProbs:           meta.LogProbs,
					Correct:            correct,
					MarginOfError:      t.MarginOfError,
					Difficulty:         t.Difficulty,
					TimeToFirstToken:   ttftMs,
					TokensPerSecond:    tokensPerSecond,
					InputTokens:        inputTokens,
					OutputTokens:       outputTokens,
					TotalDurationMs:    totalDurationMs,
					DeadlineExceeded:   false,
					DeadlineTimeoutSec: timeoutSeconds,
					ParameterTemplate:  r.host.ParameterTemplate,
				}
				applyTimingMetrics(&result, meta)

				if err := appendResult(r.model, result); err != nil {
					log.Printf("error writing result for model %s: %v", r.model, err)
				}
			}
		}(runner, totalPrompts)
	}
	wg.Wait()

	return nil
}

func runRagCompare(cfg *appconfig.Config, provider providers.ChatProvider, host appconfig.Host, modelName, systemPrompt string, test PromptTest, timeoutSeconds int) error {
	// RAG OFF: baseline run (no context).
	systemPrompt = buildAccuracySystemPrompt(systemPrompt)
	userPrompt := buildAccuracyUserPrompt(test.Prompt)
	rawResponse, response, meta, err := runPrompt(provider, host, modelName, systemPrompt, userPrompt)
	if err != nil {
		if isDeadlineExceeded(err) {
			ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
			result := AccuracyResult{
				Timestamp:          time.Now().Format(time.RFC3339),
				Host:               host.Name,
				Model:              modelName,
				PromptID:           test.ID,
				Prompt:             test.Prompt,
				ExpectedAnswer:     test.ExpectedAnswer,
				Response:           err.Error(),
				LogProbs:           meta.LogProbs,
				Correct:            false,
				MarginOfError:      test.MarginOfError,
				Difficulty:         test.Difficulty,
				TimeToFirstToken:   ttftMs,
				TokensPerSecond:    tokensPerSecond,
				InputTokens:        inputTokens,
				OutputTokens:       outputTokens,
				TotalDurationMs:    totalDurationMs,
				DeadlineExceeded:   true,
				DeadlineTimeoutSec: timeoutSeconds,
				RagMode:            "off",
				ParameterTemplate:  host.ParameterTemplate,
			}
			applyTimingMetrics(&result, meta)
			if err := appendResult(modelName, result); err != nil {
				log.Printf("error writing result for model %s: %v", modelName, err)
			}
			return nil
		}
		return err
	}
	correct := matchesExpected(response, test.ExpectedAnswer, test.MarginOfError)
	fmt.Printf("Full response (RAG off): %s\n", response)
	if response == "" && normalizeResponse(response) == "" {
		fmt.Printf("Full response (RAG off, raw): %q\n", rawResponse)
	}
	ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
	result := AccuracyResult{
		Timestamp:          time.Now().Format(time.RFC3339),
		Host:               host.Name,
		Model:              modelName,
		PromptID:           test.ID,
		Prompt:             test.Prompt,
		ExpectedAnswer:     test.ExpectedAnswer,
		Response:           response,
		EvaluatedResponse:  normalizeResponse(response),
		LogProbs:           meta.LogProbs,
		Correct:            correct,
		MarginOfError:      test.MarginOfError,
		Difficulty:         test.Difficulty,
		TimeToFirstToken:   ttftMs,
		TokensPerSecond:    tokensPerSecond,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		TotalDurationMs:    totalDurationMs,
		DeadlineExceeded:   false,
		DeadlineTimeoutSec: timeoutSeconds,
		RagMode:            "off",
		ParameterTemplate:  host.ParameterTemplate,
	}
	applyTimingMetrics(&result, meta)
	if err := appendResult(modelName, result); err != nil {
		log.Printf("error writing result for model %s: %v", modelName, err)
	}

	// RAG ON: retrieve context and inject.
	retrieval, err := rag.Retrieve(context.Background(), cfg, test.Prompt)
	if err != nil {
		return err
	}

	ragSystemPrompt := buildRagSystemPrompt(buildAccuracySystemPrompt(systemPrompt))
	promptWithContext := test.Prompt
	if retrieval.Context != "" {
		promptWithContext = retrieval.Context + "\n\n" + test.Prompt
	}

	userPrompt = buildAccuracyUserPrompt(promptWithContext)
	rawResponse, response, meta, err = runPrompt(provider, host, modelName, ragSystemPrompt, userPrompt)
	if err != nil {
		if isDeadlineExceeded(err) {
			ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
			result := AccuracyResult{
				Timestamp:          time.Now().Format(time.RFC3339),
				Host:               host.Name,
				Model:              modelName,
				PromptID:           test.ID,
				Prompt:             test.Prompt,
				ExpectedAnswer:     test.ExpectedAnswer,
				Response:           err.Error(),
				LogProbs:           meta.LogProbs,
				Correct:            false,
				MarginOfError:      test.MarginOfError,
				Difficulty:         test.Difficulty,
				TimeToFirstToken:   ttftMs,
				TokensPerSecond:    tokensPerSecond,
				InputTokens:        inputTokens,
				OutputTokens:       outputTokens,
				TotalDurationMs:    totalDurationMs,
				DeadlineExceeded:   true,
				DeadlineTimeoutSec: timeoutSeconds,
				RagMode:            "on",
				RetrievalMs:        retrieval.RetrievalMs,
				ContextTokens:      retrieval.ContextTokens,
				TopK:               len(retrieval.Chunks),
				SourceCoverage:     retrieval.SourceCoverage,
				ParameterTemplate:  host.ParameterTemplate,
			}
			applyTimingMetrics(&result, meta)
			if err := appendResult(modelName, result); err != nil {
				log.Printf("error writing result for model %s: %v", modelName, err)
			}
			return nil
		}
		return err
	}
	correct = matchesExpected(response, test.ExpectedAnswer, test.MarginOfError)
	fmt.Printf("Full response (RAG on): %s\n", response)
	if response == "" && normalizeResponse(response) == "" {
		fmt.Printf("Full response (RAG on, raw): %q\n", rawResponse)
	}
	ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs = accuracyMetrics(meta)
	result = AccuracyResult{
		Timestamp:          time.Now().Format(time.RFC3339),
		Host:               host.Name,
		Model:              modelName,
		PromptID:           test.ID,
		Prompt:             test.Prompt,
		ExpectedAnswer:     test.ExpectedAnswer,
		Response:           response,
		EvaluatedResponse:  normalizeResponse(response),
		LogProbs:           meta.LogProbs,
		Correct:            correct,
		MarginOfError:      test.MarginOfError,
		Difficulty:         test.Difficulty,
		TimeToFirstToken:   ttftMs,
		TokensPerSecond:    tokensPerSecond,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		TotalDurationMs:    totalDurationMs,
		DeadlineExceeded:   false,
		DeadlineTimeoutSec: timeoutSeconds,
		RagMode:            "on",
		RetrievalMs:        retrieval.RetrievalMs,
		ContextTokens:      retrieval.ContextTokens,
		TopK:               len(retrieval.Chunks),
		SourceCoverage:     retrieval.SourceCoverage,
		CitationsUsed:      false,
		ParameterTemplate:  host.ParameterTemplate,
	}
	applyTimingMetrics(&result, meta)
	if err := appendResult(modelName, result); err != nil {
		log.Printf("error writing result for model %s: %v", modelName, err)
	}

	return nil
}

func buildRagSystemPrompt(systemPrompt string) string {
	trimmed := strings.TrimSpace(systemPrompt)
	if trimmed == "" {
		return "If CONTEXT is provided, treat it as authoritative reference material."
	}
	line := "If CONTEXT is provided, treat it as authoritative reference material."
	if strings.Contains(trimmed, line) {
		return trimmed
	}
	return trimmed + "\n" + line
}

func runPrompt(provider providers.ChatProvider, host appconfig.Host, modelName, systemPrompt, prompt string) (string, string, providers.StreamMetadata, error) {
	var output strings.Builder
	var meta providers.StreamMetadata

	req := providers.StreamRequest{
		Host:         host,
		Model:        modelName,
		SystemPrompt: systemPrompt,
		Parameters:   host.Parameters,
		History: []providers.ChatMessage{{
			Role:    "user",
			Content: prompt,
		}},
		DisableStreaming: true,
	}

	callbacks := providers.StreamCallbacks{
		OnChunk: func(chunk providers.ChatMessage) error {
			output.WriteString(chunk.Content)
			return nil
		},
		OnComplete: func(m providers.StreamMetadata) error {
			meta = m
			return nil
		},
	}

	if err := provider.Stream(context.Background(), req, callbacks); err != nil {
		return "", "", meta, err
	}

	raw := output.String()
	return raw, strings.TrimSpace(raw), meta, nil
}

func loadPromptSuite(path string) (PromptSuite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PromptSuite{}, fmt.Errorf("error reading prompt suite: %w", err)
	}

	var suite PromptSuite
	if err := json.Unmarshal(raw, &suite); err != nil {
		return PromptSuite{}, fmt.Errorf("error parsing prompt suite: %w", err)
	}

	if len(suite.Tests) == 0 {
		return PromptSuite{}, fmt.Errorf("prompt suite contains no tests")
	}
	if strings.TrimSpace(suite.SystemPrompt) == "" {
		return PromptSuite{}, fmt.Errorf("prompt suite contains an empty system_prompt")
	}

	return suite, nil
}

func appendResult(modelName string, result AccuracyResult) error {
	fileName := fmt.Sprintf("%s.jsonl", slugify(modelName))
	path := filepath.Join(resultsDir, fileName)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("error opening results file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("error writing results: %w", err)
	}

	return nil
}

func accuracyMetrics(meta providers.StreamMetadata) (int, float64, int, int, int) {
	ttftMs := int((meta.LoadDuration + meta.PromptEvalDuration) / int64(time.Millisecond))
	totalDurationMs := int(meta.TotalDuration / int64(time.Millisecond))
	inputTokens := meta.PromptTokens
	if inputTokens == 0 {
		inputTokens = meta.PromptEvalCount
	}
	outputTokens := meta.CompletionTokens
	if outputTokens == 0 {
		outputTokens = meta.EvalCount
	}
	evalDuration := meta.EvalDuration
	if evalDuration == 0 && meta.TotalDuration > 0 && meta.PromptEvalDuration > 0 {
		remainder := meta.TotalDuration - meta.LoadDuration - meta.PromptEvalDuration
		if remainder > 0 {
			evalDuration = remainder
		}
	}
	tokensPerSecond := 0.0
	if evalDuration > 0 && outputTokens > 0 {
		tokensPerSecond = float64(outputTokens) / (float64(evalDuration) / float64(time.Second))
	}
	return ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs
}

func applyTimingMetrics(result *AccuracyResult, meta providers.StreamMetadata) {
	if result == nil {
		return
	}
	result.TotalTokens = meta.TotalTokens
	result.CacheN = meta.CacheN
	result.PromptN = meta.PromptEvalCount
	result.PromptMs = meta.PromptMs
	result.PromptPerTokenMs = meta.PromptPerTokenMs
	result.PromptPerSecond = meta.PromptPerSecond
	result.PredictedN = meta.EvalCount
	result.PredictedMs = meta.PredictedMs
	result.PredictedPerTokenMs = meta.PredictedPerTokenMs
	result.PredictedPerSecond = meta.PredictedPerSecond
}

func matchesExpected(response string, expected, marginOfError int) bool {
	trimmed := normalizeResponse(response)
	if trimmed == "" {
		return false
	}
	tail := trimmed
	if len(trimmed) > 25 {
		tail = trimmed[len(trimmed)-25:]
	}
	matches := regexp.MustCompile(`-?\d+`).FindAllString(tail, -1)
	for _, match := range matches {
		value, err := strconv.Atoi(match)
		if err != nil {
			continue
		}
		if withinTolerance(value, expected, marginOfError) {
			return true
		}
	}
	return false
}

func withinTolerance(actual, expected, tolerance int) bool {
	if tolerance < 0 {
		tolerance = 0
	}
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func normalizeResponse(response string) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return trimmed
	}
	// Strip any <think> blocks but keep any surrounding content (some models append reasoning after the answer).
	thinkBlock := regexp.MustCompile(`(?s)<think>.*?</think>`)
	trimmed = thinkBlock.ReplaceAllString(trimmed, "")
	if strings.Contains(trimmed, "<think>") {
		trimmed = trimmed[:strings.Index(trimmed, "<think>")]
	}
	trimmed = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, trimmed)
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	trimmed = strings.ReplaceAll(trimmed, ",", "")
	trimmed = strings.Trim(trimmed, " \t\"'`.,;:!?()[]{}<>")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return trimmed
	}

	// If there are multiple numbers, prefer the last one (answers are typically at the end).
	matches := regexp.MustCompile(`-?\d+`).FindAllString(trimmed, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return trimmed
}

func buildAccuracySystemPrompt(systemPrompt string) string {
	trimmed := strings.TrimSpace(systemPrompt)
	if trimmed == "" {
		return accuracyAnswerHint
	}
	if strings.Contains(trimmed, accuracyAnswerHint) {
		return trimmed
	}
	return accuracyAnswerHint + "\n" + trimmed
}

func buildAccuracyUserPrompt(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return accuracyAnswerHint + "\n" + accuracyFinalAnswerHint
	}
	if !strings.Contains(trimmed, accuracyAnswerHint) {
		trimmed = accuracyAnswerHint + "\n" + trimmed
	}
	if !strings.Contains(trimmed, accuracyFinalAnswerHint) {
		trimmed = trimmed + "\n" + accuracyFinalAnswerHint
	}
	return trimmed
}

func isDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded")
}

// slugify converts a string into a filesystem-friendly slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", "_")
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	s = re.ReplaceAllString(s, "-")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	return s
}
