package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponsesToChatCompletionsRequest converts a Responses API request into a
// Chat Completions request for upstreams that only implement
// /v1/chat/completions.
func ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
REDACTED

	messages, err := responsesInputToChatMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, err
REDACTED

	out := &ChatCompletionsRequest{
		Model:               req.Model,
		Messages:            messages,
		MaxCompletionTokens: req.MaxOutputTokens,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		Stream:              req.Stream,
		ServiceTier:         req.ServiceTier,
REDACTED
	if req.Reasoning != nil {
		out.ReasoningEffort = req.Reasoning.Effort
REDACTED
	if len(req.Tools) > 0 {
		out.Tools = responsesToolsToChatTools(req.Tools)
REDACTED
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = responsesToolChoiceToChatToolChoice(req.ToolChoice)
REDACTED
	if req.Text != nil {
		out.ResponseFormat = responsesTextFormatToChatResponseFormat(req.Text.Format)
REDACTED

	return out, nil
REDACTED

// responsesInputToChatMessages converts a Responses request's instructions +
// input[] into Chat Completions messages. It is a three-stage pipeline:
//
//	parse   — instructions become a system message; input[] is split into items
//	build   — buildChatMessagesFromItems walks items, attaching reasoning to the
//	          assistant message that produced a tool call, merging parallel tool
//	          calls into one assistant message, and skipping item types that have
//	          no Chat equivalent
//	normalize — normalizeChatMessages enforces the invariants DeepSeek requires
//
// The build + normalize split keeps every protocol rule in one place rather than
// scattered across per-item cases, and makes unknown future codex item types
// fail safe instead of leaking into the upstream request.
func responsesInputToChatMessages(instructions string, inputRaw json.RawMessage) ([]ChatMessage, error) {
	var messages []ChatMessage
	if strings.TrimSpace(instructions) != "" {
		content, _ := json.Marshal(instructions)
		messages = append(messages, ChatMessage{Role: "system", Content: contentREDACTED)
REDACTED

	inputRaw = bytesTrimSpace(inputRaw)
	if len(inputRaw) == 0 || string(inputRaw) == "null" {
		return messages, nil
REDACTED

	// Bare string input is a single user turn.
	var inputText string
	if err := json.Unmarshal(inputRaw, &inputText); err == nil {
		content, _ := json.Marshal(inputText)
		messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
		return messages, nil
REDACTED

	var rawItems []json.RawMessage
	if err := json.Unmarshal(inputRaw, &rawItems); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
REDACTED

	built, err := buildChatMessagesFromItems(messages, rawItems)
	if err != nil {
		return nil, err
REDACTED
	return normalizeChatMessages(built), nil
REDACTED

// buildChatMessagesFromItems walks the Responses input items and appends the
// corresponding Chat messages.
func buildChatMessagesFromItems(messages []ChatMessage, rawItems []json.RawMessage) ([]ChatMessage, error) {
	// pendingReasoning holds the reasoning text from a reasoning item until the
	// assistant message it belongs to is emitted. DeepSeek's thinking mode
	// requires the reasoning_content that produced a tool call to be passed back
	// on that assistant message; dropping it yields a 400. It only survives
	// across an assistant message (so a following tool call in the same turn
	// still receives it); any other role ends the thinking span.
	var pendingReasoning string

	for _, raw := range rawItems {
		raw = bytesTrimSpace(raw)
		if len(raw) == 0 || string(raw) == "null" {
			continue
	REDACTED

		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			var text string
			if textErr := json.Unmarshal(raw, &text); textErr == nil {
				content, _ := json.Marshal(text)
				messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
				pendingReasoning = ""
				continue
		REDACTED
			return nil, fmt.Errorf("parse responses input item: %w", err)
	REDACTED

		role := chatCompletionsBridgeRole(rawString(item["role"]))
		itemType := rawString(item["type"])
		switch itemType {
		case "reasoning":
			if txt := extractResponsesReasoningText(item); txt != "" {
				pendingReasoning = txt
		REDACTED
			continue
		case "function_call":
			arguments := rawString(item["arguments"])
			if strings.TrimSpace(arguments) == "" {
				arguments = "{REDACTED"
		REDACTED
			toolCall := ChatToolCall{
				ID:   rawString(item["call_id"]),
				Type: "function",
				Function: ChatFunctionCall{
					Name:      rawString(item["name"]),
					Arguments: arguments,
			REDACTED,
		REDACTED
			// Parallel tool calls arrive as consecutive function_call items and
			// must share one assistant message; the matching tool replies then
			// follow it. Merge into the immediately preceding assistant message.
			if n := len(messages); n > 0 && messages[n-1].Role == "assistant" {
				messages[n-1].ToolCalls = append(messages[n-1].ToolCalls, toolCall)
				if messages[n-1].ReasoningContent == "" {
					messages[n-1].ReasoningContent = pendingReasoning
			REDACTED
		REDACTED else {
				messages = append(messages, ChatMessage{
					Role:             "assistant",
					ToolCalls:        []ChatToolCall{toolCallREDACTED,
					ReasoningContent: pendingReasoning,
			REDACTED)
		REDACTED
			pendingReasoning = ""
			continue
		case "function_call_output":
			content, _ := json.Marshal(rawString(item["output"]))
			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: rawString(item["call_id"]),
				Content:    content,
		REDACTED)
			pendingReasoning = ""
			continue
		case "input_text", "text":
			content, _ := json.Marshal(rawString(item["text"]))
			messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
			pendingReasoning = ""
			continue
		case "input_image":
			content, err := chatContentFromSingleResponsesPart(itemType, item)
			if err != nil {
				return nil, err
		REDACTED
			messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
			pendingReasoning = ""
			continue
	REDACTED

		// Only genuine message items become chat messages. Codex emits other
		// Responses item types with no Chat equivalent (web_search_call,
		// local_shell_call, custom tool calls, file_search_call, ...). Converting
		// them via the generic path would insert a spurious message between an
		// assistant tool_calls message and its tool reply, which DeepSeek rejects
		// ("insufficient tool messages following tool_calls message"). Skip them.
		if itemType != "" && itemType != "message" {
			pendingReasoning = ""
			continue
	REDACTED

		content := item["content"]
		if len(bytesTrimSpace(content)) == 0 {
			if text := rawString(item["text"]); text != "" {
				content, _ = json.Marshal(text)
		REDACTED
	REDACTED
		chatContent, err := responsesContentToChatContent(content, role)
		if err != nil {
			return nil, err
	REDACTED
		messages = append(messages, ChatMessage{Role: role, Content: chatContentREDACTED)
		// Reasoning only survives across an assistant text message.
		if role != "assistant" {
			pendingReasoning = ""
	REDACTED
REDACTED

	return messages, nil
REDACTED

// normalizeChatMessages is the single place that enforces the tool-call
// invariant the DeepSeek / OpenAI Chat Completions schema requires: an assistant
// message with tool_calls must be immediately followed by one tool message per
// tool_call_id, in order, with nothing in between.
//
// Codex histories violate this in several ways that the builder alone can't fix:
//   - a non-tool message lands between an assistant tool_calls message and its
//     tool replies (e.g. an "Approved command prefix saved" system notice codex
//     injects mid tool-execution);
//   - a parallel tool_call's sibling output never arrives, or a call is left
//     dangling by a mid-execution reconnect (unanswered tool_call);
//   - a tool reply has no announcing assistant tool_call (orphan).
//
// It rebuilds the sequence so each assistant's answered tool_calls are followed
// directly by their replies (in call order); unanswered tool_calls are dropped
// (and an assistant left with neither tool_calls nor content is dropped); orphan
// tool replies and intervening messages are emitted in their natural position
// but never between an assistant tool_calls message and its replies.
func normalizeChatMessages(messages []ChatMessage) []ChatMessage {
	// Index every tool reply by its tool_call_id (last wins on duplicates).
	replies := make(map[string]ChatMessage)
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			replies[m.ToolCallID] = m
	REDACTED
REDACTED

	out := make([]ChatMessage, 0, len(messages))
	for _, m := range messages {
		switch {
		case m.Role == "tool":
			// A bare tool message with no tool_call_id is a direct Chat
			// Completions passthrough; keep it in place. A tool reply whose id is
			// announced by an assistant is emitted right after that assistant
			// (skip the standalone occurrence). Any other tool reply is an orphan
			// and is dropped.
			if m.ToolCallID == "" {
				out = append(out, m)
		REDACTED
			continue
		case len(m.ToolCalls) > 0:
			kept := make([]ChatToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				if tc.ID == "" {
					continue
			REDACTED
				if _, ok := replies[tc.ID]; ok {
					kept = append(kept, tc)
			REDACTED
		REDACTED
			if len(kept) == 0 {
				// No answered tool_calls left: keep as a plain message if it has
				// content, otherwise drop it entirely.
				if isBlankChatContent(m.Content) {
					continue
			REDACTED
				m.ToolCalls = nil
				out = append(out, m)
				continue
		REDACTED
			m.ToolCalls = kept
			out = append(out, m)
			for _, tc := range kept {
				out = append(out, replies[tc.ID])
		REDACTED
		default:
			out = append(out, m)
	REDACTED
REDACTED
	return out
REDACTED

// isBlankChatContent reports whether a chat message content holds no usable text.
func isBlankChatContent(raw json.RawMessage) bool {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return true
REDACTED
	return chatMessageContentText(raw) == ""
REDACTED

// extractResponsesReasoningText pulls the reasoning text out of a Responses
// reasoning item. The Chat→Responses bridge writes the upstream reasoning_content
// verbatim into the summary_text parts (see closeChatReasoningItem), so codex
// round-trips it there; prefer summary[].text and fall back to content.
func extractResponsesReasoningText(item map[string]json.RawMessage) string {
	var parts []string
	collect := func(raw json.RawMessage) {
		raw = bytesTrimSpace(raw)
		if len(raw) == 0 || string(raw) == "null" {
			return
	REDACTED
		var arr []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			for _, p := range arr {
				if t := rawString(p["text"]); t != "" {
					parts = append(parts, t)
			REDACTED
		REDACTED
			return
	REDACTED
		if t := rawString(raw); t != "" {
			parts = append(parts, t)
	REDACTED
REDACTED
	collect(item["summary"])
	if len(parts) == 0 {
		collect(item["content"])
REDACTED
	return strings.Join(parts, "\n")
REDACTED

func chatCompletionsBridgeRole(role string) string {
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		return "user"
REDACTED
	if strings.EqualFold(trimmed, "developer") {
		return "system"
REDACTED
	return role
REDACTED

func responsesContentToChatContent(raw json.RawMessage, role string) (json.RawMessage, error) {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		empty, _ := json.Marshal("")
		return empty, nil
REDACTED

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return raw, nil
REDACTED

	var rawParts []json.RawMessage
	if err := json.Unmarshal(raw, &rawParts); err == nil {
		return responsesContentPartsToChatContent(rawParts, role)
REDACTED

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		return chatContentFromSingleResponsesPart(rawString(obj["type"]), obj)
REDACTED

	return raw, nil
REDACTED

func responsesContentPartsToChatContent(rawParts []json.RawMessage, role string) (json.RawMessage, error) {
	var textParts []string
	var chatParts []ChatContentPart
	hasNonText := false

	for _, rawPart := range rawParts {
		var part map[string]json.RawMessage
		if err := json.Unmarshal(rawPart, &part); err != nil {
			continue
	REDACTED
		partType := rawString(part["type"])
		switch partType {
		case "input_text", "output_text", "text", "":
			text := rawString(part["text"])
			if text == "" {
				continue
		REDACTED
			textParts = append(textParts, text)
			chatParts = append(chatParts, ChatContentPart{Type: "text", Text: textREDACTED)
		case "input_image", "image_url":
			imageURL := rawString(part["image_url"])
			if imageURL == "" {
				imageURL = rawNestedString(part["image_url"], "url")
		REDACTED
			if imageURL == "" {
				continue
		REDACTED
			hasNonText = true
			chatParts = append(chatParts, ChatContentPart{
				Type:     "image_url",
				ImageURL: &ChatImageURL{URL: imageURLREDACTED,
		REDACTED)
	REDACTED
REDACTED

	if !hasNonText {
		joined, _ := json.Marshal(strings.Join(textParts, "\n\n"))
		return joined, nil
REDACTED
	if role != "user" {
		joined, _ := json.Marshal(strings.Join(textParts, "\n\n"))
		return joined, nil
REDACTED
	if len(chatParts) == 0 {
		empty, _ := json.Marshal("")
		return empty, nil
REDACTED
	return json.Marshal(chatParts)
REDACTED

func chatContentFromSingleResponsesPart(partType string, part map[string]json.RawMessage) (json.RawMessage, error) {
	switch partType {
	case "input_image", "image_url":
		imageURL := rawString(part["image_url"])
		if imageURL == "" {
			imageURL = rawNestedString(part["image_url"], "url")
	REDACTED
		return json.Marshal([]ChatContentPart{{
			Type:     "image_url",
			ImageURL: &ChatImageURL{URL: imageURLREDACTED,
	REDACTEDREDACTED)
	default:
		return json.Marshal(rawString(part["text"]))
REDACTED
REDACTED

func responsesToolsToChatTools(tools []ResponsesTool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
	REDACTED
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
				Strict:      tool.Strict,
		REDACTED,
	REDACTED)
REDACTED
	return out
REDACTED

func responsesToolChoiceToChatToolChoice(raw json.RawMessage) json.RawMessage {
	var choice map[string]json.RawMessage
	if err := json.Unmarshal(raw, &choice); err != nil {
		return raw
REDACTED
	if rawString(choice["type"]) != "function" {
		return raw
REDACTED
	name := rawString(choice["name"])
	if name == "" {
		name = rawNestedString(choice["function"], "name")
REDACTED
	if name == "" {
		return raw
REDACTED
	out, err := json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]string{
			"name": name,
	REDACTED,
REDACTED)
	if err != nil {
		return raw
REDACTED
	return out
REDACTED

// ChatCompletionsResponseToResponses converts a non-streaming Chat Completions
// response into a Responses API response.
func ChatCompletionsResponseToResponses(resp *ChatCompletionsResponse, model string) *ResponsesResponse {
	id := ""
	if resp != nil {
		id = resp.ID
REDACTED
	if id == "" {
		id = generateResponsesID()
REDACTED

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  model,
		Status: "completed",
REDACTED
	if resp == nil {
		out.Output = []ResponsesOutput{emptyResponsesMessageOutput()REDACTED
		return out
REDACTED
	if out.Model == "" {
		out.Model = resp.Model
REDACTED

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		out.Output = chatMessageToResponsesOutput(choice.Message)
		if choice.FinishReason == "length" {
			out.Status = "incomplete"
			out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"REDACTED
	REDACTED
REDACTED
	if len(out.Output) == 0 {
		out.Output = []ResponsesOutput{emptyResponsesMessageOutput()REDACTED
REDACTED
	if resp.Usage != nil {
		out.Usage = ChatUsageToResponsesUsage(resp.Usage)
REDACTED
	return out
REDACTED

func chatMessageToResponsesOutput(message ChatMessage) []ResponsesOutput {
	var outputs []ResponsesOutput
	if message.ReasoningContent != "" {
		outputs = append(outputs, ResponsesOutput{
			Type: "reasoning",
			ID:   generateItemID(),
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: message.ReasoningContent,
	REDACTED
	REDACTED)
REDACTED

	text := chatMessageContentText(message.Content)
	if text == "" && strings.TrimSpace(message.ReasoningContent) != "" && len(message.ToolCalls) == 0 {
		text = message.ReasoningContent
REDACTED
	if text != "" || len(message.ToolCalls) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "message",
			ID:   generateItemID(),
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: text,
	REDACTED
			Status: "completed",
	REDACTED)
REDACTED

	for _, toolCall := range message.ToolCalls {
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{REDACTED"
	REDACTED
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: arguments,
			Status:    "completed",
	REDACTED)
REDACTED

	return outputs
REDACTED

func emptyResponsesMessageOutput() ResponsesOutput {
	return ResponsesOutput{
		Type:    "message",
		ID:      generateItemID(),
		Role:    "assistant",
		Content: []ResponsesContentPart{{Type: "output_text", Text: ""REDACTEDREDACTED,
		Status:  "completed",
REDACTED
REDACTED

func chatMessageContentText(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
REDACTED
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
REDACTED
	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				texts = append(texts, part.Text)
		REDACTED
	REDACTED
		return strings.Join(texts, "\n\n")
REDACTED
	return ""
REDACTED

// ChatUsageToResponsesUsage converts Chat Completions token usage to Responses
// usage shape.
func ChatUsageToResponsesUsage(usage *ChatUsage) *ResponsesUsage {
	if usage == nil {
		return nil
REDACTED
	out := &ResponsesUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
REDACTED
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
REDACTED
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		out.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: usage.PromptTokensDetails.CachedTokens,
	REDACTED
REDACTED
	return out
REDACTED

// ChatCompletionsToResponsesStreamState tracks state while converting Chat
// Completions SSE chunks into Responses SSE events.
type ChatCompletionsToResponsesStreamState struct {
	ResponseID     string
	Model          string
	Created        int64
	SequenceNumber int
	CreatedSent    bool
	CompletedSent  bool

	// nextOutputIndex assigns sequential output_index values to items as they
	// are opened (reasoning, message, tool calls), so the streamed indices match
	// the order of items in the final response.output array.
	nextOutputIndex int

	// Reasoning item lifecycle. DeepSeek-style upstreams stream all
	// reasoning_content before any content, so reasoning is modeled as its own
	// "reasoning" output item that must be opened (output_item.added) before any
	// reasoning delta and closed before the message/tool items open.
	ReasoningItemID string
	ReasoningIndex  int
	ReasoningOpen   bool
	ReasoningDone   bool

	// Message item + output_text content-part lifecycle.
	MessageItemID string
	MessageIndex  int
	TextPartOpen  bool

	Text      strings.Builder
	Reasoning strings.Builder

	// Tool-call lifecycle, keyed by the upstream tool_call index.
	ToolCalls       map[int]*ChatToolCall
	ToolItemIDs     map[int]string
	ToolOutputIndex map[int]int

	FinishReason string
	Usage        *ResponsesUsage
REDACTED

// NewChatCompletionsToResponsesStreamState returns an initialized stream state.
func NewChatCompletionsToResponsesStreamState(model string) *ChatCompletionsToResponsesStreamState {
	return &ChatCompletionsToResponsesStreamState{
		ResponseID:      generateResponsesID(),
		Model:           model,
		Created:         time.Now().Unix(),
		ToolCalls:       make(map[int]*ChatToolCall),
		ToolItemIDs:     make(map[int]string),
		ToolOutputIndex: make(map[int]int),
REDACTED
REDACTED

func (state *ChatCompletionsToResponsesStreamState) allocOutputIndex() int {
	idx := state.nextOutputIndex
	state.nextOutputIndex++
	return idx
REDACTED

// ChatCompletionsChunkToResponsesEvents converts one Chat Completions stream
// chunk into zero or more Responses stream events.
func ChatCompletionsChunkToResponsesEvents(
	chunk *ChatCompletionsChunk,
	state *ChatCompletionsToResponsesStreamState,
) []ResponsesStreamEvent {
	if chunk == nil || state == nil {
		return nil
REDACTED
	if chunk.ID != "" {
		state.ResponseID = chunk.ID
REDACTED
	if state.Model == "" && chunk.Model != "" {
		state.Model = chunk.Model
REDACTED
	if chunk.Usage != nil {
		state.Usage = ChatUsageToResponsesUsage(chunk.Usage)
REDACTED

	var events []ResponsesStreamEvent
	events = append(events, ensureChatToResponsesCreated(state)...)

	for _, choice := range chunk.Choices {
		// Reasoning is emitted as its own output item and must be opened
		// (output_item.added + reasoning_summary_part.added) before the first
		// delta, otherwise a strict client discards the delta. The leading
		// empty-string reasoning delta upstreams send is filtered out.
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			events = append(events, ensureChatReasoningItem(state)...)
			_, _ = state.Reasoning.WriteString(*choice.Delta.ReasoningContent)
			events = append(events, chatToResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
				OutputIndex:  state.ReasoningIndex,
				SummaryIndex: 0,
				Delta:        *choice.Delta.ReasoningContent,
				ItemID:       state.ReasoningItemID,
		REDACTED))
	REDACTED
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			// First real content closes the reasoning item, then opens the
			// message item and its output_text content part.
			events = append(events, closeChatReasoningItem(state)...)
			events = append(events, ensureChatToResponsesMessageItem(state)...)
			events = append(events, ensureChatToResponsesTextPart(state)...)
			_, _ = state.Text.WriteString(*choice.Delta.Content)
			events = append(events, chatToResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
				OutputIndex:  state.MessageIndex,
				ContentIndex: 0,
				Delta:        *choice.Delta.Content,
				ItemID:       state.MessageItemID,
		REDACTED))
	REDACTED
		for _, toolCall := range choice.Delta.ToolCalls {
			idx := 0
			if toolCall.Index != nil {
				idx = *toolCall.Index
		REDACTED
			stored, ok := state.ToolCalls[idx]
			if !ok {
				// A tool call closes any open reasoning item first.
				events = append(events, closeChatReasoningItem(state)...)
				copyCall := toolCall
				if copyCall.ID == "" {
					copyCall.ID = generateItemID()
			REDACTED
				copyCall.Type = "function"
				// Arguments are accumulated by the shared block below so the
				// emitted delta and the stored value stay in sync. Some upstreams
				// (e.g. GLM/Zhipu) pack id+name+arguments into the first tool_call
				// chunk; without this reset the first chunk's arguments would be
				// counted twice (once from this copy, once from the += below),
				// producing a doubled, invalid JSON like {"a":1REDACTED{"a":1REDACTED.
				copyCall.Function.Arguments = ""
				state.ToolCalls[idx] = &copyCall
				stored = &copyCall
				itemID := generateItemID()
				state.ToolItemIDs[idx] = itemID
				state.ToolOutputIndex[idx] = state.allocOutputIndex()
				events = append(events, chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
					OutputIndex: state.ToolOutputIndex[idx],
					Item: &ResponsesOutput{
						Type:   "function_call",
						ID:     itemID,
						CallID: stored.ID,
						Name:   stored.Function.Name,
						Status: "in_progress",
				REDACTED,
			REDACTED))
		REDACTED else {
				if toolCall.ID != "" {
					stored.ID = toolCall.ID
			REDACTED
				if toolCall.Function.Name != "" {
					stored.Function.Name = toolCall.Function.Name
			REDACTED
		REDACTED
			if toolCall.Function.Arguments != "" {
				stored.Function.Arguments += toolCall.Function.Arguments
				events = append(events, chatToResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
					OutputIndex: state.ToolOutputIndex[idx],
					ItemID:      state.ToolItemIDs[idx],
					Delta:       toolCall.Function.Arguments,
					CallID:      stored.ID,
					Name:        stored.Function.Name,
			REDACTED))
		REDACTED
	REDACTED
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			state.FinishReason = *choice.FinishReason
	REDACTED
REDACTED

	return events
REDACTED

// FinalizeChatCompletionsResponsesStream emits terminal Responses events.
func FinalizeChatCompletionsResponsesStream(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state == nil || state.CompletedSent {
		return nil
REDACTED
	var events []ResponsesStreamEvent
	events = append(events, ensureChatToResponsesCreated(state)...)

	// Close a reasoning item that never transitioned to content (reasoning-only
	// or empty completion).
	events = append(events, closeChatReasoningItem(state)...)
	events = append(events, synthesizeChatReasoningFallbackMessage(state)...)

	if state.MessageItemID != "" {
		if state.TextPartOpen {
			events = append(events, chatToResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.MessageIndex,
				ContentIndex: 0,
				Text:         state.Text.String(),
				ItemID:       state.MessageItemID,
		REDACTED))
			events = append(events, chatToResponsesEvent(state, "response.content_part.done", &ResponsesStreamEvent{
				OutputIndex:  state.MessageIndex,
				ContentIndex: 0,
				ItemID:       state.MessageItemID,
				Part:         &ResponsesContentPart{Type: "output_text", Text: state.Text.String()REDACTED,
		REDACTED))
	REDACTED
		events = append(events, chatToResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: state.MessageIndex,
			Item: &ResponsesOutput{
				Type:    "message",
				ID:      state.MessageItemID,
				Role:    "assistant",
				Content: []ResponsesContentPart{{Type: "output_text", Text: state.Text.String()REDACTEDREDACTED,
				Status:  "completed",
		REDACTED,
	REDACTED))
REDACTED

	// Close every function_call item opened during the stream. Codex finalizes a
	// tool call only after function_call_arguments.done + output_item.done for
	// that item; without them the call never completes and the session wedges.
	// Mirrors cc-switch's finalize_tools.
	events = append(events, closeChatToolItems(state)...)

	status := "completed"
	var incompleteDetails *ResponsesIncompleteDetails
	if state.FinishReason == "length" {
		status = "incomplete"
		incompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"REDACTED
REDACTED

	state.CompletedSent = true
	events = append(events, chatToResponsesEvent(state, "response.completed", &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            state.chatOutput(),
			Usage:             state.Usage,
			IncompleteDetails: incompleteDetails,
	REDACTED,
REDACTED))
	return events
REDACTED

func ensureChatToResponsesCreated(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.CreatedSent {
		return nil
REDACTED
	state.CreatedSent = true
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.created", &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{REDACTED,
	REDACTED,
REDACTED)REDACTED
REDACTED

// ensureChatReasoningItem opens the reasoning output item (output_item.added +
// reasoning_summary_part.added) before the first reasoning delta. Codex renders
// streaming reasoning only when this summary-part lifecycle is present.
func ensureChatReasoningItem(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.ReasoningOpen || state.ReasoningDone {
		return nil
REDACTED
	state.ReasoningOpen = true
	state.ReasoningItemID = generateItemID()
	state.ReasoningIndex = state.allocOutputIndex()
	return []ResponsesStreamEvent{
		chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.ReasoningIndex,
			Item:        &ResponsesOutput{Type: "reasoning", ID: state.ReasoningItemID, Status: "in_progress"REDACTED,
	REDACTED),
		chatToResponsesEvent(state, "response.reasoning_summary_part.added", &ResponsesStreamEvent{
			OutputIndex:  state.ReasoningIndex,
			SummaryIndex: 0,
			ItemID:       state.ReasoningItemID,
			Part:         &ResponsesContentPart{Type: "summary_text"REDACTED,
	REDACTED),
REDACTED
REDACTED

// closeChatReasoningItem emits the reasoning item's terminal events
// (reasoning_summary_text.done + reasoning_summary_part.done + output_item.done).
func closeChatReasoningItem(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if !state.ReasoningOpen {
		return nil
REDACTED
	state.ReasoningOpen = false
	state.ReasoningDone = true
	reasoning := state.Reasoning.String()
	return []ResponsesStreamEvent{
		chatToResponsesEvent(state, "response.reasoning_summary_text.done", &ResponsesStreamEvent{
			OutputIndex:  state.ReasoningIndex,
			SummaryIndex: 0,
			Text:         reasoning,
			ItemID:       state.ReasoningItemID,
	REDACTED),
		chatToResponsesEvent(state, "response.reasoning_summary_part.done", &ResponsesStreamEvent{
			OutputIndex:  state.ReasoningIndex,
			SummaryIndex: 0,
			ItemID:       state.ReasoningItemID,
			Part:         &ResponsesContentPart{Type: "summary_text", Text: reasoningREDACTED,
	REDACTED),
		chatToResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: state.ReasoningIndex,
			Item: &ResponsesOutput{
				Type:    "reasoning",
				ID:      state.ReasoningItemID,
				Status:  "completed",
				Summary: []ResponsesSummary{{Type: "summary_text", Text: reasoningREDACTEDREDACTED,
		REDACTED,
	REDACTED),
REDACTED
REDACTED

func synthesizeChatReasoningFallbackMessage(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state == nil ||
		state.MessageItemID != "" ||
		state.Text.Len() > 0 ||
		state.Reasoning.Len() == 0 ||
		len(state.ToolCalls) > 0 {
		return nil
REDACTED

	text := state.Reasoning.String()
	if strings.TrimSpace(text) == "" {
		return nil
REDACTED

	var events []ResponsesStreamEvent
	events = append(events, ensureChatToResponsesMessageItem(state)...)
	events = append(events, ensureChatToResponsesTextPart(state)...)
	_, _ = state.Text.WriteString(text)
	events = append(events, chatToResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
		OutputIndex:  state.MessageIndex,
		ContentIndex: 0,
		Delta:        text,
		ItemID:       state.MessageItemID,
REDACTED))
	return events
REDACTED

func ensureChatToResponsesMessageItem(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.MessageItemID != "" {
		return nil
REDACTED
	state.MessageItemID = generateItemID()
	state.MessageIndex = state.allocOutputIndex()
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
		OutputIndex: state.MessageIndex,
		Item: &ResponsesOutput{
			Type:    "message",
			ID:      state.MessageItemID,
			Role:    "assistant",
			Status:  "in_progress",
			Content: []ResponsesContentPart{{Type: "output_text"REDACTEDREDACTED,
	REDACTED,
REDACTED)REDACTED
REDACTED

func ensureChatToResponsesTextPart(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.TextPartOpen {
		return nil
REDACTED
	state.TextPartOpen = true
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.content_part.added", &ResponsesStreamEvent{
		OutputIndex:  state.MessageIndex,
		ContentIndex: 0,
		ItemID:       state.MessageItemID,
		Part:         &ResponsesContentPart{Type: "output_text", Text: ""REDACTED,
REDACTED)REDACTED
REDACTED

// closeChatToolItems emits function_call_arguments.done + output_item.done for
// every tool call opened during the stream, carrying the full call_id/name/
// arguments so codex can deserialize and execute the call. Mirrors cc-switch's
// finalize_tools.
func closeChatToolItems(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if len(state.ToolCalls) == 0 {
		return nil
REDACTED
	var events []ResponsesStreamEvent
	for i := 0; i < len(state.ToolCalls); i++ {
		toolCall, ok := state.ToolCalls[i]
		if !ok || toolCall == nil {
			continue
	REDACTED
		itemID, opened := state.ToolItemIDs[i]
		if !opened {
			continue
	REDACTED
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{REDACTED"
	REDACTED
		outputIndex := state.ToolOutputIndex[i]
		events = append(events,
			chatToResponsesEvent(state, "response.function_call_arguments.done", &ResponsesStreamEvent{
				OutputIndex: outputIndex,
				ItemID:      itemID,
				CallID:      toolCall.ID,
				Name:        toolCall.Function.Name,
				Arguments:   arguments,
		REDACTED),
			chatToResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
				OutputIndex: outputIndex,
				Item: &ResponsesOutput{
					Type:      "function_call",
					ID:        itemID,
					CallID:    toolCall.ID,
					Name:      toolCall.Function.Name,
					Arguments: arguments,
					Status:    "completed",
			REDACTED,
		REDACTED),
		)
REDACTED
	return events
REDACTED

func (state *ChatCompletionsToResponsesStreamState) chatOutput() []ResponsesOutput {
	var outputs []ResponsesOutput
	if state.Reasoning.Len() > 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "reasoning",
			ID:   generateItemID(),
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: state.Reasoning.String(),
	REDACTED
	REDACTED)
REDACTED
	if state.MessageItemID != "" || len(state.ToolCalls) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "message",
			ID:   nonEmpty(state.MessageItemID, generateItemID()),
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: state.Text.String(),
	REDACTED
			Status: "completed",
	REDACTED)
REDACTED
	for i := 0; i < len(state.ToolCalls); i++ {
		toolCall, ok := state.ToolCalls[i]
		if !ok || toolCall == nil {
			continue
	REDACTED
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{REDACTED"
	REDACTED
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: arguments,
			Status:    "completed",
	REDACTED)
REDACTED
	return outputs
REDACTED

func chatToResponsesEvent(
	state *ChatCompletionsToResponsesStreamState,
	eventType string,
	template *ResponsesStreamEvent,
) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	evt := *template
	evt.Type = eventType
	evt.SequenceNumber = seq
	return evt
REDACTED

func rawString(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
REDACTED
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
REDACTED
	return ""
REDACTED

func rawNestedString(raw json.RawMessage, key string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
REDACTED
	return rawString(obj[key])
REDACTED

func bytesTrimSpace(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
REDACTED

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
REDACTED
	return fallback
REDACTED
