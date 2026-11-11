package service

import "context"

type openAIQuotaPolicyPricingContextKey struct{}

type openAIQuotaPolicyPricingSnapshot struct {
	requestedModel string
	mapping        ChannelMappingResult
}

// WithOpenAIQuotaPolicyPricingContext preserves the billing identity from before
// quota-policy mapping. Scheduling uses the policy target for account support,
// while requested/channel-mapped pricing restrictions keep the original identity.
// Callers attach this snapshot only when a quota policy was applied.
func WithOpenAIQuotaPolicyPricingContext(ctx context.Context, requestedModel string, mapping ChannelMappingResult) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIQuotaPolicyPricingContextKey{}, openAIQuotaPolicyPricingSnapshot{
		requestedModel: requestedModel,
		mapping:        mapping,
	})
}

func openAIQuotaPolicyPricingFromContext(ctx context.Context) (openAIQuotaPolicyPricingSnapshot, bool) {
	if ctx == nil {
		return openAIQuotaPolicyPricingSnapshot{}, false
	}
	snapshot, ok := ctx.Value(openAIQuotaPolicyPricingContextKey{}).(openAIQuotaPolicyPricingSnapshot)
	return snapshot, ok
}
