package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CurrentTimeDefinition describes the time tool for discovery by the MCP host.
func CurrentTimeDefinition() Definition {
	return Definition{
		Name:        CurrentTimeName,
		Description: "Use this tool *only* for queries about the current time, such as 'What time is it?' or 'What is the current time?'. **This tool cannot provide any weather information.**",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// CurrentTime returns the current system time as JSON for interpretation by the LLM.
func CurrentTime(args map[string]any) ([]ContentPart, error) {
	now := time.Now()
	payload := map[string]any{
		"local_time": now.Format(time.RFC3339),
		"timezone":   now.Location().String(),
		"unix":       now.Unix(),
	}

	jsonTime, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error preparing time response: %w", err)
	}

	interpretPrompt := strings.Join([]string{
		"You are a helpful assistant. Interpret the provided JSON time data and explain the current local date and time in natural language.",
		"Do not mention that you are translating JSON data",
	}, " ")

	return []ContentPart{
		{Type: "json", Text: string(jsonTime)},
		{Type: "interpret", Text: interpretPrompt},
	}, nil
}
