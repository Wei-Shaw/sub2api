package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// normalizeOpenAIResponsesLegacyIngress accepts the Chat Completions-shaped
// payloads that a few Responses clients still emit. Native Responses input is
// always authoritative because this path has no separate full-replay attempt.
func normalizeOpenAIResponsesLegacyIngress(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	var request map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &request); err != nil {
		return body, false, fmt.Errorf("normalize legacy Responses ingress: %w", err)
	}

	changed := false
	messagesValue, hasMessages := request["messages"]
	if hasMessages {
		messages, messagesAreArray := messagesValue.([]any)
		if messagesAreArray && len(messages) > 0 {
			legacy, err := convertLegacyResponsesMessages(body)
			if err != nil {
				return body, false, err
			}

			input, hasInput := request["input"]
			hasNativeInput := hasInput && input != nil
			if !hasNativeInput {
				request["input"] = legacy.input
				applyLegacyResponsesTopLevelFields(request, legacy)
				delete(request, "previous_response_id")
			}
		}
		// messages is not a Responses field. When native input is present but the
		// two histories are ambiguous, retain input and drop only the legacy alias.
		delete(request, "messages")
		changed = true
	}

	if prompt, hasPrompt := request["prompt"]; hasPrompt {
		// Only the legacy string alias is unambiguously equivalent to Responses
		// input. Objects are native reusable prompt templates and must remain in
		// prompt; arrays and other shapes are left for upstream validation rather
		// than being relabeled as a structurally different input value.
		if promptText, isLegacyString := prompt.(string); isLegacyString {
			if input, hasInput := request["input"]; !hasInput || input == nil {
				request["input"] = promptText
			}
			delete(request, "prompt")
			changed = true
		}
	}
	if _, hasCommands := request["commands"]; hasCommands {
		delete(request, "commands")
		changed = true
	}

	if !changed {
		return body, false, nil
	}
	normalized, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return body, false, fmt.Errorf("serialize legacy Responses ingress: %w", err)
	}
	return normalized, true, nil
}

type convertedLegacyResponsesMessages struct {
	input              any
	instructions       string
	maxOutputTokens    *int
	temperature        *float64
	topP               *float64
	tools              any
	reasoning          any
	toolChoice         any
	serviceTier        string
	hasChatTools       bool
	hasChatToolChoice  bool
	hasReasoningEffort bool
}

func convertLegacyResponsesMessages(body []byte) (convertedLegacyResponsesMessages, error) {
	var converted convertedLegacyResponsesMessages
	var chatRequest apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &chatRequest); err != nil {
		return converted, fmt.Errorf("normalize legacy Responses messages: %w", err)
	}
	responsesRequest, err := apicompat.ChatCompletionsToResponses(&chatRequest)
	if err != nil {
		return converted, fmt.Errorf("normalize legacy Responses messages: %w", err)
	}
	if len(responsesRequest.Input) == 0 {
		return converted, fmt.Errorf("normalize legacy Responses messages: empty input")
	}
	if err := json.Unmarshal(responsesRequest.Input, &converted.input); err != nil {
		return converted, fmt.Errorf("decode normalized legacy Responses input: %w", err)
	}

	converted.instructions = strings.TrimSpace(responsesRequest.Instructions)
	converted.maxOutputTokens = responsesRequest.MaxOutputTokens
	converted.temperature = responsesRequest.Temperature
	converted.topP = responsesRequest.TopP
	converted.serviceTier = strings.TrimSpace(responsesRequest.ServiceTier)
	converted.hasReasoningEffort = strings.TrimSpace(chatRequest.ReasoningEffort) != ""
	if responsesRequest.Reasoning != nil {
		converted.reasoning = responsesRequest.Reasoning
	}

	var rawRequest map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &rawRequest); err != nil {
		return converted, fmt.Errorf("inspect legacy Responses fields: %w", err)
	}
	converted.hasChatTools = hasChatStyleTools(rawRequest) || rawRequest["functions"] != nil
	if converted.hasChatTools && len(responsesRequest.Tools) > 0 {
		converted.tools = responsesRequest.Tools
	}
	converted.hasChatToolChoice = hasChatStyleToolChoice(rawRequest) || rawRequest["function_call"] != nil
	if converted.hasChatToolChoice && len(responsesRequest.ToolChoice) > 0 {
		choice, err := normalizeLegacyResponsesToolChoice(responsesRequest.ToolChoice)
		if err != nil {
			return converted, err
		}
		converted.toolChoice = choice
	}
	return converted, nil
}

func hasChatStyleTools(request map[string]any) bool {
	tools, ok := request["tools"].([]any)
	if !ok {
		return false
	}
	for _, rawTool := range tools {
		tool, _ := rawTool.(map[string]any)
		if _, ok := tool["function"].(map[string]any); ok {
			return true
		}
	}
	return false
}

func hasChatStyleToolChoice(request map[string]any) bool {
	choice, _ := request["tool_choice"].(map[string]any)
	_, ok := choice["function"].(map[string]any)
	return ok
}

func normalizeLegacyResponsesToolChoice(raw json.RawMessage) (any, error) {
	var choice any
	if err := json.Unmarshal(raw, &choice); err != nil {
		return nil, fmt.Errorf("normalize legacy Responses tool_choice: %w", err)
	}
	object, ok := choice.(map[string]any)
	if !ok || strings.TrimSpace(firstNonEmptyString(object["type"])) != "function" {
		return choice, nil
	}
	function, ok := object["function"].(map[string]any)
	if !ok {
		return choice, nil
	}
	name := strings.TrimSpace(firstNonEmptyString(function["name"]))
	if name == "" {
		return choice, nil
	}
	return map[string]any{"type": "function", "name": name}, nil
}

func applyLegacyResponsesTopLevelFields(request map[string]any, legacy convertedLegacyResponsesMessages) {
	if legacy.instructions != "" && strings.TrimSpace(firstNonEmptyString(request["instructions"])) == "" {
		request["instructions"] = legacy.instructions
	}
	if legacy.maxOutputTokens != nil {
		request["max_output_tokens"] = *legacy.maxOutputTokens
	}
	if legacy.temperature != nil {
		request["temperature"] = *legacy.temperature
	}
	if legacy.topP != nil {
		request["top_p"] = *legacy.topP
	}
	if legacy.hasChatTools && legacy.tools != nil {
		request["tools"] = legacy.tools
	}
	if legacy.hasReasoningEffort && legacy.reasoning != nil {
		request["reasoning"] = legacy.reasoning
	}
	if legacy.hasChatToolChoice && legacy.toolChoice != nil {
		request["tool_choice"] = legacy.toolChoice
	}
	if legacy.serviceTier != "" {
		request["service_tier"] = legacy.serviceTier
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens", "reasoning_effort", "functions", "function_call"} {
		delete(request, key)
	}
}
