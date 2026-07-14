package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// This file implements a DIRECT bridge between Anthropic Messages and OpenAI
// Chat Completions, skipping the Responses API intermediate representation.
//
// The existing chat-fallback path (forwardAnthropicViaRawChatCompletions) chains
// two Responses-anchored bridges — Anthropic→Responses→ChatCompletions on the
// request side and CC→Responses→Anthropic on the response side — so every
// streaming token runs through two state machines. For force-chat accounts
// (third-party OpenAI-compatible upstreams that only speak /v1/chat/completions)
// the Responses layer is pure overhead: these upstreams never see Responses
// semantics, and the clients reaching them via /v1/messages use standard
// function tools (no custom/tool_search/namespace Codex constructs).
//
// The direct bridge collapses both directions into a single conversion each:
//
//	Request:  Anthropic Messages → Chat Completions
//	Response: CC chunk/response → Anthropic events/response
//
// Helper functions from the Responses bridges (anthropicImageToDataURI,
// extractAnthropicTextFromBlocks, fromResponsesCallID, sanitizeAnthropicToolUseInput,
// parseAnthropicSystemContentParts, isReasoningModel, mapAnthropicEffortToResponses,
// normalizeToolParameters) are reused so the conversion semantics stay identical.

// ---------------------------------------------------------------------------
// Request: AnthropicRequest → ChatCompletionsRequest
// ---------------------------------------------------------------------------

// AnthropicToChatCompletionsRequest converts an Anthropic Messages request
// directly into a Chat Completions request, without transiting the Responses
// API. It is semantically equivalent to composing AnthropicToResponses +
// ResponsesToChatCompletionsRequest but avoids materializing the intermediate
// ResponsesRequest and the extra marshal/unmarshal cycle.
func AnthropicToChatCompletionsRequest(req *AnthropicRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("anthropic request is nil")
REDACTED

	messages, err := anthropicToChatMessages(req.System, req.Messages)
	if err != nil {
		return nil, err
REDACTED

	out := &ChatCompletionsRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   req.Stream,
REDACTED

	// Sampling params: reasoning models (gpt-5.x) reject temperature/top_p.
	if !isReasoningModel(req.Model) {
		out.Temperature = req.Temperature
		out.TopP = req.TopP
REDACTED

	if req.MaxTokens > 0 {
		v := req.MaxTokens
		if v < minMaxOutputTokens {
			v = minMaxOutputTokens
	REDACTED
		out.MaxCompletionTokens = &v
REDACTED

	// Tools: Anthropic input_schema is a JSON Schema, directly usable as Chat
	// function parameters. Server tools (web_search_*) have no Chat Completions
	// equivalent and are dropped (mirrors responsesToolsToChatTools).
	if len(req.Tools) > 0 {
		tools := anthropicToolsToChatTools(req.Tools)
		if len(tools) > 0 {
			out.Tools = tools
	REDACTED
REDACTED

	// tool_choice is only forwarded when tools survived the conversion
	// (upstream rejects tool_choice without tools).
	if len(out.Tools) > 0 && len(req.ToolChoice) > 0 {
		tc, err := convertAnthropicToolChoiceToChat(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
	REDACTED
		out.ToolChoice = tc
REDACTED

	// Reasoning effort: output_config.effort maps 1:1 (max→xhigh). thinking.type
	// itself is ignored (the Responses bridge behaves identically).
	effort := "medium"
	if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
		effort = req.OutputConfig.Effort
REDACTED
	out.ReasoningEffort = mapAnthropicEffortToResponses(effort)

	parallelToolCalls := true
	out.ParallelToolCalls = &parallelToolCalls

	return out, nil
REDACTED

// anthropicToChatMessages converts the Anthropic system field + message list
// into Chat Completions messages. It mirrors convertAnthropicToResponsesInput +
// responsesInputToChatMessages but produces ChatMessage directly.
func anthropicToChatMessages(system json.RawMessage, msgs []AnthropicMessage) ([]ChatMessage, error) {
	var messages []ChatMessage

	// System prompt → system message. parseAnthropicSystemContentParts handles
	// both string and []block forms and filters the billing header.
	if len(system) > 0 {
		sysParts, err := parseAnthropicSystemContentParts(system)
		if err != nil {
			return nil, err
	REDACTED
		if len(sysParts) > 0 {
			text := joinResponsesContentPartText(sysParts)
			if text != "" {
				content, _ := json.Marshal(text)
				messages = append(messages, ChatMessage{Role: "system", Content: contentREDACTED)
		REDACTED
	REDACTED
REDACTED

	for _, m := range msgs {
		converted, err := anthropicMsgToChatMessages(m)
		if err != nil {
			return nil, err
	REDACTED
		messages = append(messages, converted...)
REDACTED

	return normalizeChatMessages(messages), nil
REDACTED

// anthropicMsgToChatMessages converts one Anthropic message into one or more
// Chat messages. tool_result blocks become standalone "tool" role messages
// (the Chat Completions convention); text/image blocks stay in a user message;
// assistant tool_use blocks become tool_calls on the assistant message.
func anthropicMsgToChatMessages(m AnthropicMessage) ([]ChatMessage, error) {
	switch m.Role {
	case "assistant":
		return anthropicAssistantToChatMessages(m.Content)
	default: // "user" and any unknown role
		return anthropicUserToChatMessages(m.Content)
REDACTED
REDACTED

// anthropicUserToChatMessages handles an Anthropic user message. Content may be
// a plain string or an array of blocks. tool_result blocks are extracted into
// standalone "tool" role messages; images inside tool_results are lifted into a
// follow-up user message as image_url parts (the Responses bridge does the same
// — function_call_output only accepts strings, so images must travel separately).
func anthropicUserToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	// Plain string → single user message.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, _ := json.Marshal(s)
		return []ChatMessage{{Role: "user", Content: contentREDACTEDREDACTED, nil
REDACTED

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
REDACTED

	var out []ChatMessage
	var toolResultImageParts []ChatContentPart

	// tool_result → "tool" role messages, text extracted; images deferred.
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
	REDACTED
		text, imageParts := convertToolResultOutput(b)
		content, _ := json.Marshal(text)
		out = append(out, ChatMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: b.ToolUseID,
	REDACTED)
		for _, ip := range imageParts {
			toolResultImageParts = append(toolResultImageParts, ChatContentPart{
				Type:     "image_url",
				ImageURL: &ChatImageURL{URL: ip.ImageURLREDACTED,
		REDACTED)
	REDACTED
REDACTED

	// Remaining text + image blocks → user message with content parts.
	var parts []ChatContentPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, ChatContentPart{Type: "text", Text: b.TextREDACTED)
		REDACTED
		case "image":
			if uri := anthropicImageToDataURI(b.Source); uri != "" {
				parts = append(parts, ChatContentPart{
					Type:     "image_url",
					ImageURL: &ChatImageURL{URL: uriREDACTED,
			REDACTED)
		REDACTED
	REDACTED
REDACTED
	parts = append(parts, toolResultImageParts...)

	if len(parts) > 0 {
		// Mixed/structured content → array form; single text → string form
		// (normalizeChatMessages will collapse a single-text-part array to a
		// plain string if the upstream prefers it).
		content, err := json.Marshal(parts)
		if err != nil {
			return nil, err
	REDACTED
		out = append(out, ChatMessage{Role: "user", Content: contentREDACTED)
REDACTED

	return out, nil
REDACTED

// anthropicAssistantToChatMessages handles an Anthropic assistant message.
// Text content → assistant message content; tool_use blocks → tool_calls on the
// same assistant message; thinking blocks are dropped (Chat Completions has no
// inbound thinking field, matching anthropicAssistantToResponses).
func anthropicAssistantToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	// Plain string → single assistant message.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, _ := json.Marshal(s)
		return []ChatMessage{{Role: "assistant", Content: contentREDACTEDREDACTED, nil
REDACTED

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
REDACTED

	msg := ChatMessage{Role: "assistant"REDACTED
	text := extractAnthropicTextFromBlocks(blocks)
	if text != "" {
		content, _ := json.Marshal(text)
		msg.Content = content
REDACTED

	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
	REDACTED
		args := "{REDACTED"
		if len(b.Input) > 0 {
			args = string(b.Input)
	REDACTED
		msg.ToolCalls = append(msg.ToolCalls, ChatToolCall{
			ID:   b.ID,
			Type: "function",
			Function: ChatFunctionCall{
				Name:      b.Name,
				Arguments: args,
		REDACTED,
	REDACTED)
REDACTED

	return []ChatMessage{msgREDACTED, nil
REDACTED

// anthropicToolsToChatTools maps Anthropic tool definitions to Chat Completions
// function tools. Server-side tools (web_search_*) are dropped — they have no
// Chat Completions equivalent.
func anthropicToolsToChatTools(tools []AnthropicTool) []ChatTool {
	var out []ChatTool
	for _, t := range tools {
		if strings.HasPrefix(t.Type, "web_search") {
			continue
	REDACTED
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeToolParameters(t.InputSchema),
				Strict:      boolPtr(false),
		REDACTED,
	REDACTED)
REDACTED
	return out
REDACTED

// convertAnthropicToolChoiceToChat maps Anthropic tool_choice to Chat
// Completions tool_choice.
//
//	{"type":"auto"REDACTED            → "auto"
//	{"type":"any"REDACTED             → "required"
//	{"type":"none"REDACTED            → "none"
//	{"type":"tool","name":"X"REDACTED → {"type":"function","function":{"name":"X"REDACTEDREDACTED
func convertAnthropicToolChoiceToChat(raw json.RawMessage) (json.RawMessage, error) {
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
		return raw, nil
REDACTED
REDACTED

// joinResponsesContentPartText concatenates the text of input_text parts. Used
// for the system prompt where parseAnthropicSystemContentParts returns
// ResponsesContentPart values.
func joinResponsesContentPartText(parts []ResponsesContentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == "input_text" && p.Text != "" {
			texts = append(texts, p.Text)
	REDACTED
REDACTED
	return strings.Join(texts, "\n\n")
REDACTED

// ---------------------------------------------------------------------------
// Non-streaming response: ChatCompletionsResponse → AnthropicResponse
// ---------------------------------------------------------------------------

// ChatCompletionsResponseToAnthropic converts a Chat Completions response
// directly into an Anthropic Messages response, without materializing a
// ResponsesResponse. It is semantically equivalent to composing
// ChatCompletionsResponseToResponses + ResponsesToAnthropic.
func ChatCompletionsResponseToAnthropic(resp *ChatCompletionsResponse, model string) *AnthropicResponse {
	out := &AnthropicResponse{
		Type:  "message",
		Role:  "assistant",
		Model: model,
REDACTED

	if resp != nil {
		if out.ID == "" {
			out.ID = resp.ID
	REDACTED
		if out.Model == "" {
			out.Model = resp.Model
	REDACTED

		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			out.Content = chatMessageToAnthropicBlocks(choice.Message)
			out.StopReason = chatFinishReasonToAnthropicStopReason(choice.FinishReason, out.Content)
			// "length" → "max_tokens" is handled by chatFinishReasonToAnthropicStopReason;
			// Anthropic conveys max-tokens via stop_reason only, no incomplete_details field.
	REDACTED
		if resp.Usage != nil {
			out.Usage = chatUsageToAnthropicUsage(resp.Usage)
	REDACTED
REDACTED

	if len(out.Content) == 0 {
		out.Content = []AnthropicContentBlock{{Type: "text", Text: ""REDACTEDREDACTED
REDACTED

	return out
REDACTED

// chatMessageToAnthropicBlocks converts a Chat Completions message into
// Anthropic content blocks. Reasoning content → thinking block; text content →
// text block; tool_calls → tool_use blocks. Mirrors chatMessageToResponsesOutput
// + the reasoning→thinking mapping in ResponsesToAnthropic.
func chatMessageToAnthropicBlocks(message ChatMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock

	if message.ReasoningContent != "" {
		blocks = append(blocks, AnthropicContentBlock{
			Type:     "thinking",
			Thinking: message.ReasoningContent,
	REDACTED)
REDACTED

	text := chatMessageContentText(message.Content)
	// DeepSeek reasoning-only fallback: when there is no text and no tool calls,
	// surface the reasoning content as visible text so the turn isn't empty.
	if text == "" && strings.TrimSpace(message.ReasoningContent) != "" && len(message.ToolCalls) == 0 {
		text = message.ReasoningContent
REDACTED
	if text != "" || len(message.ToolCalls) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: textREDACTED)
REDACTED

	for _, toolCall := range message.ToolCalls {
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{REDACTED"
	REDACTED
		blocks = append(blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    fromResponsesCallID(toolCall.ID),
			Name:  toolCall.Function.Name,
			Input: sanitizeAnthropicToolUseInput(toolCall.Function.Name, arguments),
	REDACTED)
REDACTED

	return blocks
REDACTED

// chatFinishReasonToAnthropicStopReason maps Chat Completions finish_reason to
// Anthropic stop_reason.
//
//	"stop"           → "end_turn" (or "tool_use" if tool_use blocks present)
//	"length"         → "max_tokens"
//	"tool_calls"     → "tool_use"
//	"content_filter" → "end_turn"
func chatFinishReasonToAnthropicStopReason(reason string, blocks []AnthropicContentBlock) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "stop":
		if containsAnthropicToolUseBlock(blocks) {
			return "tool_use"
	REDACTED
		return "end_turn"
	default:
		return "end_turn"
REDACTED
REDACTED

// chatUsageToAnthropicUsage converts Chat Completions token usage to Anthropic
// usage shape. Mirrors ChatUsageToResponsesUsage + anthropicUsageFromResponsesUsage.
func chatUsageToAnthropicUsage(usage *ChatUsage) AnthropicUsage {
	if usage == nil {
		return AnthropicUsage{REDACTED
REDACTED

	cachedTokens := 0
	cacheCreationTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails.CachedTokens
		cacheCreationTokens = usage.PromptTokensDetails.CacheCreationTokens +
			usage.PromptTokensDetails.CacheWriteTokens
REDACTED

	inputTokens := usage.PromptTokens - cachedTokens - cacheCreationTokens
	if inputTokens < 0 {
		inputTokens = 0
REDACTED

	return AnthropicUsage{
		InputTokens:              inputTokens,
		OutputTokens:             usage.CompletionTokens,
		CacheReadInputTokens:     cachedTokens,
		CacheCreationInputTokens: cacheCreationTokens,
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// Streaming: ChatCompletionsChunk → []AnthropicStreamEvent (stateful converter)
// ---------------------------------------------------------------------------

// ChatCompletionsToAnthropicStreamState tracks state while converting Chat
// Completions SSE chunks directly into Anthropic SSE events. It collapses the
// ChatCompletionsToResponsesStreamState + ResponsesEventToAnthropicState pair
// into one state machine.
type ChatCompletionsToAnthropicStreamState struct {
	MessageStartSent bool
	MessageStopSent  bool

	// Current content block lifecycle.
	ContentBlockIndex   int
	ContentBlockOpen    bool
	CurrentBlockType    string // "text" | "thinking" | "tool_use"
	CurrentToolName     string
	CurrentToolArgs     string
	CurrentToolHadDelta bool
	HasToolCall         bool

	// Tool calls keyed by the upstream tool_call index. The Anthropic block
	// index assigned at content_block_start time is stored so later argument
	// deltas for the same tool land on the right block.
	toolBlockIndex    map[int]int
	toolAnnounced     map[int]bool
	toolName          map[int]string
	pendingToolCallID map[int]string // call ID received before the name (deferred announce)

	// Reasoning (DeepSeek-style): reasoning_content streamed before content.
	// No separate reasoning block index — it uses ContentBlockIndex like the
	// Responses bridge's ReasoningIndex, but since blocks are sequential we
	// reuse the single ContentBlockIndex counter.

	FinishReason string

	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	ResponseID string
	Model      string
	Created    int64
REDACTED

// NewChatCompletionsToAnthropicStreamState returns an initialized stream state.
func NewChatCompletionsToAnthropicStreamState(model string) *ChatCompletionsToAnthropicStreamState {
	return &ChatCompletionsToAnthropicStreamState{
		ResponseID:        generateResponsesID(),
		Model:             model,
		Created:           time.Now().Unix(),
		toolBlockIndex:    make(map[int]int),
		toolAnnounced:     make(map[int]bool),
		toolName:          make(map[int]string),
		pendingToolCallID: make(map[int]string),
REDACTED
REDACTED

// ChatCompletionsChunkToAnthropicEvents converts one Chat Completions stream
// chunk into zero or more Anthropic stream events, updating state as it goes.
func ChatCompletionsChunkToAnthropicEvents(
	chunk *ChatCompletionsChunk,
	state *ChatCompletionsToAnthropicStreamState,
) []AnthropicStreamEvent {
	if chunk == nil || state == nil {
		return nil
REDACTED
	if chunk.ID != "" {
		state.ResponseID = chunk.ID
REDACTED
	if state.Model == "" && chunk.Model != "" {
		state.Model = chunk.Model
REDACTED

	// Usage in a streaming chunk (include_usage) arrives in its own chunk,
	// often with empty choices. Capture it for the finalize message_delta.
	if chunk.Usage != nil {
		u := chatUsageToAnthropicUsage(chunk.Usage)
		state.InputTokens = u.InputTokens
		state.OutputTokens = u.OutputTokens
		state.CacheReadInputTokens = u.CacheReadInputTokens
		state.CacheCreationInputTokens = u.CacheCreationInputTokens
REDACTED

	var events []AnthropicStreamEvent
	events = append(events, ensureCCAnthropicMessageStart(state)...)

	for _, choice := range chunk.Choices {
		// Reasoning content → thinking block.
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			events = append(events, ensureCCAnthropicThinkingBlock(state)...)
			events = append(events, ccAnthropicDelta(state, &AnthropicDelta{
				Type:     "thinking_delta",
				Thinking: *choice.Delta.ReasoningContent,
		REDACTED)...)
	REDACTED

		// Text content → text block (closes any open thinking block first).
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			events = append(events, closeCCAnthropicBlockIfOpen(state, "thinking")...)
			events = append(events, ensureCCAnthropicTextBlock(state)...)
			events = append(events, ccAnthropicDelta(state, &AnthropicDelta{
				Type: "text_delta",
				Text: *choice.Delta.Content,
		REDACTED)...)
	REDACTED

		// Tool calls → tool_use blocks.
		for _, toolCall := range choice.Delta.ToolCalls {
			events = append(events, closeCCAnthropicBlockIfOpen(state, "thinking")...)
			events = append(events, handleCCAnthropicToolCall(state, &toolCall)...)
	REDACTED

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			state.FinishReason = *choice.FinishReason
	REDACTED
REDACTED

	return events
REDACTED

// FinalizeChatCompletionsAnthropicStream emits terminal Anthropic events
// (close open blocks + message_delta + message_stop) when the stream ends.
func FinalizeChatCompletionsAnthropicStream(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state == nil || state.MessageStopSent {
		return nil
REDACTED

	var events []AnthropicStreamEvent
	if !state.MessageStartSent {
		events = append(events, ensureCCAnthropicMessageStart(state)...)
REDACTED
	events = append(events, closeCCAnthropicBlock(state)...)

	stopReason := ccFinishReasonToAnthropicStopReason(state.FinishReason, state.HasToolCall)

	events = append(events,
		AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: stopReason,
		REDACTED,
			Usage: &AnthropicUsage{
				InputTokens:              state.InputTokens,
				OutputTokens:             state.OutputTokens,
				CacheReadInputTokens:     state.CacheReadInputTokens,
				CacheCreationInputTokens: state.CacheCreationInputTokens,
		REDACTED,
	REDACTED,
		AnthropicStreamEvent{Type: "message_stop"REDACTED,
	)
	state.MessageStopSent = true
	return events
REDACTED

// ensureCCAnthropicMessageStart emits message_start on the first event.
func ensureCCAnthropicMessageStart(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.MessageStartSent {
		return nil
REDACTED
	state.MessageStartSent = true
	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:      state.ResponseID,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{REDACTED,
			Model:   state.Model,
			Usage:   AnthropicUsage{InputTokens: 0, OutputTokens: 0REDACTED,
	REDACTED,
REDACTEDREDACTED
REDACTED

// ensureCCAnthropicThinkingBlock opens a thinking block if none is open.
func ensureCCAnthropicThinkingBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.ContentBlockOpen && state.CurrentBlockType == "thinking" {
		return nil
REDACTED
	events := closeCCAnthropicBlock(state)
	idx := state.ContentBlockIndex
	state.ContentBlockOpen = true
	state.CurrentBlockType = "thinking"
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx,
		ContentBlock: &AnthropicContentBlock{
			Type:     "thinking",
			Thinking: "",
	REDACTED,
REDACTED)
	return events
REDACTED

// ensureCCAnthropicTextBlock opens a text block if none is open.
func ensureCCAnthropicTextBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.ContentBlockOpen && state.CurrentBlockType == "text" {
		return nil
REDACTED
	events := closeCCAnthropicBlock(state)
	idx := state.ContentBlockIndex
	state.ContentBlockOpen = true
	state.CurrentBlockType = "text"
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx,
		ContentBlock: &AnthropicContentBlock{
			Type: "text",
			Text: "",
	REDACTED,
REDACTED)
	return events
REDACTED

// handleCCAnthropicToolCall processes one upstream tool_call delta. A new index
// opens a tool_use block (deferred if the name hasn't arrived yet); argument
// fragments emit input_json_delta on the tool's block.
func handleCCAnthropicToolCall(state *ChatCompletionsToAnthropicStreamState, toolCall *ChatToolCall) []AnthropicStreamEvent {
	idx := 0
	if toolCall.Index != nil {
		idx = *toolCall.Index
REDACTED

	var events []AnthropicStreamEvent

	if _, ok := state.toolBlockIndex[idx]; !ok {
		// New tool call. Close any open non-tool block first.
		events = append(events, closeCCAnthropicBlock(state)...)
		blockIdx := state.ContentBlockIndex
		state.toolBlockIndex[idx] = blockIdx
		state.HasToolCall = true

		// Open the tool_use block immediately if we have an ID + name; otherwise
		// defer the content_block_start until the name arrives.
		callID := toolCall.ID
		if callID == "" {
			callID = generateItemID()
	REDACTED
		name := toolCall.Function.Name
		if name != "" {
			state.toolAnnounced[idx] = true
			state.toolName[idx] = name
			state.CurrentToolName = name
			state.ContentBlockOpen = true
			state.CurrentBlockType = "tool_use"
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_start",
				Index: &blockIdx,
				ContentBlock: &AnthropicContentBlock{
					Type:  "tool_use",
					ID:    fromResponsesCallID(callID),
					Name:  name,
					Input: json.RawMessage("{REDACTED"),
			REDACTED,
		REDACTED)
	REDACTED else {
			state.toolAnnounced[idx] = false
			// Store the call ID so we can emit content_block_start when the
			// name arrives. We stash it in toolName prefixed with the ID marker
			// is unnecessary — keep the pending ID separately is cleaner, but
			// to avoid another map we re-derive: the next delta for this idx
			// with a name will announce. We still need the ID though.
			// Store ID in toolName as "id\x00" sentinel? No — add a field.
			state.pendingToolCallID[idx] = callID
	REDACTED
REDACTED else {
		// Existing tool call: update ID/name if provided.
		if toolCall.Function.Name != "" && !state.toolAnnounced[idx] {
			blockIdx := state.toolBlockIndex[idx]
			name := toolCall.Function.Name
			state.toolAnnounced[idx] = true
			state.toolName[idx] = name
			state.CurrentToolName = name
			state.ContentBlockOpen = true
			state.CurrentBlockType = "tool_use"
			callID := state.pendingToolCallID[idx]
			if toolCall.ID != "" {
				callID = toolCall.ID
		REDACTED
			if callID == "" {
				callID = generateItemID()
		REDACTED
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_start",
				Index: &blockIdx,
				ContentBlock: &AnthropicContentBlock{
					Type:  "tool_use",
					ID:    fromResponsesCallID(callID),
					Name:  name,
					Input: json.RawMessage("{REDACTED"),
			REDACTED,
		REDACTED)
	REDACTED
REDACTED

	// Argument fragment → input_json_delta on this tool's block.
	if toolCall.Function.Arguments != "" {
		state.CurrentToolArgs += toolCall.Function.Arguments
		state.CurrentToolHadDelta = true
		if blockIdx, ok := state.toolBlockIndex[idx]; ok && state.toolAnnounced[idx] {
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: &blockIdx,
				Delta: &AnthropicDelta{
					Type:        "input_json_delta",
					PartialJSON: toolCall.Function.Arguments,
			REDACTED,
		REDACTED)
	REDACTED
REDACTED

	return events
REDACTED

// ccAnthropicDelta emits a content_block_delta on the current block.
func ccAnthropicDelta(state *ChatCompletionsToAnthropicStreamState, delta *AnthropicDelta) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
REDACTED
	idx := state.ContentBlockIndex
	return []AnthropicStreamEvent{{
		Type:  "content_block_delta",
		Index: &idx,
		Delta: delta,
REDACTEDREDACTED
REDACTED

// closeCCAnthropicBlockIfOpen closes the current block only if it matches the
// given type (used to close a thinking block before opening text/tool).
func closeCCAnthropicBlockIfOpen(state *ChatCompletionsToAnthropicStreamState, blockType string) []AnthropicStreamEvent {
	if !state.ContentBlockOpen || state.CurrentBlockType != blockType {
		return nil
REDACTED
	return closeCCAnthropicBlock(state)
REDACTED

// closeCCAnthropicBlock closes the currently open content block.
func closeCCAnthropicBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
REDACTED
	idx := state.ContentBlockIndex
	state.ContentBlockOpen = false
	state.ContentBlockIndex++
	state.CurrentBlockType = ""
	state.CurrentToolName = ""
	state.CurrentToolArgs = ""
	state.CurrentToolHadDelta = false
	return []AnthropicStreamEvent{{
		Type:  "content_block_stop",
		Index: &idx,
REDACTEDREDACTED
REDACTED

// ccFinishReasonToAnthropicStopReason maps a Chat Completions finish_reason
// (captured during streaming) to an Anthropic stop_reason for message_delta.
func ccFinishReasonToAnthropicStopReason(reason string, hasToolCall bool) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "stop":
		if hasToolCall {
			return "tool_use"
	REDACTED
		return "end_turn"
	default:
		if hasToolCall {
			return "tool_use"
	REDACTED
		return "end_turn"
REDACTED
REDACTED
