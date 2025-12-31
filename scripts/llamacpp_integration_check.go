// scripts/llamacpp_integration_check.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

type llamaModel struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Model  string      `json:"model"`
	Path   string      `json:"path"`
	Status statusField `json:"status"`
}

type modelsResponse struct {
	Data   []llamaModel `json:"data"`
	Models []llamaModel `json:"models"`
}

type statusField struct {
	Value string
}

func (s *statusField) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		s.Value = ""
		return nil
	}
	if trimmed[0] == '"' {
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		s.Value = v
		return nil
	}
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	s.Value = obj.Value
	return nil
}

func main() {
	configPath := flag.String("config", appconfig.DefaultConfigPath, "Path to config JSON")
	hostURL := flag.String("url", "", "Override llama.cpp host URL")
	modelName := flag.String("model", "", "Override model name for chat probe")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP timeout")
	flag.Parse()

	host, model, err := resolveTarget(*configPath, *hostURL, *modelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: *timeout}

	fmt.Printf("Target host: %s\n", host.URL)
	fmt.Printf("Target model: %s\n\n", model)

	if err := checkModels(client, host.URL); err != nil {
		fmt.Fprintf(os.Stderr, "models check failed: %v\n", err)
	}

	if err := probeChatParams(client, host.URL, model); err != nil {
		fmt.Fprintf(os.Stderr, "chat param probe failed: %v\n", err)
	}
}

func resolveTarget(configPath, overrideURL, overrideModel string) (appconfig.Host, string, error) {
	if overrideURL != "" {
		model := overrideModel
		if model == "" {
			model = "model"
		}
		return appconfig.Host{URL: overrideURL, Name: "llama.cpp", Type: "llama.cpp"}, model, nil
	}

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return appconfig.Host{}, "", err
	}

	for _, host := range cfg.Hosts {
		kind := strings.ToLower(strings.TrimSpace(host.Type))
		if kind == "llama.cpp" || kind == "llamacpp" {
			model := overrideModel
			if model == "" {
				if len(host.Models) > 0 {
					model = host.Models[0]
				} else {
					model = "model"
				}
			}
			return host, model, nil
		}
	}

	return appconfig.Host{}, "", fmt.Errorf("no llama.cpp host found in %s", configPath)
}

func checkModels(client *http.Client, baseURL string) error {
	fmt.Println("== /models ==")
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Println("Raw:")
	fmt.Println(indentJSON(body))

	parsed, err := parseModels(body)
	if err != nil {
		fmt.Printf("Parse: %v\n\n", err)
		return nil
	}

	fmt.Printf("Parsed models: %d\n", len(parsed))
	for _, m := range parsed {
		fmt.Printf("  - %s (status=%s)\n", modelDisplayName(m), strings.TrimSpace(m.Status.Value))
	}
	fmt.Println()
	return nil
}

func probeChatParams(client *http.Client, baseURL, model string) error {
	fmt.Println("== /v1/chat/completions param probe ==")
	type paramCase struct {
		name  string
		key   string
		value any
	}
	cases := []paramCase{
		{name: "temperature", key: "temperature", value: 0.2},
		{name: "top_p", key: "top_p", value: 0.9},
		{name: "top_k", key: "top_k", value: 40},
		{name: "min_p", key: "min_p", value: 0.05},
		{name: "tfs_z", key: "tfs_z", value: 1.0},
		{name: "typical_p", key: "typical_p", value: 0.9},
		{name: "repeat_last_n", key: "repeat_last_n", value: 64},
		{name: "repeat_penalty", key: "repeat_penalty", value: 1.1},
		{name: "presence_penalty", key: "presence_penalty", value: 0.2},
		{name: "frequency_penalty", key: "frequency_penalty", value: 0.2},
	}

	basePayload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"stream": false,
	}

	for _, tc := range cases {
		payload := cloneMap(basePayload)
		payload[tc.key] = tc.value
		status, body, err := postJSON(client, baseURL+"/v1/chat/completions", payload)
		if err != nil {
			fmt.Printf("%s: error=%v\n", tc.name, err)
			continue
		}
		accepted := status >= 200 && status < 300
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		fmt.Printf("%s: status=%d accepted=%v body=%s\n", tc.name, status, accepted, msg)
	}
	fmt.Println()
	return nil
}

func postJSON(client *http.Client, url string, payload map[string]any) (int, []byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func parseModels(body []byte) ([]llamaModel, error) {
	var wrapped modelsResponse
	if err := json.Unmarshal(body, &wrapped); err == nil {
		if len(wrapped.Models) > 0 {
			return wrapped.Models, nil
		}
		if len(wrapped.Data) > 0 {
			return wrapped.Data, nil
		}
	}
	var direct []llamaModel
	if err := json.Unmarshal(body, &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}
	var names struct {
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(body, &names); err == nil && len(names.Models) > 0 {
		out := make([]llamaModel, 0, len(names.Models))
		for _, name := range names.Models {
			out = append(out, llamaModel{Name: name})
		}
		return out, nil
	}
	return nil, fmt.Errorf("unrecognized /models response")
}

func modelDisplayName(model llamaModel) string {
	if strings.TrimSpace(model.ID) != "" {
		return strings.TrimSpace(model.ID)
	}
	if strings.TrimSpace(model.Name) != "" {
		return strings.TrimSpace(model.Name)
	}
	if strings.TrimSpace(model.Model) != "" {
		return strings.TrimSpace(model.Model)
	}
	return strings.TrimSpace(model.Path)
}

func indentJSON(body []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return string(body)
	}
	return out.String()
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
