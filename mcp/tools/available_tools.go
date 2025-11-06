package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AvailableToolsDefinition describes a helper tool that lists MCP tools.
func AvailableToolsDefinition() Definition {
	return Definition{
		Name:        AvailableToolsName,
		Description: "Use this tool when the user asks which MCP tools are available or requests a summary of their capabilities. Do not call any other tool while answering this question.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// AvailableToolsTool returns the complete, wrapped tool definition.
func AvailableToolsTool() Tool {
	return Tool{
		Type:     "function",
		Function: AvailableToolsDefinition(), // Call your existing function here
	}
}

// AvailableTools returns the set of tools exposed by the MCP server in both JSON and summaries.
func AvailableTools(args map[string]any) ([]ContentPart, error) {
	definitions := []Definition{
		AvailableToolsDefinition(),
		CurrentTimeDefinition(),
		CurrentWeatherDefinition(),
	}

	payload := make([]map[string]string, 0, len(definitions))
	var summaryBuilder strings.Builder

	for _, def := range definitions {
		payload = append(payload, map[string]string{
			"name":        def.Name,
			"description": def.Description,
		})
		if summaryBuilder.Len() > 0 {
			summaryBuilder.WriteString("\n")
		}
		summaryBuilder.WriteString(fmt.Sprintf("- %s: %s", def.Name, def.Description))
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare available tools response")
	}

	interpretPrompt := strings.Join([]string{
		"You are a helpful assistant. Use the provided JSON to clearly list the available MCP tools and explain when each should be used.",
		"Keep the explanation concise.",
		"JSON Weather Data: " + string(jsonPayload),
	}, " ")

	return []ContentPart{
		{Type: "json", Text: string(jsonPayload)},
		//{Type: "text", Text: summaryBuilder.String()},
		{Type: "interpret", Text: interpretPrompt},
	}, nil
}
