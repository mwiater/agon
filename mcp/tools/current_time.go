package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// CurrentTimeDefinition describes the time tool for discovery by the MCP host.
func CurrentTimeDefinition() Definition {
	return Definition{
		Name:        CurrentTimeName,
		Description: "Provides the server's current local time and timezone. Use this tool for user queries like 'What time is it right now?'",
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
		log.Printf("Current time tool JSON marshal error: %v", err)
		return nil, fmt.Errorf("Error preparing time response.")
	}

	interpretPrompt := strings.Join([]string{
		"You are a helpful assistant. Interpret the provided JSON time data and explain the current local time in natural language.",
		"Keep the answer concise and avoid restating raw field names unless helpful.",
	}, " ")

	return []ContentPart{
		{Type: "json", Text: string(jsonTime)},
		{Type: "interpret", Text: interpretPrompt},
	}, nil
}
