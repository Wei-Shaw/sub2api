package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseGatewayRequest(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","stream":true,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"REDACTED,"system":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	require.Equal(t, "claude-3-7-sonnet", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "session_123e4567-e89b-12d3-a456-426614174000", parsed.MetadataUserID)
	require.True(t, parsed.HasSystem)
	require.NotNil(t, parsed.System)
	require.Len(t, parsed.Messages, 1)
	require.False(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_ThinkingEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"enabled"REDACTED,"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_ThinkingAdaptiveEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"adaptive"REDACTED,"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
REDACTED

func TestParseGatewayRequest_MaxTokens(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1REDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	require.Equal(t, 1, parsed.MaxTokens)
REDACTED

func TestParseGatewayRequest_MaxTokensNonIntegralIgnored(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1.5REDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	require.Equal(t, 0, parsed.MaxTokens)
REDACTED

func TestParseGatewayRequest_SystemNull(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":nullREDACTED`)
	parsed, err := ParseGatewayRequest(body, "")
REDACTED
	// 显式传入 system:null 也应视为“字段已存在”，避免默认 system 被注入。
	require.True(t, parsed.HasSystem)
	require.Nil(t, parsed.System)
REDACTED

func TestParseGatewayRequest_InvalidModelType(t *testing.T) {
	body := []byte(`{"model":123REDACTED`)
	_, err := ParseGatewayRequest(body, "")
REDACTED
REDACTED

func TestParseGatewayRequest_InvalidStreamType(t *testing.T) {
	body := []byte(`{"stream":"true"REDACTED`)
	_, err := ParseGatewayRequest(body, "")
REDACTED
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
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.Len(t, parsed.Messages, 3, "should parse contents as Messages")
	require.False(t, parsed.HasSystem, "Gemini format should not set HasSystem")
	require.Nil(t, parsed.System, "no systemInstruction means nil System")
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
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.NotNil(t, parsed.System, "should parse systemInstruction.parts as System")
	parts, ok := parsed.System.([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	partMap, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "You are a helpful assistant.", partMap["text"])
	require.Len(t, parsed.Messages, 1)
REDACTED

func TestParseGatewayRequest_GeminiWithModel(t *testing.T) {
	body := []byte(`{
		"model": "gemini-2.5-pro",
		"contents": [{"role": "user", "parts": [{"text": "test"REDACTED]REDACTED]
REDACTED`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.Equal(t, "gemini-2.5-pro", parsed.Model)
	require.Len(t, parsed.Messages, 1)
REDACTED

func TestParseGatewayRequest_GeminiIgnoresAnthropicFields(t *testing.T) {
	// Gemini 格式下 system/messages 字段应被忽略
	body := []byte(`{
		"system": "should be ignored",
		"messages": [{"role": "user", "content": "ignored"REDACTED],
		"contents": [{"role": "user", "parts": [{"text": "real content"REDACTED]REDACTED]
REDACTED`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.False(t, parsed.HasSystem, "Gemini protocol should not parse Anthropic system field")
	require.Nil(t, parsed.System, "no systemInstruction = nil System")
	require.Len(t, parsed.Messages, 1, "should use contents, not messages")
REDACTED

func TestParseGatewayRequest_GeminiEmptyContents(t *testing.T) {
	body := []byte(`{"contents": []REDACTED`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.Empty(t, parsed.Messages)
REDACTED

func TestParseGatewayRequest_GeminiNoContents(t *testing.T) {
	body := []byte(`{"model": "gemini-2.5-flash"REDACTED`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
REDACTED
	require.Nil(t, parsed.Messages)
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
	parsed, err := ParseGatewayRequest(body, domain.PlatformAnthropic)
REDACTED
	require.True(t, parsed.HasSystem)
	require.Equal(t, "real system", parsed.System)
	require.Len(t, parsed.Messages, 1)
	msg, ok := parsed.Messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "real content", msg["content"])
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
			result := FilterThinkingBlocks([]byte(tt.input))

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

	out := FilterThinkingBlocksForRetry(input)

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

	out := FilterThinkingBlocksForRetry(input)

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

	out := FilterThinkingBlocksForRetry(input)

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

func TestFilterThinkingBlocksForRetry_EmptyContentGetsPlaceholder(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled"REDACTED,
		"messages":[
			{"role":"assistant","content":[{"type":"redacted_thinking","data":"..."REDACTED]REDACTED
		]
REDACTED`)

	out := FilterThinkingBlocksForRetry(input)

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

	out := FilterSignatureSensitiveBlocksForRetry(input)

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
