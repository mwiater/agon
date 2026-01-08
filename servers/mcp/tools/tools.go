package tools

// Definition describes the metadata the MCP server exposes for a tool.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Tool wraps a Definition to match the required "function" wrapper structure.
type Tool struct {
	Type     string     `json:"type"`
	Function Definition `json:"function"`
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
	// AvailableToolsName is the canonical name for the available-tools helper.
	AvailableToolsName = "available_tools"
)
