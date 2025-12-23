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
	"unicode"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providerfactory"
	"github.com/mwiater/agon/internal/providers"
)

const (
	promptSuitePath = "accuracy/accuracy_prompts.json"
	resultsDir      = "accuracy/results"
)

// RunAccuracy executes the accuracy suite for each configured host/model pair.
func RunAccuracy(cfg *appconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if !cfg.AccuracyMode {
		return fmt.Errorf("accuracy mode is not enabled in the configuration")
	}
	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("accuracy mode requires at least one host in the configuration")
	}
	timeoutSeconds := int(cfg.RequestTimeout().Seconds())

	for _, host := range cfg.Hosts {
		if len(host.Models) != 1 {
			return fmt.Errorf("each host in accuracy mode must have exactly one model")
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

				response, meta, err := runPrompt(r.provider, r.host, r.model, suite.SystemPrompt, t.Prompt)
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
						}

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

				ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
				result := AccuracyResult{
					Timestamp:          time.Now().Format(time.RFC3339),
					Host:               r.host.Name,
					Model:              r.model,
					PromptID:           t.ID,
					Prompt:             t.Prompt,
					ExpectedAnswer:     t.ExpectedAnswer,
					Response:           response,
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
				}

				if err := appendResult(r.model, result); err != nil {
					log.Printf("error writing result for model %s: %v", r.model, err)
				}
			}
		}(runner, totalPrompts)
	}
	wg.Wait()

	return nil
}

func runPrompt(provider providers.ChatProvider, host appconfig.Host, modelName, systemPrompt, prompt string) (string, providers.StreamMetadata, error) {
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
		return "", meta, err
	}

	return strings.TrimSpace(output.String()), meta, nil
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

func parseAnswer(response string) (int, bool) {
	trimmed := strings.TrimSpace(stripThinkBlocks(response))
	if trimmed == "" {
		return 0, false
	}
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
		trimmed = strings.Trim(trimmed, "\"")
	}
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return 0, false
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return parseFromTokens(trimmed)
	}
	return value, true
}

func accuracyMetrics(meta providers.StreamMetadata) (int, float64, int, int, int) {
	ttftMs := int((meta.LoadDuration + meta.PromptEvalDuration) / int64(time.Millisecond))
	totalDurationMs := int(meta.TotalDuration / int64(time.Millisecond))
	tokensPerSecond := 0.0
	if meta.EvalDuration > 0 {
		tokensPerSecond = float64(meta.EvalCount) / (float64(meta.EvalDuration) / float64(time.Second))
	}
	return ttftMs, tokensPerSecond, meta.PromptEvalCount, meta.EvalCount, totalDurationMs
}

func matchesExpected(response string, expected, marginOfError int) bool {
	for _, token := range tokenizeResponse(stripThinkBlocks(response)) {
		value, err := strconv.Atoi(token)
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

func stripThinkBlocks(response string) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return trimmed
	}
	const startTag = "<think>"
	const endTag = "</think>"
	for {
		start := strings.Index(trimmed, startTag)
		if start == -1 {
			break
		}
		end := strings.Index(trimmed[start+len(startTag):], endTag)
		if end == -1 {
			break
		}
		end += start + len(startTag) + len(endTag)
		trimmed = strings.TrimSpace(trimmed[:start] + trimmed[end:])
	}
	return trimmed
}

func parseFromTokens(response string) (int, bool) {
	for _, token := range tokenizeResponse(response) {
		value, err := strconv.Atoi(token)
		if err == nil {
			return value, true
		}
	}
	return 0, false
}

func tokenizeResponse(response string) []string {
	var b strings.Builder
	b.Grow(len(response))
	for _, r := range response {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Fields(b.String())
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
