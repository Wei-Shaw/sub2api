package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Channel aggregate + pricing DTOs + billing constants live in domain;
// re-exported here so existing service call sites compile unchanged. Mirror of
// user/group/proxy/redeem/promo/apikey/announcement/setting.
type (
	Channel                 = domain.Channel
	ChannelModelPricing     = domain.ChannelModelPricing
	PricingInterval         = domain.PricingInterval
	AccountStatsPricingRule = domain.AccountStatsPricingRule
	SupportedModel          = domain.SupportedModel
	ChannelUsageFields      = domain.ChannelUsageFields
)

// BillingMode type + constants re-exported from domain.
type BillingMode = domain.BillingMode

const (
	BillingModeToken      = domain.BillingModeToken
	BillingModePerRequest = domain.BillingModePerRequest
	BillingModeImage      = domain.BillingModeImage
	BillingModeVideo      = domain.BillingModeVideo
)

// BillingModelSource* constants re-exported from domain.
const (
	BillingModelSourceRequested     = domain.BillingModelSourceRequested
	BillingModelSourceUpstream      = domain.BillingModelSourceUpstream
	BillingModelSourceChannelMapped = domain.BillingModelSourceChannelMapped
)

// FindMatchingInterval re-exports the domain helper.
func FindMatchingInterval(intervals []PricingInterval, totalTokens int) *PricingInterval {
	return domain.FindMatchingInterval(intervals, totalTokens)
}

// ValidateIntervals re-exports the domain helper.
func ValidateIntervals(intervals []PricingInterval, mode BillingMode) error {
	return domain.ValidateIntervals(intervals, mode)
}

// splitWildcardSuffix re-exports the domain helper for channel-BC validation
// logic that remains in service (model-pattern conflict detection).
func splitWildcardSuffix(pattern string) (prefix string, isWildcard bool) {
	return domain.SplitWildcardSuffix(pattern)
}
