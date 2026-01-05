// internal/models/types.go
package models

// ModelParameters holds the detailed parameters of a model.
type ModelParameters struct {
	Model      string       `json:"model,omitempty"`
	Parameters string       `json:"parameters,omitempty"`
	Details    ModelDetails `json:"details,omitempty"`
}

// ModelDetails holds the nested details of a model.
type ModelDetails struct {
	Family            string `json:"family,omitempty"`
	Format            string `json:"format,omitempty"`
	ParameterSize     string `json:"parameter_size,omitempty"`
	QuantizationLevel string `json:"quantization_level,omitempty"`
}

// LLMHost defines the model lifecycle and metadata operations a host must support.
// Implementations should pull, delete, list, and unload models, and expose basic metadata.
type LLMHost interface {
	// PullModel pulls a model from a remote registry.
	PullModel(model string)
	// DeleteModel deletes a model from the host.
	DeleteModel(model string)
	// ListModels lists the models available on the host.
	ListModels() ([]string, error)
	// ListRawModels lists the raw, unprocessed model names.
	ListRawModels() ([]string, error)
	// UnloadModel unloads a model from memory.
	UnloadModel(model string)
	// GetName returns the name of the host.
	GetName() string
	// GetType returns the type of the host (e.g., "ollama").
	GetType() string
	// GetModels returns the list of models configured for this host.
	GetModels() []string
	// GetModelParameters returns the detailed parameters for all models.
	GetModelParameters() ([]ModelParameters, error)
	// GetRunningModels returns a map of models currently running on the host.
	GetRunningModels() (map[string]struct{}, error)
}
