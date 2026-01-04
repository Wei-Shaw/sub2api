package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGatewayRequest(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","stream":true,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"REDACTED,"system":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(body)
REDACTED
	require.Equal(t, "claude-3-7-sonnet", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "session_123e4567-e89b-12d3-a456-426614174000", parsed.MetadataUserID)
	require.True(t, parsed.HasSystem)
	require.NotNil(t, parsed.System)
	require.Len(t, parsed.Messages, 1)
REDACTED

func TestParseGatewayRequest_SystemNull(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":nullREDACTED`)
	parsed, err := ParseGatewayRequest(body)
REDACTED
	// 显式传入 system:null 也应视为“字段已存在”，避免默认 system 被注入。
	require.True(t, parsed.HasSystem)
	require.Nil(t, parsed.System)
REDACTED

func TestParseGatewayRequest_InvalidModelType(t *testing.T) {
	body := []byte(`{"model":123REDACTED`)
	_, err := ParseGatewayRequest(body)
REDACTED
REDACTED

func TestParseGatewayRequest_InvalidStreamType(t *testing.T) {
	body := []byte(`{"stream":"true"REDACTED`)
	_, err := ParseGatewayRequest(body)
REDACTED
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

	assistant := msgs[1].(map[string]any)
	content := assistant["content"].([]any)
	require.Len(t, content, 2)

	first := content[0].(map[string]any)
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

	msgs := req["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 1)
	require.Equal(t, "text", content[0].(map[string]any)["type"])
	require.Equal(t, "Visible", content[0].(map[string]any)["text"])
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
	msgs := req["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 1)
	require.Equal(t, "text", content[0].(map[string]any)["type"])
	require.NotEmpty(t, content[0].(map[string]any)["text"])
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

	msgs := req["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 2)
	require.Equal(t, "text", content[0].(map[string]any)["type"])
	require.Equal(t, "text", content[1].(map[string]any)["type"])
	require.Contains(t, content[0].(map[string]any)["text"], "tool_use")
	require.Contains(t, content[1].(map[string]any)["text"], "tool_result")
REDACTED
