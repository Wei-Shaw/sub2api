//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ollamaCloudTestAccount 构造官方默认 base_url（credentials 不带 base_url）的
// ollama_cloud 账号，让测试同时钉住「正确回落平台默认端点」这件事。
func ollamaCloudTestAccount(id int64, credentials map[string]any) *Account {
	base := map[string]any{
		"api_key": "sk-ollama-test",
	}
	for key, value := range credentials {
		base[key] = value
	}
	return &Account{
		ID:          id,
		Name:        "ollama-cloud-test",
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: base,
		Extra: map[string]any{
			OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud"},
		},
	}
}

// C36 核心验收：adaptive（ollama_cloud 默认协议）账号依次探测 CC → Anthropic →
// Responses 三个原生端点，URL 正确、无 ?beta=true，不再落入 Claude 测试路径。
func TestAccountTestService_OllamaCloudAdaptiveProbesAllThreeEndpoints(t *testing.T) {
	account := ollamaCloudTestAccount(501, map[string]any{"api_protocol": APIProtocolAdaptive})
	svc, upstream := adaptiveCNAccountTestService(
		account,
		adaptiveCNChatTestResponse(),
		adaptiveCNAnthropicTestResponse(),
		adaptiveCNResponsesTestResponse(),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-oss:120b-cloud", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "https://ollama.com/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, "https://ollama.com/v1/messages", upstream.requests[1].URL.String())
	require.Equal(t, "https://ollama.com/v1/responses", upstream.requests[2].URL.String())
	for i, req := range upstream.requests {
		require.Empty(t, req.URL.RawQuery, "request %d must not carry the Claude tester ?beta=true suffix", i)
	}
	// ollama 官方 host 的 Anthropic 端点强制 Bearer（非 CN 的 x-api-key）。
	require.Equal(t, "Bearer sk-ollama-test", upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[1].Header.Get("x-api-key"))
	// Responses 探针按无状态端点归一化：store=false。
	require.False(t, gjson.GetBytes(upstream.bodies[2], "store").Bool())

	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_start"`))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
	require.Contains(t, recorder.Body.String(), "已通过原生 /responses 验证")
}

// 空模型默认取账号 allowed_models 首项（保存期门禁保证非空清单存在）。
func TestAccountTestService_OllamaCloudEmptyModelDefaultsToAllowedModels(t *testing.T) {
	account := ollamaCloudTestAccount(502, map[string]any{"api_protocol": APIProtocolChatCompletions})
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "gpt-oss:120b-cloud", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), "gpt-oss:120b-cloud")
}

// 钉 chat_completions：单端点探测，不触发 Anthropic/Responses。
func TestAccountTestService_OllamaCloudPinnedChatProtocol(t *testing.T) {
	account := ollamaCloudTestAccount(503, map[string]any{"api_protocol": APIProtocolChatCompletions})
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-oss:120b-cloud", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://ollama.com/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-ollama-test", upstream.requests[0].Header.Get("Authorization"))
	require.NotContains(t, upstream.requests[0].URL.Path, "/messages")
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 钉 anthropic：官方默认 Anthropic base（https://ollama.com）+ /v1/messages，
// 无 ?beta=true，Bearer 鉴权。
func TestAccountTestService_OllamaCloudPinnedAnthropicProtocol(t *testing.T) {
	account := ollamaCloudTestAccount(504, map[string]any{"api_protocol": APIProtocolAnthropic})
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://ollama.com/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)
	require.Equal(t, "Bearer sk-ollama-test", req.Header.Get("Authorization"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 钉 responses：单端点 /v1/responses + 无状态归一化（store=false）。
func TestAccountTestService_OllamaCloudPinnedResponsesProtocol(t *testing.T) {
	account := ollamaCloudTestAccount(505, map[string]any{"api_protocol": APIProtocolResponses})
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-oss:120b-cloud", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://ollama.com/v1/responses", req.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(req.Context()))
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}
