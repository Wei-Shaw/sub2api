//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// compact 请求体按字段白名单重建：已据以路由的 access_programs 必须存活。
func TestNormalizeOpenAICompactRequestBodyPreservesAccessPrograms(t *testing.T) {
	body := []byte(`{"model":"gpt-6-sol","input":[{"type":"message","role":"user","content":"compact me"}],"access_programs":{"cyber":"daybreak_blue"},"store":true}`)
	normalized, changed, err := normalizeOpenAICompactRequestBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "daybreak_blue", gjson.GetBytes(normalized, "access_programs.cyber").String())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

// WS → HTTP 桥的请求体重建与 OAuth 透传归一化都不得丢弃 access_programs。
func TestOpenAIRebuiltBodiesPreserveAccessPrograms(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	bridged, err := prepareOpenAIWSHTTPBridgeBody(account, []byte(`{"type":"response.create","model":"gpt-6-sol","input":"hi","access_programs":{"cyber":"daybreak_blue"}}`))
	require.NoError(t, err)
	require.Equal(t, "daybreak_blue", gjson.GetBytes(bridged, "access_programs.cyber").String())
	require.False(t, gjson.GetBytes(bridged, "type").Exists())

	passthrough, _, err := normalizeOpenAIPassthroughOAuthBody([]byte(`{"model":"gpt-6-sol","input":"hi","stream":true,"access_programs":{"cyber":"daybreak_red"},"metadata":{"x":1}}`), false)
	require.NoError(t, err)
	require.Equal(t, "daybreak_red", gjson.GetBytes(passthrough, "access_programs.cyber").String())
	require.False(t, gjson.GetBytes(passthrough, "metadata").Exists())

	chatBody := forwardOAuthChatCompletionsForUpstreamBody(t, []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false,"access_programs":{"cyber":"daybreak_blue"}}`))
	require.Equal(t, "daybreak_blue", gjson.GetBytes(chatBody, "access_programs.cyber").String(), "Chat Completions → Responses 转换保留 access_programs")
}
