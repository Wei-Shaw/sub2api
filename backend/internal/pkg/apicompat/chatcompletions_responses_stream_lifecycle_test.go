package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func collectStreamEvents(t *testing.T, chunks []string) []ResponsesStreamEvent {
REDACTED
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	var events []ResponsesStreamEvent
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		events = append(events, ChatCompletionsChunkToResponsesEvents(&chunk, state)...)
REDACTED
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)
	return events
REDACTED

// TestStream_ReasoningOpensItemBeforeDelta guards the bug where a strict client
// (Codex) drops reasoning deltas that reference an item not yet opened.
func TestStream_ReasoningOpensItemBeforeDelta(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"think"REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"content":"hello"REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"content":""REDACTED,"finish_reason":"stop"REDACTED],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3REDACTEDREDACTED`,
REDACTED)

	open := map[int]string{REDACTED // output_index -> item type
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			require.NotNil(t, e.Item)
			open[e.OutputIndex] = e.Item.Type
		case "response.reasoning_summary_text.delta":
			require.Equalf(t, "reasoning", open[e.OutputIndex], "reasoning delta before its item was opened")
		case "response.output_text.delta":
			require.Equalf(t, "message", open[e.OutputIndex], "text delta before its item was opened")
	REDACTED
REDACTED
REDACTED

// TestStream_ToolCallLifecycleComplete guards that a tool call is fully closed
// (function_call_arguments.done + output_item.done with full arguments), which
// codex needs to execute the call.
func TestStream_ToolCallLifecycleComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":""REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":\"ls\"REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{REDACTED,"finish_reason":"tool_calls"REDACTED],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3REDACTEDREDACTED`,
REDACTED)

	var sawAdded, sawArgsDone, sawItemDone bool
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawAdded = true
		REDACTED
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{"cmd":"ls"REDACTED`, e.Arguments)
		case "response.output_item.done":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawItemDone = true
				require.Equal(t, `{"cmd":"ls"REDACTED`, e.Item.Arguments)
				require.Equal(t, "call_a", e.Item.CallID)
		REDACTED
	REDACTED
REDACTED
	require.True(t, sawAdded, "function_call output_item.added missing")
	require.True(t, sawArgsDone, "function_call_arguments.done missing")
	require.True(t, sawItemDone, "function_call output_item.done missing")
REDACTED

// TestStream_SSEWireComplete drives the full stream through SSE encoding and
// asserts the function_call events carry complete fields on the wire.
func TestStream_SSEWireComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
		`{"choices":[{"index":0,"delta":{REDACTED,"finish_reason":"tool_calls"REDACTED]REDACTED`,
REDACTED)

	var addedLine string
	for _, e := range events {
		sse, err := ResponsesEventToSSE(e)
	REDACTED
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
			addedLine = sse
	REDACTED
REDACTED
	require.NotEmpty(t, addedLine)
	// The function_call added event must carry arguments:"" on the wire.
	require.True(t, strings.Contains(addedLine, `"arguments":""`), "added line missing arguments: %s", addedLine)
	require.Contains(t, addedLine, `"call_id":"call_a"`)
REDACTED
