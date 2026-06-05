package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func modelsListCacheKey(groupID *int64, platform string) string {
	return fmt.Sprintf("%d|%s", derefGroupID(groupID), strings.TrimSpace(platform))
}

// isModelSupportedByAccountWithContext 根据账户平台检查模型支持（带 context）
// 对于 Antigravity 平台，会先获取映射后的最终模型名（包括 thinking 后缀）再检查支持
func (s *GatewayService) isModelSupportedByAccountWithContext(ctx context.Context, account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		// 使用与转发阶段一致的映射逻辑：自定义映射优先 → 默认映射兜底
		mapped := mapAntigravityModel(account, requestedModel)
		if mapped == "" {
			return false
		}
		// 应用 thinking 后缀后检查最终模型是否在账号映射中
		if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
			finalModel := applyThinkingModelSuffix(mapped, enabled)
			if finalModel == mapped {
				return true // thinking 后缀未改变模型名，映射已通过
			}
			return account.IsModelSupported(finalModel)
		}
		return true
	}
	// Try plugin IsModelSupported RPC for plugin-owned platforms.
	// If the plugin returns an answer, use it; on error (including
	// codes.Unimplemented), fall back to the static check.
	if checker := s.loadPluginModelSupportChecker(); checker != nil {
		if supported, err := checker.Check(ctx, account, requestedModel); err == nil {
			return supported
		}
	}
	return s.isModelSupportedByAccount(account, requestedModel)
}

// isModelSupportedByAccount 根据账户平台检查模型支持（无 context，用于非 Antigravity 平台）
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	if account.IsBedrock() {
		_, ok := ResolveBedrockModelID(account, requestedModel)
		return ok
	}
	// OpenAI 透传模式：仅替换认证，允许所有模型
	if account.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
		return true
	}
	// OAuth/SetupToken 账号使用 Anthropic 标准映射（短ID → 长ID）
	if account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		if account.Type == AccountTypeServiceAccount {
			requestedModel = normalizeVertexAnthropicModelID(claude.NormalizeModelID(requestedModel))
		} else {
			requestedModel = claude.NormalizeModelID(requestedModel)
		}
	}
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
}

// ResolveChannelMapping 委托渠道缓存解析模型映射
func (s *GatewayService) ResolveChannelMapping(ctx context.Context, groupID int64, platform, model string) ChannelMappingResult {
	if s.channelCacheReader == nil {
		return ChannelMappingResult{MappedModel: model}
	}
	return s.channelCacheReader.ResolveChannelMapping(ctx, groupID, platform, model)
}

// ReplaceModelInBody 替换请求体中的模型名（导出供 handler 使用）
func (s *GatewayService) ReplaceModelInBody(body []byte, newModel string) []byte {
	return ReplaceModelInBody(body, newModel)
}

// IsModelRestricted 检查模型是否被渠道限制
func (s *GatewayService) IsModelRestricted(ctx context.Context, groupID int64, platform, model string) bool {
	if s.channelCacheReader == nil {
		return false
	}
	return s.channelCacheReader.IsModelRestricted(ctx, groupID, platform, model)
}

// ResolveChannelMappingAndRestrict 解析渠道映射。
// 模型限制检查已移至调度阶段（checkChannelPricingRestriction），restricted 始终返回 false。
func (s *GatewayService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, platform, model string) (ChannelMappingResult, bool) {
	if s.channelCacheReader == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return s.channelCacheReader.ResolveChannelMappingAndRestrict(ctx, groupID, platform, model)
}

// checkChannelPricingRestriction 根据渠道计费基准检查模型是否受定价列表限制。
// 供调度阶段预检查（requested / channel_mapped）。
// upstream 需逐账号检查，此处返回 false。
func (s *GatewayService) checkChannelPricingRestriction(ctx context.Context, groupID *int64, platform, requestedModel string) bool {
	if groupID == nil || s.channelCacheReader == nil || requestedModel == "" || platform == "" {
		return false
	}
	mapping := s.channelCacheReader.ResolveChannelMapping(ctx, *groupID, platform, requestedModel)
	billingModel := billingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
	if billingModel == "" {
		return false
	}
	return s.channelCacheReader.IsModelRestricted(ctx, *groupID, platform, billingModel)
}

// billingModelForRestriction 根据计费基准确定限制检查使用的模型。
// upstream 返回空（需逐账号检查）。
func billingModelForRestriction(source, requestedModel, channelMappedModel string) string {
	switch source {
	case BillingModelSourceRequested:
		return requestedModel
	case BillingModelSourceUpstream:
		return ""
	case BillingModelSourceChannelMapped:
		return channelMappedModel
	default:
		return channelMappedModel
	}
}

// isUpstreamModelRestrictedByChannel 检查账号映射后的上游模型是否受渠道定价限制。
// 仅在 BillingModelSource="upstream" 且 RestrictModels=true 时由调度循环调用。
func (s *GatewayService) isUpstreamModelRestrictedByChannel(ctx context.Context, groupID int64, account *Account, requestedModel string) bool {
	if s.channelCacheReader == nil || account == nil {
		return false
	}
	upstreamModel := resolveAccountUpstreamModel(account, requestedModel)
	if upstreamModel == "" {
		return false
	}
	return s.channelCacheReader.IsModelRestricted(ctx, groupID, account.Platform, upstreamModel)
}

// resolveAccountUpstreamModel 确定账号将请求模型映射为什么上游模型。
func resolveAccountUpstreamModel(account *Account, requestedModel string) string {
	if account.Platform == PlatformAntigravity {
		return mapAntigravityModel(account, requestedModel)
	}
	return account.GetMappedModel(requestedModel)
}

// needsUpstreamChannelRestrictionCheck 判断是否需要在调度循环中逐账号检查上游模型的渠道限制。
func (s *GatewayService) needsUpstreamChannelRestrictionCheck(ctx context.Context, groupID *int64, platform string) bool {
	if groupID == nil || s.channelCacheReader == nil || platform == "" {
		return false
	}
	meta, ok := s.channelCacheReader.GetChannelMeta(ctx, *groupID, platform)
	if !ok || !meta.RestrictModels {
		return false
	}
	return meta.BillingModelSource == BillingModelSourceUpstream
}

// isStickyAccountUpstreamRestricted 检查粘性会话命中的账号是否受 upstream 渠道限制。
// 合并 needsUpstreamChannelRestrictionCheck + isUpstreamModelRestrictedByChannel 两步调用，
// 供 sticky session 条件链使用，避免内联多个函数调用导致行过长。
// platform 取自 account.Platform（粘性账号的渠道键就在该平台维度下）。
func (s *GatewayService) isStickyAccountUpstreamRestricted(ctx context.Context, groupID *int64, account *Account, requestedModel string) bool {
	if groupID == nil || account == nil {
		return false
	}
	if !s.needsUpstreamChannelRestrictionCheck(ctx, groupID, account.Platform) {
		return false
	}
	return s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, requestedModel)
}

// GetAvailableModels returns the list of models available for a group
// It aggregates model_mapping keys from all schedulable accounts in the group
func (s *GatewayService) GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string {
	cacheKey := modelsListCacheKey(groupID, platform)
	if s.modelsListCache != nil {
		if cached, found := s.modelsListCache.Get(cacheKey); found {
			if models, ok := cached.([]string); ok {
				modelsListCacheHitTotal.Add(1)
				return cloneStringSlice(models)
			}
		}
	}
	modelsListCacheMissTotal.Add(1)

	var accounts []Account
	var err error

	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	} else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
	}

	if err != nil || len(accounts) == 0 {
		return nil
	}

	// Filter by platform if specified
	if platform != "" {
		filtered := make([]Account, 0)
		for _, acc := range accounts {
			if acc.Platform == platform {
				filtered = append(filtered, acc)
			}
		}
		accounts = filtered
	}

	// Collect unique models from all accounts
	modelSet := make(map[string]struct{})
	hasAnyMapping := false

	for _, acc := range accounts {
		mapping := acc.GetModelMapping()
		if len(mapping) > 0 {
			hasAnyMapping = true
			for model := range mapping {
				modelSet[model] = struct{}{}
			}
		}
	}

	// If no account has model_mapping, return nil (use default)
	if !hasAnyMapping {
		if s.modelsListCache != nil {
			s.modelsListCache.Set(cacheKey, []string(nil), s.modelsListCacheTTL)
			modelsListCacheStoreTotal.Add(1)
		}
		return nil
	}

	// Convert to slice
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)

	if s.modelsListCache != nil {
		s.modelsListCache.Set(cacheKey, cloneStringSlice(models), s.modelsListCacheTTL)
		modelsListCacheStoreTotal.Add(1)
	}
	return cloneStringSlice(models)
}
func (s *GatewayService) InvalidateAvailableModelsCache(groupID *int64, platform string) {
	if s == nil || s.modelsListCache == nil {
		return
	}

	normalizedPlatform := strings.TrimSpace(platform)
	// 完整匹配时精准失效；否则按维度批量失效。
	if groupID != nil && normalizedPlatform != "" {
		s.modelsListCache.Delete(modelsListCacheKey(groupID, normalizedPlatform))
		return
	}

	targetGroup := derefGroupID(groupID)
	for key := range s.modelsListCache.Items() {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		groupPart, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil {
			continue
		}
		if groupID != nil && groupPart != targetGroup {
			continue
		}
		if normalizedPlatform != "" && parts[1] != normalizedPlatform {
			continue
		}
		s.modelsListCache.Delete(key)
	}
}

// reconcileCachedTokens 兼容 Kimi 等上游：
// 将 OpenAI 风格的 cached_tokens 映射到 Claude 标准的 cache_read_input_tokens
func reconcileCachedTokens(usage map[string]any) bool {
	if usage == nil {
		return false
	}
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	if cacheRead > 0 {
		return false // 已有标准字段，无需处理
	}
	cached, _ := usage["cached_tokens"].(float64)
	if cached <= 0 {
		return false
	}
	usage["cache_read_input_tokens"] = cached
	return true
}
