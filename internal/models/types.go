package models

// ModelParameters holds the detailed parameters of a model.
type ModelParameters struct {
	Model      string       `json:"model,omitempty"`
	License    string       `json:"license,omitempty"`
	Modelfile  string       `json:"modelfile,omitempty"`
	Parameters string       `json:"parameters,omitempty"`
	Template   string       `json:"template,omitempty"`
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
	PullModel(model string)
	DeleteModel(model string)
	ListModels() ([]string, error)
	ListRawModels() ([]string, error)
	UnloadModel(model string)
	GetName() string
	GetType() string
	GetModels() []string
	GetModelParameters() ([]ModelParameters, error)
	GetRunningModels() (map[string]struct{}, error)
}
