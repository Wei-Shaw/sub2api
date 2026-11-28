package service

import "strings"

// GPT-6.2 appends top-level instructions after input. Keep caller-owned system
// guidance in the conversation prefix instead of copying it to an uncached tail.
func normalizeGPT62SystemMessages(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	modified := false
	for _, item := range input {
		message, ok := item.(map[string]any)
		if ok && message["role"] == "system" {
			message["role"] = "developer"
			modified = true
		}
	}
	return modified
}

// applyGPT62DefaultInstructions only supplies the gateway's missing default.
// Explicit client instructions remain turn-scoped and must not be relocated.
func applyGPT62DefaultInstructions(reqBody map[string]any, model string) bool {
	if !isInstructionsEmpty(reqBody) {
		return false
	}

	var input []any
	switch value := reqBody["input"].(type) {
	case nil:
	case []any:
		input = value
	case string:
		if strings.TrimSpace(value) != "" {
			input = []any{map[string]any{
				"type": "message", "role": "user", "content": value,
			}}
		}
	default:
		// Preserve invalid input for the existing upstream validation rather
		// than replacing caller data with a seemingly valid default message.
		return false
	}

	instructions := defaultCodexSynthInstructions(model)
	hasDefaultPrefix := false
	if len(input) > 0 {
		if message, ok := input[0].(map[string]any); ok && message["role"] == "developer" {
			text, _ := message["content"].(string)
			hasDefaultPrefix = text == instructions
		}
	}
	if !hasDefaultPrefix {
		// The raw forwarding pass and the OAuth transform can both visit this
		// request. Only recognize our exact first message to avoid duplication.
		input = append([]any{map[string]any{
			"type": "message", "role": "developer", "content": instructions,
		}}, input...)
		reqBody["input"] = input
	}
	existing, present := reqBody["instructions"]
	reqBody["instructions"] = ""
	return !hasDefaultPrefix || !present || existing != ""
}
