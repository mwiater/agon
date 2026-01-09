// accuracy/accuracyBatch.go
package accuracy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providerfactory"
	"github.com/mwiater/agon/internal/providers"
)

const modelMetadataDir = "agonData/modelMetadata"

type modelEndpointPair struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	GPU      string `json:"gpu"`
}

type endpointGroup struct {
	Endpoint string   `json:"endpoint"`
	Models   []string `json:"models"`
}

// RunAccuracyBatch is the entry point for the "run accuracy" workflow.
func RunAccuracyBatch(parameterTemplate string) error {
	suite, err := loadPromptSuite(promptSuitePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("error creating results directory: %w", err)
	}

	entries, err := os.ReadDir(modelMetadataDir)
	if err != nil {
		return fmt.Errorf("read model metadata dir: %w", err)
	}

	pairs := make([]modelEndpointPair, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}

		path := filepath.Join(modelMetadataDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read model metadata %s: %w", path, err)
		}

		var meta modelEndpointPair
		if err := json.Unmarshal(raw, &meta); err != nil {
			return fmt.Errorf("parse model metadata %s: %w", path, err)
		}
		if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Endpoint) == "" || strings.TrimSpace(meta.GPU) == "" {
			return fmt.Errorf("missing name/endpoint/gpu in model metadata %s", path)
		}
		pairs = append(pairs, meta)
	}

	grouped := make(map[string][]string)
	for _, pair := range pairs {
		grouped[pair.Endpoint] = append(grouped[pair.Endpoint], pair.Name)
	}

	ordered := make([]endpointGroup, 0, len(grouped))
	endpoints := make([]string, 0, len(grouped))
	for endpoint := range grouped {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	for _, endpoint := range endpoints {
		models := grouped[endpoint]
		sort.Strings(models)
		ordered = append(ordered, endpointGroup{
			Endpoint: endpoint,
			Models:   models,
		})
	}

	pairsByEndpoint := make(map[string][]modelEndpointPair, len(endpoints))
	for _, pair := range pairs {
		pairsByEndpoint[pair.Endpoint] = append(pairsByEndpoint[pair.Endpoint], pair)
	}
	for endpoint, list := range pairsByEndpoint {
		sort.Slice(list, func(i, j int) bool {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
		pairsByEndpoint[endpoint] = list
	}

	timeoutSeconds := int(appconfig.Config{}.RequestTimeout().Seconds())
	templateName := strings.TrimSpace(parameterTemplate)
	if templateName == "" {
		templateName = string(appconfig.ProfileGenericChat)
	}

	var wg sync.WaitGroup
	for _, group := range ordered {
		endpoint := group.Endpoint
		models := pairsByEndpoint[endpoint]

		wg.Add(1)
		go func(endpoint string, models []modelEndpointPair) {
			defer wg.Done()
			if err := runEndpointQueue(endpoint, models, suite, timeoutSeconds, templateName); err != nil {
				log.Printf("accuracy batch error for endpoint %s: %v", endpoint, err)
			}
		}(endpoint, models)
	}

	wg.Wait()

	return nil
}

func runEndpointQueue(endpoint string, models []modelEndpointPair, suite PromptSuite, timeoutSeconds int, parameterTemplate string) error {
	if len(models) == 0 {
		return nil
	}

	params := appconfig.ParamsForProfile(parameterTemplate)
	params.NProbs = intPtr(1)

	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{{
			Name:              endpoint,
			URL:               endpoint,
			ParameterTemplate: parameterTemplate,
			Parameters:        params,
		}},
	}
	provider, err := providerfactory.NewChatProvider(cfg)
	if err != nil {
		return fmt.Errorf("create provider for %s: %w", endpoint, err)
	}
	defer func() {
		_ = provider.Close()
	}()

	host := cfg.Hosts[0]
	for _, model := range models {
		log.Printf("Preparing accuracy checks for model %s on host %s...", model.Name, endpoint)
		if err := provider.EnsureModelReady(context.Background(), host, model.Name); err != nil {
			return fmt.Errorf("error ensuring model %s is ready on host %s: %w", model.Name, endpoint, err)
		}
		if err := runAccuracyForModel(provider, host, model, suite, timeoutSeconds); err != nil {
			return err
		}
	}

	return nil
}

func runAccuracyForModel(provider providers.ChatProvider, host appconfig.Host, model modelEndpointPair, suite PromptSuite, timeoutSeconds int) error {
	totalPrompts := len(suite.Tests)
	for i, t := range suite.Tests {
		iteration := i + 1
		fmt.Printf("[%d/%d] %s / %s - Prompt: %s\n", iteration, totalPrompts, host.Name, model.Name, t.Prompt)

		response, meta, err := runPrompt(provider, host, model.Name, suite.SystemPrompt, t.Prompt)
		if err != nil {
			deadlineExceeded := isDeadlineExceeded(err)
			if deadlineExceeded {
				fmt.Printf("[%d/%d] %s / %s - Result: deadlineExceeded=true error=%v\n", iteration, totalPrompts, host.Name, model.Name, err)
				ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
				result := AccuracyResult{
					Timestamp:          time.Now().Format(time.RFC3339),
					Host:               host.Name,
					Model:              model.Name,
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
				}

				if err := appendResultWithGPU(model.GPU, model.Name, result); err != nil {
					log.Printf("error writing result for model %s: %v", model.Name, err)
				}
			} else {
				fmt.Printf("[%d/%d] %s / %s - Result: error=%v\n", iteration, totalPrompts, host.Name, model.Name, err)
			}
			return err
		}

		correct := matchesExpected(response, t.ExpectedAnswer, t.MarginOfError)
		fmt.Printf("Full response: %s\n", response)
		fmt.Printf("[%d/%d] %s / %s - Result: correct=%t response=%q expected=%d\n", iteration, totalPrompts, host.Name, model.Name, correct, response, t.ExpectedAnswer)

		ttftMs, tokensPerSecond, inputTokens, outputTokens, totalDurationMs := accuracyMetrics(meta)
		result := AccuracyResult{
			Timestamp:          time.Now().Format(time.RFC3339),
			Host:               host.Name,
			Model:              model.Name,
			PromptID:           t.ID,
			Prompt:             t.Prompt,
			ExpectedAnswer:     t.ExpectedAnswer,
			Response:           response,
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
		}

		if err := appendResultWithGPU(model.GPU, model.Name, result); err != nil {
			log.Printf("error writing result for model %s: %v", model.Name, err)
		}
	}

	return nil
}

func appendResultWithGPU(gpu, modelName string, result AccuracyResult) error {
	fileName := fmt.Sprintf("%s_%s.jsonl", gpu, modelName)
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

func intPtr(v int) *int {
	return &v
}
