package service

import "github.com/tidwall/gjson"

func (s *OpenAIGatewayService) resolveOpenAIWSIngressMode(account *Account) string {
	if account.Platform == PlatformGrok || (s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account)) {
		return OpenAIWSIngressModeHTTPBridge
	}
	if s.cfg != nil && s.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled {
		return account.ResolveOpenAIResponsesWebSocketV2Mode(s.cfg.Gateway.OpenAIWS.IngressModeDefault)
	}
	return OpenAIWSIngressModeCtxPool
}

// ResolveOpenAIWSAccountAdmissionMode resolves the pre-dial admission mode.
// The forwarder reports the final mode again after payload normalization.
func (s *OpenAIGatewayService) ResolveOpenAIWSAccountAdmissionMode(account *Account, payload []byte) string {
	mode := s.resolveOpenAIWSIngressMode(account)
	if mode == OpenAIWSIngressModePassthrough {
		if s.shouldBridgeOpenAIWSPassthroughFirstMessage(account, payload) {
			return OpenAIWSIngressModeHTTPBridge
		}
	} else if mode != OpenAIWSIngressModeOff && s.shouldBridgeOpenAIWSHTTP(account, len(payload), gjson.GetBytes(payload, "previous_response_id").String()) {
		return OpenAIWSIngressModeHTTPBridge
	}
	return mode
}

func (s *OpenAIGatewayService) OpenAIWSAccountWaitPlan(account *Account) *AccountWaitPlan {
	cfg := s.schedulingConfig()
	return &AccountWaitPlan{AccountID: account.ID, MaxConcurrency: account.Concurrency, Timeout: cfg.StickySessionWaitTimeout, MaxWaiting: cfg.StickySessionMaxWaiting}
}
