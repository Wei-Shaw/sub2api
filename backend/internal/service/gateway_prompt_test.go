package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsClaudeCodeClient(t *testing.T) {
	// 合法的 legacy 格式 metadata.user_id（64位 hex + account uuid + session uuid）
	legacyUserID := "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account_550e8400-e29b-41d4-a716-446655440000_session_123e4567-e89b-12d3-a456-426614174000"
	// 合法的 JSON 格式 metadata.user_id（2.1.78+ 版本）
	jsonUserID := `{"device_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2","account_uuid":"550e8400-e29b-41d4-a716-446655440000","session_id":"123e4567-e89b-12d3-a456-426614174000"REDACTED`

	tests := []struct {
		name           string
		userAgent      string
		metadataUserID string
		want           bool
REDACTED{
		{
			name:           "Claude Code client with legacy user_id",
			userAgent:      "claude-cli/1.0.62 (darwin; arm64)",
			metadataUserID: legacyUserID,
			want:           true,
	REDACTED,
		{
			name:           "Claude Code client with JSON user_id",
			userAgent:      "claude-cli/2.1.92 (external, cli)",
			metadataUserID: jsonUserID,
			want:           true,
	REDACTED,
		{
			name:           "Claude Code case insensitive UA",
			userAgent:      "Claude-CLI/2.0.0",
			metadataUserID: legacyUserID,
			want:           true,
	REDACTED,
		{
			name:           "Missing metadata user_id",
			userAgent:      "claude-cli/1.0.0",
			metadataUserID: "",
			want:           false,
	REDACTED,
		{
			name:           "Claude CLI UA with invalid user_id format",
			userAgent:      "claude-cli/2.0.0",
			metadataUserID: "fake-user-id-12345",
			want:           false,
	REDACTED,
		{
			name:           "Different user agent with valid user_id",
			userAgent:      "curl/7.68.0",
			metadataUserID: legacyUserID,
			want:           false,
	REDACTED,
		{
			name:           "Empty user agent",
			userAgent:      "",
			metadataUserID: legacyUserID,
			want:           false,
	REDACTED,
		{
			name:           "Similar but not Claude CLI",
			userAgent:      "claude-api/1.0.0",
			metadataUserID: legacyUserID,
			want:           false,
	REDACTED,
		{
			name:           "Opencode spoofing UA with arbitrary user_id",
			userAgent:      "claude-cli/2.1.92",
			metadataUserID: "session_abc",
			want:           false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClaudeCodeClient(tt.userAgent, tt.metadataUserID)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestSystemIncludesClaudeCodePrompt(t *testing.T) {
	tests := []struct {
		name   string
		system any
		want   bool
REDACTED{
		{
			name:   "nil system",
			system: nil,
			want:   false,
	REDACTED,
		{
			name:   "empty string",
			system: "",
			want:   false,
	REDACTED,
		{
			name:   "string with Claude Code prompt",
			system: claudeCodeSystemPrompt,
			want:   true,
	REDACTED,
		{
			name:   "string with different content",
			system: "You are a helpful assistant.",
			want:   false,
	REDACTED,
		{
			name:   "empty array",
			system: []any{REDACTED,
			want:   false,
	REDACTED,
		{
			name: "array with Claude Code prompt",
			system: []any{
				map[string]any{
					"type": "text",
					"text": claudeCodeSystemPrompt,
			REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "array with Claude Code prompt in second position",
			system: []any{
				map[string]any{"type": "text", "text": "First prompt"REDACTED,
				map[string]any{"type": "text", "text": claudeCodeSystemPromptREDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "array without Claude Code prompt",
			system: []any{
				map[string]any{"type": "text", "text": "Custom prompt"REDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "array with partial match (should not match)",
			system: []any{
				map[string]any{"type": "text", "text": "You are Claude"REDACTED,
		REDACTED,
			want: false,
	REDACTED,
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:   "json.RawMessage string with Claude Code prompt",
			system: json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			want:   true,
	REDACTED,
		{
			name:   "json.RawMessage string without Claude Code prompt",
			system: json.RawMessage(`"You are a helpful assistant"`),
			want:   false,
	REDACTED,
		{
			name:   "json.RawMessage nil (empty)",
			system: json.RawMessage(nil),
			want:   false,
	REDACTED,
		{
			name:   "json.RawMessage empty string",
			system: json.RawMessage(`""`),
			want:   false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemIncludesClaudeCodePrompt(tt.system)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestInjectClaudeCodePrompt(t *testing.T) {
	claudePrefix := strings.TrimSpace(claudeCodeSystemPrompt)

	tests := []struct {
		name           string
		body           string
		system         any
		wantSystemLen  int
		wantFirstText  string
		wantSecondText string
REDACTED{
		{
			name:          "nil system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        nil,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:          "empty string system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        "",
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:           "string system",
			body:           `{"model":"claude-3"REDACTED`,
			system:         "Custom prompt",
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
	REDACTED,
		{
			name:          "string system equals Claude Code prompt",
			body:          `{"model":"claude-3"REDACTED`,
			system:        claudeCodeSystemPrompt,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:   "array system",
			body:   `{"model":"claude-3"REDACTED`,
			system: []any{map[string]any{"type": "text", "text": "Custom"REDACTEDREDACTED,
			// Claude Code + Custom = 2
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom",
	REDACTED,
		{
			name: "array system with existing Claude Code prompt (should dedupe)",
			body: `{"model":"claude-3"REDACTED`,
			system: []any{
				map[string]any{"type": "text", "text": claudeCodeSystemPromptREDACTED,
				map[string]any{"type": "text", "text": "Other"REDACTED,
		REDACTED,
			// Claude Code at start + Other = 2 (deduped)
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nOther",
	REDACTED,
		{
			name:          "empty array",
			body:          `{"model":"claude-3"REDACTED`,
			system:        []any{REDACTED,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:           "json.RawMessage string system",
			body:           `{"model":"claude-3","system":"Custom prompt"REDACTED`,
			system:         json.RawMessage(`"Custom prompt"`),
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
	REDACTED,
		{
			name:          "json.RawMessage nil system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        json.RawMessage(nil),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:          "json.RawMessage Claude Code prompt (should not duplicate)",
			body:          `{"model":"claude-3","system":"` + claudeCodeSystemPrompt + `"REDACTED`,
			system:        json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectClaudeCodePrompt([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
		REDACTED

			system, ok := parsed["system"].([]any)
			require.True(t, ok, "system should be an array")
			require.Len(t, system, tt.wantSystemLen)

			first, ok := system[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.wantFirstText, first["text"])
			require.Equal(t, "text", first["type"])

			// Check cache_control
			cc, ok := first["cache_control"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "ephemeral", cc["type"])

			if tt.wantSecondText != "" && len(system) > 1 {
				second, ok := system[1].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantSecondText, second["text"])
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestRewriteSystemForNonClaudeCode(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		system           any
		wantSystemText   string // system array 第一个 block 的 text
		wantMessagesLen  int    // messages 数组长度
		wantFirstMsgRole string // 第一条消息的 role
		wantFirstMsgText string // 第一条消息的 content[0].text
		wantAckMsgText   string // 第二条消息的 content[0].text
REDACTED{
		{
			name:            "nil system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:          nil,
			wantSystemText:  claudeCodeSystemPrompt,
			wantMessagesLen: 1, // 原始 1 条消息，不注入
	REDACTED,
		{
			name:            "empty string system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:          "",
			wantSystemText:  claudeCodeSystemPrompt,
			wantMessagesLen: 1,
	REDACTED,
		{
			name:             "custom string system - migrated to messages",
			body:             `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:           "You are a personal assistant running inside OpenClaw.",
			wantSystemText:   claudeCodeSystemPrompt,
			wantMessagesLen:  3, // instruction + ack + original
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nYou are a personal assistant running inside OpenClaw.",
			wantAckMsgText:   "Understood. I will follow these instructions.",
	REDACTED,
		{
			name:            "system equals Claude Code prompt - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:          claudeCodeSystemPrompt,
			wantSystemText:  claudeCodeSystemPrompt,
			wantMessagesLen: 1,
	REDACTED,
		{
			name: "array system with custom blocks - text joined and migrated",
			body: `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system: []any{
				map[string]any{"type": "text", "text": "First instruction"REDACTED,
				map[string]any{"type": "text", "text": "Second instruction"REDACTED,
		REDACTED,
			wantSystemText:   claudeCodeSystemPrompt,
			wantMessagesLen:  3,
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nFirst instruction\n\nSecond instruction",
			wantAckMsgText:   "Understood. I will follow these instructions.",
	REDACTED,
		{
			name:            "empty array system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:          []any{REDACTED,
			wantSystemText:  claudeCodeSystemPrompt,
			wantMessagesLen: 1,
	REDACTED,
		{
			name:             "json.RawMessage string system",
			body:             `{"model":"claude-3","system":"Custom prompt","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:           json.RawMessage(`"Custom prompt"`),
			wantSystemText:   claudeCodeSystemPrompt,
			wantMessagesLen:  3,
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nCustom prompt",
			wantAckMsgText:   "Understood. I will follow these instructions.",
	REDACTED,
		{
			name:            "json.RawMessage nil system",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`,
			system:          json.RawMessage(nil),
			wantSystemText:  claudeCodeSystemPrompt,
			wantMessagesLen: 1,
	REDACTED,
		{
			name:             "multiple original messages preserved",
			body:             `{"model":"claude-3","messages":[{"role":"user","content":"msg1"REDACTED,{"role":"assistant","content":"resp1"REDACTED,{"role":"user","content":"msg2"REDACTED]REDACTED`,
			system:           "Be helpful",
			wantSystemText:   claudeCodeSystemPrompt,
			wantMessagesLen:  5, // 2 injected + 3 original
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nBe helpful",
			wantAckMsgText:   "Understood. I will follow these instructions.",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteSystemForNonClaudeCode([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
		REDACTED

			// system 应为 array 格式，对齐真实 Claude Code CLI 的 3-block 形态：
			//   [0] billing attribution block (x-anthropic-billing-header: cc_version=...;)
			//   [1] Claude Code 身份前缀 block (不带 cache_control)
			//   [2] 工具无关的通用提示词扩充 block (带 cache_control，作为缓存断点)
			systemArr, ok := parsed["system"].([]any)
			require.True(t, ok, "system should be an array, got %T", parsed["system"])
			require.Len(t, systemArr, 3, "system array should have exactly 3 blocks (billing + cc prompt + expansion)")

			billingBlock, ok := systemArr[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "text", billingBlock["type"])
			require.Contains(t, billingBlock["text"], "x-anthropic-billing-header:")
			require.Contains(t, billingBlock["text"], "cc_version=")
			require.Contains(t, billingBlock["text"], "cc_entrypoint=cli")
			require.Contains(t, billingBlock["text"], "cch=00000")

			systemBlock, ok := systemArr[1].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "text", systemBlock["type"])
			require.Equal(t, tt.wantSystemText, systemBlock["text"])
			_, hasCC := systemBlock["cache_control"]
			require.False(t, hasCC, "身份前缀 block 不应带 cache_control（断点落在扩充块）")

			expansionBlock, ok := systemArr[2].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "text", expansionBlock["type"])
			require.Equal(t, claudeCodeSystemPromptExpansion, expansionBlock["text"])
			cc, ok := expansionBlock["cache_control"].(map[string]any)
			require.True(t, ok, "expansion block should have cache_control")
			require.Equal(t, "ephemeral", cc["type"])

			// 检查 messages
			messages, ok := parsed["messages"].([]any)
			require.True(t, ok, "messages should be an array")
			require.Len(t, messages, tt.wantMessagesLen)

			if tt.wantFirstMsgRole != "" && len(messages) >= 2 {
				// 检查注入的 instruction 消息
				firstMsg, ok := messages[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantFirstMsgRole, firstMsg["role"])

				firstContent, ok := firstMsg["content"].([]any)
				require.True(t, ok)
				require.Len(t, firstContent, 1)
				firstBlock, ok := firstContent[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantFirstMsgText, firstBlock["text"])

				// 检查注入的 ack 消息
				ackMsg, ok := messages[1].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "assistant", ackMsg["role"])

				ackContent, ok := ackMsg["content"].([]any)
				require.True(t, ok)
				require.Len(t, ackContent, 1)
				ackBlock, ok := ackContent[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantAckMsgText, ackBlock["text"])
		REDACTED
	REDACTED)
REDACTED
REDACTED
