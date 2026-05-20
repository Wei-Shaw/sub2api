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

	return out, nil
REDACTED

func responsesInputToChatMessages(instructions string, inputRaw json.RawMessage) ([]ChatMessage, error) {
	var messages []ChatMessage
	if strings.TrimSpace(instructions) != "" {
		content, _ := json.Marshal(instructions)
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: content,
	REDACTED)
REDACTED

	inputRaw = bytesTrimSpace(inputRaw)
	if len(inputRaw) == 0 || string(inputRaw) == "null" {
		return messages, nil
REDACTED

	var inputText string
	if err := json.Unmarshal(inputRaw, &inputText); err == nil {
		content, _ := json.Marshal(inputText)
		messages = append(messages, ChatMessage{
			Role:    "user",
			Content: content,
	REDACTED)
		return messages, nil
REDACTED

	var rawItems []json.RawMessage
	if err := json.Unmarshal(inputRaw, &rawItems); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
REDACTED

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
				continue
		REDACTED
			return nil, fmt.Errorf("parse responses input item: %w", err)
	REDACTED

		role := rawString(item["role"])
		itemType := rawString(item["type"])
		switch itemType {
		case "function_call":
			arguments := rawString(item["arguments"])
			if strings.TrimSpace(arguments) == "" {
				arguments = "{REDACTED"
		REDACTED
			messages = append(messages, ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   rawString(item["call_id"]),
					Type: "function",
					Function: ChatFunctionCall{
						Name:      rawString(item["name"]),
						Arguments: arguments,
				REDACTED,
		REDACTED
		REDACTED)
			continue
		case "function_call_output":
			content, _ := json.Marshal(rawString(item["output"]))
			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: rawString(item["call_id"]),
				Content:    content,
		REDACTED)
			continue
		case "input_text", "text":
			content, _ := json.Marshal(rawString(item["text"]))
			messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
			continue
		case "input_image":
			content, err := chatContentFromSingleResponsesPart(itemType, item)
			if err != nil {
				return nil, err
		REDACTED
			messages = append(messages, ChatMessage{Role: "user", Content: contentREDACTED)
			continue
	REDACTED

		if role == "" {
			role = "user"
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
		messages = append(messages, ChatMessage{
			Role:    role,
			Content: chatContent,
	REDACTED)
REDACTED

	return messages, nil
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

	MessageItemID string
	Text          strings.Builder
	Reasoning     strings.Builder
	ToolCalls     map[int]*ChatToolCall

	FinishReason string
	Usage        *ResponsesUsage
REDACTED

// NewChatCompletionsToResponsesStreamState returns an initialized stream state.
func NewChatCompletionsToResponsesStreamState(model string) *ChatCompletionsToResponsesStreamState {
	return &ChatCompletionsToResponsesStreamState{
		ResponseID: generateResponsesID(),
		Model:      model,
		Created:    time.Now().Unix(),
		ToolCalls:  make(map[int]*ChatToolCall),
REDACTED
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
		if choice.Delta.Content != nil {
			events = append(events, ensureChatToResponsesMessageItem(state)...)
			_, _ = state.Text.WriteString(*choice.Delta.Content)
			events = append(events, chatToResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
				OutputIndex:  0,
				ContentIndex: 0,
				Delta:        *choice.Delta.Content,
				ItemID:       state.MessageItemID,
		REDACTED))
	REDACTED
		if choice.Delta.ReasoningContent != nil {
			_, _ = state.Reasoning.WriteString(*choice.Delta.ReasoningContent)
			events = append(events, chatToResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
				OutputIndex:  0,
				SummaryIndex: 0,
				Delta:        *choice.Delta.ReasoningContent,
		REDACTED))
	REDACTED
		for _, toolCall := range choice.Delta.ToolCalls {
			idx := 0
			if toolCall.Index != nil {
				idx = *toolCall.Index
		REDACTED
			stored, ok := state.ToolCalls[idx]
			if !ok {
				copyCall := toolCall
				if copyCall.ID == "" {
					copyCall.ID = generateItemID()
			REDACTED
				copyCall.Type = "function"
				state.ToolCalls[idx] = &copyCall
				stored = &copyCall
				events = append(events, chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
					OutputIndex: idx + 1,
					Item: &ResponsesOutput{
						Type:   "function_call",
						ID:     generateItemID(),
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
					OutputIndex: idx + 1,
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
	if state.MessageItemID != "" {
		events = append(events, chatToResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
			OutputIndex:  0,
			ContentIndex: 0,
			Text:         state.Text.String(),
			ItemID:       state.MessageItemID,
	REDACTED))
		events = append(events, chatToResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: 0,
			Item: &ResponsesOutput{
				Type:   "message",
				ID:     state.MessageItemID,
				Role:   "assistant",
				Status: "completed",
		REDACTED,
	REDACTED))
REDACTED

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

func ensureChatToResponsesMessageItem(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.MessageItemID != "" {
		return nil
REDACTED
	state.MessageItemID = generateItemID()
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "message",
			ID:     state.MessageItemID,
			Role:   "assistant",
			Status: "in_progress",
	REDACTED,
REDACTED)REDACTED
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
