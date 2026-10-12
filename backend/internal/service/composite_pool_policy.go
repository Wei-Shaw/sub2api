package service

import "context"

// composite 账号池 per-attempt 渠道策略薄包装。
//
// 背景（审查 BUG B）：channelLookupPlatform 的作用域取 ForcePlatform >
// ResolvedTargetPlatform（composite 分组）> 分组平台。池请求在选号前 ctx 没有
// resolved 平台，调度阶段 checkChannelPricingRestriction / 渠道映射按 composite
// 分组跨全部具体平台行匹配，渠道映射与定价限制被绕过。
//
// 修复方式：handler 在账号选定后构造 attempt 局部 ctx
//（WithResolvedTargetPlatform(originalCtx, account.Platform)，仅供该 attempt 的
// 渠道/计费/forward），再用本文件的入口做平台作用域策略检查。这里只复用既有
// 语义、不新增渠道服务行为：
//   - CompositePoolAttemptChannelRestricted 与调度阶段同一组合检查：
//     checkChannelPricingRestriction（requested/channel_mapped/response 计费基准）
//     + BillingModelSource=upstream 时的逐账号 upstream 模型检查，仅作用域换为
//     选中平台。
//   - CompositePoolAttemptChannelMapping 返回平台作用域的渠道映射，供
//     per-attempt 派生 channel-mapped body 与 usage 字段。
//
// 全部为纯读 + lazy cache 加载：不扣费、不写计数、不改任何持久状态。

// compositePoolDeniedPlatformsKey 携带一次请求内的 deniedPlatforms 列表。
// 与收缩候选池不同：候选池保持 immutable，selector 在 list/sticky 门上按本列表
// 过滤，sticky 账号因 denied 暂时排除时按 excluded 语义提前返回、不清粘性绑定。
type compositePoolDeniedPlatformsKey struct{}

// WithCompositePoolDeniedPlatforms 在 ctx 上写入 deniedPlatforms 副本（去重升序）。
// 空列表不改写 ctx。请求局部、不落任何持久状态。
func WithCompositePoolDeniedPlatforms(ctx context.Context, platforms []string) context.Context {
	normalized := normalizeCompositeCandidatePlatforms(platforms)
	if ctx == nil || len(normalized) == 0 {
		return ctx
	}
	return context.WithValue(ctx, compositePoolDeniedPlatformsKey{}, normalized)
}

// CompositePoolDeniedPlatformsFromContext 返回当前请求的 deniedPlatforms 快照
// （副本）；未携带时返回 nil。导出供 handler 断言与诊断。
func CompositePoolDeniedPlatformsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	platforms, ok := ctx.Value(compositePoolDeniedPlatformsKey{}).([]string)
	if !ok {
		return nil
	}
	out := make([]string, len(platforms))
	copy(out, platforms)
	return out
}

// compositePoolPlatformDenied 报告平台是否被本次请求的策略 deny 暂时排除。
func compositePoolPlatformDenied(ctx context.Context, platform string) bool {
	if ctx == nil || platform == "" {
		return false
	}
	for _, denied := range CompositePoolDeniedPlatformsFromContext(ctx) {
		if denied == platform {
			return true
		}
	}
	return false
}

// CompositePoolAttemptChannelRestricted 报告选中账号平台作用域下该模型是否被
// 渠道定价限制拒绝。ctx 必须是带 ResolvedTargetPlatform=account.Platform 的
// attempt 局部 ctx。语义与调度阶段逐候选检查一致（checkChannelPricingRestriction
// + upstream 计费基准的逐账号检查），不重复检查用户配额或 RPM。
func (s *OpenAIGatewayService) CompositePoolAttemptChannelRestricted(ctx context.Context, groupID *int64, account *Account, requestedModel string, requireCompact bool) bool {
	if s == nil || groupID == nil || account == nil || requestedModel == "" {
		return false
	}
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		return true
	}
	if s.needsUpstreamChannelRestrictionCheck(ctx, groupID) &&
		s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, requestedModel, requireCompact) {
		return true
	}
	return false
}

// CompositePoolAttemptChannelMapping 返回选中账号平台作用域的渠道级模型映射。
// ctx 必须是带 ResolvedTargetPlatform=account.Platform 的 attempt 局部 ctx，
// 保证映射只在该平台的映射行内解析，不跨平台串行。公开请求模型仍由调用方持有，
// 映射结果仅用于该 attempt 的 body 派生 / 计费字段。
func (s *OpenAIGatewayService) CompositePoolAttemptChannelMapping(ctx context.Context, groupID *int64, requestedModel string) ChannelMappingResult {
	if s == nil || groupID == nil {
		return ChannelMappingResult{MappedModel: requestedModel}
	}
	return s.ResolveChannelMapping(ctx, *groupID, requestedModel)
}
