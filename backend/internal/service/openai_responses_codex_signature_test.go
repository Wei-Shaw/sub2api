package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// includeStrings 取 include 数组的字符串取值，便于断言补齐结果。
func includeStrings(t *testing.T, body []byte) []string {
	t.Helper()
	include := gjson.GetBytes(body, "include")
	require.True(t, include.IsArray(), "include 应为数组: %s", body)
	out := make([]string, 0, len(include.Array()))
	for _, item := range include.Array() {
		out = append(out, item.String())
	}
	return out
}

func TestEnsureOpenAIResponsesCodexSignature(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantChanged bool
		assert      func(t *testing.T, out []byte)
	}{
		{
			name:        "两项都缺时一并补齐",
			body:        `{"model":"gpt-6-astra","instructions":"You are Codex","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, []string{codexSignatureIncludeValue}, includeStrings(t, out))
				require.NotEmpty(t, gjson.GetBytes(out, "prompt_cache_key").String())
			},
		},
		{
			name:        "include 有其它取值时追加且保留原值",
			body:        `{"include":["message.output_text.logprobs"],"input":[{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, []string{"message.output_text.logprobs", codexSignatureIncludeValue}, includeStrings(t, out))
			},
		},
		{
			name:        "签名齐备时原样返回",
			body:        `{"include":["reasoning.encrypted_content"],"prompt_cache_key":"thread-1","input":[{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: false,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, "thread-1", gjson.GetBytes(out, "prompt_cache_key").String())
			},
		},
		{
			name:        "include 齐备但 prompt_cache_key 为空串时补键",
			body:        `{"include":["reasoning.encrypted_content"],"prompt_cache_key":"","input":[{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.NotEmpty(t, gjson.GetBytes(out, "prompt_cache_key").String())
			},
		},
		{
			name:        "prompt_cache_key 非字符串时改写为字符串",
			body:        `{"include":["reasoning.encrypted_content"],"prompt_cache_key":123,"input":[{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, gjson.String, gjson.GetBytes(out, "prompt_cache_key").Type)
				require.NotEmpty(t, gjson.GetBytes(out, "prompt_cache_key").String())
			},
		},
		{
			name:        "include 不是数组时整体改写为数组",
			body:        `{"include":"reasoning.encrypted_content","prompt_cache_key":"thread-1","input":[]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, []string{codexSignatureIncludeValue}, includeStrings(t, out))
			},
		},
		{
			name:        "补丁不改动其它字段",
			body:        `{"model":"gpt-6-astra","instructions":"You are Codex","store":false,"stream":true,"input":[{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: true,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, "gpt-6-astra", gjson.GetBytes(out, "model").String())
				require.Equal(t, "You are Codex", gjson.GetBytes(out, "instructions").String())
				require.False(t, gjson.GetBytes(out, "store").Bool())
				require.True(t, gjson.GetBytes(out, "stream").Bool())
				require.Len(t, gjson.GetBytes(out, "input").Array(), 1)
			},
		},
		{
			name:        "非法 JSON 原样返回",
			body:        `{"model":`,
			wantChanged: false,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, `{"model":`, string(out))
			},
		},
		{
			name:        "非对象 body 原样返回",
			body:        `[1,2,3]`,
			wantChanged: false,
			assert: func(t *testing.T, out []byte) {
				require.Equal(t, `[1,2,3]`, string(out))
			},
		},
		{
			name:        "空 body 原样返回",
			body:        ``,
			wantChanged: false,
			assert:      func(t *testing.T, out []byte) { require.Empty(t, out) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, changed, err := ensureOpenAIResponsesCodexSignature([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)
			tt.assert(t, out)
		})
	}
}

func TestEnsureOpenAIResponsesCodexSignatureDerivedCacheKey(t *testing.T) {
	// 会话标识优先：有 client_metadata.session_id 时直接采用。
	out, changed, err := ensureOpenAIResponsesCodexSignature([]byte(
		`{"client_metadata":{"session_id":"sess-9"},"input":[{"type":"message","role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "sess-9", gjson.GetBytes(out, "prompt_cache_key").String())

	// 会话标识缺失时按 instructions + 首条 input 派生：同一会话的后续轮次
	// input 更长，但共享同一前缀，因此必须得到同一个键。
	first, _, err := ensureOpenAIResponsesCodexSignature([]byte(
		`{"instructions":"You are Codex","input":[{"type":"message","role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	second, _, err := ensureOpenAIResponsesCodexSignature([]byte(
		`{"instructions":"You are Codex","input":[{"type":"message","role":"user","content":"hi"},{"type":"message","role":"assistant","content":"hello"}]}`))
	require.NoError(t, err)
	require.Equal(t,
		gjson.GetBytes(first, "prompt_cache_key").String(),
		gjson.GetBytes(second, "prompt_cache_key").String())

	// 不同会话必须得到不同的键，否则同一账号下会互相污染提示缓存。
	other, _, err := ensureOpenAIResponsesCodexSignature([]byte(
		`{"instructions":"You are Codex","input":[{"type":"message","role":"user","content":"different"}]}`))
	require.NoError(t, err)
	require.NotEqual(t,
		gjson.GetBytes(first, "prompt_cache_key").String(),
		gjson.GetBytes(other, "prompt_cache_key").String())

	// 连 instructions 与 input 都没有时退化为固定兜底值。
	fallback, _, err := ensureOpenAIResponsesCodexSignature([]byte(`{"model":"gpt-6-astra"}`))
	require.NoError(t, err)
	require.Equal(t, codexSignatureCacheKeyFallback, gjson.GetBytes(fallback, "prompt_cache_key").String())
}

func TestIsOpenAIResponsesEnsureCodexSignatureEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name:    "开关开启",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_responses_ensure_codex_signature": true}},
			want:    true,
		},
		{
			name:    "开关显式关闭",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_responses_ensure_codex_signature": false}},
			want:    false,
		},
		{
			name:    "未配置开关时默认关闭",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}},
			want:    false,
		},
		{
			name:    "非 OpenAI 平台不生效",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_responses_ensure_codex_signature": true}},
			want:    false,
		},
		{
			name:    "Extra 为 nil 不 panic",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			want:    false,
		},
		{
			name:    "nil 账号不 panic",
			account: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAIResponsesEnsureCodexSignatureEnabled())
		})
	}
}
