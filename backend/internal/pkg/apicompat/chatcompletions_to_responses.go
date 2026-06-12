package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type chatMessageContent struct {
	Text  *string
	Parts []ChatContentPart
REDACTED

// ChatCompletionsToResponses converts a Chat Completions request into a
// Responses API request. The upstream always streams, so Stream is forced to
// true. store is always false and reasoning.encrypted_content is always
// included so that the response translator has full context.
func ChatCompletionsToResponses(req *ChatCompletionsRequest) (*ResponsesRequest, error) {
	input, err := convertChatMessagesToResponsesInput(req.Messages)
	if err != nil {
		return nil, err
REDACTED

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
REDACTED

	out := &ResponsesRequest{
		Model:        req.Model,
		Instructions: req.Instructions,
		Input:        inputJSON,
		Stream:       true, // upstream always streams
		Include:      []string{"reasoning.encrypted_content"REDACTED,
		ServiceTier:  req.ServiceTier,
REDACTED

	// Reasoning models (gpt-5.x) do not accept sampling parameters.
	// See isReasoningModel in anthropic_to_responses.go.
	if !isReasoningModel(req.Model) {
		out.Temperature = req.Temperature
		out.TopP = req.TopP
REDACTED

	storeFalse := false
	out.Store = &storeFalse

	// max_tokens / max_completion_tokens → max_output_tokens, prefer max_completion_tokens
	maxTokens := 0
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
REDACTED
	if req.MaxCompletionTokens != nil {
		maxTokens = *req.MaxCompletionTokens
REDACTED
	if maxTokens > 0 {
		v := maxTokens
		if v < minMaxOutputTokens {
			v = minMaxOutputTokens
	REDACTED
		out.MaxOutputTokens = &v
REDACTED

	// reasoning_effort → reasoning.effort + reasoning.summary="auto"
	if req.ReasoningEffort != "" {
		out.Reasoning = &ResponsesReasoning{
			Effort:  req.ReasoningEffort,
			Summary: "auto",
	REDACTED
REDACTED

	// tools[] and legacy functions[] → ResponsesTool[]
	if len(req.Tools) > 0 || len(req.Functions) > 0 {
		out.Tools = convertChatToolsToResponses(req.Tools, req.Functions)
REDACTED

	// tool_choice: already compatible format — pass through directly.
	// Legacy function_call needs mapping.
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = req.ToolChoice
REDACTED else if len(req.FunctionCall) > 0 {
		tc, err := convertChatFunctionCallToToolChoice(req.FunctionCall)
		if err != nil {
			return nil, fmt.Errorf("convert function_call: %w", err)
	REDACTED
		out.ToolChoice = tc
REDACTED

	return out, nil
REDACTED

// convertChatMessagesToResponsesInput converts the Chat Completions messages
// array into a Responses API input items array.
func convertChatMessagesToResponsesInput(msgs []ChatMessage) ([]ResponsesInputItem, error) {
	var out []ResponsesInputItem
	for _, m := range msgs {
		items, err := chatMessageToResponsesItems(m)
		if err != nil {
			return nil, err
	REDACTED
		out = append(out, items...)
REDACTED
	return out, nil
REDACTED

// chatMessageToResponsesItems converts a single ChatMessage into one or more
// ResponsesInputItem values.
func chatMessageToResponsesItems(m ChatMessage) ([]ResponsesInputItem, error) {
	switch m.Role {
	case "system":
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
REDACTED
REDACTED

// chatSystemToResponses converts a system message.
func chatSystemToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	parsed, err := parseChatMessageContent(m.Content)
	if err != nil {
		return nil, err
REDACTED
	content, err := marshalChatInputContent(parsed)
	if err != nil {
		return nil, err
REDACTED
	return []ResponsesInputItem{{Role: "system", Content: contentREDACTEDREDACTED, nil
REDACTED

// chatUserToResponses converts a user message, handling both plain strings and
// multi-modal content arrays.
func chatUserToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	parsed, err := parseChatMessageContent(m.Content)
	if err != nil {
		return nil, fmt.Errorf("parse user content: %w", err)
REDACTED
	content, err := marshalChatInputContent(parsed)
	if err != nil {
		return nil, err
REDACTED
	return []ResponsesInputItem{{Role: "user", Content: contentREDACTEDREDACTED, nil
REDACTED

// chatAssistantToResponses converts an assistant message. If there is both
// text content and tool_calls, the text is emitted as an assistant message
// first, then each tool_call becomes a function_call item. If the content is
// empty/nil and there are tool_calls, only function_call items are emitted.
func chatAssistantToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	var items []ResponsesInputItem
	content := ""

	if m.ReasoningContent != "" {
		content = "<thinking>" + m.ReasoningContent + "</thinking>"
REDACTED

	// Emit assistant message with output_text if content is non-empty.
	if len(m.Content) > 0 {
		s, err := parseAssistantContent(m.Content)
		if err != nil {
			return nil, err
	REDACTED
		if s != "" {
			if content != "" {
				content += "\n"
		REDACTED
			content += s
	REDACTED
REDACTED

	if content != "" {
		parts := []ResponsesContentPart{{Type: "output_text", Text: contentREDACTEDREDACTED
		partsJSON, err := json.Marshal(parts)
		if err != nil {
			return nil, err
	REDACTED
		items = append(items, ResponsesInputItem{Role: "assistant", Content: partsJSONREDACTED)
REDACTED

	// Emit one function_call item per tool_call.
	for _, tc := range m.ToolCalls {
		args := tc.Function.Arguments
		if args == "" {
			args = "{REDACTED"
	REDACTED
		items = append(items, ResponsesInputItem{
			Type:      "function_call",
			CallID:    tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
	REDACTED)
REDACTED

	return items, nil
REDACTED

// parseAssistantContent returns assistant content as plain text.
//
// Supported formats:
// - JSON string
// - JSON array of typed parts (e.g. [{"type":"text","text":"..."REDACTED])
//
// For structured thinking/reasoning parts, it preserves semantics by wrapping
// the text in explicit tags so downstream can still distinguish it from normal text.
func parseAssistantContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
REDACTED

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
REDACTED

	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		// Keep compatibility with prior behavior: unsupported assistant content
		// formats are ignored instead of failing the whole request conversion.
		return "", nil
REDACTED

	var b strings.Builder
	write := func(v string) error {
		_, err := b.WriteString(v)
		return err
REDACTED
	for _, p := range parts {
		typ, _ := p["type"].(string)
		text, _ := p["text"].(string)
		thinking, _ := p["thinking"].(string)

		switch typ {
		case "thinking", "reasoning":
			if thinking != "" {
				if err := write("<thinking>"); err != nil {
					return "", err
			REDACTED
				if err := write(thinking); err != nil {
					return "", err
			REDACTED
				if err := write("</thinking>"); err != nil {
					return "", err
			REDACTED
		REDACTED else if text != "" {
				if err := write("<thinking>"); err != nil {
					return "", err
			REDACTED
				if err := write(text); err != nil {
					return "", err
			REDACTED
				if err := write("</thinking>"); err != nil {
					return "", err
			REDACTED
		REDACTED
		default:
			if text != "" {
				if err := write(text); err != nil {
					return "", err
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return b.String(), nil
REDACTED

// chatToolToResponses converts a tool result message (role=tool) into a
// function_call_output item.
func chatToolToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	output, err := parseChatContent(m.Content)
	if err != nil {
		return nil, err
REDACTED
	if output == "" {
		output = "(empty)"
REDACTED
	return []ResponsesInputItem{{
		Type:   "function_call_output",
		CallID: m.ToolCallID,
		Output: output,
REDACTEDREDACTED, nil
REDACTED

// chatFunctionToResponses converts a legacy function result message
// (role=function) into a function_call_output item. The Name field is used as
// call_id since legacy function calls do not carry a separate call_id.
func chatFunctionToResponses(m ChatMessage) ([]ResponsesInputItem, error) {
	output, err := parseChatContent(m.Content)
	if err != nil {
		return nil, err
REDACTED
	if output == "" {
		output = "(empty)"
REDACTED
	return []ResponsesInputItem{{
		Type:   "function_call_output",
		CallID: m.Name,
		Output: output,
REDACTEDREDACTED, nil
REDACTED

// parseChatContent returns the string value of a ChatMessage Content field.
// Content can be a JSON string or an array of typed parts. Array content is
// flattened to text by concatenating text parts and ignoring non-text parts.
func parseChatContent(raw json.RawMessage) (string, error) {
	parsed, err := parseChatMessageContent(raw)
	if err != nil {
		return "", err
REDACTED
	if parsed.Text != nil {
		return *parsed.Text, nil
REDACTED
	return flattenChatContentParts(parsed.Parts), nil
REDACTED

func parseChatMessageContent(raw json.RawMessage) (chatMessageContent, error) {
	if len(raw) == 0 {
		return chatMessageContent{Text: stringPtr("")REDACTED, nil
REDACTED

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return chatMessageContent{Text: &sREDACTED, nil
REDACTED

	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		return chatMessageContent{Parts: partsREDACTED, nil
REDACTED

	return chatMessageContent{REDACTED, fmt.Errorf("parse content as string or parts array")
REDACTED

func marshalChatInputContent(content chatMessageContent) (json.RawMessage, error) {
	if content.Text != nil {
		return json.Marshal(*content.Text)
REDACTED
	parts := convertChatContentPartsToResponses(content.Parts)
	if len(parts) == 0 {
		// A nil slice marshals to JSON null, which the upstream Responses API
		// rejects ("expected an array of objects or string, but got null").
		// Fall back to an empty string when no usable parts remain.
		return json.Marshal("")
REDACTED
	return json.Marshal(parts)
REDACTED

func convertChatContentPartsToResponses(parts []ChatContentPart) []ResponsesContentPart {
	var responseParts []ResponsesContentPart
	for _, p := range parts {
		switch p.Type {
		case "text":
			if p.Text != "" {
				responseParts = append(responseParts, ResponsesContentPart{
					Type: "input_text",
					Text: p.Text,
			REDACTED)
		REDACTED
		case "image_url":
			if p.ImageURL != nil && p.ImageURL.URL != "" && !isEmptyBase64DataURI(p.ImageURL.URL) {
				responseParts = append(responseParts, ResponsesContentPart{
					Type:     "input_image",
					ImageURL: p.ImageURL.URL,
			REDACTED)
		REDACTED
	REDACTED
REDACTED
	return responseParts
REDACTED

func isEmptyBase64DataURI(raw string) bool {
	if !strings.HasPrefix(raw, "data:") {
		return false
REDACTED
	rest := strings.TrimPrefix(raw, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return false
REDACTED
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return false
REDACTED
	return strings.TrimSpace(strings.TrimPrefix(rest, "base64,")) == ""
REDACTED

func flattenChatContentParts(parts []ChatContentPart) string {
	var textParts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			textParts = append(textParts, p.Text)
	REDACTED
REDACTED
	return strings.Join(textParts, "")
REDACTED

func stringPtr(s string) *string {
	return &s
REDACTED

// convertChatToolsToResponses maps Chat Completions tool definitions and legacy
// function definitions to Responses API tool definitions.
func convertChatToolsToResponses(tools []ChatTool, functions []ChatFunction) []ResponsesTool {
	var out []ResponsesTool

	for _, t := range tools {
		if t.Type != "function" || t.Function == nil {
			continue
	REDACTED
		rt := ResponsesTool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
			Strict:      defaultStrictFalse(t.Function.Strict),
	REDACTED
		out = append(out, rt)
REDACTED

	// Legacy functions[] are treated as function-type tools.
	for _, f := range functions {
		rt := ResponsesTool{
			Type:        "function",
			Name:        f.Name,
			Description: f.Description,
			Parameters:  f.Parameters,
			Strict:      defaultStrictFalse(f.Strict),
	REDACTED
		out = append(out, rt)
REDACTED

	return out
REDACTED

func defaultStrictFalse(src *bool) *bool {
	if src == nil {
		value := false
		return &value
REDACTED
	return src
REDACTED

// convertChatFunctionCallToToolChoice maps the legacy function_call field to a
// Responses API tool_choice value.
//
//	"auto" → "auto"
//	"none" → "none"
//	{"name":"X"REDACTED → {"type":"function","name":"X"REDACTED
func convertChatFunctionCallToToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try string first ("auto", "none", etc.) — pass through as-is.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
REDACTED

	// Object form: {"name":"X"REDACTED
	var obj struct {
		Name string `json:"name"`
REDACTED
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
REDACTED
	return json.Marshal(map[string]any{
		"type": "function",
		"name": obj.Name,
REDACTED)
REDACTED
