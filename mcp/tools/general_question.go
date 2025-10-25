package tools

// GeneralQuestionName is the canonical name for the general question tool.
const GeneralQuestionName = "answer_general_question"

// GeneralQuestionDefinition describes the general question tool for discovery by the MCP host.
func GeneralQuestionDefinition() Definition {
	return Definition{
		Name:        GeneralQuestionName,
		Description: "Use this tool to answer general questions that do not require real-time information or specific device actions. Most questions about facts, history, or general knowledge fall into this category.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The user's original question.",
				},
			},
			"required": []string{"question"},
		},
	}
}

// GeneralQuestion is a handler that simply returns the user's question, indicating that the LLM should answer it directly.
func GeneralQuestion(args map[string]any) ([]ContentPart, error) {
	question, _ := args["question"].(string)
	return []ContentPart{{Type: "text", Text: question}}, nil
}
