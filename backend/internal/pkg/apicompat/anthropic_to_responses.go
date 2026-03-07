package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnthropicToResponses converts an Anthropic Messages request directly into
// a Responses API request. This preserves fields that would be lost in a
// Chat Completions intermediary round-trip (e.g. thinking, cache_control,
// structured system prompts).
func AnthropicToResponses(req *AnthropicRequest) (*ResponsesRequest, error) {
	input, err := convertAnthropicToResponsesInput(req.System, req.Messages)
	if err != nil {
		return nil, err
REDACTED

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
REDACTED

	out := &ResponsesRequest{
		Model:       req.Model,
		Input:       inputJSON,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Include:     []string{"reasoning.encrypted_content"REDACTED,
REDACTED

	storeFalse := false
	out.Store = &storeFalse

	if req.MaxTokens > 0 {
		v := req.MaxTokens
		if v < minMaxOutputTokens {
			v = minMaxOutputTokens
	REDACTED
		out.MaxOutputTokens = &v
REDACTED

	if len(req.Tools) > 0 {
		out.Tools = convertAnthropicToolsToResponses(req.Tools)
REDACTED

	// Convert thinking → reasoning.
	// generate_summary="auto" causes the upstream to emit reasoning_summary_text
	// streaming events; the include array only needs reasoning.encrypted_content
	// (already set above) for content continuity.
	if req.Thinking != nil {
		switch req.Thinking.Type {
		case "enabled":
			out.Reasoning = &ResponsesReasoning{Effort: "high", Summary: "auto"REDACTED
		case "adaptive":
			out.Reasoning = &ResponsesReasoning{Effort: "medium", Summary: "auto"REDACTED
	REDACTED
		// "disabled" or unknown → omit reasoning
REDACTED

	// Convert tool_choice
	if len(req.ToolChoice) > 0 {
		tc, err := convertAnthropicToolChoiceToResponses(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
	REDACTED
		out.ToolChoice = tc
REDACTED

	return out, nil
REDACTED

// convertAnthropicToolChoiceToResponses maps Anthropic tool_choice to Responses format.
//
//	{"type":"auto"REDACTED            → "auto"
//	{"type":"any"REDACTED             → "required"
//	{"type":"none"REDACTED            → "none"
//	{"type":"tool","name":"X"REDACTED → {"type":"function","function":{"name":"X"REDACTEDREDACTED
func convertAnthropicToolChoiceToResponses(raw json.RawMessage) (json.RawMessage, error) {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
REDACTED
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
REDACTED

	switch tc.Type {
	case "auto":
		return json.Marshal("auto")
	case "any":
		return json.Marshal("required")
	case "none":
		return json.Marshal("none")
	case "tool":
		return json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]string{"name": tc.NameREDACTED,
	REDACTED)
	default:
		// Pass through unknown types as-is
		return raw, nil
REDACTED
REDACTED

// convertAnthropicToResponsesInput builds the Responses API input items array
// from the Anthropic system field and message list.
func convertAnthropicToResponsesInput(system json.RawMessage, msgs []AnthropicMessage) ([]ResponsesInputItem, error) {
	var out []ResponsesInputItem

	// System prompt → system role input item.
	if len(system) > 0 {
		sysText, err := parseAnthropicSystemPrompt(system)
		if err != nil {
			return nil, err
	REDACTED
		if sysText != "" {
			content, _ := json.Marshal(sysText)
			out = append(out, ResponsesInputItem{
				Role:    "system",
				Content: content,
		REDACTED)
	REDACTED
REDACTED

	for _, m := range msgs {
		items, err := anthropicMsgToResponsesItems(m)
		if err != nil {
			return nil, err
	REDACTED
		out = append(out, items...)
REDACTED
	return out, nil
REDACTED

// parseAnthropicSystemPrompt handles the Anthropic system field which can be
// a plain string or an array of text blocks.
func parseAnthropicSystemPrompt(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
REDACTED
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", err
REDACTED
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
	REDACTED
REDACTED
	return strings.Join(parts, "\n\n"), nil
REDACTED

// anthropicMsgToResponsesItems converts a single Anthropic message into one
// or more Responses API input items.
func anthropicMsgToResponsesItems(m AnthropicMessage) ([]ResponsesInputItem, error) {
	switch m.Role {
	case "user":
		return anthropicUserToResponses(m.Content)
	case "assistant":
		return anthropicAssistantToResponses(m.Content)
	default:
		return anthropicUserToResponses(m.Content)
REDACTED
REDACTED

// anthropicUserToResponses handles an Anthropic user message. Content can be a
// plain string or an array of blocks. tool_result blocks are extracted into
// function_call_output items.
func anthropicUserToResponses(raw json.RawMessage) ([]ResponsesInputItem, error) {
	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, _ := json.Marshal(s)
		return []ResponsesInputItem{{Role: "user", Content: contentREDACTEDREDACTED, nil
REDACTED

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
REDACTED

	var out []ResponsesInputItem

	// Extract tool_result blocks → function_call_output items.
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
	REDACTED
		text := extractAnthropicToolResultText(b)
		if text == "" {
			// OpenAI Responses API requires "output" field; use placeholder for empty results.
			text = "(empty)"
	REDACTED
		out = append(out, ResponsesInputItem{
			Type:   "function_call_output",
			CallID: toResponsesCallID(b.ToolUseID),
			Output: text,
	REDACTED)
REDACTED

	// Remaining text blocks → user message.
	text := extractAnthropicTextFromBlocks(blocks)
	if text != "" {
		content, _ := json.Marshal(text)
		out = append(out, ResponsesInputItem{Role: "user", Content: contentREDACTED)
REDACTED

	return out, nil
REDACTED

// anthropicAssistantToResponses handles an Anthropic assistant message.
// Text content → assistant message with output_text parts.
// tool_use blocks → function_call items.
// thinking blocks → ignored (OpenAI doesn't accept them as input).
func anthropicAssistantToResponses(raw json.RawMessage) ([]ResponsesInputItem, error) {
	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parts := []ResponsesContentPart{{Type: "output_text", Text: sREDACTEDREDACTED
		partsJSON, err := json.Marshal(parts)
		if err != nil {
			return nil, err
	REDACTED
		return []ResponsesInputItem{{Role: "assistant", Content: partsJSONREDACTEDREDACTED, nil
REDACTED

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
REDACTED

	var items []ResponsesInputItem

	// Text content → assistant message with output_text content parts.
	text := extractAnthropicTextFromBlocks(blocks)
	if text != "" {
		parts := []ResponsesContentPart{{Type: "output_text", Text: textREDACTEDREDACTED
		partsJSON, err := json.Marshal(parts)
		if err != nil {
			return nil, err
	REDACTED
		items = append(items, ResponsesInputItem{Role: "assistant", Content: partsJSONREDACTED)
REDACTED

	// tool_use → function_call items.
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
	REDACTED
		args := "{REDACTED"
		if len(b.Input) > 0 {
			args = string(b.Input)
	REDACTED
		fcID := toResponsesCallID(b.ID)
		items = append(items, ResponsesInputItem{
			Type:      "function_call",
			CallID:    fcID,
			Name:      b.Name,
			Arguments: args,
			ID:        fcID,
	REDACTED)
REDACTED

	return items, nil
REDACTED

// toResponsesCallID converts an Anthropic tool ID (toolu_xxx / call_xxx) to a
// Responses API function_call ID that starts with "fc_".
func toResponsesCallID(id string) string {
	if strings.HasPrefix(id, "fc_") {
		return id
REDACTED
	return "fc_" + id
REDACTED

// fromResponsesCallID reverses toResponsesCallID, stripping the "fc_" prefix
// that was added during request conversion.
func fromResponsesCallID(id string) string {
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		// Only strip if the remainder doesn't look like it was already "fc_" prefixed.
		// E.g. "fc_toolu_xxx" → "toolu_xxx", "fc_call_xxx" → "call_xxx"
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
	REDACTED
REDACTED
	return id
REDACTED

// extractAnthropicToolResultText gets the text content from a tool_result block.
func extractAnthropicToolResultText(b AnthropicContentBlock) string {
	if len(b.Content) == 0 {
		return ""
REDACTED
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return s
REDACTED
	var inner []AnthropicContentBlock
	if err := json.Unmarshal(b.Content, &inner); err == nil {
		var parts []string
		for _, ib := range inner {
			if ib.Type == "text" && ib.Text != "" {
				parts = append(parts, ib.Text)
		REDACTED
	REDACTED
		return strings.Join(parts, "\n\n")
REDACTED
	return ""
REDACTED

// extractAnthropicTextFromBlocks joins all text blocks, ignoring thinking/
// tool_use/tool_result blocks.
func extractAnthropicTextFromBlocks(blocks []AnthropicContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
	REDACTED
REDACTED
	return strings.Join(parts, "\n\n")
REDACTED

// convertAnthropicToolsToResponses maps Anthropic tool definitions to
// Responses API tools. Server-side tools like web_search are mapped to their
// OpenAI equivalents; regular tools become function tools.
func convertAnthropicToolsToResponses(tools []AnthropicTool) []ResponsesTool {
	var out []ResponsesTool
	for _, t := range tools {
		// Anthropic server tools like "web_search_20250305" → OpenAI {"type":"web_search"REDACTED
		if strings.HasPrefix(t.Type, "web_search") {
			out = append(out, ResponsesTool{Type: "web_search"REDACTED)
			continue
	REDACTED
		out = append(out, ResponsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
	REDACTED)
REDACTED
	return out
REDACTED
