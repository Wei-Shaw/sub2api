package dto

import (
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
)

// ChannelToResponse maps a service.Channel into the JSON-wire ChannelResponse.
//
// All slice and map fields are normalised to non-nil so the frontend never
// sees JSON null where it expects an empty list. BillingModelSource defaults
// to service.BillingModelSourceChannelMapped when the service struct leaves
// it blank — the historical wire format always carried a value.
func ChannelToResponse(ch *service.Channel) *ChannelResponse {
	if ch == nil {
		return nil
	}
	resp := channelMainFieldsToResponse(ch)
	applyChannelResponseDefaults(resp)
	resp.ModelPricing = pricingsToResponse(ch.ModelPricing)
	resp.AccountStatsPricingRules = rulesToResponse(ch.AccountStatsPricingRules)
	return resp
}

// channelMainFieldsToResponse copies the primitive / scalar fields from the
// service struct into the JSON DTO. Slice / map fields are intentionally
// not normalised here — see applyChannelResponseDefaults.
func channelMainFieldsToResponse(ch *service.Channel) *ChannelResponse {
	return &ChannelResponse{
		ID:                         ch.ID,
		Name:                       ch.Name,
		Description:                ch.Description,
		Status:                     ch.Status,
		BillingModelSource:         ch.BillingModelSource,
		RestrictModels:             ch.RestrictModels,
		Features:                   ch.Features,
		FeaturesConfig:             ch.FeaturesConfig,
		ApplyPricingToAccountStats: ch.ApplyPricingToAccountStats,
		GroupIDs:                   ch.GroupIDs,
		ModelMapping:               ch.ModelMapping,
		CreatedAt:                  ch.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:                  ch.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// applyChannelResponseDefaults normalises BillingModelSource plus the slice
// and map fields so the wire format never carries JSON null where the
// frontend expects a value or an empty container.
func applyChannelResponseDefaults(resp *ChannelResponse) {
	if resp.BillingModelSource == "" {
		resp.BillingModelSource = service.BillingModelSourceChannelMapped
	}
	if resp.GroupIDs == nil {
		resp.GroupIDs = []int64{}
	}
	if resp.FeaturesConfig == nil {
		resp.FeaturesConfig = map[string]any{}
	}
	if resp.ModelMapping == nil {
		resp.ModelMapping = map[string]map[string]string{}
	}
}

// pricingsToResponse maps the slice of pricing rows into JSON shape.
func pricingsToResponse(rows []service.ChannelModelPricing) []ChannelModelPricingResponse {
	out := make([]ChannelModelPricingResponse, 0, len(rows))
	for i := range rows {
		out = append(out, PricingToResponse(&rows[i]))
	}
	return out
}

// rulesToResponse maps the slice of account-stats rules into JSON shape.
func rulesToResponse(rules []service.AccountStatsPricingRule) []AccountStatsPricingRuleResponse {
	out := make([]AccountStatsPricingRuleResponse, 0, len(rules))
	for i := range rules {
		out = append(out, accountStatsRuleToResponse(&rules[i]))
	}
	return out
}

// accountStatsRuleToResponse maps a service rule to its JSON shape. Slice
// fields are normalised to non-nil so the frontend never deals with
// undefined.
func accountStatsRuleToResponse(rule *service.AccountStatsPricingRule) AccountStatsPricingRuleResponse {
	groupIDs := rule.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	accountIDs := rule.AccountIDs
	if accountIDs == nil {
		accountIDs = []int64{}
	}
	pricing := make([]ChannelModelPricingResponse, 0, len(rule.Pricing))
	for i := range rule.Pricing {
		pricing = append(pricing, PricingToResponse(&rule.Pricing[i]))
	}
	return AccountStatsPricingRuleResponse{
		ID:         rule.ID,
		Name:       rule.Name,
		GroupIDs:   groupIDs,
		AccountIDs: accountIDs,
		SortOrder:  rule.SortOrder,
		Pricing:    pricing,
	}
}

// PricingToResponse maps one service pricing row into JSON shape.
//
// Empty BillingMode and Platform default to BillingModeToken and
// PlatformAnthropic respectively, mirroring the historical wire format.
func PricingToResponse(p *service.ChannelModelPricing) ChannelModelPricingResponse {
	models, billingMode, platform := pricingHeaderDefaults(p)
	intervals := make([]PricingIntervalResponse, 0, len(p.Intervals))
	for _, iv := range p.Intervals {
		intervals = append(intervals, intervalToResponse(iv))
	}
	return ChannelModelPricingResponse{
		ID:               p.ID,
		Platform:         platform,
		Models:           models,
		BillingMode:      billingMode,
		InputPrice:       decimalx.ToFloat64Ptr(p.InputPrice),
		OutputPrice:      decimalx.ToFloat64Ptr(p.OutputPrice),
		CacheWritePrice:  decimalx.ToFloat64Ptr(p.CacheWritePrice),
		CacheReadPrice:   decimalx.ToFloat64Ptr(p.CacheReadPrice),
		ImageOutputPrice: decimalx.ToFloat64Ptr(p.ImageOutputPrice),
		PerRequestPrice:  decimalx.ToFloat64Ptr(p.PerRequestPrice),
		Intervals:        intervals,
	}
}

// pricingHeaderDefaults returns the (Models, BillingMode, Platform) triple
// with the historical defaults applied: empty Models becomes [], empty
// BillingMode becomes "token", empty Platform becomes "anthropic".
func pricingHeaderDefaults(p *service.ChannelModelPricing) ([]string, string, string) {
	models := p.Models
	if models == nil {
		models = []string{}
	}
	billingMode := string(p.BillingMode)
	if billingMode == "" {
		billingMode = string(service.BillingModeToken)
	}
	platform := p.Platform
	if platform == "" {
		platform = service.PlatformAnthropic
	}
	return models, billingMode, platform
}

// intervalToResponse maps one service interval into JSON shape.
func intervalToResponse(iv service.PricingInterval) PricingIntervalResponse {
	return PricingIntervalResponse{
		ID:               iv.ID,
		MinTokens:        iv.MinTokens,
		MaxTokens:        iv.MaxTokens,
		TierLabel:        iv.TierLabel,
		InputPrice:       decimalx.ToFloat64Ptr(iv.InputPrice),
		OutputPrice:      decimalx.ToFloat64Ptr(iv.OutputPrice),
		CacheWritePrice:  decimalx.ToFloat64Ptr(iv.CacheWritePrice),
		CacheReadPrice:   decimalx.ToFloat64Ptr(iv.CacheReadPrice),
		ImageOutputPrice: decimalx.ToFloat64Ptr(iv.ImageOutputPrice),
		PerRequestPrice:  decimalx.ToFloat64Ptr(iv.PerRequestPrice),
		SortOrder:        iv.SortOrder,
	}
}

// PricingFromRequest converts the legacy *float64 JSON shape into the
// service decimal.NullDecimal representation. This is the only
// JSON->decimal hop in the plugin; once it returns, no float64 price
// values circulate downstream.
func PricingFromRequest(reqs []ChannelModelPricingRequest) []service.ChannelModelPricing {
	out := make([]service.ChannelModelPricing, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, pricingMainFieldsFromRequest(r))
	}
	return out
}

// pricingMainFieldsFromRequest builds one service pricing row from its
// JSON-shape counterpart, including the nested intervals.
func pricingMainFieldsFromRequest(r ChannelModelPricingRequest) service.ChannelModelPricing {
	billingMode := service.BillingMode(r.BillingMode)
	if billingMode == "" {
		billingMode = service.BillingModeToken
	}
	platform := r.Platform
	if platform == "" {
		platform = service.PlatformAnthropic
	}
	return service.ChannelModelPricing{
		Platform:         platform,
		Models:           r.Models,
		BillingMode:      billingMode,
		InputPrice:       decimalx.FromFloat64Ptr(r.InputPrice),
		OutputPrice:      decimalx.FromFloat64Ptr(r.OutputPrice),
		CacheWritePrice:  decimalx.FromFloat64Ptr(r.CacheWritePrice),
		CacheReadPrice:   decimalx.FromFloat64Ptr(r.CacheReadPrice),
		ImageOutputPrice: decimalx.FromFloat64Ptr(r.ImageOutputPrice),
		PerRequestPrice:  decimalx.FromFloat64Ptr(r.PerRequestPrice),
		Intervals:        intervalsFromRequest(r.Intervals),
	}
}

// intervalsFromRequest converts the JSON-shape interval list into the
// service representation.
func intervalsFromRequest(reqs []PricingIntervalRequest) []service.PricingInterval {
	out := make([]service.PricingInterval, 0, len(reqs))
	for _, iv := range reqs {
		out = append(out, service.PricingInterval{
			MinTokens:        iv.MinTokens,
			MaxTokens:        iv.MaxTokens,
			TierLabel:        iv.TierLabel,
			InputPrice:       decimalx.FromFloat64Ptr(iv.InputPrice),
			OutputPrice:      decimalx.FromFloat64Ptr(iv.OutputPrice),
			CacheWritePrice:  decimalx.FromFloat64Ptr(iv.CacheWritePrice),
			CacheReadPrice:   decimalx.FromFloat64Ptr(iv.CacheReadPrice),
			ImageOutputPrice: decimalx.FromFloat64Ptr(iv.ImageOutputPrice),
			PerRequestPrice:  decimalx.FromFloat64Ptr(iv.PerRequestPrice),
			SortOrder:        iv.SortOrder,
		})
	}
	return out
}

// AccountStatsRulesFromRequest converts the JSON-shape rule list into the
// service-domain slice the channel service expects.
func AccountStatsRulesFromRequest(reqs []AccountStatsPricingRuleRequest) []service.AccountStatsPricingRule {
	out := make([]service.AccountStatsPricingRule, 0, len(reqs))
	for i, r := range reqs {
		out = append(out, ruleFromRequest(r, i))
	}
	return out
}

// ruleFromRequest builds one service rule. The fallback SortOrder = idx
// preserves the historical client-side ordering when sort_order is omitted.
func ruleFromRequest(r AccountStatsPricingRuleRequest, idx int) service.AccountStatsPricingRule {
	groupIDs := r.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	accountIDs := r.AccountIDs
	if accountIDs == nil {
		accountIDs = []int64{}
	}
	sortOrder := r.SortOrder
	if sortOrder == 0 {
		sortOrder = idx
	}
	return service.AccountStatsPricingRule{
		Name:       r.Name,
		GroupIDs:   groupIDs,
		AccountIDs: accountIDs,
		SortOrder:  sortOrder,
		Pricing:    PricingFromRequest(r.Pricing),
	}
}

// CreateChannelInputFromRequest builds the service-layer create input from
// the JSON request DTO. Keeps the handler side free of decimalx and
// service struct construction details.
func CreateChannelInputFromRequest(req *CreateChannelRequest) *service.CreateChannelInput {
	return &service.CreateChannelInput{
		Name:                       req.Name,
		Description:                req.Description,
		GroupIDs:                   req.GroupIDs,
		ModelPricing:               PricingFromRequest(req.ModelPricing),
		ModelMapping:               req.ModelMapping,
		BillingModelSource:         req.BillingModelSource,
		RestrictModels:             req.RestrictModels,
		Features:                   req.Features,
		FeaturesConfig:             req.FeaturesConfig,
		ApplyPricingToAccountStats: req.ApplyPricingToAccountStats,
		AccountStatsPricingRules:   AccountStatsRulesFromRequest(req.AccountStatsPricingRules),
	}
}

// UpdateChannelInputFromRequest builds the service-layer update input from
// the JSON request DTO. Pointer-typed fields preserve the "nil = no change"
// semantics across the boundary.
func UpdateChannelInputFromRequest(req *UpdateChannelRequest) *service.UpdateChannelInput {
	input := &service.UpdateChannelInput{
		Name:                       req.Name,
		Description:                req.Description,
		Status:                     req.Status,
		GroupIDs:                   req.GroupIDs,
		ModelMapping:               req.ModelMapping,
		BillingModelSource:         req.BillingModelSource,
		RestrictModels:             req.RestrictModels,
		Features:                   req.Features,
		FeaturesConfig:             req.FeaturesConfig,
		ApplyPricingToAccountStats: req.ApplyPricingToAccountStats,
	}
	if req.ModelPricing != nil {
		pricing := PricingFromRequest(*req.ModelPricing)
		input.ModelPricing = &pricing
	}
	if req.AccountStatsPricingRules != nil {
		rules := AccountStatsRulesFromRequest(*req.AccountStatsPricingRules)
		input.AccountStatsPricingRules = &rules
	}
	return input
}
