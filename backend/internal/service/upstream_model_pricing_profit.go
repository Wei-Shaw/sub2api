package service

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type upstreamPricingRouteKey struct{}

type upstreamPricingRoute struct {
	publicModel  string
	forwardModel string
	mapping      ChannelMappingResult
	// The Messages-to-Gemini bridge can use Antigravity's dynamic model variants.
	antigravityGeminiBridge bool
}

// WithUpstreamPricingRoute preserves the customer's requested model and the
// channel mapping before scheduling receives the already-mapped forward model.
func WithUpstreamPricingRoute(ctx context.Context, publicModel string, mapping ChannelMappingResult) context.Context {
	return context.WithValue(ctx, upstreamPricingRouteKey{}, upstreamPricingRoute{publicModel: publicModel, mapping: mapping})
}

// WithGatewayUpstreamPricingRoute records the exact model used by shared-gateway
// forwarding, which can differ from the model passed to account selection.
func WithGatewayUpstreamPricingRoute(ctx context.Context, publicModel, forwardModel string, mapping ChannelMappingResult, antigravityGeminiBridge bool) context.Context {
	return context.WithValue(ctx, upstreamPricingRouteKey{}, upstreamPricingRoute{
		publicModel: publicModel, forwardModel: forwardModel, mapping: mapping,
		antigravityGeminiBridge: antigravityGeminiBridge,
	})
}

type upstreamPricingAdmission struct {
	openAI            *OpenAIGatewayService
	gateway           *GatewayService
	group             *Group
	route             upstreamPricingRoute
	forwardModel      string
	requireCompact    bool
	forceCacheBilling bool
	quotes            sync.Map
}

type localAdmissionPriceKey struct {
	model         string
	upstreamModel string
	longContext   bool
	forcedTier    string
}

type localAdmissionPrice struct {
	price UpstreamModelPrice
	valid bool
}

// WithUpstreamPricingModel binds the per-request model after group and channel
// resolution. A WebSocket turn rebinds it because the model can change.
func (s *OpenAIGatewayService) WithUpstreamPricingModel(ctx context.Context, forwardModel string, requireCompact bool) context.Context {
	gate, _ := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	if gate == nil || gate.billingGroup == nil || s.resolver == nil || forwardModel == "" {
		return ctx
	}
	route, _ := ctx.Value(upstreamPricingRouteKey{}).(upstreamPricingRoute)
	bound := *gate
	bound.modelPricing = &upstreamPricingAdmission{openAI: s, group: gate.billingGroup, route: route, forwardModel: forwardModel, requireCompact: requireCompact}
	return context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &bound)
}

func (s *GatewayService) withGatewayUpstreamPricingModel(ctx context.Context) context.Context {
	gate, _ := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	route, ok := ctx.Value(upstreamPricingRouteKey{}).(upstreamPricingRoute)
	if gate == nil || !ok || route.forwardModel == "" {
		return ctx
	}
	bound := *gate
	bound.modelPricing = &upstreamPricingAdmission{
		gateway: s, group: gate.billingGroup, route: route, forwardModel: route.forwardModel,
		forceCacheBilling: IsForceCacheBilling(ctx),
	}
	return context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &bound)
}

func (gate *openAIProfitControlGate) upstreamModelPriceVetoReason(ctx context.Context, account *Account) string {
	admission := gate.modelPricing
	if admission == nil || admission.group == nil || admission.requireCompact || admission.route.mapping.BillingModelSource == BillingModelSourceResponse {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	model, sent := admission.modelsForAccount(ctx, account)
	upstream, rate, ok := upstreamModelPriceAt(account, sent, gate.pricingAt, timezone.Now())
	if !ok {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	key := localAdmissionPriceKey{model: model, upstreamModel: sent}
	if admission.openAI != nil {
		key.longContext = account.IsOpenAILongContextBillingEnabled()
		key.forcedTier = openAIGroupForcedServiceTier(ctx, account, sent)
	}
	value, ok := admission.quotes.Load(key)
	if !ok {
		var price UpstreamModelPrice
		var valid bool
		if admission.openAI != nil {
			price, valid = admission.openAI.modelPriceForAccount(ctx, admission.group, account, model, sent, gate.pricingAt)
		} else {
			price, valid = admission.gateway.modelPriceForModel(ctx, admission.group, model, gate.pricingAt)
		}
		value, _ = admission.quotes.LoadOrStore(key, localAdmissionPrice{price: price, valid: valid})
	}
	downstream := value.(localAdmissionPrice)
	if !downstream.valid || upstream.Mode != downstream.price.Mode {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	if !upstreamTariffFits(upstream, downstream.price, rate, gate.threshold, gate.pricingAt, admission.forceCacheBilling) {
		return openAIProfitFilterReasonThreshold
	}
	return ""
}

func (a *upstreamPricingAdmission) modelsForAccount(ctx context.Context, account *Account) (billing, sent string) {
	if a.openAI != nil {
		mapped, sent := resolveOpenAIForwardMappedModels(account, a.forwardModel, a.requireCompact)
		return keyQuoteBillingModel(a.route.publicModel, mapped, a.route.mapping), sent
	}
	if a.route.antigravityGeminiBridge && account.Platform == PlatformAntigravity {
		return "", ""
	}
	return a.gateway.sharedPricingModels(ctx, a.group, account, a.route.publicModel, a.forwardModel, a.route.mapping)
}

// Without knowing the token mix in advance, require that every meter fits the
// margin in every overlapping band. Check the union of configured effort levels
// and all settled service tiers, including possible upstream tier promotions.
func upstreamTariffFits(upstream, downstream UpstreamModelPrice, rate, threshold float64, at time.Time, forceCacheBilling bool) bool {
	upRate := rate * upstream.TimePricing.MultiplierAt(at)
	downRate := threshold * downstream.TimePricing.MultiplierAt(at)
	for i, j := 0, 0; i < len(upstream.Bands) && j < len(downstream.Bands); {
		up, down := upstream.Bands[i], downstream.Bands[j]
		// An absent effort uses 1x. A routed request can get a different
		// effective effort at the next provider, so use the adverse ratio.
		maxUpEffort, minDownEffort := 1.0, 1.0
		for _, rate := range up.EffortRates {
			maxUpEffort = math.Max(maxUpEffort, rate)
		}
		for _, rate := range down.EffortRates {
			minDownEffort = math.Min(minDownEffort, rate)
		}
		upperRate, lowerRate := upRate*maxUpEffort, downRate*minDownEffort
		if up.RequestPrice != nil && down.RequestPrice != nil {
			if priceExceeds(*up.RequestPrice*upperRate, *down.RequestPrice*lowerRate) {
				return false
			}
		} else {
			for _, tier := range upstreamPriceTiers {
				for _, meter := range upstreamPriceMeters {
					upPrice := up.TokenPrices[tier][meter]
					if upstream.ForcedTier != "" {
						upPrice = math.Max(upPrice, up.TokenPrices[upstream.ForcedTier][meter])
					}
					downMeter := meter
					if forceCacheBilling && meter == "input" {
						downMeter = "cache_read"
					}
					downPrice := down.TokenPrices[tier][downMeter]
					if downstream.ForcedTier != "" {
						downPrice = math.Min(downPrice, down.TokenPrices[downstream.ForcedTier][downMeter])
					}
					if priceExceeds(upPrice*upperRate, downPrice*lowerRate) {
						return false
					}
				}
			}
		}
		upEnd, downEnd := math.MaxInt, math.MaxInt
		if up.ThroughContext != nil {
			upEnd = *up.ThroughContext
		}
		if down.ThroughContext != nil {
			downEnd = *down.ThroughContext
		}
		if upEnd <= downEnd {
			i++
		}
		if downEnd <= upEnd {
			j++
		}
	}
	return true
}

func priceExceeds(cost, limit float64) bool {
	return math.IsNaN(cost) || math.IsInf(cost, 0) || math.IsNaN(limit) || math.IsInf(limit, 0) || cost-limit > math.Max(cost, limit)*1e-12
}
