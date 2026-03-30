package service

import (
	"context"
	"log/slog"
)

// ResolvedPricing 统一定价解析结果
type ResolvedPricing struct {
	// Mode 计费模式
	Mode BillingMode

	// Token 模式：基础定价（来自 LiteLLM 或 fallback）
	BasePricing *ModelPricing

	// Token 模式：区间定价列表（如有，覆盖 BasePricing 中的对应字段）
	Intervals []PricingInterval

	// 按次/图片模式：分层定价
	RequestTiers []PricingInterval

	// 按次/图片模式：默认价格（未命中层级时使用）
	DefaultPerRequestPrice float64

	// 来源标识
	Source string // "channel", "litellm", "fallback"

	// 是否支持缓存细分
	SupportsCacheBreakdown bool
}

// ModelPricingResolver 统一模型定价解析器。
// 解析链：Channel → LiteLLM → Fallback。
type ModelPricingResolver struct {
	channelService *ChannelService
	billingService *BillingService
}

// NewModelPricingResolver 创建定价解析器实例
func NewModelPricingResolver(channelService *ChannelService, billingService *BillingService) *ModelPricingResolver {
	return &ModelPricingResolver{
		channelService: channelService,
		billingService: billingService,
	}
}

// PricingInput 定价解析输入
type PricingInput struct {
	Model   string
	GroupID *int64 // nil 表示不检查渠道
}

// Resolve 解析模型定价。
// 1. 获取基础定价（LiteLLM → Fallback）
// 2. 如果指定了 GroupID，查找渠道定价并覆盖
func (r *ModelPricingResolver) Resolve(ctx context.Context, input PricingInput) *ResolvedPricing {
	// 1. 获取基础定价
	basePricing, source := r.resolveBasePricing(input.Model)

	resolved := &ResolvedPricing{
		Mode:                   BillingModeToken,
		BasePricing:            basePricing,
		Source:                 source,
		SupportsCacheBreakdown: basePricing != nil && basePricing.SupportsCacheBreakdown,
	}

	// 2. 如果有 GroupID，尝试渠道覆盖
	if input.GroupID != nil {
		r.applyChannelOverrides(ctx, *input.GroupID, input.Model, resolved)
	}

	return resolved
}

// resolveBasePricing 从 LiteLLM 或 Fallback 获取基础定价
func (r *ModelPricingResolver) resolveBasePricing(model string) (*ModelPricing, string) {
	pricing, err := r.billingService.GetModelPricing(model)
	if err != nil {
		slog.Debug("failed to get model pricing from LiteLLM, using fallback",
			"model", model, "error", err)
		return nil, "fallback"
	}
	return pricing, "litellm"
}

// applyChannelOverrides 应用渠道定价覆盖
func (r *ModelPricingResolver) applyChannelOverrides(ctx context.Context, groupID int64, model string, resolved *ResolvedPricing) {
	chPricing := r.channelService.GetChannelModelPricing(ctx, groupID, model)
	if chPricing == nil {
		return
	}

	resolved.Source = "channel"
	resolved.Mode = chPricing.BillingMode
	if resolved.Mode == "" {
		resolved.Mode = BillingModeToken
	}

	switch resolved.Mode {
	case BillingModeToken:
		r.applyTokenOverrides(chPricing, resolved)
	case BillingModePerRequest, BillingModeImage:
		r.applyRequestTierOverrides(chPricing, resolved)
	}
}

// applyTokenOverrides 应用 token 模式的渠道覆盖
func (r *ModelPricingResolver) applyTokenOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	// 如果有区间定价，使用区间
	if len(chPricing.Intervals) > 0 {
		resolved.Intervals = chPricing.Intervals
		return
	}

	// 否则用 flat 字段覆盖 BasePricing
	if resolved.BasePricing == nil {
		resolved.BasePricing = &ModelPricing{}
	}

	if chPricing.InputPrice != nil {
		resolved.BasePricing.InputPricePerToken = *chPricing.InputPrice
		resolved.BasePricing.InputPricePerTokenPriority = *chPricing.InputPrice
	}
	if chPricing.OutputPrice != nil {
		resolved.BasePricing.OutputPricePerToken = *chPricing.OutputPrice
		resolved.BasePricing.OutputPricePerTokenPriority = *chPricing.OutputPrice
	}
	if chPricing.CacheWritePrice != nil {
		resolved.BasePricing.CacheCreationPricePerToken = *chPricing.CacheWritePrice
		resolved.BasePricing.CacheCreation5mPrice = *chPricing.CacheWritePrice
		resolved.BasePricing.CacheCreation1hPrice = *chPricing.CacheWritePrice
	}
	if chPricing.CacheReadPrice != nil {
		resolved.BasePricing.CacheReadPricePerToken = *chPricing.CacheReadPrice
		resolved.BasePricing.CacheReadPricePerTokenPriority = *chPricing.CacheReadPrice
	}
}

// applyRequestTierOverrides 应用按次/图片模式的渠道覆盖
func (r *ModelPricingResolver) applyRequestTierOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	resolved.RequestTiers = chPricing.Intervals
	if chPricing.PerRequestPrice != nil {
		resolved.DefaultPerRequestPrice = *chPricing.PerRequestPrice
	}
}

// GetIntervalPricing 根据 context token 数获取区间定价。
// 如果有区间列表，找到匹配区间并构造 ModelPricing；否则直接返回 BasePricing。
func (r *ModelPricingResolver) GetIntervalPricing(resolved *ResolvedPricing, totalContextTokens int) *ModelPricing {
	if len(resolved.Intervals) == 0 {
		return resolved.BasePricing
	}

	iv := FindMatchingInterval(resolved.Intervals, totalContextTokens)
	if iv == nil {
		return resolved.BasePricing
	}

	return intervalToModelPricing(iv, resolved.SupportsCacheBreakdown)
}

// intervalToModelPricing 将区间定价转换为 ModelPricing
func intervalToModelPricing(iv *PricingInterval, supportsCacheBreakdown bool) *ModelPricing {
	pricing := &ModelPricing{
		SupportsCacheBreakdown: supportsCacheBreakdown,
	}
	if iv.InputPrice != nil {
		pricing.InputPricePerToken = *iv.InputPrice
		pricing.InputPricePerTokenPriority = *iv.InputPrice
	}
	if iv.OutputPrice != nil {
		pricing.OutputPricePerToken = *iv.OutputPrice
		pricing.OutputPricePerTokenPriority = *iv.OutputPrice
	}
	if iv.CacheWritePrice != nil {
		pricing.CacheCreationPricePerToken = *iv.CacheWritePrice
		pricing.CacheCreation5mPrice = *iv.CacheWritePrice
		pricing.CacheCreation1hPrice = *iv.CacheWritePrice
	}
	if iv.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *iv.CacheReadPrice
		pricing.CacheReadPricePerTokenPriority = *iv.CacheReadPrice
	}
	return pricing
}

// GetRequestTierPrice 根据层级标签获取按次价格
func (r *ModelPricingResolver) GetRequestTierPrice(resolved *ResolvedPricing, tierLabel string) float64 {
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == tierLabel && tier.PerRequestPrice != nil {
			return *tier.PerRequestPrice
		}
	}
	return 0
}

// GetRequestTierPriceByContext 根据 context token 数获取按次价格
func (r *ModelPricingResolver) GetRequestTierPriceByContext(resolved *ResolvedPricing, totalContextTokens int) float64 {
	iv := FindMatchingInterval(resolved.RequestTiers, totalContextTokens)
	if iv != nil && iv.PerRequestPrice != nil {
		return *iv.PerRequestPrice
	}
	return 0
}
