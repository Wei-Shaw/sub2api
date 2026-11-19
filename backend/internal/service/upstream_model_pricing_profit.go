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
	publicModel string
	mapping     ChannelMappingResult
}

// WithUpstreamPricingRoute preserves the customer's requested model and the
// channel mapping before scheduling receives the already-mapped forward model.
func WithUpstreamPricingRoute(ctx context.Context, publicModel string, mapping ChannelMappingResult) context.Context {
	return context.WithValue(ctx, upstreamPricingRouteKey{}, upstreamPricingRoute{publicModel: publicModel, mapping: mapping})
}

type upstreamPricingAdmission struct {
	gateway        *OpenAIGatewayService
	group          *Group
	forwardModel   string
	requireCompact bool
	quotes         sync.Map
}

type localAdmissionPriceKey struct {
	model       string
	longContext bool
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
	bound := *gate
	bound.modelPricing = &upstreamPricingAdmission{gateway: s, group: gate.billingGroup, forwardModel: forwardModel, requireCompact: requireCompact}
	return context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &bound)
}

func (gate *openAIProfitControlGate) upstreamModelPriceVetoReason(ctx context.Context, account *Account) string {
	admission := gate.modelPricing
	if admission == nil || admission.requireCompact {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	route, _ := ctx.Value(upstreamPricingRouteKey{}).(upstreamPricingRoute)
	if route.mapping.BillingModelSource == BillingModelSourceResponse {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	mapped, sent := resolveOpenAIForwardMappedModels(account, admission.forwardModel, admission.requireCompact)
	upstream, rate, ok := upstreamModelPriceAt(account, sent, gate.pricingAt, timezone.Now())
	if !ok {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	model := keyQuoteBillingModel(route.publicModel, mapped, route.mapping)
	key := localAdmissionPriceKey{model: model, longContext: account.IsOpenAILongContextBillingEnabled()}
	value, ok := admission.quotes.Load(key)
	if !ok {
		price, valid := admission.gateway.modelPriceForAccount(ctx, admission.group, account, model, sent, gate.pricingAt)
		value, _ = admission.quotes.LoadOrStore(key, localAdmissionPrice{price: price, valid: valid})
	}
	downstream := value.(localAdmissionPrice)
	if !downstream.valid || upstream.Mode != downstream.price.Mode {
		return openAIProfitFilterReasonUpstreamPriceUnavailable
	}
	if !upstreamTariffFits(upstream, downstream.price, rate, gate.threshold, gate.pricingAt) {
		return openAIProfitFilterReasonThreshold
	}
	return ""
}

// Without knowing the token mix in advance, require that every meter fits the
// margin in every overlapping band. Check the union of configured effort levels
// and all settled service tiers, including possible upstream tier promotions.
func upstreamTariffFits(upstream, downstream UpstreamModelPrice, rate, threshold float64, at time.Time) bool {
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
					if priceExceeds(upPrice*upperRate, down.TokenPrices[tier][meter]*lowerRate) {
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
