// internal/providers/provider.go

// Package providers defines the interfaces for interacting with different AI model providers.
// It provides a common abstraction layer for sending requests, handling streaming responses,
// and managing models, regardless of the underlying provider implementation (e.g., Ollama, MCP).
package providers

import (
	"context"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

// ChatMessage represents a single message in a chat conversation.
// It contains the role of the message sender (e.g., "user", "assistant") and the message content.
type ChatMessage struct {
	Role    string
	Content string
}

// ToolDefinition defines the structure of a tool that can be invoked by a provider.
// It includes the tool's name, a description of its purpose, and a schema for its parameters.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolExecutor is a function type for executing a tool.
// It takes the tool's name and arguments and returns the result as a string.
type ToolExecutor func(ctx context.Context, name string, args map[string]any) (string, error)

// StreamMetadata contains metadata about a completed chat stream,
// including performance metrics like timing and token counts.
type StreamMetadata struct {
	Model              string
	CreatedAt          time.Time
	Done               bool
	TotalDuration      int64
	LoadDuration       int64
	PromptEvalCount    int
	PromptEvalDuration int64
	EvalCount          int
	EvalDuration       int64
}

// StreamRequest encapsulates all the information needed to initiate a chat stream.
type StreamRequest struct {
	Host             appconfig.Host
	Model            string
	History          []ChatMessage
	SystemPrompt     string
	Parameters       appconfig.Parameters
	JSONMode         bool
	Tools            []ToolDefinition
	DisableStreaming bool
	ToolExecutor     ToolExecutor
}

// StreamCallbacks defines the callback functions that are invoked during a chat stream.
// OnChunk is called for each message chunk received, and OnComplete is called when the stream is finished.
type StreamCallbacks struct {
	OnChunk    func(ChatMessage) error
	OnComplete func(StreamMetadata) error
}

// ChatProvider is the interface that all model providers must implement.
// It defines the core functionalities for managing models and conducting chat streams.
type ChatProvider interface {
	// LoadedModels returns a list of models that are currently loaded into memory for a given host.
	LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error)
	// EnsureModelReady checks if a model is ready to be used and loads it if necessary.
	EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error
	// Stream initiates a chat stream with the provider, sending and receiving messages.
	Stream(ctx context.Context, req StreamRequest, callbacks StreamCallbacks) error
	// Close cleans up any resources used by the provider.
	Close() error
}