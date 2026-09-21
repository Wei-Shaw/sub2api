package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestOpenAIWSIngressTransportRejectsTypeSafeAccount 钉住「说法一」的两个机制点：
// typesafe 账号在 WSv2 ingress 下解析出的 mode 恒为 off（ResolveOpenAIResponsesWebSocketV2Mode
// 对 !IsOpenAI() 直接返回 off），因此 isOpenAIAccountTransportCompatible 判定其与
// OpenAIUpstreamTransportResponsesWebsocketV2Ingress 不兼容 —— 调度器不会选中它。
func TestOpenAIWSIngressTransportRejectsTypeSafeAccount(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	svc := &OpenAIGatewayService{cfg: cfg}

	typesafeAccount := &Account{
		ID:          1,
		Platform:    PlatformTypeSafe,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		// 即便显式写死 ctx_pool，非 openai 平台也无法获得 WS ingress 资格。
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
		},
	}
	require.Equal(t, OpenAIWSIngressModeOff,
		typesafeAccount.ResolveOpenAIResponsesWebSocketV2Mode(cfg.Gateway.OpenAIWS.IngressModeDefault))
	require.False(t, svc.isOpenAIAccountTransportCompatible(
		typesafeAccount, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))

	openAIAccount := &Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
		},
	}
	require.True(t, svc.isOpenAIAccountTransportCompatible(
		openAIAccount, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))
}
