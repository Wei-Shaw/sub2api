package service

import (
	"context"
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

func TestOpenAIAdmissionOptionsFromContext(t *testing.T) {
	require.Equal(t, OpenAIAdmissionOptions{ContinuationEligible: true}, openAIAdmissionOptionsFromContext(context.Background()), "缺省合格、不改逃逸")
	ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: false, StickyFullWaits: true})
	require.Equal(t, OpenAIAdmissionOptions{ContinuationEligible: false, StickyFullWaits: true}, openAIAdmissionOptionsFromContext(ctx))
}

func TestOpenAIWSAccountWaitPlanIsContinuationClass(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 120 * time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	cfg.Gateway.Scheduling.ContinuationMaxWaiting = 100
	svc := &OpenAIGatewayService{cfg: cfg}
	plan := svc.OpenAIWSAccountWaitPlan(&Account{ID: 31, Concurrency: 2})
	require.Equal(t, AccountWaitClassContinuation, plan.Class)
	require.Equal(t, int64(31), plan.AccountID)
	require.Equal(t, 100, plan.MaxWaiting)
}

func TestAccountWaitClassString(t *testing.T) {
	require.Equal(t, "legacy", AccountWaitClassLegacy.String())
	require.Equal(t, "new_session", AccountWaitClassNewSession.String())
	require.Equal(t, "continuation", AccountWaitClassContinuation.String())
}

func TestOpenAIWSTurnSlotHold(t *testing.T) {
	require.Equal(t, time.Duration(0), (&OpenAIGatewayService{}).OpenAIWSTurnSlotHold(PlatformOpenAI))
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds = 10
	require.Equal(t, 10*time.Second, (&OpenAIGatewayService{cfg: cfg}).OpenAIWSTurnSlotHold(PlatformOpenAI))
	cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds = 0
	require.Equal(t, time.Duration(0), (&OpenAIGatewayService{cfg: cfg}).OpenAIWSTurnSlotHold(PlatformOpenAI))
}
