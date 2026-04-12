package service

import (
	"context"
	"sort"
	"strings"
)

// resolveAccountStatsCost 计算账号统计定价费用。
// 返回 nil 表示不覆盖，使用默认公式（total_cost × account_rate_multiplier）。
// 仅匹配自定义规则（AccountStatsPricingRules），按数组顺序先命中为准。
// upstreamModel 是最终发往上游的模型 ID，用于匹配自定义规则中的模型定价。
func resolveAccountStatsCost(
	ctx context.Context,
	channelService *ChannelService,
	accountID int64,
	groupID int64,
	upstreamModel string,
	tokens UsageTokens,
	requestCount int,
) *float64 {
	if channelService == nil || upstreamModel == "" {
		return nil
	}
	channel, err := channelService.GetChannelForGroup(ctx, groupID)
	if err != nil || channel == nil || !channel.ApplyPricingToAccountStats {
		return nil
	}

	platform := channelService.GetGroupPlatform(ctx, groupID)
	return tryCustomRules(channel, accountID, groupID, platform, upstreamModel, tokens, requestCount)
}

// tryCustomRules 遍历自定义规则，按数组顺序先命中为准。
func tryCustomRules(
	channel *Channel, accountID, groupID int64,
	platform, model string, tokens UsageTokens, requestCount int,
) *float64 {
	modelLower := strings.ToLower(model)
	for _, rule := range channel.AccountStatsPricingRules {
		if !matchAccountStatsRule(&rule, accountID, groupID) {
			continue
		}
		pricing := findPricingForModel(rule.Pricing, platform, modelLower)
		if pricing == nil {
			continue // 规则匹配但模型不在规则定价中，继续下一条
		}
		return calculateStatsCost(pricing, tokens, requestCount)
	}
	return nil
}

// matchAccountStatsRule 检查规则是否匹配指定的 accountID 和 groupID。
// 匹配条件：accountID ∈ rule.AccountIDs 或 groupID ∈ rule.GroupIDs。
// 如果规则的 AccountIDs 和 GroupIDs 都为空，视为不匹配。
func matchAccountStatsRule(rule *AccountStatsPricingRule, accountID, groupID int64) bool {
	if len(rule.AccountIDs) == 0 && len(rule.GroupIDs) == 0 {
		return false
	}
	for _, id := range rule.AccountIDs {
		if id == accountID {
			return true
		}
	}
	for _, id := range rule.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

// wildcardMatch 通配符匹配候选项（用于排序）
type wildcardMatch struct {
	prefixLen int
	pricing   *ChannelModelPricing
}

// findPricingForModel 在定价列表中查找匹配的模型定价。
// 先精确匹配，再通配符匹配（前缀越长优先级越高）。
func findPricingForModel(pricingList []ChannelModelPricing, platform, modelLower string) *ChannelModelPricing {
	// 精确匹配优先
	for i := range pricingList {
		p := &pricingList[i]
		if !isPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			if strings.ToLower(m) == modelLower {
				return p
			}
		}
	}
	// 通配符匹配：收集所有匹配项，按前缀长度降序取最长
	var matches []wildcardMatch
	for i := range pricingList {
		p := &pricingList[i]
		if !isPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			ml := strings.ToLower(m)
			if !strings.HasSuffix(ml, "*") {
				continue
			}
			prefix := strings.TrimSuffix(ml, "*")
			if strings.HasPrefix(modelLower, prefix) {
				matches = append(matches, wildcardMatch{prefixLen: len(prefix), pricing: p})
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].prefixLen > matches[j].prefixLen
	})
	return matches[0].pricing
}

// isPlatformMatch 判断平台是否匹配（空平台视为不限平台）。
func isPlatformMatch(queryPlatform, pricingPlatform string) bool {
	if queryPlatform == "" || pricingPlatform == "" {
		return true
	}
	return queryPlatform == pricingPlatform
}

// calculateStatsCost 使用给定的定价计算费用（不含任何倍率，原始费用）。
func calculateStatsCost(pricing *ChannelModelPricing, tokens UsageTokens, requestCount int) *float64 {
	if pricing == nil {
		return nil
	}
	switch pricing.BillingMode {
	case BillingModePerRequest, BillingModeImage:
		return calculatePerRequestStatsCost(pricing, requestCount)
	default:
		return calculateTokenStatsCost(pricing, tokens)
	}
}

// calculatePerRequestStatsCost 按次/图片计费。
func calculatePerRequestStatsCost(pricing *ChannelModelPricing, requestCount int) *float64 {
	if pricing.PerRequestPrice == nil || *pricing.PerRequestPrice <= 0 {
		return nil
	}
	cost := *pricing.PerRequestPrice * float64(requestCount)
	return &cost
}

// calculateTokenStatsCost Token 计费。
func calculateTokenStatsCost(pricing *ChannelModelPricing, tokens UsageTokens) *float64 {
	deref := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}
	cost := float64(tokens.InputTokens)*deref(pricing.InputPrice) +
		float64(tokens.OutputTokens)*deref(pricing.OutputPrice) +
		float64(tokens.CacheCreationTokens)*deref(pricing.CacheWritePrice) +
		float64(tokens.CacheReadTokens)*deref(pricing.CacheReadPrice) +
		float64(tokens.ImageOutputTokens)*deref(pricing.ImageOutputPrice)
	if cost == 0 {
		return nil
	}
	return &cost
}
