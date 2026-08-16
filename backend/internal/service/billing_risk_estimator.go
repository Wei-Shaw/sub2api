package service

import (
	"context"
	"fmt"
	"math"
	"strings"
)

type BillingRiskEstimateInput struct {
	Model                     string
	GroupID                   *int64
	Group                     *Group
	Tokens                    UsageTokens
	RequestCount              int
	UsageUnits                float64
	SizeTier                  string
	RateMultiplier            float64
	ServiceTier               string
	LongContextBillingEnabled *bool
	LongContextThreshold      int
	LongContextMultiplier     float64
}

type BillingRiskEstimate struct {
	Model   string
	Cost    float64
	Certain bool
	Source  string
	Mode    BillingMode
}

// BillingRiskEstimator 复用最终账单的统一定价器，只负责把预估用量换算为费用。
type BillingRiskEstimator struct {
	billing  *BillingService
	resolver *ModelPricingResolver
}

func NewBillingRiskEstimator(billing *BillingService, resolver *ModelPricingResolver) *BillingRiskEstimator {
	return &BillingRiskEstimator{billing: billing, resolver: resolver}
}

func (e *BillingRiskEstimator) Estimate(ctx context.Context, input BillingRiskEstimateInput) (BillingRiskEstimate, error) {
	model := strings.TrimSpace(input.Model)
	result := BillingRiskEstimate{Model: model}
	if e == nil || e.billing == nil || e.resolver == nil {
		return result, fmt.Errorf("余额风险估价服务不可用")
	}
	if model == "" {
		return result, fmt.Errorf("余额风险估价模型不能为空")
	}
	if math.IsNaN(input.RateMultiplier) || math.IsInf(input.RateMultiplier, 0) || input.RateMultiplier < 0 {
		return result, fmt.Errorf("余额风险估价倍率无效")
	}

	resolved := e.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: input.GroupID, Group: input.Group})
	if resolved == nil {
		return result, nil
	}
	result.Source = resolved.Source
	result.Mode = resolved.Mode
	if !billingRiskPricingCertain(e.resolver, resolved, input) {
		return result, nil
	}

	var cost *CostBreakdown
	var err error
	useFixedLongContext := input.LongContextThreshold > 0 && input.LongContextMultiplier > 1 &&
		(input.Group == nil || input.Group.LongContextPricingEnabled) &&
		resolved.Source != PricingSourceGroup && resolved.Source != PricingSourceChannel
	if useFixedLongContext {
		cost, err = e.billing.CalculateCostWithLongContext(
			model, input.Tokens, input.RateMultiplier, input.LongContextThreshold, input.LongContextMultiplier,
		)
	} else {
		cost, err = e.billing.CalculateCostUnified(CostInput{
			Ctx:                       ctx,
			Model:                     model,
			GroupID:                   input.GroupID,
			Group:                     input.Group,
			Tokens:                    input.Tokens,
			RequestCount:              input.RequestCount,
			UsageUnits:                input.UsageUnits,
			SizeTier:                  input.SizeTier,
			RateMultiplier:            input.RateMultiplier,
			ServiceTier:               input.ServiceTier,
			Resolver:                  e.resolver,
			Resolved:                  resolved,
			LongContextBillingEnabled: input.LongContextBillingEnabled,
		})
	}
	if err != nil {
		return result, nil
	}
	if cost == nil || math.IsNaN(cost.ActualCost) || math.IsInf(cost.ActualCost, 0) || cost.ActualCost < 0 {
		return result, fmt.Errorf("余额风险估价结果无效")
	}
	result.Cost = cost.ActualCost
	result.Certain = true
	return result, nil
}

func billingRiskPricingCertain(resolver *ModelPricingResolver, resolved *ResolvedPricing, input BillingRiskEstimateInput) bool {
	if resolver == nil || resolved == nil {
		return false
	}
	switch resolved.Mode {
	case BillingModePerRequest, BillingModeImage, BillingModeVideo:
		if len(resolved.RequestTiers) > 0 {
			return true
		}
		return resolved.channelPricing != nil && resolved.channelPricing.PerRequestPrice != nil
	default:
		totalContext := input.Tokens.InputTokens + input.Tokens.CacheCreationTokens + input.Tokens.CacheReadTokens
		return resolver.GetIntervalPricing(resolved, totalContext) != nil
	}
}
