package tools

// Definition describes the metadata the MCP server exposes for a tool.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ContentPart represents a piece of data returned from a tool invocation.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Handler executes a tool using the provided arguments and returns content for the LLM.
type Handler func(map[string]any) ([]ContentPart, error)

const (
	// CurrentWeatherName is the canonical name for the weather tool.
	CurrentWeatherName = "current_weather"
	// CurrentTimeName is the canonical name for the time tool.
	CurrentTimeName = "current_time"
)
