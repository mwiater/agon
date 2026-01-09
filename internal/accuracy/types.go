// accuracy/types.go
package accuracy

import "encoding/json"

// PromptSuite defines the accuracy test cases loaded from JSON.
type PromptSuite struct {
	SystemPrompt string       `json:"system_prompt"`
	Tests        []PromptTest `json:"tests"`
}

// PromptTest defines a single prompt and expected answer.
type PromptTest struct {
	ID             int    `json:"id"`
	Prompt         string `json:"prompt"`
	ExpectedAnswer int    `json:"expected_answer"`
	MarginOfError  int    `json:"marginOfError,omitempty"`
	Difficulty     int    `json:"difficulty,omitempty"`
	Category       string `json:"category"`
}

// AccuracyResult records a single model response and its correctness.
type AccuracyResult struct {
	Timestamp           string          `json:"timestamp"`
	Host                string          `json:"host"`
	Model               string          `json:"model"`
	PromptID            int             `json:"promptId"`
	Prompt              string          `json:"prompt"`
	ExpectedAnswer      int             `json:"expectedAnswer"`
	Response            string          `json:"response"`
	EvaluatedResponse   string          `json:"evaluatedResponse,omitempty"`
	LogProbs            json.RawMessage `json:"logprobs,omitempty"`
	Correct             bool            `json:"correct"`
	MarginOfError       int             `json:"marginOfError"`
	Difficulty          int             `json:"difficulty"`
	TimeToFirstToken    int             `json:"time_to_first_token"`
	TokensPerSecond     float64         `json:"tokens_per_second"`
	InputTokens         int             `json:"input_tokens"`
	OutputTokens        int             `json:"output_tokens"`
	TotalDurationMs     int             `json:"total_duration_ms"`
	TotalTokens         int             `json:"total_tokens,omitempty"`
	CacheN              int             `json:"cache_n,omitempty"`
	PromptN             int             `json:"prompt_n,omitempty"`
	PromptMs            float64         `json:"prompt_ms,omitempty"`
	PromptPerTokenMs    float64         `json:"prompt_per_token_ms,omitempty"`
	PromptPerSecond     float64         `json:"prompt_per_second,omitempty"`
	PredictedN          int             `json:"predicted_n,omitempty"`
	PredictedMs         float64         `json:"predicted_ms,omitempty"`
	PredictedPerTokenMs float64         `json:"predicted_per_token_ms,omitempty"`
	PredictedPerSecond  float64         `json:"predicted_per_second,omitempty"`
	DeadlineExceeded    bool            `json:"deadlineExceeded"`
	DeadlineTimeoutSec  int             `json:"deadlineTimeout"`
	RagMode             string          `json:"rag_mode,omitempty"`
	RetrievalMs         int             `json:"retrieval_ms,omitempty"`
	ContextTokens       int             `json:"context_tokens,omitempty"`
	TopK                int             `json:"top_k,omitempty"`
	SourceCoverage      int             `json:"source_coverage,omitempty"`
	CitationsUsed       bool            `json:"citations_used,omitempty"`
	ParameterTemplate   string          `json:"parameterTemplate,omitempty"`
}
