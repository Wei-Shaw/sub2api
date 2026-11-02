package service

import (
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSFailoverRouteModes(t *testing.T) {
	for _, router := range []bool{false, true} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			for _, mode := range []string{
				OpenAIWSIngressModeOff, OpenAIWSIngressModeCtxPool,
				OpenAIWSIngressModePassthrough, OpenAIWSIngressModeHTTPBridge,
			} {
				t.Run(fmt.Sprintf("router=%t/%s/%s", router, accountType, mode), func(t *testing.T) {
					cfg := &config.Config{}
					cfg.Gateway.OpenAIWS.Enabled = true
					cfg.Gateway.OpenAIWS.OAuthEnabled = true
					cfg.Gateway.OpenAIWS.APIKeyEnabled = true
					cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
					cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = router
					cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
					prefix := "openai_apikey_responses_websockets_v2_"
					if accountType == AccountTypeOAuth {
						prefix = "openai_oauth_responses_websockets_v2_"
					}
					account := &Account{
						Platform: PlatformOpenAI, Type: accountType, Concurrency: 1,
						Extra: map[string]any{
							prefix + "mode":    mode,
							prefix + "enabled": mode != OpenAIWSIngressModeOff,
						},
					}
					svc := &OpenAIGatewayService{cfg: cfg, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
					decision := svc.getOpenAIWSProtocolResolver().Resolve(account)
					expected := OpenAIUpstreamTransportResponsesWebsocketV2
					if mode == OpenAIWSIngressModeOff || (router && mode == OpenAIWSIngressModeHTTPBridge) {
						expected = OpenAIUpstreamTransportHTTPSSE
					}
					require.Equal(t, expected, decision.Transport)
					require.Equal(t, mode != OpenAIWSIngressModeOff,
						svc.isOpenAIAccountTransportCompatible(account, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))
					require.Equal(t, OpenAIUpstreamTransportHTTPSSE,
						resolveOpenAIWSDecisionByClientTransport(decision, OpenAIClientTransportHTTP).Transport)
				})
			}
		}
	}
}
