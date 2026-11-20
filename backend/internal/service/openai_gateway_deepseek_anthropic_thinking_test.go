package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// deepSeekAnthropicOfficialAccount 构造 DeepSeek 官方 Anthropic 原生端点账号。
func deepSeekAnthropicOfficialAccount() *Account {
	return &Account{
		ID:       88,
		Name:     "deepseek-anthropic-official",
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"base_url":     DefaultDeepseekBaseURL,
			"api_protocol": APIProtocolAnthropic,
		},
	}
}

// ollamaCloudAnthropicAccount 构造 base_url 指向 ollama.com 的非 DeepSeek 账号。
func ollamaCloudAnthropicAccount() *Account {
	return &Account{
		ID:       89,
		Name:     "ollama-cloud",
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "ollama-key",
			"base_url": "https://ollama.com/v1",
		},
	}
}

func TestEnsureDeepSeekAnthropicThinkingBlocks(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantChanged bool
		check       func(t *testing.T, got []byte)
	}{
		{
			name: "assistant_tool_use_without_thinking_gets_placeholder_block_first",
			body: `{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[` +
				`{"role":"user","content":"hi"},` +
				`{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"exec","input":{}}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"ok"}]}]}`,
			wantChanged: true,
			check: func(t *testing.T, got []byte) {
				t.Helper()
				require.True(t, json.Valid(got))
				require.Equal(t, "thinking", gjson.GetBytes(got, "messages.1.content.0.type").String())
				require.Equal(t, " ", gjson.GetBytes(got, "messages.1.content.0.thinking").String())
				require.False(t, gjson.GetBytes(got, "messages.1.content.0.signature").Exists())
				require.Equal(t, "tool_use", gjson.GetBytes(got, "messages.1.content.1.type").String())
				require.Equal(t, "call_1", gjson.GetBytes(got, "messages.1.content.1.id").String())
				require.Equal(t, "user", gjson.GetBytes(got, "messages.0.role").String())
				require.Equal(t, 3, len(gjson.GetBytes(got, "messages").Array()))
			},
		},
		{
			name: "assistant_with_thinking_and_tool_use_unchanged",
			body: `{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[` +
				`{"role":"assistant","content":[{"type":"thinking","thinking":"real","signature":"sig"},` +
				`{"type":"tool_use","id":"call_1","name":"exec","input":{}}]}]}`,
			wantChanged: false,
			check: func(t *testing.T, got []byte) {
				t.Helper()
				require.Equal(t, 2, len(gjson.GetBytes(got, "messages.0.content").Array()))
				require.Equal(t, "real", gjson.GetBytes(got, "messages.0.content.0.thinking").String())
			},
		},
		{
			name: "assistant_text_only_unchanged",
			body: `{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[` +
				`{"role":"assistant","content":[{"type":"text","text":"done"}]}]}`,
			wantChanged: false,
		},
		{
			name: "only_message_missing_thinking_is_modified",
			body: `{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[` +
				`{"role":"assistant","content":[{"type":"text","text":"first"}]},` +
				`{"role":"assistant","content":[{"type":"tool_use","id":"call_2","name":"exec","input":{}}]},` +
				`{"role":"assistant","content":[{"type":"thinking","thinking":"kept"},{"type":"tool_use","id":"call_3","name":"exec","input":{}}]}]}`,
			wantChanged: true,
			check: func(t *testing.T, got []byte) {
				t.Helper()
				require.True(t, json.Valid(got))
				require.Equal(t, "text", gjson.GetBytes(got, "messages.0.content.0.type").String())
				require.Equal(t, "thinking", gjson.GetBytes(got, "messages.1.content.0.type").String())
				require.Equal(t, "tool_use", gjson.GetBytes(got, "messages.1.content.1.type").String())
				require.Equal(t, "kept", gjson.GetBytes(got, "messages.2.content.0.thinking").String())
				require.Equal(t, 2, len(gjson.GetBytes(got, "messages.2.content").Array()))
			},
		},
		{
			name: "content_string_unchanged",
			body: `{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[` +
				`{"role":"assistant","content":"plain text"}]}`,
			wantChanged: false,
		},
		{
			name:        "messages_not_array_unchanged",
			body:        `{"model":"deepseek-flash","messages":{"role":"assistant","content":[{"type":"tool_use"}]}}`,
			wantChanged: false,
		},
		{
			name:        "invalid_json_unchanged",
			body:        `{"messages":[{"role":"assistant","content":[`,
			wantChanged: false,
		},
		{
			name:        "empty_body_unchanged",
			body:        ``,
			wantChanged: false,
		},
		{
			name:        "empty_messages_unchanged",
			body:        `{"model":"deepseek-flash","messages":[]}`,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got := ensureDeepSeekAnthropicThinkingBlocks(body)
			require.Equal(t, tt.wantChanged, string(got) != tt.body)
			if !tt.wantChanged {
				require.Equal(t, tt.body, string(got), "无需改动时必须原样返回")
				if len(body) > 0 {
					require.True(t, &body[0] == &got[0], "无需改动时必须返回同一份 body")
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestAnthropicBodyThinkingEnabled(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "enabled", body: `{"thinking":{"type":"enabled"}}`, want: true},
		{name: "adaptive", body: `{"thinking":{"type":"adaptive"}}`, want: true},
		{name: "no_thinking_field", body: `{"model":"deepseek-flash"}`, want: true},
		{name: "effort_none_disables", body: `{"output_config":{"effort":"none"}}`, want: false},
		{name: "effort_high_enables", body: `{"output_config":{"effort":"high"}}`, want: true},
		{name: "disabled", body: `{"thinking":{"type":"disabled"}}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, anthropicBodyThinkingEnabled([]byte(tt.body)))
		})
	}
}

func TestShouldRepairDeepSeekAnthropicThinking(t *testing.T) {
	thinkingBody := []byte(`{"model":"deepseek-flash","thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"exec","input":{}}]}]}`)
	disabledBody := []byte(`{"model":"deepseek-flash","thinking":{"type":"disabled"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"exec","input":{}}]}]}`)

	tests := []struct {
		name    string
		account *Account
		body    []byte
		want    bool
	}{
		{
			name:    "deepseek_official_thinking_enabled",
			account: deepSeekAnthropicOfficialAccount(),
			body:    thinkingBody,
			want:    true,
		},
		{
			name:    "deepseek_official_thinking_disabled",
			account: deepSeekAnthropicOfficialAccount(),
			body:    disabledBody,
			want:    false,
		},
		{
			name:    "non_deepseek_upstream",
			account: ollamaCloudAnthropicAccount(),
			body:    thinkingBody,
			want:    false,
		},
		{
			name:    "nil_account",
			account: nil,
			body:    thinkingBody,
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldRepairDeepSeekAnthropicThinking(tt.account, tt.body))
		})
	}
}
