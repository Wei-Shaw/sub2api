package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 上游响应中的 service_tier 必须如实回传，不被重写/丢弃：
// 非流式（ResponsesResponse → ChatCompletionsResponse）与流式 chunk 均覆盖。

func TestResponsesToChatCompletions_PreservesUpstreamServiceTier(t *testing.T) {
	resp := &ResponsesResponse{
		ID:          "resp_1",
		Object:      "response",
		Model:       "gpt-5.5",
		Status:      "completed",
		ServiceTier: "priority",
		Output: []ResponsesOutput{{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: "hi",
	REDACTED
REDACTED
		Usage: &ResponsesUsage{InputTokens: 1, OutputTokens: 1REDACTED,
REDACTED

	chat := ResponsesToChatCompletions(resp, "gpt-5.5")
	require.Equal(t, "priority", chat.ServiceTier)

	// 序列化后字段仍在（omitempty 不丢非空值）。
	raw, err := json.Marshal(chat)
REDACTED
	require.Contains(t, string(raw), `"service_tier":"priority"`)
REDACTED

func TestResponsesToChatCompletions_OmitsMissingServiceTier(t *testing.T) {
	resp := &ResponsesResponse{ID: "resp_1", Model: "gpt-5.5", Status: "completed"REDACTED
	chat := ResponsesToChatCompletions(resp, "gpt-5.5")
	require.Empty(t, chat.ServiceTier)
	raw, err := json.Marshal(chat)
REDACTED
	require.NotContains(t, string(raw), "service_tier")
REDACTED

func TestResponsesEventToChatChunks_PreservesUpstreamServiceTier(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.IncludeUsage = true

	created := &ResponsesStreamEvent{Type: "response.created"REDACTED
	require.NoError(t, json.Unmarshal([]byte(`{"type":"response.created","response":{"id":"resp_s1","model":"gpt-5.5","service_tier":"priority","status":"in_progress"REDACTEDREDACTED`), created))

	chunks := ResponsesEventToChatChunks(created, state)
	require.NotEmpty(t, chunks)
	for _, chunk := range chunks {
		require.Equal(t, "priority", chunk.ServiceTier)
REDACTED

	// 后续 delta chunk 继续携带（OpenAI 流式 chunk 的 service_tier 语义）。
	delta := &ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "hi"REDACTED
	chunks = ResponsesEventToChatChunks(delta, state)
	require.NotEmpty(t, chunks)
	require.Equal(t, "priority", chunks[0].ServiceTier)

	// 终止事件同样携带。
	completed := &ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
		ID: "resp", Model: "gpt-5.5", Status: "completed",
		Usage: &ResponsesUsage{InputTokens: 1, OutputTokens: 1REDACTED,
REDACTEDREDACTED
	chunks = ResponsesEventToChatChunks(completed, state)
	require.NotEmpty(t, chunks)
	for _, chunk := range chunks {
		require.Equal(t, "priority", chunk.ServiceTier)
REDACTED
REDACTED

func TestResponsesEventToChatChunks_NoServiceTierStaysClean(t *testing.T) {
	state := NewResponsesEventToChatState()
	created := &ResponsesStreamEvent{Type: "response.created", Response: &ResponsesResponse{ID: "resp", Model: "gpt-5.5"REDACTEDREDACTED
	chunks := ResponsesEventToChatChunks(created, state)
	require.NotEmpty(t, chunks)
	require.Empty(t, chunks[0].ServiceTier)
	raw, err := json.Marshal(chunks[0])
REDACTED
	require.NotContains(t, string(raw), "service_tier")
REDACTED

// 上游 JSON 反序列化时 service_tier 进入 ResponsesResponse（缓冲桥读取链路）。
func TestResponsesResponse_UnmarshalPreservesServiceTier(t *testing.T) {
	var resp ResponsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{"id":"resp_1","object":"response","model":"gpt-5.5","status":"completed","service_tier":"flex","output":[]REDACTED`), &resp))
	require.Equal(t, "flex", resp.ServiceTier)
REDACTED

// ---------------------------------------------------------------------------
// 反向转换（Chat-only fallback）：CC 响应/流 chunk 的 service_tier 保留到
// Responses 形态，客户端与计费都能拿到上游回显。
// ---------------------------------------------------------------------------

func TestChatCompletionsResponseToResponses_PreservesServiceTier(t *testing.T) {
	cc := &ChatCompletionsResponse{
		ID:          "chatcmpl-1",
		Model:       "gpt-5.5",
		ServiceTier: "default",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)REDACTED,
			FinishReason: "stop",
REDACTED
		Usage: &ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2REDACTED,
REDACTED
	resp := ChatCompletionsResponseToResponses(cc, "gpt-5.5", nil, nil, false, nil)
	require.Equal(t, "default", resp.ServiceTier)

	raw, err := json.Marshal(resp)
REDACTED
	require.Contains(t, string(raw), `"service_tier":"default"`)
REDACTED

func TestChatCompletionsResponseToResponses_NilRespOmitsServiceTier(t *testing.T) {
	resp := ChatCompletionsResponseToResponses(nil, "gpt-5.5", nil, nil, false, nil)
	require.Empty(t, resp.ServiceTier)
REDACTED

func TestChatCompletionsChunkToResponsesEvents_PreservesServiceTier(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gpt-5.5")
	chunk := &ChatCompletionsChunk{
		ID:          "chatcmpl-2",
		Model:       "gpt-5.5",
		ServiceTier: "flex",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{Content: strPtr("hi")REDACTED,
REDACTED
REDACTED
	events := ChatCompletionsChunkToResponsesEvents(chunk, state)
	require.NotEmpty(t, events)

	// response.created 携带 service_tier。
	created := findEvent(events, "response.created")
	require.NotNil(t, created)
	require.NotNil(t, created.Response)
	require.Equal(t, "flex", created.Response.ServiceTier)

	// 终止事件同样携带。
	final := FinalizeChatCompletionsResponsesStream(state)
	completed := findEvent(final, "response.completed")
	require.NotNil(t, completed)
	require.NotNil(t, completed.Response)
	require.Equal(t, "flex", completed.Response.ServiceTier)
REDACTED

func TestChatCompletionsChunkToResponsesEvents_NoTierStaysClean(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gpt-5.5")
	chunk := &ChatCompletionsChunk{ID: "chatcmpl-3", Model: "gpt-5.5"REDACTED
	events := ChatCompletionsChunkToResponsesEvents(chunk, state)
	created := findEvent(events, "response.created")
	require.NotNil(t, created)
	require.Empty(t, created.Response.ServiceTier)
	raw, err := json.Marshal(created)
REDACTED
	require.NotContains(t, string(raw), "service_tier")
REDACTED

func findEvent(events []ResponsesStreamEvent, eventType string) *ResponsesStreamEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
	REDACTED
REDACTED
	return nil
REDACTED
