//go:build unit

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// commandCodeTestAccount 只带 api_key：端点与分流规则全部来自 provider profile。
func commandCodeTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "cc",
		Platform:    PlatformCommandCode,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "user_test_key"},
	}
}

func TestCommandCodeProfileRoutesByModel(t *testing.T) {
	account := commandCodeTestAccount(1)
	require.True(t, account.IsMultiProtocolAPIKey())
	require.True(t, account.routesByModel())
	require.False(t, account.RoutesProtocolByInbound())
	require.Equal(t, APIProtocolAdaptive, account.GetAPIProtocol())
	require.Equal(t, DefaultCommandCodeBaseURL, account.GetOpenAIBaseURL())
	require.Equal(t, DefaultCommandCodeAnthropicBaseURL, account.GetCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, DefaultCommandCodeBaseURL, account.GetCNProtocolBaseURL(APIProtocolResponses))

	// 未取到模型目录时按内置规则：Claude 只走 Anthropic；GPT 支持 Responses 与 Chat
	// Completions（同协议直通，Anthropic 入站走首选 Responses）；其余只保证 Chat Completions。
	for _, inbound := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		require.Equal(t, APIProtocolAnthropic, resolveUpstreamProtocol(account, inbound, "claude-sonnet-4-6", nil), inbound)
		require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, inbound, "deepseek/deepseek-v4-flash", nil), inbound)
	}
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "gpt-5.5", nil))
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolChatCompletions, "gpt-5.5", nil))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolAnthropic, "gpt-5.5", nil))

	// 显式协议优先于内置规则。
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolAnthropic, "claude-sonnet-4-6", nil))
}

func TestCommandCodeNativeAnthropicTargetURL(t *testing.T) {
	svc := &OpenAIGatewayService{}
	targetURL, err := svc.nativeAnthropicTargetURL(commandCodeTestAccount(2))
	require.NoError(t, err)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/messages", targetURL)
}

func TestAccountTestService_CommandCodeClaudeUsesAnthropicMessages(t *testing.T) {
	account := commandCodeTestAccount(501)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "claude-sonnet-4-6", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/messages", upstream.requests[0].URL.String())
	require.Equal(t, "user_test_key", upstream.requests[0].Header.Get("x-api-key"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 连接测试与转发共用端点拼接：按模型分流的平台，Anthropic 基址带 /v1 也合法。
func TestAccountTestService_CommandCodeAnthropicBaseWithVersion(t *testing.T) {
	account := commandCodeTestAccount(504)
	account.Credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: "https://api.commandcode.ai/provider/v1"}
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "claude-sonnet-4-6", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/messages", upstream.requests[0].URL.String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)

	target, err := (&OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}).nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, upstream.requests[0].URL.String(), target)
}

// 连接测试与网关同一判定：模型目录决定协议时，测试也按目录选择端点。
func TestAccountTestService_CommandCodeUsesModelCatalog(t *testing.T) {
	base := fmt.Sprintf("http://cc-test-%d.example/provider/v1", time.Now().UnixNano())
	account := commandCodeTestAccount(505)
	account.Credentials["api_base_urls"] = map[string]any{
		APIProtocolChatCompletions: base,
		APIProtocolResponses:       base,
	}
	url := buildOpenAIModelsURL(base)
	upstreamModelProtocols.store(modelProtocolCatalogKey(account, base, url),
		map[string][]string{"vendor/responses-only": {APIProtocolResponses}}, nil, time.Now())
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	svc.openaiGatewayService = &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "vendor/responses-only", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, base+"/responses", upstream.requests[0].URL.String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_CommandCodeGPTUsesResponses(t *testing.T) {
	account := commandCodeTestAccount(502)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.5", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/responses", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer user_test_key", upstream.requests[0].Header.Get("Authorization"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_CommandCodeEmptyModelUsesProfileDefault(t *testing.T) {
	account := commandCodeTestAccount(503)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, DefaultCommandCodeTestModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 仅在 profile 登记、按入站协议分流的新供应商：adaptive 测试走原生端点探测，
// 跳过 profile 未提供的 Anthropic 端点，不再落入 Claude 测试路径。
func TestAccountTestService_ByInboundProviderSkipsMissingNativeEndpoints(t *testing.T) {
	const platform = "test_inbound_only_chat"
	providerProfiles[platform] = &ProviderProfile{
		Platform: platform, DefaultMode: AccountModePayG, Routing: ProviderRoutingByInbound,
		DefaultTestModel: "inbound-test-model",
		Modes: map[string]ProviderEndpoints{AccountModePayG: {BaseURLs: map[string]string{
			APIProtocolChatCompletions: "https://inbound.example.com/v1",
		}}},
	}
	t.Cleanup(func() { delete(providerProfiles, platform) })

	account := &Account{
		ID: 601, Name: "inbound", Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-inbound", "api_protocol": APIProtocolAdaptive},
	}
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://inbound.example.com/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, "inbound-test-model", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 官方主机不做上游计费探测：中转站的计费接口在官方 API 上不存在，只会把 Key 发往官方主机。
func TestCommandCodeOfficialHostSkipsUpstreamBillingProbe(t *testing.T) {
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI(DefaultCommandCodeBaseURL))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI(DefaultCommandCodeAnthropicBaseURL))
}
