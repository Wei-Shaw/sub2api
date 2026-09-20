package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSAccountAdmissionMode(t *testing.T) {
	for _, tc := range []struct {
		name, mode, payload, want string
		legacy, bridge            bool
	}{
		{name: "pool", mode: OpenAIWSIngressModeCtxPool, want: OpenAIWSIngressModeCtxPool},
		{name: "passthrough", mode: OpenAIWSIngressModePassthrough, want: OpenAIWSIngressModePassthrough},
		{name: "bridge", mode: OpenAIWSIngressModeHTTPBridge, want: OpenAIWSIngressModeHTTPBridge},
		{name: "off", mode: OpenAIWSIngressModeOff, bridge: true, want: OpenAIWSIngressModeOff},
		{name: "legacy", mode: OpenAIWSIngressModePassthrough, legacy: true, want: OpenAIWSIngressModeCtxPool},
		{name: "legacy_auto_bridge", legacy: true, bridge: true, want: OpenAIWSIngressModeHTTPBridge},
		{name: "pool_auto_bridge", mode: OpenAIWSIngressModeCtxPool, bridge: true, want: OpenAIWSIngressModeHTTPBridge},
		{name: "passthrough_auto_bridge", mode: OpenAIWSIngressModePassthrough, bridge: true, want: OpenAIWSIngressModeHTTPBridge},
		{name: "passthrough_continuation", mode: OpenAIWSIngressModePassthrough, bridge: true, payload: `{"type":"response.create","previous_response_id":"resp_existing"}`, want: OpenAIWSIngressModePassthrough},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = !tc.legacy
			cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = tc.bridge
			cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
			s := &OpenAIGatewayService{cfg: cfg}
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_apikey_responses_websockets_v2_mode": tc.mode}}
			payload := tc.payload
			if payload == "" {
				payload = `{"type":"response.create","model":"gpt-5.1","input":"hello"}`
			}
			require.Equal(t, tc.want, s.ResolveOpenAIWSAccountAdmissionMode(account, []byte(payload)))
		})
	}
}

func TestOpenAIWSAccountWaitPlanUsesStickyLimits(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 7 * time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 4
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Minute
	s := &OpenAIGatewayService{cfg: cfg}
	plan := s.OpenAIWSAccountWaitPlan(&Account{ID: 801, Concurrency: 2})
	require.Equal(t, &AccountWaitPlan{AccountID: 801, MaxConcurrency: 2, Timeout: 7 * time.Second, MaxWaiting: 4}, plan)
}
