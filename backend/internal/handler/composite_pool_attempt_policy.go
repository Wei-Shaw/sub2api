package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// composite 账号池 per-attempt 平台策略（审查修复增量4）。
//
// 设计C契约：7 兼容族池请求进入原 OpenAIGatewayHandler/selector；池请求的公开
// request context / 公开 body 保持 immutable（不把选中平台写回 c.Request）。账号
// 选定后，本文件为该 attempt 构造局部 ctx（WithResolvedTargetPlatform，仅作用于
// 该次渠道/计费/forward），并对实际平台补做 user×platform 配额预检与渠道限制检
// 查——准入 CheckBillingEligibility 时池没有 resolved 平台，这两个维度都被跳过，
// 不得只后扣不预检。
//
// 业务限制的平台级 deny 通过 compositePoolPlatformDenials 记录，再选时以「filtered
// candidate ctx」传 selector 做整平台 mask（不把兄弟账号逐个试穿 maxAccountSwitches，
// 不写 selector 的原 ctx）。所有候选被业务限制时按原 billingErrorDetails / 渠道
// 限制错误语义明确终止，不误报 model unsupported。本文件全部为纯读检查，不写任何
// 生产持久状态、不新增 RPM 计数。

// compositePoolAttemptPolicyApplies 报告当前请求是否按 composite 账号池做
// per-attempt 平台策略检查：composite 分组 + ctx 携带候选池 + 无 resolved 单目标。
// 显式 pin / single / 普通分组保持旧行为，不做 per-attempt 补检。
func compositePoolAttemptPolicyApplies(c *gin.Context, apiKey *service.APIKey) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return false
	}
	if _, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context()); resolved {
		return false
	}
	_, isPool := compositeAccountPoolCandidates(c)
	return isPool
}

// compositePoolAttemptContext 为选中账号派生 attempt 局部 ctx：在原请求 ctx 之上
// 携带 ResolvedTargetPlatform=account.Platform，仅供该 attempt 的渠道作用域、配额
// 预检、forward 与计费使用。base（c.Request.Context()）不被修改；下一次 selector
// 仍看原候选池。已带 resolved 目标的 ctx 原样返回（单目标请求不走池语义）。
func compositePoolAttemptContext(base context.Context, account *service.Account) context.Context {
	if base == nil || account == nil || account.Platform == "" {
		return base
	}
	if _, resolved := service.ResolvedTargetPlatformFromContext(base); resolved {
		return base
	}
	return service.WithResolvedTargetPlatform(base, account.Platform)
}

// compositePoolPlatformDenials 记录一次请求内被业务限制（配额/渠道限制）的平台，
// 供 failover 再选时做整平台 mask。每个请求一个实例，仅在 handler 的顺序 failover
// 循环里读写，无并发访问。
type compositePoolPlatformDenials struct {
	denied map[string]struct{}
}

func newCompositePoolPlatformDenials() *compositePoolPlatformDenials {
	return &compositePoolPlatformDenials{denied: make(map[string]struct{})}
}

func (d *compositePoolPlatformDenials) deny(platform string) {
	if d == nil || platform == "" {
		return
	}
	if d.denied == nil {
		d.denied = make(map[string]struct{})
	}
	d.denied[platform] = struct{}{}
}

// filterCandidates 返回剔除被 deny 平台后的候选副本；入参不被修改。
func (d *compositePoolPlatformDenials) filterCandidates(candidates []string) []string {
	if d == nil || len(d.denied) == 0 {
		return append([]string(nil), candidates...)
	}
	remaining := make([]string, 0, len(candidates))
	for _, platform := range candidates {
		if _, blocked := d.denied[platform]; blocked {
			continue
		}
		remaining = append(remaining, platform)
	}
	return remaining
}

// list 返回已 deny 平台的排序副本（写入 ctx 前归一化由 service 侧完成）。
func (d *compositePoolPlatformDenials) list() []string {
	if d == nil {
		return nil
	}
	platforms := make([]string, 0, len(d.denied))
	for platform := range d.denied {
		platforms = append(platforms, platform)
	}
	return platforms
}

// compositePoolSelectionRetryContext 返回下一次选号使用的 ctx：候选池保持
// immutable（原样保留，不收缩），只叠加请求局部的 deniedPlatforms 列表——
// selector 在 list/sticky 门按该列表过滤，sticky 账号因 denied 暂时排除时按
// excluded 语义提前返回、不清粘性绑定（改由后续成功请求的正常 Set 改绑）。
// 返回 false 表示候选已全部被业务限制，调用方应终止请求而非再选号。未 deny
// 任何平台时原样返回 base。
func compositePoolSelectionRetryContext(base context.Context, denials *compositePoolPlatformDenials) (context.Context, bool) {
	if base == nil {
		return nil, false
	}
	candidates := service.CompositeCandidatePlatformsFromContext(base)
	if len(candidates) == 0 {
		return base, true
	}
	if denials == nil || len(denials.denied) == 0 {
		return base, true
	}
	// 候选已全部被业务限制：终止而不是再选号。
	if len(denials.filterCandidates(candidates)) == 0 {
		return nil, false
	}
	return service.WithCompositePoolDeniedPlatforms(base, denials.list()), true
}

// compositePoolAttemptPolicyFailure 描述选中账号未通过 per-attempt 平台策略的
// 原因。两者互斥评估，但结构上可同时存在（先 quota 后渠道都拒绝时以 quota 为准）。
type compositePoolAttemptPolicyFailure struct {
	// QuotaErr 非 nil 表示 user×platform 日/周/月配额耗尽
	//（ErrUserPlatform{Daily,Weekly,Monthly}QuotaExhausted，带 window_resets_at
	// metadata），按 billingErrorDetails 语义终止。
	QuotaErr error
	// ChannelRestricted 表示该模型在选中平台作用域下被渠道定价限制拒绝，按调度
	// 阶段渠道限制同语义终止（账号存在且模型受支持，仅当前平台被限制）。
	ChannelRestricted bool
}

func (f compositePoolAttemptPolicyFailure) denied() bool {
	return f.QuotaErr != nil || f.ChannelRestricted
}

// compositePoolAttemptPolicy 汇总一次 attempt 的策略检查结果。
type compositePoolAttemptPolicy struct {
	// AttemptCtx 仅供该 attempt 的渠道/计费/forward 使用；不写回 c.Request。
	AttemptCtx context.Context
	// Mapping 是选中平台作用域的渠道映射；公开 body immutable，channel-mapped
	// body 由调用方从 canonical body 派生。
	Mapping service.ChannelMappingResult
	// Failure 非 denied 时才允许 forward。
	Failure compositePoolAttemptPolicyFailure
}

// evaluateCompositePoolAttemptPolicy 对选中账号做 attempt 级平台策略检查：
//  1. user×platform 配额预检（纯检查 + cache 回填，不写 RPM、不扣费、不再调用
//     CheckBillingEligibility——那会二次计数）；
//  2. 平台作用域渠道限制（requested/channel_mapped/response 计费基准 + upstream
//     基准的逐账号检查，与调度阶段同语义、仅作用域为选中平台）。
//
// requireCompact 须与本次选号调用一致（/responses/compact 路径为 true）。
func evaluateCompositePoolAttemptPolicy(
	baseCtx context.Context,
	billing *service.BillingCacheService,
	gateway *service.OpenAIGatewayService,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	account *service.Account,
	requestedModel string,
	requireCompact bool,
) compositePoolAttemptPolicy {
	result := compositePoolAttemptPolicy{AttemptCtx: baseCtx}
	if baseCtx == nil || account == nil || account.Platform == "" {
		return result
	}
	result.AttemptCtx = compositePoolAttemptContext(baseCtx, account)
	result.Failure.QuotaErr = compositePoolAttemptQuotaError(billing, result.AttemptCtx, apiKey, subscription, account.Platform)
	if result.Failure.QuotaErr != nil {
		// 平台配额已耗尽：渠道映射仍按平台作用域解析，但请求不再外发。
		if gateway != nil && apiKey != nil && apiKey.GroupID != nil {
			result.Mapping = gateway.CompositePoolAttemptChannelMapping(result.AttemptCtx, apiKey.GroupID, requestedModel)
		}
		return result
	}
	if gateway != nil && apiKey != nil && apiKey.GroupID != nil {
		result.Mapping = gateway.CompositePoolAttemptChannelMapping(result.AttemptCtx, apiKey.GroupID, requestedModel)
		result.Failure.ChannelRestricted = gateway.CompositePoolAttemptChannelRestricted(result.AttemptCtx, apiKey.GroupID, account, requestedModel, requireCompact)
	}
	return result
}

// compositePoolAttemptQuotaError 是 service 纯配额预检的 handler 侧薄入口。
func compositePoolAttemptQuotaError(billing *service.BillingCacheService, attemptCtx context.Context, apiKey *service.APIKey, subscription *service.UserSubscription, platform string) error {
	if billing == nil || apiKey == nil || platform == "" {
		return nil
	}
	return billing.CheckUserPlatformQuotaEligibilityForRequest(attemptCtx, apiKey.User, apiKey.Group, subscription, platform)
}

// respondCompositePoolAttemptPolicyFailure 按 deny 原因以既有错误语义明确终止请求：
//   - 配额耗尽 → billingErrorDetails（429 rate_limit_exceeded + Retry-After）；
//   - 渠道限制 → 503 api_error（与调度阶段渠道限制的分类一致；账号与模型持久
//     配置都存在，不误报 model_not_found / model unsupported）。
//
// write 由调用方按入站端点选择（OpenAI 信封 handleStreamingAwareError / Anthropic
// 信封 anthropicStreamingAwareError），并自行闭包 streamStarted。
func respondCompositePoolAttemptPolicyFailure(c *gin.Context, failure compositePoolAttemptPolicyFailure, write func(*gin.Context, int, string, string)) {
	if write == nil || !failure.denied() {
		return
	}
	if failure.QuotaErr != nil {
		status, code, message, retryAfter := billingErrorDetails(failure.QuotaErr)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		write(c, status, code, message)
		return
	}
	write(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
}

// compositePoolAccountDelegatesToOpenAI 报告池请求选中的账号是否需要委派 OpenAI
// 网关的 /v1/messages 兼容链转发。generic（Anthropic 原生）转发路径覆盖：
// anthropic 原生、gemini / antigravity 既有调度平台，以及能解析出 Anthropic 协议
// 上游 base 的 anthropic-协议 / adaptive 多协议账号（service 层已按账号协议化
// 上游 base）；其余纯 OpenAI 族账号（openai/grok/国产供应商/OpenCode Go，无
// Anthropic 协议能力）由委派链完成协议转换。仅池激活时调用。
func compositePoolAccountDelegatesToOpenAI(account *service.Account) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case service.PlatformAnthropic, service.PlatformGemini, service.PlatformAntigravity:
		return false
	}
	if account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol() {
		// 多协议账号走既有 generic Forward（servable 门已保证 Anthropic 协议
		// base 非空）；不应停留在 Chat Completions 协议上被误委派。
		return false
	}
	return true
}

// compositePoolAccountDelegatesResponsesToOpenAI 报告池请求选中的账号在入站
// /v1/responses 时是否委派 OpenAI 网关 Responses 链（OpenAIGatewayService.Forward）
// 转发。入站同族优先（零或近零转换）：纯 OpenAI 族账号（openai/grok/国产 CC 固定/
// OpenCode Go）与具备原生 Responses 端点的国产账号（UsesNativeCNResponses，含
// fixed responses 与 adaptive）交该链按账号协议直连或轻量转换；anthropic 原生 /
// gemini / antigravity / anthropic-协议 / 无原生 Responses 的 adaptive 账号走
// 既有 generic Responses→Anthropic 转换。仅池激活时调用。
func compositePoolAccountDelegatesResponsesToOpenAI(account *service.Account) bool {
	if account == nil {
		return false
	}
	return compositePoolAccountDelegatesToOpenAI(account) || account.UsesNativeCNResponses()
}

// compositePoolAccountDelegatesChatCompletionsToOpenAI 报告池请求选中的账号在入站
// /v1/chat/completions 时是否委派 OpenAI 网关 CC 链（OpenAIGatewayService.
// ForwardAsChatCompletions）转发。入站同族优先（零转换）：OpenAI 兼容族账号交该链，
// 其中 adaptive 国产账号由该链直转供应商原生 CC 端点（forwardAsChatCompletions 的
// adaptive 分支，与其在纯 OpenAI 族池中的行为一致）；anthropic-协议 / gemini /
// antigravity 走既有 generic CC→Anthropic 转换。仅池激活时调用。
func compositePoolAccountDelegatesChatCompletionsToOpenAI(account *service.Account) bool {
	if account == nil {
		return false
	}
	return account.IsOpenAICompatible() && !account.IsAnthropicProtocol()
}

// adaptOpenAIForwardResultToForwardResult 把委派链（OpenAIGatewayService 转发方法）
// 的转发结果适配进 generic 计费链的 ForwardResult：usage token 计数、模型、请求
// 元数据与图片/搜索计费字段逐字段映射，经既有 usage 提交流入账，计费不丢。
// nil 安全；错误路径携带的部分结果（流中断排水 usage）同样适配。
func adaptOpenAIForwardResultToForwardResult(src *service.OpenAIForwardResult) *service.ForwardResult {
	if src == nil {
		return nil
	}
	return &service.ForwardResult{
		RequestID:       src.RequestID,
		UpstreamHeaders: src.UpstreamHeaders,
		Usage: service.ClaudeUsage{
			InputTokens:              src.Usage.InputTokens,
			OutputTokens:             src.Usage.OutputTokens,
			CacheCreationInputTokens: src.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     src.Usage.CacheReadInputTokens,
			ImageOutputTokens:        src.Usage.ImageOutputTokens,
		},
		Model:                         src.Model,
		UpstreamModel:                 src.UpstreamModel,
		UpstreamResponseModel:         src.UpstreamResponseModel,
		UpstreamResponseModelConflict: src.UpstreamResponseModelConflict,
		UpstreamResponseServiceTier:   src.UpstreamResponseServiceTier,
		Stream:                        src.Stream,
		Duration:                      src.Duration,
		FirstTokenMs:                  src.FirstTokenMs,
		ClientDisconnect:              src.ClientDisconnect,
		ReasoningEffort:               src.ReasoningEffort,
		RequestedReasoningEffort:      src.RequestedReasoningEffort,
		ServiceTier:                   src.ServiceTier,
		SearchCount:                   src.SearchCount,
		AudioUsage:                    src.AudioUsage,
		// 生图计费字段：/v1/responses 是 token+图片混合计费端点，委派链产出的
		// 图片尺寸/数量必须随 usage 进入 generic 计费链（B2 扩展）。
		ImageCount:         src.ImageCount,
		ImageSize:          src.ImageSize,
		ImageInputSize:     src.ImageInputSize,
		ImageOutputSize:    src.ImageOutputSize,
		ImageOutputSizes:   src.ImageOutputSizes,
		ImageSizeSource:    src.ImageSizeSource,
		ImageSizeBreakdown: src.ImageSizeBreakdown,
	}
}
