package service

import (
	"context"
	"time"
)

// LegacyLongContextRule 平台级"超出阈值部分按倍率计费"的旧规则。
//
// 语义为边际计费：仅 input/cache_read 中超过 Threshold 的部分乘 Multiplier，
// output 与 cache_write 不受影响（见 CalculateCostWithLongContext）。
// 只有 Gemini 原生 /v1beta 入口在该分组对该模型没有分组/渠道定价时才使用；
// 规则常量由 BillingService 统一持有，网关与模型广场都从这里读取。
type LegacyLongContextRule struct {
	Threshold  int
	Multiplier float64
}

const (
	geminiLegacyLongContextThreshold  = 200000
	geminiLegacyLongContextMultiplier = 2.0
)

// LegacyLongContextRule 返回平台的旧长上下文规则；无规则的平台返回 nil。
func (s *BillingService) LegacyLongContextRule(platform string) *LegacyLongContextRule {
	if platform == PlatformGemini {
		return &LegacyLongContextRule{
			Threshold:  geminiLegacyLongContextThreshold,
			Multiplier: geminiLegacyLongContextMultiplier,
		}
	}
	return nil
}

// TokenCostRequest 通用网关 token 计费请求。
type TokenCostRequest struct {
	Ctx            context.Context
	Model          string
	Group          *Group
	Tokens         UsageTokens
	RateMultiplier float64
	PricingAt      time.Time
	ServiceTier    string
	Resolver       *ModelPricingResolver
	// Resolved 为调用方预先解析的定价（Resolver.Resolve 的结果），nil 表示未解析。
	Resolved *ResolvedPricing
	// LegacyLongContext 入口携带的旧长上下文规则，nil 表示该入口不使用。
	LegacyLongContext *LegacyLongContextRule
}

// legacyLongContextApplies 判定请求是否走旧长上下文规则：
// 分组/渠道显式定价优先；否则在规则存在且分组长上下文开关开启时生效。
func legacyLongContextApplies(resolved *ResolvedPricing, group *Group, rule *LegacyLongContextRule) bool {
	if rule == nil || rule.Threshold <= 0 {
		return false
	}
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		return false
	}
	return group == nil || group.LongContextPricingEnabled
}

// CalculateTokenCostForRequest 按通用网关的路径选择计算 token 费用：
//  1. 分组/渠道显式定价 → 统一计费（区间、分组卡、目录阶梯均在其中）；
//  2. 否则入口带旧长上下文规则且分组开关开启 → 旧边际计费；
//  3. 否则有解析器与分组 → 统一计费（内置目录定价）；
//  4. 否则按模型目录直接计费。
//
// 模型广场的阶梯表查询与网关使用同一入口，保证展示与扣费同源。
func (s *BillingService) CalculateTokenCostForRequest(req TokenCostRequest) (*CostBreakdown, error) {
	resolved := req.Resolved
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		return s.CalculateCostUnified(s.tokenCostInput(req, resolved))
	}
	if legacyLongContextApplies(resolved, req.Group, req.LegacyLongContext) {
		return s.CalculateCostWithLongContext(req.Model, req.Tokens, req.RateMultiplier,
			req.LegacyLongContext.Threshold, req.LegacyLongContext.Multiplier)
	}
	if req.Resolver != nil && req.Group != nil {
		return s.CalculateCostUnified(s.tokenCostInput(req, resolved))
	}
	return s.CalculateCost(req.Model, req.Tokens, req.RateMultiplier)
}

func (s *BillingService) tokenCostInput(req TokenCostRequest, resolved *ResolvedPricing) CostInput {
	input := CostInput{
		Ctx:            req.Ctx,
		Model:          req.Model,
		Group:          req.Group,
		Tokens:         req.Tokens,
		RequestCount:   1,
		RateMultiplier: req.RateMultiplier,
		PricingAt:      req.PricingAt,
		ServiceTier:    req.ServiceTier,
		Resolver:       req.Resolver,
		Resolved:       resolved,
	}
	if req.Group != nil {
		gid := req.Group.ID
		input.GroupID = &gid
	}
	return input
}
