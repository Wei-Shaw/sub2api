package apicompat

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

type chatMessageContent struct {
	Text  *string
	Parts []ChatContentPart
}

// ChatCompletionsToResponses converts a Chat Completions request into a
// Responses API request. The upstream always streams, so Stream is forced to
// true. store is always false and reasoning.encrypted_content is always
// included so that the response translator has full context.
func ChatCompletionsToResponses(req *ChatCompletionsRequest) (*ResponsesRequest, error) {
	return chatCompletionsToResponses(req, false)
}

var ErrUnsupportedInputAudio = errors.New("input_audio is not supported on this conversion route")

func ChatCompletionsToResponsesForGemini(req *ChatCompletionsRequest) (*ResponsesRequest, error) {
	return chatCompletionsToResponses(req, true)
}

func chatCompletionsToResponses(req *ChatCompletionsRequest, allowAudio bool) (*ResponsesRequest, error) {
	if req.ambiguousInputAudio {
		if !allowAudio {
			return nil, fmt.Errorf("duplicate input_audio message or content fields: %w", ErrUnsupportedInputAudio)
		}
		return nil, fmt.Errorf("invalid input_audio: duplicate message or content fields")
	}
	converted := *req
	converted.Messages = append([]ChatMessage(nil), req.Messages...)
	messageAudio := make(map[int]map[int]string)
	for index, message := range converted.Messages {
		var rawParts []json.RawMessage
		if json.Unmarshal(message.Content, &rawParts) != nil {
			continue
		}
		hasAudio, ambiguous := chatContentAudioStatus(message.Content)
		if !hasAudio {
			continue
		}
		if !allowAudio || message.Role != "user" {
			return nil, ErrUnsupportedInputAudio
		}
		if ambiguous {
			return nil, fmt.Errorf("invalid input_audio: duplicate content fields")
		}
		if err := validateChatAudioParts(rawParts); err != nil {
			return nil, fmt.Errorf("invalid input_audio message content: %w", err)
		}
		var parts []ChatContentPart
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			return nil, fmt.Errorf("invalid input_audio message content: %w", err)
		}
		changed := false
		audioParts := make(map[int]string)
		responsePartIndex := 0
		for partIndex, part := range parts {
			if part.Type != "input_audio" {
				responsePartIndex += len(convertChatContentPartsToResponses(parts[partIndex : partIndex+1]))
				continue
			}
			var audio struct {
				Data   string `json:"data"`
				Format string `json:"format"`
			}
			if err := json.Unmarshal(part.InputAudio, &audio); err != nil {
				return nil, fmt.Errorf("invalid input_audio: expected data and format strings")
			}
			mimeType := map[string]string{
				"wav": "audio/wav", "mp3": "audio/mpeg", "ogg": "audio/ogg",
				"flac": "audio/flac", "aac": "audio/aac", "mp4": "audio/mp4", "m4a": "audio/mp4",
			}[audio.Format]
			if mimeType == "" {
				return nil, fmt.Errorf("unsupported input_audio format %q", audio.Format)
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(audio.Data)
			if err != nil || len(decoded) == 0 {
				return nil, fmt.Errorf("invalid input_audio data: expected non-empty base64")
			}
			parts[partIndex] = ChatContentPart{
				Type: "file", PromptCacheBreakpoint: part.PromptCacheBreakpoint,
				File: &ChatFile{FileData: "data:" + mimeType + ";base64," + audio.Data},
			}
			audioParts[responsePartIndex] = parts[partIndex].File.FileData
			responsePartIndex++
			changed = true
		}
		if changed {
			content, err := json.Marshal(parts)
			if err != nil {
				return nil, err
			}
			converted.Messages[index].Content = content
			messageAudio[index] = audioParts
		}
	}
	req = &converted
	if err := openai.ValidateGPT61SolReasoningEffort(req.Model, req.ReasoningEffort); err != nil {
		return nil, err
	}
	var input []ResponsesInputItem
	var inputAudio map[int]map[int]string
	legacyIDs := legacyFunctionCallIDs{msgs: req.Messages}
	for messageIndex, message := range req.Messages {
		switch {
		case message.Role == "assistant" && message.FunctionCall != nil && len(message.ToolCalls) == 0:
			message.ToolCalls = []ChatToolCall{{
				ID:       legacyIDs.assign(message.FunctionCall.Name),
				Type:     "function",
				Function: *message.FunctionCall,
			}}
		case message.Role == "function" && message.ToolCallID == "":
			message.ToolCallID = legacyIDs.claim(message.Name)
		}
		items, err := chatMessageToResponsesItems(message)
		if err != nil {
			return nil, err
		}
		if audioParts := messageAudio[messageIndex]; len(audioParts) > 0 {
			if inputAudio == nil {
				inputAudio = make(map[int]map[int]string)
			}
			inputAudio[len(input)] = audioParts
		}
		input = append(input, items...)
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	out := &ResponsesRequest{
		chatInputAudio:     inputAudio,
		Model:              req.Model,
		Instructions:       req.Instructions,
		Input:              inputJSON,
		Stream:             true, // upstream always streams
		Include:            []string{"reasoning.encrypted_content"},
		ServiceTier:        req.ServiceTier,
		PromptCacheOptions: req.PromptCacheOptions,
		ParallelToolCalls:  req.ParallelToolCalls,
	}

	// Reasoning models (gpt-5.x) do not accept sampling parameters.
	// See isReasoningModel in anthropic_to_responses.go.
	if !isReasoningModel(req.Model) || (openai.IsGPT6SolOrLunaModelSpelling(req.Model) && req.ReasoningEffort == "none") {
		out.Temperature = req.Temperature
		out.TopP = req.TopP
	}

	storeFalse := false
	out.Store = &storeFalse

	// max_tokens / max_completion_tokens → max_output_tokens, prefer max_completion_tokens
	maxTokens := 0
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil {
		maxTokens = *req.MaxCompletionTokens
	}
	if maxTokens > 0 {
		v := maxTokens
		if v < minMaxOutputTokens {
			v = minMaxOutputTokens
		}
		out.MaxOutputTokens = &v
	}

	// reasoning_effort → reasoning.effort + reasoning.summary="auto"
	if req.ReasoningEffort != "" {
		out.Reasoning = &ResponsesReasoning{
			Effort:  req.ReasoningEffort,
			Summary: "auto",
		}
	}

	if format := chatResponseFormatToResponsesTextFormat(req.ResponseFormat); len(format) > 0 {
		if out.Text == nil {
			out.Text = &ResponsesText{}
		}
		out.Text.Format = format
	}

	// tools[] and legacy functions[] → ResponsesTool[]
	if len(req.Tools) > 0 || len(req.Functions) > 0 {
		out.Tools = convertChatToolsToResponses(req.Tools, req.Functions)
	}

	// tool_choice: strings and Responses-shaped objects pass through; a named
	// Chat choice nests the name under "function" and must be flattened.
	// Legacy function_call needs mapping.
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = convertChatToolChoiceToResponses(req.ToolChoice)
	} else if len(req.FunctionCall) > 0 {
		tc, err := convertChatFunctionCallToToolChoice(req.FunctionCall)
		if err != nil {
			return nil, fmt.Errorf("convert function_call: %w", err)
		}
		out.ToolChoice = tc
	}

	return out, nil
}

func validateChatAudioParts(parts []json.RawMessage) error {
	for _, raw := range parts {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return fmt.Errorf("expected content part object")
		}
		var part ChatContentPart
		if err := json.Unmarshal(raw, &part); err != nil {
			return err
		}
		switch part.Type {
		case "input_audio":
		case "text":
			var rawText json.RawMessage
			for name, value := range fields {
				if strings.EqualFold(name, "text") {
					rawText = value
					break
				}
			}
			var text *string
			if err := json.Unmarshal(rawText, &text); err != nil || text == nil {
				return fmt.Errorf("expected text string")
			}
		case "image_url":
			if part.ImageURL == nil || part.ImageURL.URL == "" || isEmptyBase64DataURI(part.ImageURL.URL) {
				return fmt.Errorf("expected non-empty image_url")
			}
		case "file":
			if part.File == nil || (part.File.FileData == "" && part.File.FileID == "") {
				return fmt.Errorf("expected file_data or file_id")
			}
		default:
			return fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}
	return nil
}

// legacyFunctionCallIDs pairs legacy function calls with their results. Legacy
// function calling carries no call IDs: the assistant turn has function_call
// and the role=function result refers to it only by name, while the Responses
// API pairs function_call and function_call_output by call_id. IDs follow
// message order, so a replayed history converts to the same input, and skip
// IDs already used by tool_calls in the same conversation.
type legacyFunctionCallIDs struct {
	msgs    []ChatMessage
	used    map[string]bool
	next    int
	pending map[string][]string
}

// assign returns a new call ID for a legacy function call and queues it for
// the next result with the same function name.
func (ids *legacyFunctionCallIDs) assign(name string) string {
	if ids.used == nil {
		ids.used = make(map[string]bool)
		ids.pending = make(map[string][]string)
		for _, m := range ids.msgs {
			for _, tc := range m.ToolCalls {
				ids.used[tc.ID] = true
			}
			if m.ToolCallID != "" {
				ids.used[m.ToolCallID] = true
			}
		}
	}
	var id string
	for {
		ids.next++
		id = fmt.Sprintf("call_legacy_%d", ids.next)
		if !ids.used[id] {
			break
		}
	}
	ids.used[id] = true
	ids.pending[name] = append(ids.pending[name], id)
	return id
}

// claim returns the oldest unanswered call ID for name, or "" when no legacy
// call with that name precedes the result.
func (ids *legacyFunctionCallIDs) claim(name string) string {
	queue := ids.pending[name]
	if len(queue) == 0 {
		return ""
	}
	ids.pending[name] = queue[1:]
	return queue[0]
}

// chatMessageToResponsesItems converts a single ChatMessage into one or more
// ResponsesInputItem values.
func chatMessageToResponsesItems(m ChatMessage) ([]ResponsesInputItem, error) {
	switch m.Role {
	case "system", "developer":
		return chatSystemToResponses(m)
	case "user":
		return chatUserToResponses(m)
	case "assistant":
		return chatAssistantToResponses(m)
	case "tool":
		return chatToolToResponses(m)
	case "function":
		return chatFunctionToResponses(m)
	default:
		return chatUserToResponses(m)
	}
}

// chatSystemToResponses converts a system or developer message, keeping its
// role: the Responses API accepts both, and developer must not be demoted to
// a user turn.
func chatSystemToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	parsed, err := parseChatMessageContent(m.Content)
	if err != nil {
		return nil, err
	}
	content, err := marshalChatInputContent(parsed)
	if err != nil {
		return nil, err
	}
	return []ResponsesInputItem{{Type: "message", Role: m.Role, Content: content}}, nil
}

// chatUserToResponses converts a user message, handling both plain strings and
// multi-modal content arrays.
func chatUserToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	parsed, err := parseChatMessageContent(m.Content)
	if err != nil {
		return nil, fmt.Errorf("parse user content: %w", err)
	}
	content, err := marshalChatInputContent(parsed)
	if err != nil {
		return nil, err
	}
	return []ResponsesInputItem{{Type: "message", Role: "user", Content: content}}, nil
}

// chatAssistantToResponses converts an assistant message. If there is both
// text content and tool_calls, the text is emitted as an assistant message
// first, then each tool_call becomes a function_call item. If the content is
// empty/nil and there are tool_calls, only function_call items are emitted.
func chatAssistantToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	var items []ResponsesInputItem
	content := ""

	if m.ReasoningContent != "" {
		content = "<thinking>" + m.ReasoningContent + "</thinking>"
	}

	// Emit assistant message with output_text if content is non-empty.
	if len(m.Content) > 0 {
		s, err := parseAssistantContent(m.Content)
		if err != nil {
			return nil, err
		}
		if s != "" {
			if content != "" {
				content += "\n"
			}
			content += s
		}
	}

	if content != "" {
		parts := []ResponsesContentPart{{Type: "output_text", Text: content}}
		partsJSON, err := json.Marshal(parts)
		if err != nil {
			return nil, err
		}
		items = append(items, ResponsesInputItem{Type: "message", Role: "assistant", Content: partsJSON})
	}

	// Emit one function_call item per tool_call.
	for _, tc := range m.ToolCalls {
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		items = append(items, ResponsesInputItem{
			Type:      "function_call",
			CallID:    tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return items, nil
}

// parseAssistantContent returns assistant content as plain text.
//
// Supported formats:
// - JSON string
// - JSON array of typed parts (e.g. [{"type":"text","text":"..."}])
//
// For structured thinking/reasoning parts, it preserves semantics by wrapping
// the text in explicit tags so downstream can still distinguish it from normal text.
func parseAssistantContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		// Keep compatibility with prior behavior: unsupported assistant content
		// formats are ignored instead of failing the whole request conversion.
		return "", nil
	}

	var b strings.Builder
	write := func(v string) error {
		_, err := b.WriteString(v)
		return err
	}
	for _, p := range parts {
		typ, _ := p["type"].(string)
		text, _ := p["text"].(string)
		thinking, _ := p["thinking"].(string)

		switch typ {
		case "thinking", "reasoning":
			if thinking != "" {
				if err := write("<thinking>"); err != nil {
					return "", err
				}
				if err := write(thinking); err != nil {
					return "", err
				}
				if err := write("</thinking>"); err != nil {
					return "", err
				}
			} else if text != "" {
				if err := write("<thinking>"); err != nil {
					return "", err
				}
				if err := write(text); err != nil {
					return "", err
				}
				if err := write("</thinking>"); err != nil {
					return "", err
				}
			}
		default:
			if text != "" {
				if err := write(text); err != nil {
					return "", err
				}
			}
		}
	}

	return b.String(), nil
}

// chatToolToResponses converts a tool result message (role=tool) into a
// function_call_output item.
func chatToolToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	output, err := parseChatContent(m.Content)
	if err != nil {
		return nil, err
	}
	if output == "" {
		output = "(empty)"
	}
	return []ResponsesInputItem{{
		Type:   "function_call_output",
		CallID: m.ToolCallID,
		Output: output,
	}}, nil
}

// chatFunctionToResponses converts a legacy function result message
// (role=function) into a function_call_output item. ToolCallID holds the ID
// assigned to the matching legacy call (see legacyFunctionCallIDs); without a
// matching call the Name field is used as call_id.
func chatFunctionToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	output, err := parseChatContent(m.Content)
	if err != nil {
		return nil, err
	}
	if output == "" {
		output = "(empty)"
	}
	callID := m.ToolCallID
	if callID == "" {
		callID = m.Name
	}
	return []ResponsesInputItem{{
		Type:   "function_call_output",
		CallID: callID,
		Output: output,
	}}, nil
}

// parseChatContent returns the string value of a ChatMessage Content field.
// Content can be a JSON string or an array of typed parts. Array content is
// flattened to text by concatenating text parts and ignoring non-text parts.
func parseChatContent(raw json.RawMessage) (string, error) {
	parsed, err := parseChatMessageContent(raw)
	if err != nil {
		return "", err
	}
	if parsed.Text != nil {
		return *parsed.Text, nil
	}
	return flattenChatContentParts(parsed.Parts), nil
}

func parseChatMessageContent(raw json.RawMessage) (chatMessageContent, error) {
	if len(raw) == 0 {
		return chatMessageContent{Text: stringPtr("")}, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return chatMessageContent{Text: &s}, nil
	}

	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		return chatMessageContent{Parts: parts}, nil
	}

	return chatMessageContent{}, fmt.Errorf("parse content as string or parts array")
}

func marshalChatInputContent(content chatMessageContent) (json.RawMessage, error) {
	if content.Text != nil {
		return json.Marshal(*content.Text)
	}
	parts := convertChatContentPartsToResponses(content.Parts)
	if len(parts) == 0 {
		// A nil slice marshals to JSON null, which the upstream Responses API
		// rejects ("expected an array of objects or string, but got null").
		// Fall back to an empty string when no usable parts remain.
		return json.Marshal("")
	}
	return json.Marshal(parts)
}

func convertChatContentPartsToResponses(parts []ChatContentPart) []ResponsesContentPart {
	var responseParts []ResponsesContentPart
	for _, p := range parts {
		switch p.Type {
		case "text":
			if p.Text != "" || len(p.PromptCacheBreakpoint) > 0 {
				responseParts = append(responseParts, ResponsesContentPart{
					PromptCacheBreakpoint: p.PromptCacheBreakpoint,
					Type:                  "input_text",
					Text:                  p.Text,
				})
			}
		case "image_url":
			if p.ImageURL != nil && p.ImageURL.URL != "" && !isEmptyBase64DataURI(p.ImageURL.URL) {
				responseParts = append(responseParts, ResponsesContentPart{
					PromptCacheBreakpoint: p.PromptCacheBreakpoint,
					Type:                  "input_image",
					ImageURL:              p.ImageURL.URL,
				})
			}
		case "file":
			if p.File != nil && (p.File.FileData != "" || p.File.FileID != "") {
				responseParts = append(responseParts, ResponsesContentPart{
					PromptCacheBreakpoint: p.PromptCacheBreakpoint,
					Type:                  "input_file",
					Filename:              p.File.Filename,
					FileData:              p.File.FileData,
					FileID:                p.File.FileID,
				})
			}
		}
	}
	return responseParts
}

func isEmptyBase64DataURI(raw string) bool {
	if !strings.HasPrefix(raw, "data:") {
		return false
	}
	rest := strings.TrimPrefix(raw, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return false
	}
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "base64,")) == ""
}

func flattenChatContentParts(parts []ChatContentPart) string {
	var textParts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			textParts = append(textParts, p.Text)
		}
	}
	return strings.Join(textParts, "")
}

func stringPtr(s string) *string {
	return &s
}

// convertChatToolsToResponses maps Chat Completions tool definitions and legacy
// function definitions to Responses API tool definitions.
func convertChatToolsToResponses(tools []ChatTool, functions []ChatFunction) []ResponsesTool {
	var out []ResponsesTool

	for _, t := range tools {
		toolType := strings.ToLower(strings.TrimSpace(t.Type))
		if toolType == "x_search" {
			out = append(out, ResponsesTool{
				Type:                     "x_search",
				AllowedXHandles:          t.AllowedXHandles,
				ExcludedXHandles:         t.ExcludedXHandles,
				FromDate:                 t.FromDate,
				ToDate:                   t.ToDate,
				EnableImageUnderstanding: t.EnableImageUnderstanding,
				EnableVideoUnderstanding: t.EnableVideoUnderstanding,
			})
			continue
		}
		if toolType == "web_search" || toolType == "code_execution" {
			out = append(out, ResponsesTool{Type: toolType})
			continue
		}
		if t.Type != "function" || t.Function == nil {
			continue
		}
		rt := ResponsesTool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
			Strict:      defaultStrictFalse(t.Function.Strict),
		}
		out = append(out, rt)
	}

	// Legacy functions[] are treated as function-type tools.
	for _, f := range functions {
		rt := ResponsesTool{
			Type:        "function",
			Name:        f.Name,
			Description: f.Description,
			Parameters:  f.Parameters,
			Strict:      defaultStrictFalse(f.Strict),
		}
		out = append(out, rt)
	}

	return out
}

func defaultStrictFalse(src *bool) *bool {
	if src == nil {
		value := false
		return &value
	}
	return src
}

// convertChatToolChoiceToResponses maps a Chat Completions tool_choice to the
// Responses API shape.
//
//	{"type":"function","function":{"name":"X"}} → {"type":"function","name":"X"}
//
// Strings ("auto", "none", "required") and objects that already use the
// Responses shape are returned unchanged.
func convertChatToolChoiceToResponses(raw json.RawMessage) json.RawMessage {
	var choice struct {
		Type     string `json:"type"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &choice); err != nil || choice.Type != "function" || choice.Function == nil {
		return raw
	}
	name := strings.TrimSpace(choice.Function.Name)
	if name == "" {
		return raw
	}
	flat, err := json.Marshal(map[string]string{"type": "function", "name": name})
	if err != nil {
		return raw
	}
	return flat
}

// convertChatFunctionCallToToolChoice maps the legacy function_call field to a
// Responses API tool_choice value.
//
//	"auto" → "auto"
//	"none" → "none"
//	{"name":"X"} → {"type":"function","name":"X"}
func convertChatFunctionCallToToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try string first ("auto", "none", etc.) — pass through as-is.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	// Object form: {"name":"X"}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"type": "function",
		"name": obj.Name,
	})
}
