//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func routingTestAccount(platform, accountType string, credentials, extra map[string]any) *Account {
	if credentials == nil {
		credentials = map[string]any{}
	}
	credentials["api_key"] = "sk-test"
	return &Account{Platform: platform, Type: accountType, Credentials: credentials, Extra: extra}
}

func TestResolveUpstreamProtocol(t *testing.T) {
	t.Parallel()

	const (
		cc        = APIProtocolChatCompletions
		responses = APIProtocolResponses
		anthropic = APIProtocolAnthropic
	)
	adaptive := func(platform string) *Account {
		return routingTestAccount(platform, AccountTypeAPIKey, map[string]any{"api_protocol": APIProtocolAdaptive}, nil)
	}
	pinned := func(platform, protocol string) *Account {
		return routingTestAccount(platform, AccountTypeAPIKey, map[string]any{"api_protocol": protocol}, nil)
	}
	openCode := func(mode string, credentials map[string]any) *Account {
		if credentials == nil {
			credentials = map[string]any{}
		}
		credentials["account_mode"] = mode
		return routingTestAccount(PlatformOpenCodeGo, AccountTypeAPIKey, credentials, nil)
	}
	openAIWithProbe := func(accountType string, supported any) *Account {
		extra := map[string]any{}
		if supported != nil {
			extra[openai_compat.ExtraKeyResponsesSupported] = supported
		}
		return routingTestAccount(PlatformOpenAI, accountType, nil, extra)
	}

	cases := []struct {
		name    string
		account *Account
		inbound string
		model   string
		want    string
	}{
		// 按入站协议分流（国产单一厂商 adaptive）
		{"kimi adaptive anthropic inbound", adaptive(PlatformKimi), anthropic, "", anthropic},
		{"kimi adaptive cc inbound", adaptive(PlatformKimi), cc, "", cc},
		{"kimi adaptive responses inbound", adaptive(PlatformKimi), responses, "", responses},
		{"zhipu adaptive responses inbound falls back to cc", adaptive(PlatformZhipu), responses, "", cc},
		{"zhipu adaptive anthropic inbound", adaptive(PlatformZhipu), anthropic, "", anthropic},

		// 显式固定协议，与入站无关
		{"kimi pinned anthropic", pinned(PlatformKimi, anthropic), cc, "", anthropic},
		{"kimi pinned cc", pinned(PlatformKimi, cc), responses, "", cc},
		{"kimi pinned responses", pinned(PlatformKimi, responses), anthropic, "", responses},
		{"zhipu pinned responses is unsupported and stays cc", pinned(PlatformZhipu, responses), responses, "", cc},

		// 按模型分流（多模型聚合平台），与入站无关
		{"opencode go gpt", openCode(AccountModeGo, nil), anthropic, "gpt-5.6-luna", responses},
		{"opencode go minimax", openCode(AccountModeGo, nil), responses, "minimax-m3", anthropic},
		{"opencode go glm falls back to cc", openCode(AccountModeGo, nil), anthropic, "glm-5.3", cc},
		{"opencode go claude has no rule", openCode(AccountModeGo, nil), anthropic, "claude-sonnet-4", cc},
		{"opencode zen claude", openCode(AccountModeZen, nil), cc, "claude-sonnet-4", anthropic},
		{"opencode account rules override defaults", openCode(AccountModeGo, map[string]any{
			"protocol_rules": []any{map[string]any{"pattern": "glm-*", "protocol": anthropic}},
		}), cc, "glm-5.3", anthropic},
		{"opencode account rules miss falls back to cc", openCode(AccountModeGo, map[string]any{
			"protocol_rules": []any{map[string]any{"pattern": "glm-*", "protocol": anthropic}},
		}), cc, "gpt-5.6-luna", cc},
		{"opencode pinned protocol beats rules", openCode(AccountModeGo, map[string]any{"api_protocol": cc}), anthropic, "gpt-5.6-luna", cc},
		{"opencode ignores responses probe", func() *Account {
			account := openCode(AccountModeGo, nil)
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: false}
			return account
		}(), cc, "gpt-5.6-luna", responses},

		// 无 profile 的账号按探针决定 Responses / CC
		{"openai apikey unknown probe", openAIWithProbe(AccountTypeAPIKey, nil), cc, "", responses},
		{"openai apikey probe unsupported", openAIWithProbe(AccountTypeAPIKey, false), anthropic, "", cc},
		{"openai oauth ignores probe", openAIWithProbe(AccountTypeOAuth, false), cc, "", responses},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, resolveUpstreamProtocol(tc.account, tc.inbound, tc.model, nil), tc.name)
	}
}

func TestRoutesByModelOnlyForAggregators(t *testing.T) {
	t.Parallel()

	aggregators := map[string]bool{PlatformOpenCodeGo: true, PlatformCommandCode: true}
	for platform, profile := range providerProfiles {
		account := &Account{Platform: platform}
		require.Equal(t, aggregators[platform], account.routesByModel(), platform)
		require.Equal(t, aggregators[platform], profile.Routing == ProviderRoutingByModel, platform)
	}
	require.False(t, (&Account{Platform: PlatformOpenAI}).routesByModel())
	require.False(t, (*Account)(nil).routesByModel())
}

func TestUpstreamRoutingModelOnlyForModelRoutedAccounts(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"gpt-5.6-luna","input":"hi"}`)
	openCode := routingTestAccount(PlatformOpenCodeGo, AccountTypeAPIKey, map[string]any{
		"model_mapping": map[string]any{"gpt-5.6-luna": "glm-5.3"},
	}, nil)
	require.Equal(t, "glm-5.3", upstreamRoutingModel(openCode, body, ""))
	require.Empty(t, upstreamRoutingModel(routingTestAccount(PlatformKimi, AccountTypeAPIKey, nil, nil), body, ""))
}

// /v1/responses 转 Anthropic 上游时，计费模型名与上游模型名都应是去除首尾空白的
// 请求模型；按模型分流（OpenCode）与显式 anthropic 协议（Kimi）账号行为一致。
func TestResponsesToNativeAnthropicTrimsBillingModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openCode := &Account{
		ID:          703,
		Name:        "opencode-go",
		Platform:    PlatformOpenCodeGo,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"account_mode": AccountModeGo,
			"api_base_urls": map[string]any{
				APIProtocolAnthropic: "http://anthropic.example",
			},
		},
	}
	for _, tc := range []struct {
		name    string
		account *Account
		model   string
	}{
		{"opencode by model", openCode, "minimax-m3"},
		{"kimi pinned anthropic", nativeAnthropicTestAccount(), "kimi-k2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":" ` + tc.model + ` ","input":"hello","stream":false}`)
			upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			result, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), tc.account, body)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/v1/messages"), upstream.lastReq.URL.String())
			require.Equal(t, tc.model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, tc.model, result.BillingModel)
		})
	}
}
