package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsClaudeCodeClient(t *testing.T) {
	tests := []struct {
		name           string
		userAgent      string
		metadataUserID string
		want           bool
	}{
		{
			name:           "Claude Code client",
			userAgent:      "claude-cli/1.0.62 (darwin; arm64)",
			metadataUserID: "session_123e4567-e89b-12d3-a456-426614174000",
			want:           true,
		},
		{
			name:           "Claude Code without version suffix",
			userAgent:      "claude-cli/2.0.0",
			metadataUserID: "session_abc",
			want:           true,
		},
		{
			name:           "Missing metadata user_id",
			userAgent:      "claude-cli/1.0.0",
			metadataUserID: "",
			want:           false,
		},
		{
			name:           "Different user agent",
			userAgent:      "curl/7.68.0",
			metadataUserID: "user123",
			want:           false,
		},
		{
			name:           "Empty user agent",
			userAgent:      "",
			metadataUserID: "user123",
			want:           false,
		},
		{
			name:           "Similar but not Claude CLI",
			userAgent:      "claude-api/1.0.0",
			metadataUserID: "user123",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClaudeCodeClient(tt.userAgent, tt.metadataUserID)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSystemIncludesClaudeCodePrompt(t *testing.T) {
	tests := []struct {
		name   string
		system any
		want   bool
	}{
		{
			name:   "nil system",
			system: nil,
			want:   false,
		},
		{
			name:   "empty string",
			system: "",
			want:   false,
		},
		{
			name:   "string with Claude Code prompt",
			system: claudeCodeSystemPrompt,
			want:   true,
		},
		{
			name:   "string with different content",
			system: "You are a helpful assistant.",
			want:   false,
		},
		{
			name:   "empty array",
			system: []any{},
			want:   false,
		},
		{
			name: "array with Claude Code prompt",
			system: []any{
				map[string]any{
					"type": "text",
					"text": claudeCodeSystemPrompt,
				},
			},
			want: true,
		},
		{
			name: "array with Claude Code prompt in second position",
			system: []any{
				map[string]any{"type": "text", "text": "First prompt"},
				map[string]any{"type": "text", "text": claudeCodeSystemPrompt},
			},
			want: true,
		},
		{
			name: "array without Claude Code prompt",
			system: []any{
				map[string]any{"type": "text", "text": "Custom prompt"},
			},
			want: false,
		},
		{
			name: "array with partial match (should not match)",
			system: []any{
				map[string]any{"type": "text", "text": "You are Claude"},
			},
			want: false,
		},
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:   "json.RawMessage string with Claude Code prompt",
			system: json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			want:   true,
		},
		{
			name:   "json.RawMessage string without Claude Code prompt",
			system: json.RawMessage(`"You are a helpful assistant"`),
			want:   false,
		},
		{
			name:   "json.RawMessage nil (empty)",
			system: json.RawMessage(nil),
			want:   false,
		},
		{
			name:   "json.RawMessage empty string",
			system: json.RawMessage(`""`),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemIncludesClaudeCodePrompt(tt.system)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestInjectClaudeCodePrompt(t *testing.T) {
	claudePrefix := strings.TrimSpace(claudeCodeSystemPrompt)

	tests := []struct {
		name           string
		body           string
		system         any
		wantSystemLen  int
		wantFirstText  string
		wantSecondText string
	}{
		{
			name:          "nil system",
			body:          `{"model":"claude-3"}`,
			system:        nil,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
		{
			name:          "empty string system",
			body:          `{"model":"claude-3"}`,
			system:        "",
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
		{
			name:           "string system",
			body:           `{"model":"claude-3"}`,
			system:         "Custom prompt",
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
		},
		{
			name:          "string system equals Claude Code prompt",
			body:          `{"model":"claude-3"}`,
			system:        claudeCodeSystemPrompt,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
		{
			name:   "array system",
			body:   `{"model":"claude-3"}`,
			system: []any{map[string]any{"type": "text", "text": "Custom"}},
			// Claude Code + Custom = 2
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom",
		},
		{
			name: "array system with existing Claude Code prompt (should dedupe)",
			body: `{"model":"claude-3"}`,
			system: []any{
				map[string]any{"type": "text", "text": claudeCodeSystemPrompt},
				map[string]any{"type": "text", "text": "Other"},
			},
			// Claude Code at start + Other = 2 (deduped)
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nOther",
		},
		{
			name:          "empty array",
			body:          `{"model":"claude-3"}`,
			system:        []any{},
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:           "json.RawMessage string system",
			body:           `{"model":"claude-3","system":"Custom prompt"}`,
			system:         json.RawMessage(`"Custom prompt"`),
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
		},
		{
			name:          "json.RawMessage nil system",
			body:          `{"model":"claude-3"}`,
			system:        json.RawMessage(nil),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
		{
			name:          "json.RawMessage Claude Code prompt (should not duplicate)",
			body:          `{"model":"claude-3","system":"` + claudeCodeSystemPrompt + `"}`,
			system:        json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectClaudeCodePrompt([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
			require.NoError(t, err)

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
			}
		})
	}
}

func TestRewriteSystemForNonClaudeCode(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		system           any
		wantSystemStr    string // system 应为纯字符串
		wantMessagesLen  int    // messages 数组长度
		wantFirstMsgRole string // 第一条消息的 role
		wantFirstMsgText string // 第一条消息的 content[0].text
		wantAckMsgText   string // 第二条消息的 content[0].text
	}{
		{
			name:            "nil system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          nil,
			wantSystemStr:   claudeCodeSystemPrompt,
			wantMessagesLen: 1, // 原始 1 条消息，不注入
		},
		{
			name:            "empty string system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          "",
			wantSystemStr:   claudeCodeSystemPrompt,
			wantMessagesLen: 1,
		},
		{
			name:             "custom string system - migrated to messages",
			body:             `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:           "You are a personal assistant running inside OpenClaw.",
			wantSystemStr:    claudeCodeSystemPrompt,
			wantMessagesLen:  3, // instruction + ack + original
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nYou are a personal assistant running inside OpenClaw.",
			wantAckMsgText:   "Understood. I will follow these instructions.",
		},
		{
			name:            "system equals Claude Code prompt - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          claudeCodeSystemPrompt,
			wantSystemStr:   claudeCodeSystemPrompt,
			wantMessagesLen: 1,
		},
		{
			name: "array system with custom blocks - text joined and migrated",
			body: `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system: []any{
				map[string]any{"type": "text", "text": "First instruction"},
				map[string]any{"type": "text", "text": "Second instruction"},
			},
			wantSystemStr:    claudeCodeSystemPrompt,
			wantMessagesLen:  3,
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nFirst instruction\n\nSecond instruction",
			wantAckMsgText:   "Understood. I will follow these instructions.",
		},
		{
			name:            "empty array system - no messages injected",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          []any{},
			wantSystemStr:   claudeCodeSystemPrompt,
			wantMessagesLen: 1,
		},
		{
			name:             "json.RawMessage string system",
			body:             `{"model":"claude-3","system":"Custom prompt","messages":[{"role":"user","content":"hello"}]}`,
			system:           json.RawMessage(`"Custom prompt"`),
			wantSystemStr:    claudeCodeSystemPrompt,
			wantMessagesLen:  3,
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nCustom prompt",
			wantAckMsgText:   "Understood. I will follow these instructions.",
		},
		{
			name:            "json.RawMessage nil system",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          json.RawMessage(nil),
			wantSystemStr:   claudeCodeSystemPrompt,
			wantMessagesLen: 1,
		},
		{
			name:             "multiple original messages preserved",
			body:             `{"model":"claude-3","messages":[{"role":"user","content":"msg1"},{"role":"assistant","content":"resp1"},{"role":"user","content":"msg2"}]}`,
			system:           "Be helpful",
			wantSystemStr:    claudeCodeSystemPrompt,
			wantMessagesLen:  5, // 2 injected + 3 original
			wantFirstMsgRole: "user",
			wantFirstMsgText: "[System Instructions]\nBe helpful",
			wantAckMsgText:   "Understood. I will follow these instructions.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteSystemForNonClaudeCode([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
			require.NoError(t, err)

			// system 应为纯字符串
			systemVal, ok := parsed["system"].(string)
			require.True(t, ok, "system should be a string, got %T", parsed["system"])
			require.Equal(t, tt.wantSystemStr, systemVal)

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
			}
		})
	}
}
