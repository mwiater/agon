// Package providers defines abstractions for routing chat traffic to different backends.
package providers

import (
	"context"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

// ChatMessage represents a single message exchanged with a provider.
type ChatMessage struct {
	Role    string
	Content string
}

// ToolDefinition describes a tool exposed by an upstream provider.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolExecutor invokes a tool by name with arbitrary arguments.
type ToolExecutor func(ctx context.Context, name string, args map[string]any) (string, error)

// StreamMetadata captures timing and token metrics returned by a provider.
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

// StreamRequest bundles all inputs necessary to start a chat stream.
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

// StreamCallbacks are invoked as the provider yields output.
type StreamCallbacks struct {
	OnChunk    func(ChatMessage) error
	OnComplete func(StreamMetadata) error
}

// ChatProvider exposes the minimal surface needed by both regular and multimodel flows.
type ChatProvider interface {
	LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error)
	EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error
	Stream(ctx context.Context, req StreamRequest, callbacks StreamCallbacks) error
	Close() error
}
