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
	jsonUserID := `{"device_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2","account_uuid":"550e8400-e29b-41d4-a716-446655440000","session_id":"123e4567-e89b-12d3-a456-426614174000"}`

	tests := []struct {
		name           string
		userAgent      string
		metadataUserID string
		want           bool
	}{
		{
			name:           "Claude Code client with legacy user_id",
			userAgent:      "claude-cli/1.0.62 (darwin; arm64)",
			metadataUserID: legacyUserID,
			want:           true,
		},
		{
			name:           "Claude Code client with JSON user_id",
			userAgent:      "claude-cli/2.1.92 (external, cli)",
			metadataUserID: jsonUserID,
			want:           true,
		},
		{
			name:           "Claude Code case insensitive UA",
			userAgent:      "Claude-CLI/2.0.0",
			metadataUserID: legacyUserID,
			want:           true,
		},
		{
			name:           "Missing metadata user_id",
			userAgent:      "claude-cli/1.0.0",
			metadataUserID: "",
			want:           false,
		},
		{
			name:           "Claude CLI UA with invalid user_id format",
			userAgent:      "claude-cli/2.0.0",
			metadataUserID: "fake-user-id-12345",
			want:           false,
		},
		{
			name:           "Different user agent with valid user_id",
			userAgent:      "curl/7.68.0",
			metadataUserID: legacyUserID,
			want:           false,
		},
		{
			name:           "Empty user agent",
			userAgent:      "",
			metadataUserID: legacyUserID,
			want:           false,
		},
		{
			name:           "Similar but not Claude CLI",
			userAgent:      "claude-api/1.0.0",
			metadataUserID: legacyUserID,
			want:           false,
		},
		{
			name:           "Opencode spoofing UA with arbitrary user_id",
			userAgent:      "claude-cli/2.1.92",
			metadataUserID: "session_abc",
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
	// Updated contract (v0.1.125+): client system prompt moves to messages[]
	// as user/assistant pair, NOT appended to system[]. Rationale: client-
	// specific fingerprint text (e.g. "running inside OpenClaw", AGENTS.md)
	// leaks from system into Anthropic's semantic detection; messages content
	// is ignored by that detection.
	//
	// system array: [CC_prompt] only — always exactly one block.
	// messages array: [fake_user_instr, fake_assistant_ack, ...original] if
	//   client supplied a non-empty, non-CC system prompt.
	tests := []struct {
		name              string
		body              string
		system            any
		wantInjected      bool   // client prompt became messages[0:2] pair?
		wantInjectText    string // substring of the injected user instruction
		wantMessagesLen   int
	}{
		{
			name:            "nil system - no injection",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          nil,
			wantInjected:    false,
			wantMessagesLen: 1,
		},
		{
			name:            "empty string system - no injection",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          "",
			wantInjected:    false,
			wantMessagesLen: 1,
		},
		{
			name:            "custom string system - inject user/assistant pair",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          "You are a personal assistant running inside OpenClaw.",
			wantInjected:    true,
			wantInjectText:  "You are a personal assistant running inside OpenClaw.",
			wantMessagesLen: 3, // instr + ack + original
		},
		{
			name:            "system equals Claude Code prompt - no injection",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          claudeCodeSystemPrompt,
			wantInjected:    false,
			wantMessagesLen: 1,
		},
		{
			name: "array system with custom blocks - injected as joined instruction",
			body: `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system: []any{
				map[string]any{"type": "text", "text": "First instruction"},
				map[string]any{"type": "text", "text": "Second instruction"},
			},
			wantInjected:    true,
			wantInjectText:  "First instruction\n\nSecond instruction",
			wantMessagesLen: 3,
		},
		{
			name:            "empty array system - no injection",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          []any{},
			wantInjected:    false,
			wantMessagesLen: 1,
		},
		{
			name:            "json.RawMessage string system - injected",
			body:            `{"model":"claude-3","system":"Custom prompt","messages":[{"role":"user","content":"hello"}]}`,
			system:          json.RawMessage(`"Custom prompt"`),
			wantInjected:    true,
			wantInjectText:  "Custom prompt",
			wantMessagesLen: 3,
		},
		{
			name:            "json.RawMessage nil system - no injection",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`,
			system:          json.RawMessage(nil),
			wantInjected:    false,
			wantMessagesLen: 1,
		},
		{
			name:            "multiple original messages preserved with instr+ack prepended",
			body:            `{"model":"claude-3","messages":[{"role":"user","content":"msg1"},{"role":"assistant","content":"resp1"},{"role":"user","content":"msg2"}]}`,
			system:          "Be helpful",
			wantInjected:    true,
			wantInjectText:  "Be helpful",
			wantMessagesLen: 5, // instr + ack + 3 originals
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteSystemForNonClaudeCode([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
			require.NoError(t, err)

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

			// Guard: client fingerprint text must not leak into the CC prompt block.
			if text, _ := systemBlock["text"].(string); text != "" {
				require.False(t, strings.HasPrefix(text, "x-anthropic-billing-header"),
					"system[1] (cc prompt) must not contain billing header")
			}

			// messages: instruction + ack pair + originals when injected.
			messages, ok := parsed["messages"].([]any)
			require.True(t, ok, "messages should be an array")
			require.Len(t, messages, tt.wantMessagesLen)

			if tt.wantInjected {
				// messages[0]: user instruction
				instr, ok := messages[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "user", instr["role"])
				content, ok := instr["content"].([]any)
				require.True(t, ok, "instr content must be array, got %T", instr["content"])
				require.NotEmpty(t, content)
				block, ok := content[0].(map[string]any)
				require.True(t, ok)
				text, _ := block["text"].(string)
				require.True(t, strings.HasPrefix(text, "[System Instructions]\n"),
					"instr text must start with [System Instructions] marker, got: %s", text)
				require.Contains(t, text, tt.wantInjectText)

				// messages[1]: assistant ack
				ack, ok := messages[1].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "assistant", ack["role"])
				ackContent, ok := ack["content"].([]any)
				require.True(t, ok)
				require.NotEmpty(t, ackContent)
				ackBlock, ok := ackContent[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "Understood. I will follow these instructions.", ackBlock["text"])
			}
		})
	}
}
