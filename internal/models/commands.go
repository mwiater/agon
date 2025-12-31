// internal/models/commands.go
package models

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/k0kubun/pp"
	"github.com/mwiater/agon/internal/appconfig"
)

// createHosts creates LLMHost implementations for each configured host entry.
func createHosts(config appconfig.Config) []LLMHost {
	var hosts []LLMHost
	timeout := config.RequestTimeout()
	client := &http.Client{
		Timeout: timeout,
	}
	for _, hostConfig := range config.Hosts {
		switch hostConfig.Type {
		case "ollama":
			hosts = append(hosts, &OllamaHost{
				Name:           hostConfig.Name,
				URL:            hostConfig.URL,
				Models:         hostConfig.Models,
				client:         client,
				requestTimeout: timeout,
			})
		case "llama.cpp":
			hosts = append(hosts, &LlamaCppHost{
				Name:           hostConfig.Name,
				URL:            hostConfig.URL,
				Models:         hostConfig.Models,
				client:         client,
				requestTimeout: timeout,
			})
		default:
			fmt.Printf("Unknown host type: %s\n", hostConfig.Type)
		}
	}
	return hosts
}

// PullModels reads models from the provided configuration and pulls them to each supported host.
func PullModels(config *appconfig.Config) {
	if config == nil {
		fmt.Println("configuration is not initialized")
		return
	}

	hosts := createHosts(*config)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h LLMHost) {
			defer wg.Done()
			if h.GetType() != "ollama" {
				fmt.Printf("Pulling models is not supported for %s (%s)\n", h.GetName(), h.GetType())
				return
			}
			fmt.Printf("Starting model pulls for %s...\n", h.GetName())
			for _, model := range h.GetModels() {
				fmt.Printf("  -> Pulling model: %s on %s\n", model, h.GetName())
				h.PullModel(model)
			}
		}(host)
	}
	wg.Wait()
	fmt.Println("All model pull commands have finished.")
}

// DeleteModels uses the provided configuration and deletes any models not on the list from each supported host.
func DeleteModels(config *appconfig.Config) {
	if config == nil {
		fmt.Println("configuration is not initialized")
		return
	}

	if config.BenchmarkMode {
		fmt.Println("Benchmark mode is enabled; skipping model deletion.")
		return
	}

	hosts := createHosts(*config)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h LLMHost) {
			defer wg.Done()
			if h.GetType() != "ollama" {
				fmt.Printf("Deleting models is not supported for %s (%s)\n", h.GetName(), h.GetType())
				return
			}
			deleteModelsOnNode(h, h.GetModels())
		}(host)
	}
	wg.Wait()
	fmt.Println("All model cleanup commands have finished.")
}

// deleteModelsOnNode deletes models on a single host that are not present in modelsToKeep.
func deleteModelsOnNode(host LLMHost, modelsToKeep []string) {
	fmt.Printf("Starting model cleanup for %s...\n", host.GetName())
	models, err := host.ListRawModels()
	if err != nil {
		fmt.Printf("Error getting models from %s: %v\n", host.GetName(), err)
		return
	}

	modelsToKeepSet := make(map[string]struct{})
	for _, m := range modelsToKeep {
		modelsToKeepSet[m] = struct{}{}
	}

	for _, installedModelName := range models {
		modelName := installedModelName
		if _, keep := modelsToKeepSet[modelName]; !keep {
			fmt.Printf("  -> Deleting model: %s on %s\n", modelName, host.GetName())
			host.DeleteModel(modelName)
		} else {
			fmt.Printf("  -> Keeping model: %s on %s\n", modelName, host.GetName())
		}
	}
}

// UnloadModels unloads all currently loaded models on each supported host.
func UnloadModels(config *appconfig.Config) {
	if config == nil {
		fmt.Println("configuration is not initialized")
		return
	}

	hosts := createHosts(*config)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h LLMHost) {
			defer wg.Done()
			if h.GetType() != "ollama" && h.GetType() != "llama.cpp" {
				fmt.Printf("Unloading models is not supported for %s (%s)\n", h.GetName(), h.GetType())
				return
			}
			fmt.Printf("Unloading models for %s...\n", h.GetName())
			runningModels, err := h.GetRunningModels()
			if err != nil {
				fmt.Printf("Error getting running models from %s: %v\n", h.GetName(), err)
				return
			}
			for model := range runningModels {
				fmt.Printf("  -> Unloading model: %s on %s\n", model, h.GetName())
				h.UnloadModel(model)
			}
		}(host)
	}
	wg.Wait()
	fmt.Println("All model unload commands have finished.")
}

var (
	deleteModelsFunc = DeleteModels
	pullModelsFunc   = PullModels
)

// SyncModels deletes any models not in config and then pulls missing models.
func SyncModels(config *appconfig.Config) {
	if config.BenchmarkMode {
		fmt.Println("Benchmark mode is enabled; skipping model sync.")
		return
	}

	deleteModelsFunc(config)
	pullModelsFunc(config)
}

// SyncConfigs prints the current configuration.
func SyncConfigs(config *appconfig.Config) {
	pp.Println(config)
}

// ListModels lists models on each configured host, indicating which are currently loaded for Ollama hosts.
func ListModels(config *appconfig.Config) {
	if config == nil {
		fmt.Println("configuration is not initialized")
		return
	}

	hosts := createHosts(*config)
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	nodeModels := make(map[string][]string)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, host := range hosts {
		wg.Add(1)
		go func(h LLMHost) {
			defer wg.Done()
			models, err := h.ListModels()
			mu.Lock()
			if err != nil {
				nodeModels[h.GetName()] = []string{fmt.Sprintf("Error: %v", err)}
			} else {
				nodeModels[h.GetName()] = models
			}
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	var sortedNodes []string
	for node := range nodeModels {
		sortedNodes = append(sortedNodes, node)
	}
	sort.Strings(sortedNodes)

	for _, node := range sortedNodes {
		fmt.Println(nodeStyle.Render(fmt.Sprintf("%s:", node)))
		for _, model := range nodeModels[node] {
			cleanedModelString := strings.TrimSpace(strings.ReplaceAll(model, "-", ""))
			fmt.Println("  >>> " + cleanedModelString)
		}
		fmt.Println()
	}
}

// ListModelParameters prints the exposed parameters for each model on every configured host.
func ListModelParameters(config *appconfig.Config) {
	if config == nil {
		fmt.Println("configuration is not initialized")
		return
	}

	hosts := createHosts(*config)
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	for _, host := range hosts {
		fmt.Println(nodeStyle.Render(fmt.Sprintf("%s:", host.GetName())))
		if host.GetType() != "ollama" {
			fmt.Printf("Listing model parameters is not supported for %s (%s)\n", host.GetName(), host.GetType())
			continue
		}

		params, err := host.GetModelParameters()
		if err != nil {
			fmt.Printf("Error getting model parameters from %s: %v\n", host.GetName(), err)
			continue
		}

		for _, p := range params {
			fmt.Printf("  >>> %s\n", p.Model)

			settings := extractSettings(p.Parameters)

			fmt.Println("----------------------------------------------------------------")
			pp.Println(params)
			fmt.Println("********************")
			pp.Println(settings)
			fmt.Println("----------------------------------------------------------------")
			fmt.Printf("      temperature: %s\n", settings["temperature"])
			fmt.Printf("      top_p: %s\n", settings["top_p"])
			fmt.Printf("      top_k: %s\n", settings["top_k"])
			fmt.Printf("      repeat_penalty: %s\n", settings["repeat_penalty"])
			fmt.Printf("      min_p: %s\n", settings["min_p"])
		}
		fmt.Println()
	}
}

func ShowModelInfo(config *appconfig.Config) {
	hosts := createHosts(*config)
	var wg sync.WaitGroup

	allModels := make(map[string]struct{})

	var mu sync.Mutex

	messagesChan := make(chan string, len(hosts))

	for _, host := range hosts {
		wg.Add(1)
		go func(h LLMHost) {
			defer wg.Done()
			if h.GetType() != "ollama" {
				messagesChan <- fmt.Sprintf("Skipping %s: Not an 'ollama' host type (%s)", h.GetName(), h.GetType())
				return
			}

			modelParams, err := h.GetModelParameters()
			if err != nil {
				fmt.Println("Error fetching model parameters from ", h.GetName(), ": ", err)
				os.Exit(1)
			}

			pp.Println(modelParams)

			messagesChan <- fmt.Sprintf("Fetching models from %s...", h.GetName())
			models, err := h.ListRawModels()
			if err != nil {
				messagesChan <- fmt.Sprintf("Error from %s: %v", h.GetName(), err)
				return
			}

			mu.Lock()
			for _, model := range models {
				allModels[model] = struct{}{}
			}
			mu.Unlock()

		}(host)
	}

	go func() {
		wg.Wait()
		close(messagesChan)
	}()

	fmt.Println("--- Host Status & Errors ---")
	for msg := range messagesChan {
		fmt.Println(msg)
	}
	fmt.Println("----------------------------")

	uniqueModels := make([]string, 0, len(allModels))
	for modelName := range allModels {
		uniqueModels = append(uniqueModels, modelName)
	}

	sort.Strings(uniqueModels)

	for _, modelName := range uniqueModels {
		pp.Println(modelName)
	}
}
