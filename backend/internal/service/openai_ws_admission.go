package service

import (
	"context"
	"time"

	"github.com/tidwall/gjson"
)

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

// OpenAIAdmissionOptions 由入口在调用调度器前给出，随 ctx 进入调度请求。
type OpenAIAdmissionOptions struct {
	// ContinuationEligible 为真表示会话哈希来自真实会话标识：命中粘性或 previous_response 时
	// 计划标续聊类并参与账号连续准入计数。回退种子哈希的连接置假，计划标新会话类。
	ContinuationEligible bool
	// StickyFullWaits 为真时高级调度对粘性账号满槽不逃逸，改返回粘性等待计划。只有 WS 入口置位。
	StickyFullWaits bool
	// DeferStickyBinding 为真时 bindOpenAIStickySessionDuringSelection 不做选号期写入，
	// 由入口在准入完成后按策略补写。WS 入口（仅 openai 平台）置位：透传续聊的不可迁移判定
	// 要读到选号前的原绑定，否则加权粘性子模式在负载层选中别的账号时会先改写绑定，判定随之失效。
	// 非高级调度的选号期写入只发生在无活绑定时（溢出不写），不受本字段影响。
	DeferStickyBinding bool
}

type openAIAdmissionOptionsContextKey struct{}

func WithOpenAIAdmissionOptions(ctx context.Context, opts OpenAIAdmissionOptions) context.Context {
	return context.WithValue(ctx, openAIAdmissionOptionsContextKey{}, opts)
}

func openAIAdmissionOptionsFromContext(ctx context.Context) OpenAIAdmissionOptions {
	if ctx != nil {
		if opts, ok := ctx.Value(openAIAdmissionOptionsContextKey{}).(OpenAIAdmissionOptions); ok {
			return opts
		}
	}
	return OpenAIAdmissionOptions{ContinuationEligible: true}
}

func (s *OpenAIGatewayService) OpenAIWSAccountWaitPlan(account *Account) *AccountWaitPlan {
	cfg := s.schedulingConfig()
	if !openAITieredAdmissionApplies(account.Platform) {
		return stickyWaitPlanFor(cfg, account, true)
	}
	return &AccountWaitPlan{AccountID: account.ID, MaxConcurrency: account.Concurrency, Timeout: cfg.StickySessionWaitTimeout, MaxWaiting: cfg.ContinuationMaxWaiting, Class: AccountWaitClassContinuation}
}

// OpenAIWSTurnSlotHold 返回轮间账号槽保留时长；0 表示关闭。只作用于 openai 平台。
func (s *OpenAIGatewayService) OpenAIWSTurnSlotHold(platform string) time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds <= 0 || !openAITieredAdmissionApplies(platform) {
		return 0
	}
	return time.Duration(s.cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds) * time.Second
}
