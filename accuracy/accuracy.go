// accuracy/accuracy.go
package accuracy

import (
	"context"
	"encoding/json"
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
)

const (
	systemPromptPath = "accuracy/system_prompt_example.txt"
	userPromptPath   = "accuracy/user_prompt_examples.json"
	resultsDir       = "accuracy/results"
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

	for _, host := range cfg.Hosts {
		if len(host.Models) != 1 {
			return fmt.Errorf("each host in accuracy mode must have exactly one model")
		}
	}

	systemPrompt, err := loadSystemPrompt(systemPromptPath)
	if err != nil {
		return err
	}

	suite, err := loadPromptSuite(userPromptPath)
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
	for i, test := range suite.Tests {
		fmt.Printf("Iteration: %d/%d user prompts\n", i+1, totalPrompts)

		var wg sync.WaitGroup
		for _, runner := range runners {
			wg.Add(1)
			go func(r hostRunner, t PromptTest) {
				defer wg.Done()

				fmt.Printf("Host/Model: %s / %s\n", r.host.Name, r.model)
				fmt.Printf("Prompt: %s\n", t.Prompt)

				response, err := runPrompt(r.provider, r.host, r.model, systemPrompt, t.Prompt)
				if err != nil {
					fmt.Printf("Result: error=%v\n", err)
					return
				}

				parsedAnswer, ok := parseAnswer(response)
				correct := ok && parsedAnswer == t.ExpectedAnswer
				fmt.Printf("Result: correct=%t response=%q expected=%d\n", correct, response, t.ExpectedAnswer)

				result := AccuracyResult{
					Timestamp:      time.Now().Format(time.RFC3339),
					Host:           r.host.Name,
					Model:          r.model,
					PromptID:       t.ID,
					Prompt:         t.Prompt,
					ExpectedAnswer: t.ExpectedAnswer,
					Response:       response,
					Correct:        correct,
				}

				if err := appendResult(r.model, result); err != nil {
					log.Printf("error writing result for model %s: %v", r.model, err)
				}
			}(runner, test)
		}
		wg.Wait()
	}

	return nil
}

func runPrompt(provider providers.ChatProvider, host appconfig.Host, modelName, systemPrompt, prompt string) (string, error) {
	var output strings.Builder

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
	}

	if err := provider.Stream(context.Background(), req, callbacks); err != nil {
		return "", err
	}

	return strings.TrimSpace(output.String()), nil
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

	return suite, nil
}

func loadSystemPrompt(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("error reading system prompt: %w", err)
	}
	prompt := strings.TrimSpace(string(raw))
	if len(prompt) >= 2 && strings.HasPrefix(prompt, "\"") && strings.HasSuffix(prompt, "\"") {
		prompt = strings.Trim(prompt, "\"")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("system prompt is empty")
	}
	return prompt, nil
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
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return 0, false
	}
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
		trimmed = strings.Trim(trimmed, "\"")
	}
	trimmed = strings.TrimSpace(trimmed)
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return value, true
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
