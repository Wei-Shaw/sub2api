package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

// APIReferencePricingSnapshot records the inputs and price card used for an
// immutable API-price comparison. It is not a provider-issued dollar quota.
// Source deliberately says model_catalog: GetModelPricing can use either the
// downloaded catalog or built-in prices, including model-specific policies.
type APIReferencePricingSnapshot struct {
	SchemaVersion       int           `json:"schema_version"`
	Model               string        `json:"model"`
	PricingModel        string        `json:"pricing_model"`
	Source              string        `json:"source"`
	PricingAt           time.Time     `json:"pricing_at"`
	ServiceTier         string        `json:"service_tier"`
	ObservedServiceTier string        `json:"observed_service_tier,omitempty"`
	PriceCard           ModelPricing  `json:"price_card"`
	Tokens              UsageTokens   `json:"tokens"`
	Costs               CostBreakdown `json:"costs"`
}

// applyOpenAIAPIReferenceCost is deliberately independent of customer billing:
// no group/channel resolver or account/user multiplier enters this calculation.
// Unknown prices and unsupported media/tool billing remain NULL, not zero.
func (s *OpenAIGatewayService) applyOpenAIAPIReferenceCost(ctx context.Context, log *UsageLog, account *Account, result *OpenAIForwardResult, tokens UsageTokens, serviceTier string, pricingAt time.Time) {
	if s == nil || s.billingService == nil || log == nil || account == nil || result == nil ||
		!account.IsOpenAIOAuth() || account.IsShadow() || account.IsOpenAIAgentIdentity() || account.IsOpenAIPersonalAccessToken() {
		return
	}
	// These modes require separate catalog units. Do not record a token-only
	// subtotal as if it represented the entire request's reference cost.
	if result.ImageCount > 0 || result.VideoCount > 0 || result.AudioUsage != nil || result.WebSearchCalls > 0 || result.SearchCount > 0 {
		return
	}
	model := strings.TrimSpace(upstreamSentModel(result.Model, result.UpstreamModel))
	pricingModel := model
	if !s.billingService.HasIdentifiedTokenPricing(pricingModel) {
		if normalized := normalizeKnownOpenAICodexModel(model); normalized != "" {
			pricingModel = normalized
		}
	}
	if !s.billingService.HasIdentifiedTokenPricing(pricingModel) {
		return
	}
	resolver := NewModelPricingResolver(nil, s.billingService)
	resolved := resolver.Resolve(ctx, PricingInput{Model: pricingModel})
	if resolved == nil || resolved.BasePricing == nil {
		return
	}
	serviceTier = normalizeBillingServiceTier(serviceTier)
	if pricingAt.IsZero() {
		pricingAt = time.Now()
	}
	cost, err := s.billingService.CalculateCostUnified(CostInput{
		Ctx: ctx, Model: pricingModel, Tokens: tokens, RateMultiplier: 1,
		ServiceTier: serviceTier, PricingAt: pricingAt, Resolver: resolver, Resolved: resolved,
	})
	if err != nil || cost == nil || cost.TotalCost < 0 || math.IsNaN(cost.TotalCost) || math.IsInf(cost.TotalCost, 0) {
		return
	}
	snapshot := &APIReferencePricingSnapshot{
		SchemaVersion: 1, Model: model, PricingModel: pricingModel,
		Source: "model_catalog", PricingAt: pricingAt.UTC(), ServiceTier: serviceTier,
		ObservedServiceTier: normalizeBillingServiceTier(result.UpstreamResponseServiceTier),
		PriceCard:           *resolved.BasePricing, Tokens: tokens, Costs: *cost,
	}
	// A corrupt/non-finite catalog entry must not break usage persistence or
	// create an amount without its corresponding pricing evidence.
	if _, err := json.Marshal(snapshot); err != nil {
		return
	}
	log.APIReferenceCost = &cost.TotalCost
	log.APIReferencePricing = snapshot
}
