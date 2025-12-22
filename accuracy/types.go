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
	Tolerance      int    `json:"tolerance,omitempty"`
	Category       string `json:"category"`
}

// AccuracyResult records a single model response and its correctness.
type AccuracyResult struct {
	Timestamp      string `json:"timestamp"`
	Host           string `json:"host"`
	Model          string `json:"model"`
	PromptID       int    `json:"promptId"`
	Prompt         string `json:"prompt"`
	ExpectedAnswer int    `json:"expectedAnswer"`
	Response       string `json:"response"`
	Correct        bool   `json:"correct"`
}
