// accuracy/types.go
package accuracy

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
	Timestamp          string `json:"timestamp"`
	Host               string `json:"host"`
	Model              string `json:"model"`
	PromptID           int    `json:"promptId"`
	Prompt             string `json:"prompt"`
	ExpectedAnswer     int    `json:"expectedAnswer"`
	Response           string `json:"response"`
	Correct            bool   `json:"correct"`
	MarginOfError      int    `json:"marginOfError"`
	Difficulty         int    `json:"difficulty"`
	TimeToFirstToken   int    `json:"time_to_first_token"`
	TokensPerSecond    float64 `json:"tokens_per_second"`
	InputTokens        int    `json:"input_tokens"`
	OutputTokens       int    `json:"output_tokens"`
	TotalDurationMs    int    `json:"total_duration_ms"`
	DeadlineExceeded   bool   `json:"deadlineExceeded"`
	DeadlineTimeoutSec int    `json:"deadlineTimeout"`
}
