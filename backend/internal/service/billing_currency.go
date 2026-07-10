package service

import "strings"

const (
	BillingCurrencyUSD = "USD"
	BillingCurrencyCNY = "CNY"

	DefaultUSDCNYRate = 7.20

	PricingVersionModelPricing20260705 = "model-pricing-2026-07-05"
	PricingVersionFallback20260705     = "fallback-2026-07-05"
	PricingVersionChannel              = "channel"
)

func NormalizeBillingCurrency(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case BillingCurrencyCNY:
		return BillingCurrencyCNY
	default:
		return BillingCurrencyUSD
	}
}

func NormalizeBillingPricingSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case PricingSourceChannel, PricingSourceLiteLLM, PricingSourceProviderUsage:
		return source
	default:
		return PricingSourceFallback
	}
}

func NormalizeBillingPricingVersion(version string) string {
	return strings.TrimSpace(version)
}

func PricingVersionForSource(source string) string {
	switch NormalizeBillingPricingSource(source) {
	case PricingSourceChannel:
		return PricingVersionChannel
	case PricingSourceLiteLLM:
		return PricingVersionModelPricing20260705
	case PricingSourceProviderUsage:
		return ""
	default:
		return PricingVersionFallback20260705
	}
}

func ApplyCostBillingMetadata(cost *CostBreakdown, currency string, source string, version string) *CostBreakdown {
	if cost == nil {
		return nil
	}
	cost.Currency = NormalizeBillingCurrency(currency)
	cost.PricingSource = NormalizeBillingPricingSource(source)
	cost.PricingVersion = NormalizeBillingPricingVersion(version)
	if cost.PricingVersion == "" {
		cost.PricingVersion = PricingVersionForSource(cost.PricingSource)
	}
	return cost
}

func ConvertBillingAmount(amount float64, fromCurrency string, toCurrency string, usdCNYRate float64) float64 {
	from := NormalizeBillingCurrency(fromCurrency)
	to := NormalizeBillingCurrency(toCurrency)
	if from == to {
		return amount
	}
	if usdCNYRate <= 0 {
		return amount
	}
	if from == BillingCurrencyCNY && to == BillingCurrencyUSD {
		return amount / usdCNYRate
	}
	if from == BillingCurrencyUSD && to == BillingCurrencyCNY {
		return amount * usdCNYRate
	}
	return amount
}
