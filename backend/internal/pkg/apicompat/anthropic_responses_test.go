package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AnthropicToResponses tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_BasicText(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Stream:    true,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	assert.Equal(t, "gpt-5.2", resp.Model)
	assert.True(t, resp.Stream)
	assert.Equal(t, 1024, *resp.MaxOutputTokens)
	assert.False(t, *resp.Store)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "user", items[0].Role)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "Hello", parts[0].Text)
REDACTED

func TestAnthropicToResponses_SystemPrompt(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		req := &AnthropicRequest{
			Model:     "gpt-5.2",
			MaxTokens: 100,
			System:    json.RawMessage(`"You are helpful."`),
			Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)REDACTEDREDACTED,
	REDACTED
		resp, err := AnthropicToResponses(req)
	REDACTED

		var items []ResponsesInputItem
		require.NoError(t, json.Unmarshal(resp.Input, &items))
		require.Len(t, items, 2)
		assert.Equal(t, "developer", items[0].Role)
		var parts []ResponsesContentPart
		require.NoError(t, json.Unmarshal(items[0].Content, &parts))
		require.Len(t, parts, 1)
		assert.Equal(t, "input_text", parts[0].Type)
		assert.Equal(t, "You are helpful.", parts[0].Text)
REDACTED)

	t.Run("array", func(t *testing.T) {
		req := &AnthropicRequest{
			Model:     "gpt-5.2",
			MaxTokens: 100,
			System:    json.RawMessage(`[{"type":"text","text":"Part 1"REDACTED,{"type":"text","text":"Part 2"REDACTED]`),
			Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)REDACTEDREDACTED,
	REDACTED
		resp, err := AnthropicToResponses(req)
	REDACTED

		var items []ResponsesInputItem
		require.NoError(t, json.Unmarshal(resp.Input, &items))
		require.Len(t, items, 2)
		assert.Equal(t, "developer", items[0].Role)
		var parts []ResponsesContentPart
		require.NoError(t, json.Unmarshal(items[0].Content, &parts))
		require.Len(t, parts, 2)
		assert.Equal(t, "input_text", parts[0].Type)
		assert.Equal(t, "Part 1", parts[0].Text)
		assert.Equal(t, "input_text", parts[1].Type)
		assert.Equal(t, "Part 2", parts[1].Text)
REDACTED)

	t.Run("billing header skipped", func(t *testing.T) {
		req := &AnthropicRequest{
			Model:     "gpt-5.2",
			MaxTokens: 100,
			System:    json.RawMessage(`[{"type":"text","text":"x-anthropic-billing-header: cc_version=1;"REDACTED,{"type":"text","text":"Project prompt"REDACTED]`),
			Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)REDACTEDREDACTED,
	REDACTED
		resp, err := AnthropicToResponses(req)
	REDACTED

		var items []ResponsesInputItem
		require.NoError(t, json.Unmarshal(resp.Input, &items))
		require.Len(t, items, 2)
		var parts []ResponsesContentPart
		require.NoError(t, json.Unmarshal(items[0].Content, &parts))
		require.Len(t, parts, 1)
		assert.Equal(t, "Project prompt", parts[0].Text)
REDACTED)
REDACTED

func TestAnthropicToResponses_ToolUse(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"What is the weather?"`)REDACTED,
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"Let me check."REDACTED,{"type":"tool_use","id":"call_1","name":"get_weather","input":{"city":"NYC"REDACTEDREDACTED]`)REDACTED,
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"Sunny, 72°F"REDACTED]`)REDACTED,
	REDACTED,
		Tools: []AnthropicTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"REDACTEDREDACTEDREDACTED`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	// Check tools
	require.Len(t, resp.Tools, 1)
	assert.Equal(t, "function", resp.Tools[0].Type)
	assert.Equal(t, "get_weather", resp.Tools[0].Name)
	require.NotNil(t, resp.Tools[0].Strict)
	assert.False(t, *resp.Tools[0].Strict)

	// Check input items
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + assistant + function_call + function_call_output = 4
	require.Len(t, items, 4)

	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "assistant", items[1].Role)
	assert.Equal(t, "function_call", items[2].Type)
	assert.Equal(t, "call_1", items[2].CallID)
	assert.Empty(t, items[2].ID)
	assert.Equal(t, "function_call_output", items[3].Type)
	assert.Equal(t, "call_1", items[3].CallID)
	assert.Equal(t, "Sunny, 72°F", items[3].Output)
REDACTED

func TestAnthropicToResponses_ThinkingIgnored(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTED,
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"deep thought"REDACTED,{"type":"text","text":"Hi!"REDACTED]`)REDACTED,
			{Role: "user", Content: json.RawMessage(`"More"`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + assistant(text only, thinking ignored) + user = 3
	require.Len(t, items, 3)
	assert.Equal(t, "assistant", items[1].Role)
	// Assistant content should only have text, not thinking.
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[1].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "output_text", parts[0].Type)
	assert.Equal(t, "Hi!", parts[0].Text)
REDACTED

func TestAnthropicToResponses_MaxTokensFloor(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 10, // below minMaxOutputTokens (128)
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)REDACTEDREDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	assert.Equal(t, 128, *resp.MaxOutputTokens)
REDACTED

// ---------------------------------------------------------------------------
// ResponsesToAnthropic (non-streaming) tests
// ---------------------------------------------------------------------------

func TestResponsesToAnthropic_TextOnly(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_123",
		Model:  "gpt-5.2",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "Hello there!"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		Usage: &ResponsesUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	assert.Equal(t, "resp_123", anth.ID)
	assert.Equal(t, "claude-opus-4-6", anth.Model)
	assert.Equal(t, "end_turn", AnthropicStopReasonString(anth.StopReason))
	require.Len(t, anth.Content, 1)
	assert.Equal(t, "text", anth.Content[0].Type)
	assert.Equal(t, "Hello there!", anth.Content[0].Text)
	assert.Equal(t, 10, anth.Usage.InputTokens)
	assert.Equal(t, 5, anth.Usage.OutputTokens)
REDACTED

func TestResponsesToAnthropic_CachedTokensUseAnthropicInputSemantics(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_cached",
		Model:  "gpt-5.2",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "Cached response"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		Usage: &ResponsesUsage{
			InputTokens:  54006,
			OutputTokens: 123,
			TotalTokens:  54129,
			InputTokensDetails: &ResponsesInputTokensDetails{
				CachedTokens: 50688,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-sonnet-4-5-20250929")
	assert.Equal(t, 3318, anth.Usage.InputTokens)
	assert.Equal(t, 50688, anth.Usage.CacheReadInputTokens)
	assert.Equal(t, 123, anth.Usage.OutputTokens)
REDACTED

func TestResponsesToAnthropic_CachedTokensClampInputTokens(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_cached_clamp",
		Model:  "gpt-5.2",
		Status: "completed",
		Usage: &ResponsesUsage{
			InputTokens:  100,
			OutputTokens: 5,
			InputTokensDetails: &ResponsesInputTokensDetails{
				CachedTokens: 150,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-sonnet-4-5-20250929")
	assert.Equal(t, 0, anth.Usage.InputTokens)
	assert.Equal(t, 150, anth.Usage.CacheReadInputTokens)
	assert.Equal(t, 5, anth.Usage.OutputTokens)
REDACTED

func TestResponsesToAnthropic_ToolUse(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_456",
		Model:  "gpt-5.2",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "Let me check."REDACTED,
			REDACTED,
		REDACTED,
			{
				Type:      "function_call",
				CallID:    "call_1",
				Name:      "get_weather",
				Arguments: `{"city":"NYC"REDACTED`,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	assert.Equal(t, "tool_use", AnthropicStopReasonString(anth.StopReason))
	require.Len(t, anth.Content, 2)
	assert.Equal(t, "text", anth.Content[0].Type)
	assert.Equal(t, "tool_use", anth.Content[1].Type)
	assert.Equal(t, "call_1", anth.Content[1].ID)
	assert.Equal(t, "get_weather", anth.Content[1].Name)
	assert.JSONEq(t, `{"city":"NYC"REDACTED`, string(anth.Content[1].Input))
REDACTED

func TestResponsesToAnthropic_ToolUseStopReasonDoesNotDependOnLastBlock(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_tool_then_text",
		Model:  "gpt-5.5",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:      "function_call",
				CallID:    "call_todo",
				Name:      "TodoWrite",
				Arguments: `{"todos":[{"content":"review changes","status":"in_progress"REDACTED]REDACTED`,
		REDACTED,
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "Task list updated."REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	assert.Equal(t, "tool_use", AnthropicStopReasonString(anth.StopReason))
	require.Len(t, anth.Content, 2)
	assert.Equal(t, "tool_use", anth.Content[0].Type)
	assert.Equal(t, "text", anth.Content[1].Type)
REDACTED

func TestResponsesToAnthropic_ReadToolDropsEmptyPages(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_read",
		Model:  "gpt-5.5",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:      "function_call",
				CallID:    "call_read",
				Name:      "Read",
				Arguments: `{"file_path":"/tmp/demo.py","limit":2000,"offset":0,"pages":""REDACTED`,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	require.Len(t, anth.Content, 1)
	assert.Equal(t, "tool_use", anth.Content[0].Type)
	assert.JSONEq(t, `{"file_path":"/tmp/demo.py","limit":2000,"offset":0REDACTED`, string(anth.Content[0].Input))
REDACTED

func TestResponsesToAnthropic_PreservesEmptyStringsForOtherTools(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_other",
		Model:  "gpt-5.5",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:      "function_call",
				CallID:    "call_other",
				Name:      "Search",
				Arguments: `{"query":""REDACTED`,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	require.Len(t, anth.Content, 1)
	assert.JSONEq(t, `{"query":""REDACTED`, string(anth.Content[0].Input))
REDACTED

func TestResponsesToAnthropic_Reasoning(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_789",
		Model:  "gpt-5.2",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "reasoning",
				Summary: []ResponsesSummary{
					{Type: "summary_text", Text: "Thinking about the answer..."REDACTED,
			REDACTED,
		REDACTED,
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "42"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	require.Len(t, anth.Content, 2)
	assert.Equal(t, "thinking", anth.Content[0].Type)
	assert.Equal(t, "Thinking about the answer...", anth.Content[0].Thinking)
	assert.Equal(t, "text", anth.Content[1].Type)
	assert.Equal(t, "42", anth.Content[1].Text)
REDACTED

func TestResponsesToAnthropic_Incomplete(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_inc",
		Model:  "gpt-5.2",
		Status: "incomplete",
		IncompleteDetails: &ResponsesIncompleteDetails{
			Reason: "max_output_tokens",
	REDACTED,
		Output: []ResponsesOutput{
			{
				Type:    "message",
				Content: []ResponsesContentPart{{Type: "output_text", Text: "Partial..."REDACTEDREDACTED,
		REDACTED,
	REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	assert.Equal(t, "max_tokens", AnthropicStopReasonString(anth.StopReason))
REDACTED

func TestResponsesToAnthropic_EmptyOutput(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_empty",
		Model:  "gpt-5.2",
		Status: "completed",
		Output: []ResponsesOutput{REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	require.Len(t, anth.Content, 1)
	assert.Equal(t, "text", anth.Content[0].Type)
	assert.Equal(t, "", anth.Content[0].Text)
REDACTED

// ---------------------------------------------------------------------------
// Streaming: ResponsesEventToAnthropicEvents tests
// ---------------------------------------------------------------------------

func TestStreamingTextOnly(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	// 1. response.created
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.created",
		Response: &ResponsesResponse{
			ID:    "resp_1",
			Model: "gpt-5.2",
	REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "message_start", events[0].Type)

	// 2. output_item.added (message)
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "message"REDACTED,
REDACTED, state)
	assert.Len(t, events, 0) // message item doesn't emit events

	// 3. text delta
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "Hello",
REDACTED, state)
	require.Len(t, events, 2) // content_block_start + content_block_delta
	assert.Equal(t, "content_block_start", events[0].Type)
	assert.Equal(t, "text", events[0].ContentBlock.Type)
	assert.Equal(t, "content_block_delta", events[1].Type)
	assert.Equal(t, "text_delta", events[1].Delta.Type)
	assert.Equal(t, "Hello", events[1].Delta.Text)

	// 4. more text
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: " world",
REDACTED, state)
	require.Len(t, events, 1) // only delta, no new block start
	assert.Equal(t, "content_block_delta", events[0].Type)

	// 5. text done
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.output_text.done",
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	// 6. completed
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 10, OutputTokens: 5REDACTED,
	REDACTED,
REDACTED, state)
	require.Len(t, events, 2) // message_delta + message_stop
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, "end_turn", events[0].Delta.StopReason)
	assert.Equal(t, 10, events[0].Usage.InputTokens)
	assert.Equal(t, 5, events[0].Usage.OutputTokens)
	assert.Equal(t, "message_stop", events[1].Type)
REDACTED

func TestResponsesEventToAnthropicEvents_ResponseDone(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "gpt-4o"

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.done",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 12, OutputTokens: 4REDACTED,
	REDACTED,
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, "end_turn", events[0].Delta.StopReason)
	assert.Equal(t, 12, events[0].Usage.InputTokens)
	assert.Equal(t, 4, events[0].Usage.OutputTokens)
	assert.Equal(t, "message_stop", events[1].Type)
	assert.Nil(t, FinalizeResponsesAnthropicStream(state))
REDACTED

func TestResponsesEventToAnthropicEvents_TopLevelTerminalUsage(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "gpt-4o"

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
	REDACTED,
		Usage: &ResponsesUsage{
			InputTokens:  20,
			OutputTokens: 6,
			InputTokensDetails: &ResponsesInputTokensDetails{
				CachedTokens: 5,
		REDACTED,
	REDACTED,
REDACTED, state)

	require.Len(t, events, 2)
	assert.Equal(t, "message_delta", events[0].Type)
	require.NotNil(t, events[0].Usage)
	assert.Equal(t, 15, events[0].Usage.InputTokens)
	assert.Equal(t, 5, events[0].Usage.CacheReadInputTokens)
	assert.Equal(t, 6, events[0].Usage.OutputTokens)
	assert.Equal(t, "message_stop", events[1].Type)
REDACTED

func TestResponsesEventToAnthropicEvents_ResponseDoneIncomplete(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "gpt-4o"

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.done",
		Response: &ResponsesResponse{
			Status:            "incomplete",
			IncompleteDetails: &ResponsesIncompleteDetails{Reason: "max_output_tokens"REDACTED,
			Usage:             &ResponsesUsage{InputTokens: 12, OutputTokens: 4REDACTED,
	REDACTED,
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, "max_tokens", events[0].Delta.StopReason)
	assert.Equal(t, "message_stop", events[1].Type)
	assert.Nil(t, FinalizeResponsesAnthropicStream(state))
REDACTED

func TestStreamingCachedTokensUseAnthropicInputSemantics(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_cached_stream", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage: &ResponsesUsage{
				InputTokens:  54006,
				OutputTokens: 123,
				TotalTokens:  54129,
				InputTokensDetails: &ResponsesInputTokensDetails{
					CachedTokens: 50688,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED, state)

	require.Len(t, events, 2)
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, 3318, events[0].Usage.InputTokens)
	assert.Equal(t, 50688, events[0].Usage.CacheReadInputTokens)
	assert.Equal(t, 123, events[0].Usage.OutputTokens)
	assert.Equal(t, "message_stop", events[1].Type)
REDACTED

func TestStreamingToolCall(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	// 1. response.created
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_2", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	// 2. function_call added
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "get_weather"REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)
	assert.Equal(t, "tool_use", events[0].ContentBlock.Type)
	assert.Equal(t, "call_1", events[0].ContentBlock.ID)

	// 3. arguments delta
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"city":`,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)
	assert.Equal(t, `{"city":`, events[0].Delta.PartialJSON)

	// 4. arguments done
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.function_call_arguments.done",
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	// 5. completed with tool_calls
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 20, OutputTokens: 10REDACTED,
	REDACTED,
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "tool_use", events[0].Delta.StopReason)
REDACTED

func TestStreamingToolCallStopReasonSurvivesLaterText(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_tool_then_text", Model: "gpt-5.5"REDACTED,
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_todo", Name: "TodoWrite"REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 0,
		Arguments:   `{"todos":[{"content":"review changes","status":"in_progress","activeForm":"reviewing changes"REDACTED]REDACTED`,
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "content_block_stop", events[1].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_text.delta",
		OutputIndex: 1,
		Delta:       "I will continue after the task list updates.",
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_start", events[0].Type)
	assert.Equal(t, "content_block_delta", events[1].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 20, OutputTokens: 10REDACTED,
	REDACTED,
REDACTED, state)
	require.Len(t, events, 3)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "tool_use", events[1].Delta.StopReason)
	assert.Equal(t, "message_stop", events[2].Type)
REDACTED

func TestStreamingToolCallDoneWithoutDeltaEmitsArguments(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_bash", Model: "gpt-5.5"REDACTED,
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_bash", Name: "Bash"REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 0,
		Arguments:   `{"command":"git -C \"/mnt/d/nodejs/other/edmt\" status --short --ignored"REDACTED`,
REDACTED, state)
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)
	assert.JSONEq(t, `{"command":"git -C \"/mnt/d/nodejs/other/edmt\" status --short --ignored"REDACTED`, events[0].Delta.PartialJSON)
	assert.Equal(t, "content_block_stop", events[1].Type)
REDACTED

func TestStreamingReadToolStreamsDeltas(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_read_stream", Model: "gpt-5.5"REDACTED,
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/demo.py","limit":2000,"offset":0,"pages":""REDACTED`,
REDACTED, state)
	require.Len(t, events, 1, "Read tool deltas must be streamed like any other tool")
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 0,
		Arguments:   `{"file_path":"/tmp/demo.py","limit":2000,"offset":0,"pages":""REDACTED`,
REDACTED, state)
	require.Len(t, events, 1, "after streaming deltas, .done should just close the block")
	assert.Equal(t, "content_block_stop", events[0].Type)
REDACTED

func TestStreamingReasoning(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_3", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	// reasoning item added
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning"REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)
	assert.Equal(t, "thinking", events[0].ContentBlock.Type)

	sse, err := ResponsesAnthropicEventToSSE(events[0])
REDACTED
	assert.Contains(t, sse, `"content_block":{"thinking":"","type":"thinking"REDACTED`)

	// reasoning text delta
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.delta",
		OutputIndex: 0,
		Delta:       "Let me think...",
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "thinking_delta", events[0].Delta.Type)
	assert.Equal(t, "Let me think...", events[0].Delta.Thinking)

	// reasoning done
	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.reasoning_summary_text.done",
REDACTED, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)
REDACTED

func TestStreamingIncomplete(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_4", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "Partial output...",
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.incomplete",
		Response: &ResponsesResponse{
			Status:            "incomplete",
			IncompleteDetails: &ResponsesIncompleteDetails{Reason: "max_output_tokens"REDACTED,
			Usage:             &ResponsesUsage{InputTokens: 100, OutputTokens: 4096REDACTED,
	REDACTED,
REDACTED, state)

	// Should close the text block + message_delta + message_stop
	require.Len(t, events, 3)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "message_delta", events[1].Type)
	assert.Equal(t, "max_tokens", events[1].Delta.StopReason)
	assert.Equal(t, "message_stop", events[2].Type)
REDACTED

func TestFinalizeStream_NeverStarted(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	events := FinalizeResponsesAnthropicStream(state)
	assert.Nil(t, events)
REDACTED

func TestFinalizeStream_AlreadyCompleted(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.MessageStopSent = true
	events := FinalizeResponsesAnthropicStream(state)
	assert.Nil(t, events)
REDACTED

func TestFinalizeStream_AbnormalTermination(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	// Simulate a stream that started but never completed
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_5", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "Interrupted...",
REDACTED, state)

	// Stream ends without response.completed
	events := FinalizeResponsesAnthropicStream(state)
	require.Len(t, events, 3) // content_block_stop + message_delta + message_stop
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "message_delta", events[1].Type)
	assert.Equal(t, "end_turn", events[1].Delta.StopReason)
	assert.Equal(t, "message_stop", events[2].Type)
REDACTED

func TestFinalizeStream_ToolCallAbnormalTerminationKeepsToolUseStopReason(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_tool_interrupted", Model: "gpt-5.5"REDACTED,
REDACTED, state)
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_todo", Name: "TodoWrite"REDACTED,
REDACTED, state)

	events := FinalizeResponsesAnthropicStream(state)
	require.Len(t, events, 3)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "message_delta", events[1].Type)
	assert.Equal(t, "tool_use", events[1].Delta.StopReason)
	assert.Equal(t, "message_stop", events[2].Type)
REDACTED

func TestStreamingEmptyResponse(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_6", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 5, OutputTokens: 0REDACTED,
	REDACTED,
REDACTED, state)

	require.Len(t, events, 2) // message_delta + message_stop
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, "end_turn", events[0].Delta.StopReason)
REDACTED

func TestResponsesAnthropicEventToSSE(t *testing.T) {
	evt := AnthropicStreamEvent{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:   "resp_1",
			Type: "message",
			Role: "assistant",
	REDACTED,
REDACTED
	sse, err := ResponsesAnthropicEventToSSE(evt)
REDACTED
	assert.Contains(t, sse, "event: message_start\n")
	assert.Contains(t, sse, "data: ")
	assert.Contains(t, sse, `"resp_1"`)
REDACTED

// ---------------------------------------------------------------------------
// response.failed tests
// ---------------------------------------------------------------------------

func TestStreamingFailed(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	// 1. response.created
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_fail_1", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	// 2. Some text output before failure
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "Partial output before failure",
REDACTED, state)

	// 3. response.failed
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.failed",
		Response: &ResponsesResponse{
			Status: "failed",
			Error:  &ResponsesError{Code: "server_error", Message: "Internal error"REDACTED,
			Usage:  &ResponsesUsage{InputTokens: 50, OutputTokens: 10REDACTED,
	REDACTED,
REDACTED, state)

	// Should close text block + message_delta + message_stop
	require.Len(t, events, 3)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "message_delta", events[1].Type)
	assert.Equal(t, "end_turn", events[1].Delta.StopReason)
	assert.Equal(t, 50, events[1].Usage.InputTokens)
	assert.Equal(t, 10, events[1].Usage.OutputTokens)
	assert.Equal(t, "message_stop", events[2].Type)
REDACTED

func TestStreamingFailedNoOutput(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	// 1. response.created
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_fail_2", Model: "gpt-5.2"REDACTED,
REDACTED, state)

	// 2. response.failed with no prior output
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.failed",
		Response: &ResponsesResponse{
			Status: "failed",
			Error:  &ResponsesError{Code: "rate_limit_error", Message: "Too many requests"REDACTED,
			Usage:  &ResponsesUsage{InputTokens: 20, OutputTokens: 0REDACTED,
	REDACTED,
REDACTED, state)

	// Should emit message_delta + message_stop (no block to close)
	require.Len(t, events, 2)
	assert.Equal(t, "message_delta", events[0].Type)
	assert.Equal(t, "end_turn", events[0].Delta.StopReason)
	assert.Equal(t, "message_stop", events[1].Type)
REDACTED

func TestResponsesToAnthropic_Failed(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_fail_3",
		Model:  "gpt-5.2",
		Status: "failed",
		Error:  &ResponsesError{Code: "server_error", Message: "Something went wrong"REDACTED,
		Output: []ResponsesOutput{REDACTED,
		Usage:  &ResponsesUsage{InputTokens: 30, OutputTokens: 0REDACTED,
REDACTED

	anth := ResponsesToAnthropic(resp, "claude-opus-4-6")
	// Failed status defaults to "end_turn" stop reason
	assert.Equal(t, "end_turn", AnthropicStopReasonString(anth.StopReason))
	// Should have at least an empty text block
	require.Len(t, anth.Content, 1)
	assert.Equal(t, "text", anth.Content[0].Type)
REDACTED

// ---------------------------------------------------------------------------
// thinking → reasoning conversion tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_ThinkingEnabled(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Thinking:  &AnthropicThinking{Type: "enabled", BudgetTokens: 10000REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	// thinking.type is ignored for effort; Codex bridge default medium applies.
	assert.Equal(t, "medium", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
	assert.Contains(t, resp.Include, "reasoning.encrypted_content")
	assert.NotContains(t, resp.Include, "reasoning.summary")
REDACTED

func TestAnthropicToResponses_ThinkingAdaptive(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Thinking:  &AnthropicThinking{Type: "adaptive", BudgetTokens: 5000REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	// thinking.type is ignored for effort; Codex bridge default medium applies.
	assert.Equal(t, "medium", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
	assert.NotContains(t, resp.Include, "reasoning.summary")
REDACTED

func TestAnthropicToResponses_ThinkingDisabled(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Thinking:  &AnthropicThinking{Type: "disabled"REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	// Default effort applies (medium) even when thinking is disabled.
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "medium", resp.Reasoning.Effort)
REDACTED

func TestAnthropicToResponses_NoThinking(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	// Default effort applies (medium) when no thinking/output_config is set.
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "medium", resp.Reasoning.Effort)
REDACTED

// ---------------------------------------------------------------------------
// output_config.effort override tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_OutputConfigOverridesDefault(t *testing.T) {
	// Default is medium, but output_config.effort="low" overrides. low→low after mapping.
	req := &AnthropicRequest{
		Model:        "gpt-5.2",
		MaxTokens:    1024,
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Thinking:     &AnthropicThinking{Type: "enabled", BudgetTokens: 10000REDACTED,
		OutputConfig: &AnthropicOutputConfig{Effort: "low"REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "low", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
REDACTED

func TestAnthropicToResponses_OutputConfigWithoutThinking(t *testing.T) {
	// No thinking field, but output_config.effort="medium" → creates reasoning.
	// medium→medium after 1:1 mapping.
	req := &AnthropicRequest{
		Model:        "gpt-5.2",
		MaxTokens:    1024,
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		OutputConfig: &AnthropicOutputConfig{Effort: "medium"REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "medium", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
REDACTED

func TestAnthropicToResponses_OutputConfigHigh(t *testing.T) {
	// output_config.effort="high" → mapped to "high" (1:1).
	req := &AnthropicRequest{
		Model:        "gpt-5.2",
		MaxTokens:    1024,
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		OutputConfig: &AnthropicOutputConfig{Effort: "high"REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "high", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
REDACTED

func TestAnthropicToResponses_OutputConfigMax(t *testing.T) {
	// output_config.effort="max" → mapped to OpenAI's highest supported level "xhigh".
	req := &AnthropicRequest{
		Model:        "gpt-5.2",
		MaxTokens:    1024,
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		OutputConfig: &AnthropicOutputConfig{Effort: "max"REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "xhigh", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
REDACTED

func TestAnthropicToResponses_NoOutputConfig(t *testing.T) {
	// No output_config → default medium regardless of thinking.type.
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Thinking:  &AnthropicThinking{Type: "enabled", BudgetTokens: 10000REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "medium", resp.Reasoning.Effort)
REDACTED

func TestAnthropicToResponses_OutputConfigWithoutEffort(t *testing.T) {
	// output_config present but effort empty (e.g. only format set) → default medium.
	req := &AnthropicRequest{
		Model:        "gpt-5.2",
		MaxTokens:    1024,
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		OutputConfig: &AnthropicOutputConfig{REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "medium", resp.Reasoning.Effort)
REDACTED

// ---------------------------------------------------------------------------
// tool_choice conversion tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_ToolChoiceAuto(t *testing.T) {
	req := &AnthropicRequest{
		Model:      "gpt-5.2",
		MaxTokens:  1024,
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		ToolChoice: json.RawMessage(`{"type":"auto"REDACTED`),
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var tc string
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "auto", tc)
REDACTED

func TestAnthropicToResponses_ToolChoiceAny(t *testing.T) {
	req := &AnthropicRequest{
		Model:      "gpt-5.2",
		MaxTokens:  1024,
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		ToolChoice: json.RawMessage(`{"type":"any"REDACTED`),
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var tc string
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "required", tc)
REDACTED

func TestAnthropicToResponses_ToolChoiceSpecific(t *testing.T) {
	req := &AnthropicRequest{
		Model:      "gpt-5.2",
		MaxTokens:  1024,
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		ToolChoice: json.RawMessage(`{"type":"tool","name":"get_weather"REDACTED`),
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var tc map[string]any
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "function", tc["type"])
	assert.Equal(t, "get_weather", tc["name"])
	assert.NotContains(t, tc, "function")
REDACTED

func TestResponsesToAnthropicRequest_ToolChoiceFunctionName(t *testing.T) {
	req := &ResponsesRequest{
		Model:      "gpt-5.2",
		Input:      json.RawMessage(`[{"role":"user","content":"Hello"REDACTED]`),
		ToolChoice: json.RawMessage(`{"type":"function","name":"get_weather"REDACTED`),
REDACTED

	resp, err := ResponsesToAnthropicRequest(req)
REDACTED

	var tc map[string]string
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "tool", tc["type"])
	assert.Equal(t, "get_weather", tc["name"])
REDACTED

func TestResponsesToAnthropicRequest_ToolChoiceLegacyFunctionName(t *testing.T) {
	req := &ResponsesRequest{
		Model:      "gpt-5.2",
		Input:      json.RawMessage(`[{"role":"user","content":"Hello"REDACTED]`),
		ToolChoice: json.RawMessage(`{"type":"function","function":{"name":"get_weather"REDACTEDREDACTED`),
REDACTED

	resp, err := ResponsesToAnthropicRequest(req)
REDACTED

	var tc map[string]string
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "tool", tc["type"])
	assert.Equal(t, "get_weather", tc["name"])
REDACTED

// ---------------------------------------------------------------------------
// Image content block conversion tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_UserImageBlock(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"text","text":"What is in this image?"REDACTED,
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBOR"REDACTEDREDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "user", items[0].Role)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 2)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "What is in this image?", parts[0].Text)
	assert.Equal(t, "input_image", parts[1].Type)
	assert.Equal(t, "data:image/png;base64,iVBOR", parts[1].ImageURL)
REDACTED

func TestAnthropicToResponses_ImageOnlyUserMessage(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"/9j/4AAQ"REDACTEDREDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_image", parts[0].Type)
	assert.Equal(t, "data:image/jpeg;base64,/9j/4AAQ", parts[0].ImageURL)
REDACTED

func TestAnthropicToResponses_ToolResultWithImage(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Read the screenshot"`)REDACTED,
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/tmp/screen.png"REDACTEDREDACTED]`)REDACTED,
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"toolu_1","content":[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBOR"REDACTEDREDACTED
				]REDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + function_call + function_call_output + user(image) = 4
	require.Len(t, items, 4)

	// function_call_output should have text-only output (no image).
	assert.Equal(t, "function_call_output", items[2].Type)
	assert.Equal(t, "toolu_1", items[2].CallID)
	assert.Equal(t, "(empty)", items[2].Output)

	// Image should be in a separate user message.
	assert.Equal(t, "user", items[3].Role)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[3].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_image", parts[0].Type)
	assert.Equal(t, "data:image/png;base64,iVBOR", parts[0].ImageURL)
REDACTED

func TestAnthropicToResponses_ToolResultMixed(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Describe the file"`)REDACTED,
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"/tmp/photo.png"REDACTEDREDACTED]`)REDACTED,
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"toolu_2","content":[
					{"type":"text","text":"File metadata: 800x600 PNG"REDACTED,
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"REDACTEDREDACTED
				]REDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + function_call + function_call_output + user(image) = 4
	require.Len(t, items, 4)

	// function_call_output should have text-only output.
	assert.Equal(t, "function_call_output", items[2].Type)
	assert.Equal(t, "File metadata: 800x600 PNG", items[2].Output)

	// Image should be in a separate user message.
	assert.Equal(t, "user", items[3].Role)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[3].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_image", parts[0].Type)
	assert.Equal(t, "data:image/png;base64,AAAA", parts[0].ImageURL)
REDACTED

func TestAnthropicToResponses_TextOnlyToolResultBackwardCompat(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Check weather"`)REDACTED,
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"get_weather","input":{"city":"NYC"REDACTEDREDACTED]`)REDACTED,
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","content":[
					{"type":"text","text":"Sunny, 72°F"REDACTED
				]REDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + function_call + function_call_output = 3
	require.Len(t, items, 3)

	// Text-only tool_result should produce a plain string.
	assert.Equal(t, "Sunny, 72°F", items[2].Output)
REDACTED

func TestAnthropicToResponses_ImageEmptyMediaType(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"image","source":{"type":"base64","media_type":"","data":"iVBOR"REDACTEDREDACTED
			]`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_image", parts[0].Type)
	// Should default to image/png when media_type is empty.
	assert.Equal(t, "data:image/png;base64,iVBOR", parts[0].ImageURL)
REDACTED

// ---------------------------------------------------------------------------
// normalizeToolParameters tests
// ---------------------------------------------------------------------------

func TestNormalizeToolParameters(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
REDACTED{
		{
			name:     "nil input",
			input:    nil,
			expected: `{"type":"object","properties":{REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "empty input",
			input:    json.RawMessage(``),
			expected: `{"type":"object","properties":{REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "null input",
			input:    json.RawMessage(`null`),
			expected: `{"type":"object","properties":{REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "object without properties",
			input:    json.RawMessage(`{"type":"object"REDACTED`),
			expected: `{"type":"object","properties":{REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "object with properties",
			input:    json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"REDACTEDREDACTEDREDACTED`),
			expected: `{"type":"object","properties":{"city":{"type":"string"REDACTEDREDACTEDREDACTED`,
	REDACTED,
		{
			name:     "non-object type",
			input:    json.RawMessage(`{"type":"string"REDACTED`),
			expected: `{"type":"string"REDACTED`,
	REDACTED,
		{
			name:     "object with additional fields preserved",
			input:    json.RawMessage(`{"type":"object","required":["name"]REDACTED`),
			expected: `{"type":"object","required":["name"],"properties":{REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "invalid JSON passthrough",
			input:    json.RawMessage(`not json`),
			expected: `not json`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeToolParameters(tt.input)
			if tt.name == "invalid JSON passthrough" {
				assert.Equal(t, tt.expected, string(result))
		REDACTED else {
				assert.JSONEq(t, tt.expected, string(result))
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAnthropicToResponses_ToolWithoutProperties(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTED,
	REDACTED,
		Tools: []AnthropicTool{
			{Name: "mcp__pencil__get_style_guide_tags", Description: "Get style tags", InputSchema: json.RawMessage(`{"type":"object"REDACTED`)REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	require.Len(t, resp.Tools, 1)
	assert.Equal(t, "function", resp.Tools[0].Type)
	assert.Equal(t, "mcp__pencil__get_style_guide_tags", resp.Tools[0].Name)

	// Parameters must have "properties" field after normalization.
	var params map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Tools[0].Parameters, &params))
	assert.Contains(t, params, "properties")
REDACTED

func TestAnthropicToResponses_ToolWithNilSchema(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTED,
	REDACTED,
		Tools: []AnthropicTool{
			{Name: "simple_tool", Description: "A tool"REDACTED,
	REDACTED,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED

	require.Len(t, resp.Tools, 1)
	var params map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Tools[0].Parameters, &params))
	assert.JSONEq(t, `"object"`, string(params["type"]))
	assert.JSONEq(t, `{REDACTED`, string(params["properties"]))
REDACTED

// ---------------------------------------------------------------------------
// isReasoningModel / temperature-stripping tests
// ---------------------------------------------------------------------------

func TestAnthropicToResponses_TemperatureStrippedForReasoningModel(t *testing.T) {
	temp := 0.7
	req := &AnthropicRequest{
		Model:       "gpt-5.2",
		MaxTokens:   1024,
		Messages:    []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
		Temperature: &temp,
		TopP:        &temp,
REDACTED

	resp, err := AnthropicToResponses(req)
REDACTED
	assert.Nil(t, resp.Temperature, "reasoning model: temperature must be stripped")
	assert.Nil(t, resp.TopP, "reasoning model: top_p must be stripped")

	// Verify the fields are absent from the serialised JSON.
	b, err := json.Marshal(resp)
REDACTED
	assert.NotContains(t, string(b), `"temperature"`)
	assert.NotContains(t, string(b), `"top_p"`)
REDACTED

func TestAnthropicToResponses_TemperatureStrippedForAllGpt5Variants(t *testing.T) {
	temp := 1.0
	models := []string{"gpt-5.2", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.5"REDACTED
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:       model,
				MaxTokens:   1024,
				Messages:    []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)REDACTEDREDACTED,
				Temperature: &temp,
				TopP:        &temp,
		REDACTED
			resp, err := AnthropicToResponses(req)
		REDACTED
			assert.Nil(t, resp.Temperature, "model %s: temperature must be stripped", model)
			assert.Nil(t, resp.TopP, "model %s: top_p must be stripped", model)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// AnthropicToResponsesResponse: Anthropic input_tokens excludes cached tokens
// while OpenAI Responses input_tokens is the total including cached tokens.
// ---------------------------------------------------------------------------

func TestAnthropicToResponsesResponse_CacheTokensUseOpenAIInputSemantics(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_cache",
		Model: "claude-sonnet-4-5-20250929",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "ok"REDACTED,
	REDACTED,
		StopReason: AnthropicStopReasonPtr("end_turn"),
		Usage: AnthropicUsage{
			InputTokens:              3318,
			OutputTokens:             123,
			CacheReadInputTokens:     50688,
			CacheCreationInputTokens: 200,
	REDACTED,
REDACTED

	out := AnthropicToResponsesResponse(resp)
	require.NotNil(t, out.Usage)
	// 3318 (uncached) + 50688 (read) + 200 (creation) = 54206
	assert.Equal(t, 54206, out.Usage.InputTokens)
	assert.Equal(t, 123, out.Usage.OutputTokens)
	assert.Equal(t, 54329, out.Usage.TotalTokens)
	require.NotNil(t, out.Usage.InputTokensDetails)
	assert.Equal(t, 50688, out.Usage.InputTokensDetails.CachedTokens)
REDACTED

func TestAnthropicToResponsesResponse_NoCacheTokens(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_nocache",
		Model: "claude-sonnet-4-5-20250929",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "ok"REDACTED,
	REDACTED,
		StopReason: AnthropicStopReasonPtr("end_turn"),
		Usage: AnthropicUsage{
			InputTokens:  100,
			OutputTokens: 50,
	REDACTED,
REDACTED

	out := AnthropicToResponsesResponse(resp)
	require.NotNil(t, out.Usage)
	assert.Equal(t, 100, out.Usage.InputTokens)
	assert.Equal(t, 50, out.Usage.OutputTokens)
	assert.Equal(t, 150, out.Usage.TotalTokens)
	assert.Nil(t, out.Usage.InputTokensDetails)
REDACTED

func TestAnthropicEventToResponses_CacheTokensRoundTripFromMessageStart(t *testing.T) {
	state := NewAnthropicEventToResponsesState()

	// message_start carries cache fields on the initial Usage object.
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:    "msg_stream_cache",
			Model: "claude-sonnet-4-5-20250929",
			Usage: AnthropicUsage{
				InputTokens:              12,
				CacheReadInputTokens:     9,
				CacheCreationInputTokens: 3,
		REDACTED,
	REDACTED,
REDACTED, state)

	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_delta",
		Usage: &AnthropicUsage{
			OutputTokens: 7,
	REDACTED,
REDACTED, state)

	events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"REDACTED, state)

	// The terminal response.completed event must include OpenAI-semantic usage.
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
	REDACTED
REDACTED
	require.NotNil(t, completed, "response.completed event must be emitted")
	require.NotNil(t, completed.Response)
	require.NotNil(t, completed.Response.Usage)
	// 12 (uncached) + 9 (read) + 3 (creation) = 24
	assert.Equal(t, 24, completed.Response.Usage.InputTokens)
	assert.Equal(t, 7, completed.Response.Usage.OutputTokens)
	assert.Equal(t, 31, completed.Response.Usage.TotalTokens)
	require.NotNil(t, completed.Response.Usage.InputTokensDetails)
	assert.Equal(t, 9, completed.Response.Usage.InputTokensDetails.CachedTokens)
REDACTED

func TestAnthropicEventToResponses_CacheTokensFromMessageDelta(t *testing.T) {
	state := NewAnthropicEventToResponsesState()

	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:    "msg_delta_cache",
			Model: "claude-sonnet-4-5-20250929",
			Usage: AnthropicUsage{InputTokens: 20REDACTED,
	REDACTED,
REDACTED, state)

	// Some upstreams only emit cache fields on the final message_delta.
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_delta",
		Usage: &AnthropicUsage{
			OutputTokens:             8,
			CacheReadInputTokens:     11,
			CacheCreationInputTokens: 4,
	REDACTED,
REDACTED, state)

	events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"REDACTED, state)

	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
	REDACTED
REDACTED
	require.NotNil(t, completed)
	require.NotNil(t, completed.Response.Usage)
	// 20 (uncached) + 11 (read) + 4 (creation) = 35
	assert.Equal(t, 35, completed.Response.Usage.InputTokens)
	assert.Equal(t, 8, completed.Response.Usage.OutputTokens)
	require.NotNil(t, completed.Response.Usage.InputTokensDetails)
	assert.Equal(t, 11, completed.Response.Usage.InputTokensDetails.CachedTokens)
REDACTED

func TestMessageStartSSE_StopReasonIsJSONNull(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "grok-4.5"
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.created",
		Response: &ResponsesResponse{
			ID:    "resp_test",
			Model: "grok-4.5",
	REDACTED,
REDACTED, state)
	require.Len(t, events, 1)
	require.Equal(t, "message_start", events[0].Type)
	require.NotNil(t, events[0].Message)
	require.Nil(t, events[0].Message.StopReason, "message_start must use null stop_reason, not empty string")

	sse, err := ResponsesAnthropicEventToSSE(events[0])
REDACTED
	// Official Anthropic wire: "stop_reason":null
	require.Contains(t, sse, `"stop_reason":null`)
	require.NotContains(t, sse, `"stop_reason":""`)
REDACTED
