//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

// ollamaMaxTokensCapTestAccount 构造带自定义 cap 的 Ollama Cloud usage 账号。
func ollamaMaxTokensCapTestAccount(id int64, cap any) *Account {
	account := ollamaUsageAccount(id)
	account.Extra[OllamaCloudMaxTokensCapExtraKey] = cap
	return account
}

func TestOllamaCloudMaxTokensClamp(t *testing.T) {
	ollama := ollamaUsageAccount(101)

	tests := []struct {
		name    string
		account *Account
		body    string
		want    string
		raw     bool // want 非法 JSON 时按原始字节比较
	}{
		{
			name:    "max_tokens above default cap is clamped",
			account: ollama,
			body:    `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
			want:    `{"model":"gpt-oss:120b-cloud","max_tokens":65535}`,
		},
		{
			name:    "max_completion_tokens above default cap is clamped",
			account: ollama,
			body:    `{"model":"gpt-oss:120b-cloud","max_completion_tokens":131072}`,
			want:    `{"model":"gpt-oss:120b-cloud","max_completion_tokens":65535}`,
		},
		{
			name:    "both fields above cap are clamped",
			account: ollama,
			body:    `{"model":"m","max_tokens":80000,"max_completion_tokens":90000}`,
			want:    `{"model":"m","max_tokens":65535,"max_completion_tokens":65535}`,
		},
		{
			name:    "values at or below default cap are kept",
			account: ollama,
			body:    `{"model":"m","max_tokens":65535,"max_completion_tokens":4096}`,
			want:    `{"model":"m","max_tokens":65535,"max_completion_tokens":4096}`,
		},
		{
			name:    "custom extra cap is applied",
			account: ollamaMaxTokensCapTestAccount(102, 32768),
			body:    `{"model":"m","max_tokens":50000}`,
			want:    `{"model":"m","max_tokens":32768}`,
		},
		{
			name:    "extra cap zero disables clamping",
			account: ollamaMaxTokensCapTestAccount(103, 0),
			body:    `{"model":"m","max_tokens":50000}`,
			want:    `{"model":"m","max_tokens":50000}`,
		},
		{
			name:    "non-numeric extra cap falls back to default",
			account: ollamaMaxTokensCapTestAccount(104, "abc"),
			body:    `{"model":"m","max_tokens":100000}`,
			want:    `{"model":"m","max_tokens":65535}`,
		},
		{
			name:    "invalid json is left untouched",
			account: ollama,
			body:    `{"model":"m","max_tokens":`,
			want:    `{"model":"m","max_tokens":`,
			raw:     true,
		},
		{
			name:    "non-integer max_tokens is left untouched",
			account: ollama,
			body:    `{"model":"m","max_tokens":1.5}`,
			want:    `{"model":"m","max_tokens":1.5}`,
		},
		{
			name:    "missing max_tokens is left untouched",
			account: ollama,
			body:    `{"model":"m"}`,
			want:    `{"model":"m"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := clampOllamaCloudMaxTokens(test.account, []byte(test.body))
			if test.raw {
				require.Equal(t, test.want, string(got))
				return
			}
			require.JSONEq(t, test.want, string(got))
		})
	}
}

func TestOllamaCloudMaxTokensCap(t *testing.T) {
	require.Equal(t, int64(65535), ollamaCloudMaxTokensCap(nil))
	require.Equal(t, int64(65535), ollamaCloudMaxTokensCap(ollamaUsageAccount(201)))

	tests := []struct {
		name string
		cap  any
		want int64
	}{
		{"float64", float64(32768), 32768},
		{"int", 40000, 40000},
		{"int64", int64(50000), 50000},
		{"json.Number", json.Number("60000"), 60000},
		{"json.Number invalid", json.Number("abc"), 65535},
		{"zero disables", 0, 0},
		{"negative disables", int64(-1), -1},
		{"string falls back", "abc", 65535},
		{"bool falls back", true, 65535},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account := ollamaMaxTokensCapTestAccount(202, test.cap)
			require.Equal(t, test.want, ollamaCloudMaxTokensCap(account))
		})
	}
}

// TestApplyOllamaCloudRawChatCompletionsRequestClampsMaxTokens 验证 max_tokens clamp
// 已接入组合钩子 applyOllamaCloudRawChatCompletionsRequest，并遵循该钩子的账号判定门槛
// （isOllamaCloudRawChatCompletionsAccount：platform openai + type apikey +
// force_chat_completions + ollama.com 或 Ollama usage extra）。
func TestApplyOllamaCloudRawChatCompletionsRequestClampsMaxTokens(t *testing.T) {
	body := []byte(`{"model":"deepseek-chat","max_tokens":100000}`)

	// Ollama Cloud 账号（ollama.com + force_chat_completions）→ clamp 到 65535。
	ollama := ollamaCloudRawChatCompletionsTestAccount()
	require.JSONEq(t, `{"model":"deepseek-chat","max_tokens":65535}`,
		string(applyOllamaCloudRawChatCompletionsRequest(ollama, body)))

	// 官方 DeepSeek（api.deepseek.com + force_chat_completions）→ 字节级不变。
	official := rawChatCompletionsTestAccount()
	official.Credentials["base_url"] = "https://api.deepseek.com"
	official.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	require.Equal(t, body, applyOllamaCloudRawChatCompletionsRequest(official, body))

	// ollama.com 但无 force_chat_completions（Extra 缺键）→ 不通过钩子判定门槛，字节级不变。
	noForce := ollamaCloudRawChatCompletionsTestAccount()
	noForce.Extra = nil
	require.Equal(t, body, applyOllamaCloudRawChatCompletionsRequest(noForce, body))

	// 空 body → 原样返回。
	require.Equal(t, []byte(nil), applyOllamaCloudRawChatCompletionsRequest(ollama, nil))
	require.Equal(t, []byte{}, applyOllamaCloudRawChatCompletionsRequest(ollama, []byte{}))
}
