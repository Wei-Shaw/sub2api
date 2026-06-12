//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseGatewayRequest(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","stream":true,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"REDACTED,"system":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	require.Equal(t, "claude-3-7-sonnet", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "session_123e4567-e89b-12d3-a456-426614174000", parsed.MetadataUserID)
	require.True(t, parsed.HasSystem)
	require.NotEmpty(t, parsed.SystemRaw())
	require.NotEmpty(t, parsed.MessagesRaw())
	require.False(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_ThinkingEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"enabled"REDACTED,"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_ThinkingAdaptiveEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"adaptive"REDACTED,"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_MaxTokens(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	require.Equal(t, 1, parsed.MaxTokens)
REDACTED

func TestParseGatewayRequest_MaxTokensNonIntegralIgnored(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1.5REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	require.Equal(t, 0, parsed.MaxTokens)
REDACTED

func TestParseGatewayRequest_SystemNull(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":nullREDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
	// 显式传入 system:null 也应视为“字段已存在”，避免默认 system 被注入。
	require.True(t, parsed.HasSystem)
	require.Equal(t, []byte("null"), parsed.SystemRaw())
REDACTED

func TestParseGatewayRequest_InvalidModelType(t *testing.T) {
	body := []byte(`{"model":123REDACTED`)
	_, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
REDACTED

func TestParseGatewayRequest_InvalidStreamType(t *testing.T) {
	body := []byte(`{"stream":"true"REDACTED`)
	_, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
REDACTED
REDACTED

func TestParseGatewayRequest_ResponsesInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"REDACTED]REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "responses")
REDACTED
	require.NotEmpty(t, parsed.InputRaw())
	require.Nil(t, parsed.MessagesRaw())
	require.Equal(t, "hello", gjson.ParseBytes(parsed.InputRaw()).Get("0.content.0.text").String())
REDACTED

// ============ Gemini 原生格式解析测试 ============

func TestParseGatewayRequest_GeminiContents(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "Hello"REDACTED]REDACTED,
			{"role": "model", "parts": [{"text": "Hi there"REDACTED]REDACTED,
			{"role": "user", "parts": [{"text": "How are you?"REDACTED]REDACTED
		]
REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	require.Len(t, gjson.ParseBytes(parsed.MessagesRaw()).Array(), 3, "should parse contents as Messages")
	require.False(t, parsed.HasSystem, "Gemini format should not set HasSystem")
	require.Nil(t, parsed.SystemRaw(), "no systemInstruction means nil System")
REDACTED

func TestParseGatewayRequest_GeminiSystemInstruction(t *testing.T) {
	body := []byte(`{
		"systemInstruction": {
			"parts": [{"text": "You are a helpful assistant."REDACTED]
	REDACTED,
		"contents": [
			{"role": "user", "parts": [{"text": "Hello"REDACTED]REDACTED
		]
REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	system := gjson.ParseBytes(parsed.SystemRaw())
	require.True(t, system.IsArray(), "should parse systemInstruction.parts as System")
	require.Len(t, system.Array(), 1)
	require.Equal(t, "You are a helpful assistant.", system.Get("0.text").String())
	require.Len(t, gjson.ParseBytes(parsed.MessagesRaw()).Array(), 1)
REDACTED

func TestParseGatewayRequest_GeminiWithModel(t *testing.T) {
	body := []byte(`{
		"model": "gemini-2.5-pro",
		"contents": [{"role": "user", "parts": [{"text": "test"REDACTED]REDACTED]
REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	require.Equal(t, "gemini-2.5-pro", parsed.Model)
	require.Len(t, gjson.ParseBytes(parsed.MessagesRaw()).Array(), 1)
REDACTED

func TestParseGatewayRequest_GeminiIgnoresAnthropicFields(t *testing.T) {
	// Gemini 格式下 system/messages 字段应被忽略
	body := []byte(`{
		"system": "should be ignored",
		"messages": [{"role": "user", "content": "ignored"REDACTED],
		"contents": [{"role": "user", "parts": [{"text": "real content"REDACTED]REDACTED]
REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	require.False(t, parsed.HasSystem, "Gemini protocol should not parse Anthropic system field")
	require.Nil(t, parsed.SystemRaw(), "no systemInstruction = nil System")
	require.Len(t, gjson.ParseBytes(parsed.MessagesRaw()).Array(), 1, "should use contents, not messages")
REDACTED

func TestParseGatewayRequest_GeminiEmptyContents(t *testing.T) {
	body := []byte(`{"contents": []REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	require.Empty(t, gjson.ParseBytes(parsed.MessagesRaw()).Array())
REDACTED

func TestParseGatewayRequest_GeminiNoContents(t *testing.T) {
	body := []byte(`{"model": "gemini-2.5-flash"REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
REDACTED
	require.Nil(t, parsed.MessagesRaw())
	require.Equal(t, "gemini-2.5-flash", parsed.Model)
REDACTED

func TestParseGatewayRequest_AnthropicIgnoresGeminiFields(t *testing.T) {
	// Anthropic 格式下 contents/systemInstruction 字段应被忽略
	body := []byte(`{
		"system": "real system",
		"messages": [{"role": "user", "content": "real content"REDACTED],
		"contents": [{"role": "user", "parts": [{"text": "ignored"REDACTED]REDACTED],
		"systemInstruction": {"parts": [{"text": "ignored"REDACTED]REDACTED
REDACTED`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
REDACTED
	require.True(t, parsed.HasSystem)
	require.Equal(t, "real system", gjson.ParseBytes(parsed.SystemRaw()).String())
	messages := gjson.ParseBytes(parsed.MessagesRaw()).Array()
	require.Len(t, messages, 1)
	require.Equal(t, "real content", messages[0].Get("content").String())
REDACTED

func TestFilterThinkingBlocks(t *testing.T) {
	containsThinkingBlock := func(body []byte) bool {
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			return false
	REDACTED
		messages, ok := req["messages"].([]any)
		if !ok {
			return false
	REDACTED
		for _, msg := range messages {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
		REDACTED
			content, ok := msgMap["content"].([]any)
			if !ok {
				continue
		REDACTED
			for _, block := range content {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
			REDACTED
				blockType, _ := blockMap["type"].(string)
				if blockType == "thinking" {
					return true
			REDACTED
				if blockType == "" {
					if _, hasThinking := blockMap["thinking"]; hasThinking {
						return true
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		return false
REDACTED

	tests := []struct {
		name         string
		input        string
		shouldFilter bool
		expectError  bool
REDACTED{
		{
			name:         "filters thinking blocks",
			input:        `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":[{"type":"text","text":"Hello"REDACTED,{"type":"thinking","thinking":"internal","signature":"invalid"REDACTED,{"type":"text","text":"World"REDACTED]REDACTED]REDACTED`,
			shouldFilter: true,
	REDACTED,
		{
			name:         "does not filter signed thinking blocks when thinking adaptive",
			input:        `{"thinking":{"type":"adaptive"REDACTED,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"ok","signature":"sig_real_123"REDACTED,{"type":"text","text":"B"REDACTED]REDACTED]REDACTED`,
			shouldFilter: false,
	REDACTED,
		{
			name:         "filters unsigned thinking blocks when thinking adaptive",
			input:        `{"thinking":{"type":"adaptive"REDACTED,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"internal","signature":""REDACTED,{"type":"text","text":"B"REDACTED]REDACTED]REDACTED`,
			shouldFilter: true,
	REDACTED,
		{
			name:         "handles no thinking blocks",
			input:        `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":[{"type":"text","text":"Hello"REDACTED]REDACTED]REDACTED`,
			shouldFilter: false,
	REDACTED,
		{
			name:         "handles invalid JSON gracefully",
			input:        `{invalid json`,
			shouldFilter: false,
			expectError:  true,
	REDACTED,
		{
			name:         "handles multiple messages with thinking blocks",
			input:        `{"messages":[{"role":"user","content":[{"type":"text","text":"A"REDACTED]REDACTED,{"role":"assistant","content":[{"type":"thinking","thinking":"think"REDACTED,{"type":"text","text":"B"REDACTED]REDACTED]REDACTED`,
			shouldFilter: true,
	REDACTED,
		{
			name:         "filters thinking blocks without type discriminator",
			input:        `{"messages":[{"role":"assistant","content":[{"thinking":{"text":"internal"REDACTEDREDACTED,{"type":"text","text":"B"REDACTED]REDACTED]REDACTED`,
			shouldFilter: true,
	REDACTED,
		{
			name:         "does not filter tool_use input fields named thinking",
			input:        `{"messages":[{"role":"user","content":[{"type":"tool_use","id":"t1","name":"foo","input":{"thinking":"keepme","x":1REDACTEDREDACTED,{"type":"text","text":"Hello"REDACTED]REDACTED]REDACTED`,
			shouldFilter: false,
	REDACTED,
		{
			name:         "handles empty messages array",
			input:        `{"messages":[]REDACTED`,
			shouldFilter: false,
	REDACTED,
		{
			name:         "handles missing messages field",
			input:        `{"model":"claude-3"REDACTED`,
			shouldFilter: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterThinkingBlocks([]byte(tt.input), "claude-sonnet-4-5")

			if tt.expectError {
				// For invalid JSON, should return original
				require.Equal(t, tt.input, string(result))
				return
		REDACTED

			if tt.shouldFilter {
				require.False(t, containsThinkingBlock(result))
		REDACTED else {
				// Ensure we don't rewrite JSON when no filtering is needed.
				require.Equal(t, tt.input, string(result))
		REDACTED

			// Verify valid JSON returned (unless input was invalid)
			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestFilterThinkingBlocksForRetry_DisablesThinkingAndPreservesAsText(t *testing.T) {
	input := []byte(`{
		"model":"claude-3-5-sonnet-20241022",
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hi"REDACTED]REDACTED,
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"Let me think...","signature":"bad_sig"REDACTED,
				{"type":"text","text":"Answer"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	require.Len(t, msgs, 2)

	assistant, ok := msgs[1].(map[string]any)
	require.True(t, ok)
	content, ok := assistant["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)

	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", first["type"])
	require.Equal(t, "Let me think...", first["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_DisablesThinkingEvenWithoutThinkingBlocks(t *testing.T) {
	input := []byte(`{
		"model":"claude-3-5-sonnet-20241022",
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hi"REDACTED]REDACTED,
			{"role":"assistant","content":[{"type":"text","text":"Prefill"REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)
REDACTED

func TestFilterThinkingBlocksForRetry_RemovesRedactedThinkingAndKeepsValidContent(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"redacted_thinking","data":"..."REDACTED,
				{"type":"text","text":"Visible"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.Equal(t, "Visible", content0["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_DropsThinkingBlockWithEmptyContent(t *testing.T) {
	// 跨模型场景：其他模型回过的 assistant 历史里携带了 type=thinking 但 thinking 字段为空，
	// 喂给开启 extended thinking 的 claude 时上游会报：
	//   "messages.1.content.0.thinking: each thinking block must contain thinking"
	// 重试应当把空 thinking 块丢弃，并保留其它有效内容。
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hi"REDACTED]REDACTED,
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"","signature":"sig"REDACTED,
				{"type":"text","text":"Answer"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "top-level thinking should be removed")

	msgs := req["messages"].([]any)
	assistant := msgs[1].(map[string]any)
	content := assistant["content"].([]any)
	require.Len(t, content, 1, "empty thinking block should be dropped, only text remains")
	require.Equal(t, "text", content[0].(map[string]any)["type"])
	require.Equal(t, "Answer", content[0].(map[string]any)["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_EmptyContentGetsPlaceholder(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled"REDACTED,
		"messages":[
			{"role":"assistant","content":[{"type":"redacted_thinking","data":"..."REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.NotEmpty(t, content0["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_StripsEmptyTextBlocks(t *testing.T) {
	// Empty text blocks cause upstream 400: "text content blocks must be non-empty"
	input := []byte(`{
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"REDACTED,{"type":"text","text":""REDACTED]REDACTED,
			{"role":"assistant","content":[{"type":"text","text":""REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs, ok := req["messages"].([]any)
	require.True(t, ok)

	// First message: empty text block stripped, "hello" preserved
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 1)
	require.Equal(t, "hello", content0[0].(map[string]any)["text"])

	// Second message: only had empty text block → gets placeholder
	msg1 := msgs[1].(map[string]any)
	content1 := msg1["content"].([]any)
	require.Len(t, content1, 1)
	block1 := content1[0].(map[string]any)
	require.Equal(t, "text", block1["type"])
	require.NotEmpty(t, block1["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_StripsNestedEmptyTextInToolResult(t *testing.T) {
	// Empty text blocks nested inside tool_result content should also be stripped
	input := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"t1","content":[
					{"type":"text","text":"valid result"REDACTED,
					{"type":"text","text":""REDACTED
				]REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs := req["messages"].([]any)
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 1)
	toolResult := content0[0].(map[string]any)
	require.Equal(t, "tool_result", toolResult["type"])
	nestedContent := toolResult["content"].([]any)
	require.Len(t, nestedContent, 1)
	require.Equal(t, "valid result", nestedContent[0].(map[string]any)["text"])
REDACTED

func TestFilterThinkingBlocksForRetry_NestedAllEmptyGetsEmptySlice(t *testing.T) {
	// If all nested content blocks in tool_result are empty text, content becomes empty slice
	input := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"t1","content":[
					{"type":"text","text":""REDACTED
				]REDACTED,
				{"type":"text","text":"hello"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs := req["messages"].([]any)
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 2)
	toolResult := content0[0].(map[string]any)
	nestedContent := toolResult["content"].([]any)
	require.Len(t, nestedContent, 0)
REDACTED

func TestStripEmptyTextBlocks(t *testing.T) {
	t.Run("strips top-level empty text", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED,{"type":"text","text":""REDACTED]REDACTED]REDACTED`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		require.Len(t, content, 1)
		require.Equal(t, "hello", content[0].(map[string]any)["text"])
REDACTED)

	t.Run("strips nested empty text in tool_result", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok"REDACTED,{"type":"text","text":""REDACTED]REDACTED]REDACTED]REDACTED`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		toolResult := content[0].(map[string]any)
		nestedContent := toolResult["content"].([]any)
		require.Len(t, nestedContent, 1)
		require.Equal(t, "ok", nestedContent[0].(map[string]any)["text"])
REDACTED)

	t.Run("no-op when no empty text", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED]REDACTED]REDACTED`)
		out := StripEmptyTextBlocks(input)
		require.Equal(t, input, out)
REDACTED)

	t.Run("preserves non-map blocks in content", func(t *testing.T) {
		// tool_result content can be a string; non-map blocks should pass through unchanged
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"string content"REDACTED,{"type":"text","text":""REDACTED]REDACTED]REDACTED`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		require.Len(t, content, 1)
		toolResult := content[0].(map[string]any)
		require.Equal(t, "tool_result", toolResult["type"])
		require.Equal(t, "string content", toolResult["content"])
REDACTED)

	t.Run("handles deeply nested tool_result", func(t *testing.T) {
		// Recursive: tool_result containing another tool_result with empty text
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"tool_result","tool_use_id":"t2","content":[{"type":"text","text":""REDACTED,{"type":"text","text":"deep"REDACTED]REDACTED]REDACTED]REDACTED]REDACTED`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		outer := content[0].(map[string]any)
		innerContent := outer["content"].([]any)
		inner := innerContent[0].(map[string]any)
		deepContent := inner["content"].([]any)
		require.Len(t, deepContent, 1)
		require.Equal(t, "deep", deepContent[0].(map[string]any)["text"])
REDACTED)
REDACTED

func TestFilterThinkingBlocksForRetry_PreservesNonEmptyTextBlocks(t *testing.T) {
	// Non-empty text blocks should pass through unchanged
	input := []byte(`{
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"REDACTED,{"type":"text","text":"world"REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	// Fast path: no thinking content, no empty content, no empty text blocks → unchanged
	require.Equal(t, input, out)
REDACTED

func TestFilterSignatureSensitiveBlocksForRetry_DowngradesTools(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"REDACTEDREDACTED,
				{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":falseREDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterSignatureSensitiveBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	content1, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.Equal(t, "text", content1["type"])
	require.Contains(t, content0["text"], "tool_use")
	require.Contains(t, content1["text"], "tool_result")
REDACTED

// ============ Group 6b: context_management.edits 清理测试 ============

// removeThinkingDependentContextStrategies — 边界用例

func TestRemoveThinkingDependentContextStrategies_NoContextManagement(t *testing.T) {
	input := []byte(`{"thinking":{"type":"enabled"REDACTED,"messages":[]REDACTED`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "无 context_management 字段时应原样返回")
REDACTED

func TestRemoveThinkingDependentContextStrategies_EmptyEdits(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[]REDACTED,"messages":[]REDACTED`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "edits 为空数组时应原样返回")
REDACTED

func TestRemoveThinkingDependentContextStrategies_NoClearThinkingEntry(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"other_strategy"REDACTED]REDACTED,"messages":[]REDACTED`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "edits 中无 clear_thinking_20251015 时应原样返回")
REDACTED

func TestRemoveThinkingDependentContextStrategies_RemovesSingleEntry(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED]REDACTED,"messages":[]REDACTED`)
	out := removeThinkingDependentContextStrategies(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	_, hasEdits := cm["edits"]
	require.False(t, hasEdits, "所有 edits 均为 clear_thinking_20251015 时应删除 edits 键")
REDACTED

func TestRemoveThinkingDependentContextStrategies_MixedEntries(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED,{"type":"other_strategy","param":1REDACTED]REDACTED,"messages":[]REDACTED`)
	out := removeThinkingDependentContextStrategies(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留其他条目")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "other_strategy", edit0["type"])
REDACTED

// FilterThinkingBlocksForRetry — 包含 context_management 的场景

func TestFilterThinkingBlocksForRetry_RemovesClearThinkingStrategy_FastPath(t *testing.T) {
	// 快速路径：messages 中无 thinking 块，仅有顶层 thinking 字段
	// 这条路径曾因提前 return 跳过 removeThinkingDependentContextStrategies 而存在 bug
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED]REDACTED,
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hello"REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	_, hasEdits := cm["edits"]
	require.False(t, hasEdits, "fast path 下 clear_thinking_20251015 应被移除，edits 键应被删除")
REDACTED

func TestFilterThinkingBlocksForRetry_RemovesClearThinkingStrategy_WithThinkingBlocks(t *testing.T) {
	// 完整路径：messages 中有 thinking 块（非 fast path）
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED,{"type":"keep_this"REDACTED]REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"some thought","signature":"sig"REDACTED,
				{"type":"text","text":"Answer"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留 keep_this")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "keep_this", edit0["type"])
REDACTED

func TestFilterThinkingBlocksForRetry_NoContextManagement_Unaffected(t *testing.T) {
	// 无 context_management 时不应报错，且 thinking 正常被移除
	input := []byte(`{
		"thinking":{"type":"enabled"REDACTED,
		"messages":[{"role":"user","content":[{"type":"text","text":"Hi"REDACTED]REDACTED]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)
	_, hasCM := req["context_management"]
	require.False(t, hasCM)
REDACTED

// FilterSignatureSensitiveBlocksForRetry — 包含 context_management 的场景

func TestFilterSignatureSensitiveBlocksForRetry_RemovesClearThinkingStrategy(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024REDACTED,
		"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED]REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"thought","signature":"sig"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterSignatureSensitiveBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	if rawEdits, hasEdits := cm["edits"]; hasEdits {
		edits, ok := rawEdits.([]any)
		require.True(t, ok)
		for _, e := range edits {
			em, ok := e.(map[string]any)
			require.True(t, ok)
			require.NotEqual(t, "clear_thinking_20251015", em["type"], "clear_thinking_20251015 应被移除")
	REDACTED
REDACTED
REDACTED

func TestFilterSignatureSensitiveBlocksForRetry_PreservesNonThinkingStrategies(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled"REDACTED,
		"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED,{"type":"other_edit"REDACTED]REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"t","signature":"s"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterSignatureSensitiveBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留 other_edit")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "other_edit", edit0["type"])
REDACTED

func TestFilterSignatureSensitiveBlocksForRetry_NoThinkingField_ContextManagementUntouched(t *testing.T) {
	// 没有顶层 thinking 字段时，context_management 不应被修改
	input := []byte(`{
		"context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED]REDACTED,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"t","signature":"s"REDACTED
			]REDACTED
		]
REDACTED`)

	out := FilterSignatureSensitiveBlocksForRetry(input, "claude-sonnet-4-5")

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "无顶层 thinking 时 context_management 不应被修改")
REDACTED

// ============ Group 7: ParseGatewayRequest 补充单元测试 ============

// Task 7.1 — 类型校验边界测试
func TestParseGatewayRequest_TypeValidation(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   bool
		errSubstr string // 期望的错误信息子串（为空则不检查）
REDACTED{
		{
			name:      "model 为 int",
			body:      `{"model":123REDACTED`,
			wantErr:   true,
			errSubstr: "invalid model field type",
	REDACTED,
		{
			name:      "model 为 array",
			body:      `{"model":[]REDACTED`,
			wantErr:   true,
			errSubstr: "invalid model field type",
	REDACTED,
		{
			name:      "model 为 bool",
			body:      `{"model":trueREDACTED`,
			wantErr:   true,
			errSubstr: "invalid model field type",
	REDACTED,
		{
			name:      "model 为 null — gjson Null 类型触发类型校验错误",
			body:      `{"model":nullREDACTED`,
			wantErr:   true, // gjson: Exists()=true, Type=Null != String → 返回错误
			errSubstr: "invalid model field type",
	REDACTED,
		{
			name:      "stream 为 string",
			body:      `{"stream":"true"REDACTED`,
			wantErr:   true,
			errSubstr: "invalid stream field type",
	REDACTED,
		{
			name:      "stream 为 int",
			body:      `{"stream":1REDACTED`,
			wantErr:   true,
			errSubstr: "invalid stream field type",
	REDACTED,
		{
			name:      "stream 为 null — gjson Null 类型触发类型校验错误",
			body:      `{"stream":nullREDACTED`,
			wantErr:   true, // gjson: Exists()=true, Type=Null != True && != False → 返回错误
			errSubstr: "invalid stream field type",
	REDACTED,
		{
			name:      "model 为 object",
			body:      `{"model":{REDACTEDREDACTED`,
			wantErr:   true,
			errSubstr: "invalid model field type",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseGatewayRequest(NewRequestBodyRef([]byte(tt.body)), "")
			if tt.wantErr {
			REDACTED
				if tt.errSubstr != "" {
					require.Contains(t, err.Error(), tt.errSubstr)
			REDACTED
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

// Task 7.2 — 可选字段缺失测试
func TestParseGatewayRequest_OptionalFieldsMissing(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantModel       string
		wantStream      bool
		wantMetadataUID string
		wantHasSystem   bool
		wantThinking    bool
		wantMaxTokens   int
		wantMessagesNil bool
		wantMessagesLen int
REDACTED{
		{
			name:            "完全空 JSON — 所有字段零值",
			body:            `{REDACTED`,
			wantModel:       "",
			wantStream:      false,
			wantMetadataUID: "",
			wantHasSystem:   false,
			wantThinking:    false,
			wantMaxTokens:   0,
			wantMessagesNil: true,
	REDACTED,
		{
			name:            "metadata 无 user_id",
			body:            `{"model":"test"REDACTED`,
			wantModel:       "test",
			wantMetadataUID: "",
			wantHasSystem:   false,
			wantThinking:    false,
	REDACTED,
		{
			name:         "thinking 非 enabled（type=disabled）",
			body:         `{"model":"test","thinking":{"type":"disabled"REDACTEDREDACTED`,
			wantModel:    "test",
			wantThinking: false,
	REDACTED,
		{
			name:         "thinking 字段缺失",
			body:         `{"model":"test"REDACTED`,
			wantModel:    "test",
			wantThinking: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(tt.body)), "")
		REDACTED

			require.Equal(t, tt.wantModel, parsed.Model)
			require.Equal(t, tt.wantStream, parsed.Stream)
			require.Equal(t, tt.wantMetadataUID, parsed.MetadataUserID)
			require.Equal(t, tt.wantHasSystem, parsed.HasSystem)
			require.Equal(t, tt.wantThinking, parsed.ThinkingEnabled)
			require.Equal(t, tt.wantMaxTokens, parsed.MaxTokens)

			if tt.wantMessagesNil {
				require.Nil(t, parsed.MessagesRaw())
		REDACTED
			if tt.wantMessagesLen > 0 {
				require.Len(t, gjson.ParseBytes(parsed.MessagesRaw()).Array(), tt.wantMessagesLen)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// Task 7.3 — Gemini 协议分支测试
// 已有测试覆盖：
// - TestParseGatewayRequest_GeminiSystemInstruction: 正常 systemInstruction+contents
// - TestParseGatewayRequest_GeminiNoContents: 缺失 contents
// - TestParseGatewayRequest_GeminiContents: 正常 contents（无 systemInstruction）
// 因此跳过。

// Task 7.4 — max_tokens 边界测试
func TestParseGatewayRequest_MaxTokensBoundary(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantMaxTokens int
		wantErr       bool
REDACTED{
		{
			name:          "正常整数",
			body:          `{"max_tokens":1024REDACTED`,
			wantMaxTokens: 1024,
	REDACTED,
		{
			name:          "浮点数（非整数）被忽略",
			body:          `{"max_tokens":10.5REDACTED`,
			wantMaxTokens: 0,
	REDACTED,
		{
			name:          "负整数可以通过",
			body:          `{"max_tokens":-1REDACTED`,
			wantMaxTokens: -1,
	REDACTED,
		{
			name:          "超大值不 panic",
			body:          `{"max_tokens":9999999999999999REDACTED`,
			wantMaxTokens: 10000000000000000, // float64 精度导致 9999999999999999 → 1e16
	REDACTED,
		{
			name:          "null 值被忽略",
			body:          `{"max_tokens":nullREDACTED`,
			wantMaxTokens: 0, // gjson Type=Null != Number → 条件不满足，跳过
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(tt.body)), "")
			if tt.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.wantMaxTokens, parsed.MaxTokens)
	REDACTED)
REDACTED
REDACTED

// ============ Task 7.5: Benchmark 测试 ============

// parseGatewayRequestOld 是基于完整 json.Unmarshal 的旧实现，用于 benchmark 对比基线。
// 核心路径：先 Unmarshal 到 map[string]any，再逐字段提取。
func parseGatewayRequestOld(body []byte, protocol string) (*ParsedRequest, error) {
	parsed := &ParsedRequest{
		Body: NewRequestBodyRef(body),
REDACTED

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
REDACTED

	// model
	if raw, ok := req["model"]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("invalid model field type")
	REDACTED
		parsed.Model = s
REDACTED

	// stream
	if raw, ok := req["stream"]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid stream field type")
	REDACTED
		parsed.Stream = b
REDACTED

	// metadata.user_id
	if meta, ok := req["metadata"].(map[string]any); ok {
		if uid, ok := meta["user_id"].(string); ok {
			parsed.MetadataUserID = uid
	REDACTED
REDACTED

	// thinking.type
	if thinking, ok := req["thinking"].(map[string]any); ok {
		if thinkType, ok := thinking["type"].(string); ok && thinkType == "enabled" {
			parsed.ThinkingEnabled = true
	REDACTED
REDACTED

	// max_tokens
	if raw, ok := req["max_tokens"]; ok {
		if n, ok := parseIntegralNumber(raw); ok {
			parsed.MaxTokens = n
	REDACTED
REDACTED

	if err := refreshGatewayRequestRanges(parsed, protocol); err != nil {
		return nil, err
REDACTED

	return parsed, nil
REDACTED

// buildSmallJSON 构建 ~500B 的小型测试 JSON
func buildSmallJSON() []byte {
	return []byte(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":4096,"metadata":{"user_id":"user-abc123"REDACTED,"thinking":{"type":"enabled","budget_tokens":2048REDACTED,"system":"You are a helpful assistant.","messages":[{"role":"user","content":"What is the meaning of life?"REDACTED,{"role":"assistant","content":"The meaning of life is a philosophical question."REDACTED,{"role":"user","content":"Can you elaborate?"REDACTED]REDACTED`)
REDACTED

// buildLargeJSON 构建 ~50KB 的大型测试 JSON（大量 messages）
func buildLargeJSON() []byte {
	var b strings.Builder
	b.WriteString(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":8192,"metadata":{"user_id":"user-xyz789"REDACTED,"system":[{"type":"text","text":"You are a detailed assistant.","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[`)

	msgCount := 200
	for i := 0; i < msgCount; i++ {
		if i > 0 {
			b.WriteByte(',')
	REDACTED
		if i%2 == 0 {
			b.WriteString(fmt.Sprintf(`{"role":"user","content":"This is user message number %d with some extra padding text to make the message reasonably long for benchmarking purposes. Lorem ipsum dolor sit amet."REDACTED`, i))
	REDACTED else {
			b.WriteString(fmt.Sprintf(`{"role":"assistant","content":[{"type":"text","text":"This is assistant response number %d. I will provide a detailed answer with multiple sentences to simulate real conversation content for benchmark testing."REDACTED]REDACTED`, i))
	REDACTED
REDACTED

	b.WriteString(`]REDACTED`)
	return []byte(b.String())
REDACTED

func BenchmarkParseGatewayRequest_Old_Small(b *testing.B) {
	data := buildSmallJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseGatewayRequestOld(data, "")
REDACTED
REDACTED

func BenchmarkParseGatewayRequest_New_Small(b *testing.B) {
	data := buildSmallJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseGatewayRequest(NewRequestBodyRef(data), "")
REDACTED
REDACTED

func BenchmarkParseGatewayRequest_Old_Large(b *testing.B) {
	data := buildLargeJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseGatewayRequestOld(data, "")
REDACTED
REDACTED

func TestParseGatewayRequest_OutputEffort(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantEffort string
REDACTED{
		{
			name:       "output_config.effort present",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":"medium"REDACTED,"messages":[]REDACTED`,
			wantEffort: "medium",
	REDACTED,
		{
			name:       "output_config.effort max",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":"max"REDACTED,"messages":[]REDACTED`,
			wantEffort: "max",
	REDACTED,
		{
			name:       "output_config.effort xhigh",
			body:       `{"model":"claude-opus-4-7","output_config":{"effort":"xhigh"REDACTED,"messages":[]REDACTED`,
			wantEffort: "xhigh",
	REDACTED,
		{
			name:       "output_config without effort",
			body:       `{"model":"claude-opus-4-6","output_config":{REDACTED,"messages":[]REDACTED`,
			wantEffort: "",
	REDACTED,
		{
			name:       "no output_config",
			body:       `{"model":"claude-opus-4-6","messages":[]REDACTED`,
			wantEffort: "",
	REDACTED,
		{
			name:       "effort with whitespace trimmed",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":" high "REDACTED,"messages":[]REDACTED`,
			wantEffort: "high",
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(tt.body)), "")
		REDACTED
			require.Equal(t, tt.wantEffort, parsed.OutputEffort)
	REDACTED)
REDACTED
REDACTED

func TestNormalizeClaudeOutputEffort(t *testing.T) {
	tests := []struct {
		input string
		want  *string
REDACTED{
		{"low", strPtr("low")REDACTED,
		{"medium", strPtr("medium")REDACTED,
		{"high", strPtr("high")REDACTED,
		{"max", strPtr("max")REDACTED,
		{"LOW", strPtr("low")REDACTED,
		{"Max", strPtr("max")REDACTED,
		{" medium ", strPtr("medium")REDACTED,
		{"xhigh", strPtr("xhigh")REDACTED,
		{"XHIGH", strPtr("xhigh")REDACTED,
		{"", nilREDACTED,
		{"unknown", nilREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeClaudeOutputEffort(tt.input)
			if tt.want == nil {
				require.Nil(t, got)
		REDACTED else {
				require.NotNil(t, got)
				require.Equal(t, *tt.want, *got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func BenchmarkParseGatewayRequest_New_Large(b *testing.B) {
	data := buildLargeJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseGatewayRequest(NewRequestBodyRef(data), "")
REDACTED
REDACTED

func TestNormalizeChineseLLMThinking(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		input         string
		wantApplied   bool
		wantTypeValue string // expected thinking.type after rewrite; "" = must not exist
		wantUnchanged bool   // body must be byte-for-byte unchanged
REDACTED{
		// MiniMax M3 / M2.x — passback-required path: rewrite enabled -> adaptive
		{
			name:          "minimax m3 enabled -> adaptive",
			model:         "MiniMax-M3",
			input:         `{"model":"MiniMax-M3","thinking":{"type":"enabled","budget_tokens":8192REDACTED,"messages":[]REDACTED`,
			wantApplied:   true,
			wantTypeValue: "adaptive",
	REDACTED,
		{
			name:          "minimax m2.7 enabled -> adaptive",
			model:         "MiniMax-M2.7",
			input:         `{"model":"MiniMax-M2.7","thinking":{"type":"enabled","budget_tokens":4096REDACTED,"messages":[]REDACTED`,
			wantApplied:   true,
			wantTypeValue: "adaptive",
	REDACTED,
		{
			name:          "minimax m3 adaptive is left alone",
			model:         "MiniMax-M3",
			input:         `{"model":"MiniMax-M3","thinking":{"type":"adaptive","budget_tokens":8192REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		{
			name:          "minimax m3 disabled is left alone",
			model:         "MiniMax-M3",
			input:         `{"model":"MiniMax-M3","thinking":{"type":"disabled"REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		{
			name:          "minimax m3 with no thinking field is no-op",
			model:         "MiniMax-M3",
			input:         `{"model":"MiniMax-M3","messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		// Non-MiniMax Chinese LLMs: no-op (Kimi/GLM/DeepSeek accept enabled as-is)
		{
			name:          "kimi k2.6 with enabled left alone",
			model:         "kimi-k2.6",
			input:         `{"model":"kimi-k2.6","thinking":{"type":"enabled","budget_tokens":8192REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		{
			name:          "glm-5.1 with enabled left alone",
			model:         "glm-5.1",
			input:         `{"model":"glm-5.1","thinking":{"type":"enabled"REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		{
			name:          "deepseek v4-pro with enabled left alone",
			model:         "deepseek-v4-pro",
			input:         `{"model":"deepseek-v4-pro","thinking":{"type":"enabled"REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		// Anthropic-strict model: never rewritten even though prefix would not match anyway
		{
			name:          "claude opus 4.6 with enabled left alone",
			model:         "claude-opus-4.6-20260201",
			input:         `{"model":"claude-opus-4.6-20260201","thinking":{"type":"enabled","budget_tokens":8192REDACTED,"messages":[]REDACTED`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
		// Edge case: invalid JSON — fail-safe return original
		{
			name:          "invalid json returned unchanged",
			model:         "MiniMax-M3",
			input:         `{not json`,
			wantApplied:   false,
			wantUnchanged: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := NormalizeChineseLLMThinking([]byte(tt.input), tt.model)
			require.Equal(t, tt.wantApplied, applied, "applied mismatch")

			if tt.wantUnchanged {
				require.Equal(t, tt.input, string(got), "body must be byte-for-byte unchanged")
				return
		REDACTED

			// Parsed-back validation: output must be valid JSON with the expected thinking.type
			var parsed struct {
				Thinking struct {
					Type string `json:"type"`
			REDACTED `json:"thinking"`
		REDACTED
			require.NoError(t, json.Unmarshal(got, &parsed), "output must be valid JSON")
			require.Equal(t, tt.wantTypeValue, parsed.Thinking.Type)
	REDACTED)
REDACTED
REDACTED

func TestDefaultEffortForThinkingEnabled(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  *string // nil = expect no fallback
REDACTED{
		// passback-required 上游中不支持 effort 档位的国产模型→补默认 high
		{name: "glm-5.1", model: "glm-5.1", want: strPtr("high")REDACTED,
		{name: "glm-4.7", model: "glm-4.7", want: strPtr("high")REDACTED,
		{name: "kimi-k2.6", model: "kimi-k2.6", want: strPtr("high")REDACTED,
		{name: "kimi-k2-thinking", model: "kimi-k2-thinking", want: strPtr("high")REDACTED,
		{name: "moonshot-v1-8k", model: "moonshot-v1-8k", want: strPtr("high")REDACTED,
		{name: "minimax-m3 (lowercase)", model: "minimax-m3", want: strPtr("high")REDACTED,
		{name: "MiniMax-M3 (mixed case)", model: "MiniMax-M3", want: strPtr("high")REDACTED,
		{name: "qwen3-thinking variant", model: "qwen3-235b-a22b-thinking-2507", want: strPtr("high")REDACTED,

		// DeepSeek 有原生 effort 支持→不注入默认，让客户端意图透传
		{name: "deepseek-v4-pro excluded", model: "deepseek-v4-pro", want: nilREDACTED,
		{name: "deepseek-v4-flash excluded", model: "deepseek-v4-flash", want: nilREDACTED,
		{name: "deepseek-chat excluded", model: "deepseek-chat", want: nilREDACTED,

		// 非 passback-required 模型一律返回 nil
		{name: "claude opus 4.6 (anthropic-strict)", model: "claude-opus-4.6-20260201", want: nilREDACTED,
		{name: "gpt-5.5 (unknown)", model: "gpt-5.5", want: nilREDACTED,
		{name: "gemini-3.1-pro (unknown)", model: "gemini-3.1-pro", want: nilREDACTED,
		{name: "empty", model: "", want: nilREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultEffortForThinkingEnabled(tt.model)
			if tt.want == nil {
				require.Nil(t, got)
				return
		REDACTED
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIBodyHasThinkingEnabled(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
REDACTED{
		{name: "enabled", body: `{"thinking":{"type":"enabled"REDACTEDREDACTED`, want: trueREDACTED,
		{name: "adaptive", body: `{"thinking":{"type":"adaptive"REDACTEDREDACTED`, want: trueREDACTED,
		{name: "ENABLED (uppercase)", body: `{"thinking":{"type":"ENABLED"REDACTEDREDACTED`, want: trueREDACTED,
		{name: "disabled", body: `{"thinking":{"type":"disabled"REDACTEDREDACTED`, want: falseREDACTED,
		{name: "empty body", body: ``, want: falseREDACTED,
		{name: "no thinking field", body: `{"model":"gpt-5"REDACTED`, want: falseREDACTED,
		{name: "thinking object but no type", body: `{"thinking":{"budget_tokens":1024REDACTEDREDACTED`, want: falseREDACTED,
		{name: "invalid json", body: `{not json`, want: falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, OpenAIBodyHasThinkingEnabled([]byte(tt.body)))
	REDACTED)
REDACTED
REDACTED

func TestApplyThinkingEnabledFallback(t *testing.T) {
	tests := []struct {
		name        string
		effort      *string
		body        string
		model       string
		want        *string
		wantPassThr bool // 为 true 时 want 是传入 effort 原指针
REDACTED{
		// effort 非 nil → 原值透传，不覆盖
		{
			name:        "existing effort never overridden (kimi + thinking)",
			effort:      strPtr("medium"),
			body:        `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:       "kimi-k2.6",
			wantPassThr: true,
	REDACTED,
		{
			name:        "existing low effort kept for deepseek",
			effort:      strPtr("low"),
			body:        `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:       "deepseek-v4-pro",
			wantPassThr: true,
	REDACTED,

		// effort=nil + thinking enabled + passback-required 模型 → 填 high
		{
			name:   "glm-5.1 + thinking enabled -> high",
			effort: nil,
			body:   `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:  "glm-5.1",
			want:   strPtr("high"),
	REDACTED,
		{
			name:   "kimi-k2.6 + adaptive -> high",
			effort: nil,
			body:   `{"thinking":{"type":"adaptive"REDACTEDREDACTED`,
			model:  "kimi-k2.6",
			want:   strPtr("high"),
	REDACTED,
		{
			name:   "MiniMax-M3 + enabled -> high",
			effort: nil,
			body:   `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:  "MiniMax-M3",
			want:   strPtr("high"),
	REDACTED,

		// effort=nil + thinking disabled → nil
		{
			name:   "glm + thinking disabled -> nil",
			effort: nil,
			body:   `{"thinking":{"type":"disabled"REDACTEDREDACTED`,
			model:  "glm-5.1",
			want:   nil,
	REDACTED,
		{
			name:   "glm + no thinking field -> nil",
			effort: nil,
			body:   `{"model":"glm-5.1"REDACTED`,
			model:  "glm-5.1",
			want:   nil,
	REDACTED,

		// effort=nil + thinking enabled + non-passback → nil
		{
			name:   "deepseek + thinking enabled -> nil (deepseek excluded)",
			effort: nil,
			body:   `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:  "deepseek-v4-pro",
			want:   nil,
	REDACTED,
		{
			name:   "claude + thinking enabled -> nil (strict not passback)",
			effort: nil,
			body:   `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:  "claude-opus-4.6",
			want:   nil,
	REDACTED,
		{
			name:   "gpt-5 + thinking enabled -> nil (unknown)",
			effort: nil,
			body:   `{"thinking":{"type":"enabled"REDACTEDREDACTED`,
			model:  "gpt-5.5",
			want:   nil,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyThinkingEnabledFallback(tt.effort, []byte(tt.body), tt.model)
			if tt.wantPassThr {
				require.Same(t, tt.effort, got, "non-nil effort must be returned unchanged (same pointer)")
				return
		REDACTED
			if tt.want == nil {
				require.Nil(t, got)
				return
		REDACTED
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
	REDACTED)
REDACTED
REDACTED
