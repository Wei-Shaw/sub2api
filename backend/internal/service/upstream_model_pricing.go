package service

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const (
	UpstreamModelPricingSyncExtraKey = "upstream_billing_model_pricing_sync_enabled"
	UpstreamModelPricingDataKey      = "model_pricing"
	MaxUpstreamPricingModels         = 64
	maxUpstreamPricingBands          = 24
	maxUpstreamPricingBytes          = 256 << 10
	maxUpstreamContextTokens         = 100_000_000
)

// UpstreamModelPricingCatalog is an optional addition to key_billing v1. Prices
// are in USD before the key's group/user rate and the key's peak multiplier.
// The model keys are the exact names sent in requests to this upstream.
type UpstreamModelPricingCatalog struct {
	Version  int                           `json:"version"`
	Currency string                        `json:"currency"`
	Models   map[string]UpstreamModelPrice `json:"models"`
}

type UpstreamModelPrice struct {
	Mode        BillingMode         `json:"mode"`
	Bands       []upstreamPriceBand `json:"bands"`
	TimePricing *ChannelTimePricing `json:"time_pricing,omitempty"`
	ForcedTier  string              `json:"forced_tier,omitempty"`
	ValidBefore *time.Time          `json:"valid_before,omitempty"`
}

// Bands start at zero. Each upper bound is inclusive and the final band is
// unbounded. Nil is used only for the final band.
type upstreamPriceBand struct {
	ThroughContext *int                          `json:"through_context"`
	TokenPrices    map[string]upstreamUnitPrices `json:"token_prices,omitempty"`
	RequestPrice   *float64                      `json:"request_price,omitempty"`
	EffortRates    map[string]float64            `json:"effort_rates,omitempty"`
}

type upstreamUnitPrices map[string]float64

var upstreamPriceTiers = []string{"default", OpenAIFastTierPriority, "flex", OpenAIFastTierUltrafast}

// A null price would otherwise decode as a free meter.
func (p *upstreamUnitPrices) UnmarshalJSON(raw []byte) error {
	var decoded map[string]*float64
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	prices := make(upstreamUnitPrices, len(decoded))
	for meter, price := range decoded {
		if price != nil {
			prices[meter] = *price
		}
	}
	*p = prices
	return nil
}

var upstreamPriceMeters = []string{
	"input", "output", "cache_read", "cache_write", "cache_write_1h", "cache_write_30m", "cache_write_12h",
	"image_input", "image_output", "image_cache_read",
}

func ValidUpstreamPricingModel(model string) bool {
	return model != "" && len(model) <= 160 && !strings.ContainsAny(model, "*,") &&
		strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) == -1
}

func validUpstreamPrice(value float64) bool {
	return value >= 0 && value <= 1_000_000
}

func (p UpstreamModelPrice) valid() bool {
	if len(p.Bands) == 0 || len(p.Bands) > maxUpstreamPricingBands ||
		(p.Mode != BillingModeToken && p.Mode != BillingModePerRequest) ||
		(p.ForcedTier != "" && p.ForcedTier != OpenAIFastTierPriority && p.ForcedTier != OpenAIFastTierUltrafast) {
		return false
	}
	if p.TimePricing != nil && (p.Mode != BillingModeToken || len(p.TimePricing.Periods) > 48 || validateChannelTimePricing(p.TimePricing) != nil) {
		return false
	}
	previous := -1
	for i, band := range p.Bands {
		if band.ThroughContext == nil {
			if i != len(p.Bands)-1 {
				return false
			}
		} else {
			bound := *band.ThroughContext
			if i == len(p.Bands)-1 || bound <= previous || bound > maxUpstreamContextTokens {
				return false
			}
			previous = bound
		}
		if len(band.EffortRates) > 12 {
			return false
		}
		for effort, rate := range band.EffortRates {
			if len(effort) > 32 || !validUpstreamPrice(rate) || rate == 0 {
				return false
			}
		}
		if p.Mode == BillingModePerRequest {
			if band.RequestPrice == nil || !validUpstreamPrice(*band.RequestPrice) || len(band.TokenPrices) != 0 {
				return false
			}
			continue
		}
		if band.RequestPrice != nil || len(band.TokenPrices) != len(upstreamPriceTiers) {
			return false
		}
		for _, tier := range upstreamPriceTiers {
			prices := band.TokenPrices[tier]
			if len(prices) != len(upstreamPriceMeters) {
				return false
			}
			for _, meter := range upstreamPriceMeters {
				price, ok := prices[meter]
				if !ok || !validUpstreamPrice(price) {
					return false
				}
			}
		}
	}
	return true
}

func (p UpstreamModelPrice) bandAt(contextTokens int) upstreamPriceBand {
	for _, band := range p.Bands {
		if band.ThroughContext == nil || contextTokens <= *band.ThroughContext {
			return band
		}
	}
	return upstreamPriceBand{}
}

func upstreamPriceTier(tier string) string {
	switch normalizeBillingServiceTier(tier) {
	case OpenAIFastTierPriority, "fast":
		return OpenAIFastTierPriority
	case "flex":
		return "flex"
	case OpenAIFastTierUltrafast:
		return OpenAIFastTierUltrafast
	default:
		return "default"
	}
}

// buildUpstreamModelPrice samples the existing calculation on each piecewise
// constant band. Keep operator schedules separate from unit prices so a cached
// quote can still be used on either side of a time-of-day boundary.
func (s *BillingService) buildUpstreamModelPrice(input CostInput, freeFast bool) (UpstreamModelPrice, bool) {
	if s == nil || input.Resolver == nil || input.Resolved == nil || input.PricingAt.IsZero() {
		return UpstreamModelPrice{}, false
	}
	resolved := *input.Resolved
	quote := UpstreamModelPrice{Mode: resolved.Mode}
	var bounds []int
	switch resolved.Mode {
	case BillingModeToken:
		// DeepSeek's calendar policy is different from a channel time schedule.
		if resolved.Source == PricingSourceLiteLLM && isDeepSeekModel(input.Model) {
			return quote, false
		}
		if input.LongContextBillingEnabled != nil && *input.LongContextBillingEnabled {
			resolved.longContextPricingEnabled = true
		}
		bounds = s.contextPricingBreakpoints(input.Resolver, &resolved, input.Model).bounds
		if resolved.Source == PricingSourceChannel && resolved.channelPricing != nil {
			quote.TimePricing = resolved.channelPricing.TimePricing
			if quote.TimePricing != nil {
				config := *resolved.channelPricing
				config.TimePricing = nil
				resolved.channelPricing = &config
			}
		}
		if before, ok := openAIUltrafastDiscountEnd(input.Model); ok && input.PricingAt.Before(before) {
			quote.ValidBefore = &before
		}
	case BillingModePerRequest:
		for _, tier := range resolved.RequestTiers {
			bounds = append(bounds, tier.MinTokens)
			if tier.MaxTokens != nil {
				bounds = append(bounds, *tier.MaxTokens)
			}
		}
		slices.Sort(bounds)
	default:
		return quote, false
	}
	bounds = slices.Compact(bounds)
	if len(bounds)+2 > maxUpstreamPricingBands || (len(bounds) > 0 && bounds[len(bounds)-1] > maxUpstreamContextTokens) {
		return quote, false
	}
	input.Resolved = &resolved
	input.RateMultiplier, input.RequestCount = 1, 1
	input.ReasoningEffort = ""
	segments := buildContextSegments(bounds)
	// Zero never matches a configured (min,max] interval. Even a request with
	// zero input may bill output tokens or incur a flat per-request charge.
	zero := 0
	quote.Bands = make([]upstreamPriceBand, 0, len(segments)+1)
	for i := -1; i < len(segments); i++ {
		count, end := 0, &zero
		if i >= 0 {
			count, end = segments[i].min+1, segments[i].max
		}
		band := upstreamPriceBand{ThroughContext: end}
		if resolved.Mode == BillingModePerRequest {
			input.Tokens = UsageTokens{InputTokens: count}
			cost, err := s.CalculateCostUnified(input)
			if err != nil {
				return quote, false
			}
			band.RequestPrice = &cost.TotalCost
			if resolved.channelPricing != nil {
				band.EffortRates = resolved.channelPricing.ReasoningEffortMultipliers
			}
		} else {
			contextTokens := count
			if !resolved.longContextPricingEnabled {
				contextTokens = 1
			}
			pricing := input.Resolver.GetIntervalPricing(&resolved, contextTokens)
			if pricing == nil {
				return quote, false
			}
			band.EffortRates = pricing.ReasoningEffortMultipliers
			band.TokenPrices = make(map[string]upstreamUnitPrices, len(upstreamPriceTiers))
			for _, tier := range upstreamPriceTiers {
				input.ServiceTier = tier
				if freeFast && tier == OpenAIFastTierPriority {
					input.ServiceTier = "default"
				}
				prices, ok := s.probeUpstreamUnitPrices(input, count)
				if !ok {
					return quote, false
				}
				band.TokenPrices[tier] = prices
			}
		}
		quote.Bands = append(quote.Bands, band)
	}
	return quote, quote.valid()
}

func (s *BillingService) probeUpstreamUnitPrices(input CostInput, count int) (upstreamUnitPrices, bool) {
	input.Tokens = UsageTokens{InputTokens: count, OutputTokens: 1}
	text, err := s.CalculateCostUnified(input)
	if err != nil {
		return nil, false
	}
	input.Tokens.ImageInputTokens, input.Tokens.ImageOutputTokens = count, 1
	image, err := s.CalculateCostUnified(input)
	if err != nil {
		return nil, false
	}
	prices := upstreamUnitPrices{
		"output": text.OutputCost, "image_output": image.ImageOutputCost,
		"input": 0, "image_input": 0, "cache_read": 0, "image_cache_read": 0,
		"cache_write": 0, "cache_write_1h": 0, "cache_write_30m": 0, "cache_write_12h": 0,
	}
	if count == 0 {
		return prices, true
	}
	n := float64(count)
	prices["input"], prices["image_input"] = text.InputCost/n, image.ImageInputCost/n
	input.Tokens = UsageTokens{CacheReadTokens: count, ImageCacheReadTokens: count}
	imageRead, _ := s.CalculateCostUnified(input)
	readCount := float64(input.Tokens.CacheReadTokens)
	input.Tokens.ImageCacheReadTokens = 0
	read, _ := s.CalculateCostUnified(input)
	prices["cache_read"], prices["image_cache_read"] = read.CacheReadCost/readCount, imageRead.CacheReadCost/readCount

	input.Tokens = UsageTokens{CacheCreationTokens: count}
	write, _ := s.CalculateCostUnified(input)
	input.Tokens.CacheCreation1hTokens = count
	write1h, _ := s.CalculateCostUnified(input)
	input.Tokens.CacheCreation1hTokens, input.Tokens.CacheCreation30mTokens = 0, count
	write30m, _ := s.CalculateCostUnified(input)
	input.Tokens.CacheCreation30mTokens, input.Tokens.CacheCreation12hTokens = 0, count
	write12h, _ := s.CalculateCostUnified(input)
	prices["cache_write"], prices["cache_write_1h"] = write.CacheCreationCost/n, write1h.CacheCreationCost/n
	prices["cache_write_30m"], prices["cache_write_12h"] = write30m.CacheCreationCost/n, write12h.CacheCreationCost/n
	return prices, true
}

func (p UpstreamModelPrice) baseCost(tokens UsageTokens, tier, effort string, at time.Time) (float64, bool) {
	band := p.bandAt(tokens.InputTokens + tokens.CacheCreationTokens + tokens.CacheReadTokens)
	if band.RequestPrice != nil {
		cost := *band.RequestPrice * reasoningEffortBillingMultiplier(effort, band.EffortRates)
		return cost, cost >= 0 && !math.IsInf(cost, 0)
	}
	prices := band.TokenPrices[upstreamPriceTier(tier)]
	if prices == nil {
		return 0, false
	}
	imageInput := min(max(tokens.ImageInputTokens, 0), max(tokens.InputTokens, 0))
	imageRead := min(max(tokens.ImageCacheReadTokens, 0), max(tokens.CacheReadTokens, 0))
	cost := float64(max(tokens.InputTokens-imageInput, 0))*prices["input"] + float64(imageInput)*prices["image_input"] +
		float64(max(tokens.OutputTokens-tokens.ImageOutputTokens, 0))*prices["output"] + float64(max(tokens.ImageOutputTokens, 0))*prices["image_output"] +
		float64(max(tokens.CacheReadTokens-imageRead, 0))*prices["cache_read"] + float64(imageRead)*prices["image_cache_read"]
	longWrite := prices["cache_write_12h"]
	if ttlCost, ok := openAICacheCreationTTLCost(tokens, prices["cache_write_30m"], &longWrite); ok {
		cost += ttlCost
	} else {
		short, long := allocateCacheCreationBillingTokens(tokens)
		cost += float64(short)*prices["cache_write"] + float64(long)*prices["cache_write_1h"]
	}
	cost *= p.TimePricing.MultiplierAt(at) * reasoningEffortBillingMultiplier(effort, band.EffortRates)
	return cost, cost >= 0 && !math.IsInf(cost, 0)
}

// SupportsUpstreamModelPricingSync limits the initial contract to OpenAI pool
// accounts with a single base URL and key.
func SupportsUpstreamModelPricingSync(account *Account) bool {
	return account != nil && account.IsOpenAI() && account.Type == AccountTypeAPIKey && account.IsPoolMode() &&
		!account.RoutesProtocolByInbound() && !account.IsShadow()
}

func upstreamModelPricingSyncEnabled(account *Account) bool {
	return account != nil && account.IsOpenAI() && account.Type == AccountTypeAPIKey && account.IsPoolMode() &&
		account.Extra[UpstreamModelPricingSyncExtraKey] == true
}

// CanReuseUpstreamPricingConnection prevents a quote for refreshed credentials
// or routing settings from authorizing a turn on an already-open connection.
func CanReuseUpstreamPricingConnection(connected, latest *Account) bool {
	wasSynced, isSynced := upstreamModelPricingSyncEnabled(connected), upstreamModelPricingSyncEnabled(latest)
	if !wasSynced && !isSynced {
		return true
	}
	return wasSynced == isSynced && SupportsUpstreamModelPricingSync(latest) &&
		reflect.DeepEqual(upstreamBillingProbeIdentity(connected), upstreamBillingProbeIdentity(latest)) &&
		reflect.DeepEqual(connected.Credentials, latest.Credentials) &&
		connected.IsOpenAIPassthroughEnabled() == latest.IsOpenAIPassthroughEnabled() &&
		connected.IsOpenAILongContextBillingEnabled() == latest.IsOpenAILongContextBillingEnabled()
}

func selectedUpstreamPricingModels(account *Account) []string {
	var models []string
	for _, mapping := range []map[string]string{account.GetModelMapping(), account.GetCompactModelMapping()} {
		for _, model := range mapping {
			model = strings.TrimSpace(model)
			if ValidUpstreamPricingModel(model) {
				models = append(models, model)
			}
		}
	}
	slices.Sort(models)
	models = slices.Compact(models)
	return models[:min(len(models), MaxUpstreamPricingModels)]
}

func parseUpstreamModelPricing(raw json.RawMessage) map[string]any {
	var catalog UpstreamModelPricingCatalog
	if len(raw) > maxUpstreamPricingBytes || json.Unmarshal(raw, &catalog) != nil || catalog.Version != 1 ||
		catalog.Currency != "USD" || len(catalog.Models) > MaxUpstreamPricingModels {
		return nil
	}
	for model, price := range catalog.Models {
		if !ValidUpstreamPricingModel(model) || !price.valid() {
			delete(catalog.Models, model)
		}
	}
	// Re-encode the typed representation to strip any unknown fields supplied by
	// the upstream. Stored values also need the same JSON shape before a reload.
	if len(catalog.Models) == 0 {
		return nil
	}
	encoded, err := json.Marshal(catalog.Models)
	if err != nil {
		return nil
	}
	var data map[string]any
	if json.Unmarshal(encoded, &data) != nil {
		return nil
	}
	return data
}

func upstreamModelPriceAt(account *Account, model string, at, now time.Time) (UpstreamModelPrice, float64, bool) {
	var price UpstreamModelPrice
	if !upstreamModelPricingSyncEnabled(account) || !SupportsUpstreamModelPricingSync(account) || !upstreamBillingProbeEnabled(account) {
		return price, 0, false
	}
	snapshot, _ := account.Extra[UpstreamBillingProbeExtraKey].(map[string]any)
	if snapshot["status"] != UpstreamBillingProbeStatusOK {
		return price, 0, false
	}
	freshUntil := parseExtraTime(snapshot["fresh_until"])
	if !now.Before(freshUntil) {
		return price, 0, false
	}
	data, _ := snapshot["data"].(map[string]any)
	models, _ := data[UpstreamModelPricingDataKey].(map[string]any)
	if models[model] == nil {
		return price, 0, false
	}
	raw, err := json.Marshal(models[model])
	if err != nil || json.Unmarshal(raw, &price) != nil || !price.valid() ||
		(price.ValidBefore != nil && (!at.Before(*price.ValidBefore) || !now.Before(*price.ValidBefore))) {
		return price, 0, false
	}
	rate, ok := upstreamBillingRateAt(data, at)
	return price, rate, ok && rate >= 0 && rate <= upstreamBillingRateSyncMaxMultiplier
}

// ResolveKeyModelPricing emits quotes only when the active routes for the key's
// requested model agree on billing. An account selection that can change the
// tariff cannot be represented by a single static quote.
func (s *OpenAIGatewayService) ResolveKeyModelPricing(ctx context.Context, group *Group, models []string, at time.Time) *UpstreamModelPricingCatalog {
	if s == nil || s.accountRepo == nil || s.resolver == nil || group == nil || group.Platform != PlatformOpenAI {
		return nil
	}
	// Rules that replace the requested service tier can make the same tier
	// quote settle differently. Until that policy is part of the contract,
	// only publish prices when tiers pass through or are rejected.
	if s.settingService != nil {
		policy, err := s.settingService.GetOpenAIFastPolicySettings(ctx)
		if err != nil || slices.ContainsFunc(policy.Rules, func(rule OpenAIFastPolicyRule) bool {
			return rewritesPricingTier(rule.Action) || rewritesPricingTier(rule.FallbackAction)
		}) {
			return nil
		}
	}
	accounts, err := s.accountRepo.ListByGroup(ctx, group.ID)
	if err != nil || len(accounts) > 128 {
		return nil
	}
	catalog := &UpstreamModelPricingCatalog{Version: 1, Currency: "USD", Models: make(map[string]UpstreamModelPrice)}
	if len(models) == 0 {
		models = defaultUpstreamPricingModels(group, accounts)
	}
	// Budget the serialized catalog, excluding the small legacy key billing
	// response. Stop before an oversized catalog would invalidate rate sync.
	remaining := maxUpstreamPricingBytes - 4096
	for _, model := range models[:min(len(models), MaxUpstreamPricingModels)] {
		if !ValidUpstreamPricingModel(model) || !group.ModelAllowlist.Allows(model) {
			continue
		}
		quote, ok := s.resolveKeyModelPrice(ctx, group, accounts, model, at)
		if !ok {
			continue
		}
		row, err := json.Marshal(quote)
		if err != nil || len(row)+len(model)*6+8 > remaining {
			continue
		}
		remaining -= len(row) + len(model)*6 + 8
		catalog.Models[model] = quote
	}
	return catalog
}

func rewritesPricingTier(action string) bool {
	return action == BetaPolicyActionFilter || action == OpenAIFastPolicyActionForcePriority
}

func defaultUpstreamPricingModels(group *Group, accounts []Account) []string {
	var mapped, defaults []string
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() {
			continue
		}
		for model := range account.GetModelMapping() {
			mapped = append(mapped, model)
		}
	}
	for i := range accounts {
		if accounts[i].IsOpenAI() && (len(accounts[i].GetModelMapping()) == 0 || accounts[i].IsOpenAIPassthroughEnabled()) {
			defaults = openai.DefaultModelIDs()
			break
		}
	}
	slices.Sort(mapped)
	models := append(slices.Clone(group.ModelAllowlist.Models), mapped...)
	for _, pricing := range group.ModelPricing {
		models = append(models, pricing.Models...)
	}
	models = append(models, defaults...)
	seen := make(map[string]bool)
	out := make([]string, 0, MaxUpstreamPricingModels)
	for _, model := range models {
		if !seen[model] && ValidUpstreamPricingModel(model) && group.ModelAllowlist.Allows(model) {
			seen[model] = true
			out = append(out, model)
			if len(out) == MaxUpstreamPricingModels {
				break
			}
		}
	}
	return out
}

func (s *OpenAIGatewayService) resolveKeyModelPrice(ctx context.Context, group *Group, accounts []Account, model string, at time.Time) (UpstreamModelPrice, bool) {
	var quote UpstreamModelPrice
	found := false
	mapping := s.ResolveChannelMapping(ctx, group.ID, model)
	if mapping.BillingModelSource == BillingModelSourceResponse {
		return quote, false
	}
	forwardModel := model
	if mapping.Mapped && mapping.MappedModel != "" {
		forwardModel = mapping.MappedModel
	}
	ctx = context.WithValue(ctx, ctxkey.Group, group)
	// Different routes may bill by account mapping or toggle long-context
	// pricing. Emit a single quote only if all matching routes agree.
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() || !account.IsModelSupported(forwardModel) {
			continue
		}
		if account.IsShadow() || account.RoutesProtocolByInbound() || account.IsOpenAIPassthroughEnabled() && account.IsOpenAIOAuthLike() {
			return quote, false
		}
		billingModel, upstreamModel := resolveOpenAIForwardMappedModels(account, forwardModel, false)
		billingModel = keyQuoteBillingModel(model, billingModel, mapping)
		accountQuote, ok := s.modelPriceForAccount(ctx, group, account, billingModel, upstreamModel, at)
		if !ok || (found && !reflect.DeepEqual(quote, accountQuote)) {
			return quote, false
		}
		quote, found = accountQuote, true
	}
	return quote, found
}

func keyQuoteBillingModel(requested, mapped string, channel ChannelMappingResult) string {
	switch channel.BillingModelSource {
	case BillingModelSourceRequested:
		if requested != "" {
			return requested
		}
	case BillingModelSourceChannelMapped:
		if channel.Mapped && channel.MappedModel != "" && channel.MappedModel != requested {
			return channel.MappedModel
		}
	}
	return mapped
}

func (s *OpenAIGatewayService) modelPriceForAccount(ctx context.Context, group *Group, account *Account, model, upstreamModel string, at time.Time) (UpstreamModelPrice, bool) {
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: &group.ID, Group: group})
	quote, ok := s.billingService.buildUpstreamModelPrice(CostInput{
		Ctx: ctx, Model: model, Resolver: s.resolver, Resolved: resolved, PricingAt: at,
		LongContextBillingEnabled:      openAILongContextBillingGate(account),
		DisableModelLongContextPricing: openAISubscriptionLongContextExempt(account, &OpenAIForwardResult{UpstreamModel: upstreamModel}),
	}, group.FreeOpenAIFast)
	quote.ForcedTier = openAIGroupForcedServiceTier(ctx, account, upstreamModel)
	return quote, ok
}

func syncedOpenAIUsageCost(account *Account, result *OpenAIForwardResult, tokens UsageTokens, localMode string, tier ServiceTierBillingResolution, at time.Time) (base, rate float64, ok bool) {
	if result.VideoCount > 0 || result.AudioUsage != nil || result.WebSearchCalls > 0 || result.SearchCount > 0 ||
		(result.ImageCount > 0 && localMode != string(BillingModeToken)) || !result.SucceededForScheduling() {
		return 0, 0, false
	}
	model := upstreamSentModel(result.Model, result.UpstreamModel)
	price, rate, ok := upstreamModelPriceAt(account, model, at, at)
	if !ok || (result.ImageCount > 0 && price.Mode != BillingModeToken) {
		return 0, 0, false
	}
	billedTier := tier.Billing
	if price.ForcedTier != "" {
		billedTier = ResolveBillingServiceTier(price.ForcedTier, tier.Observed).Billing
	}
	base, ok = price.baseCost(tokens, billedTier, optionalStringValue(result.ReasoningEffort), at)
	return base, rate, ok
}
