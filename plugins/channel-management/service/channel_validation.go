package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/wildcard"

	"github.com/shopspring/decimal"
)

// validateChannelConfig runs all the per-update validation rules in one place
// so Create and Update share the same checks.
func validateChannelConfig(pricing []ChannelModelPricing, mapping map[string]map[string]string) error {
	if err := validateNoConflictingModels(pricing); err != nil {
		return err
	}
	if err := validatePricingIntervals(pricing); err != nil {
		return err
	}
	if err := validateNoConflictingMappings(mapping); err != nil {
		return err
	}
	return validatePricingBillingMode(pricing)
}

func validatePricingBillingMode(pricing []ChannelModelPricing) error {
	for _, p := range pricing {
		if err := checkBillingModeRequirements(p); err != nil {
			return err
		}
		if err := checkPricesNotNegative(p); err != nil {
			return err
		}
		if err := checkIntervalsHavePrices(p); err != nil {
			return err
		}
	}
	return nil
}

func checkBillingModeRequirements(p ChannelModelPricing) error {
	if p.BillingMode == BillingModePerRequest || p.BillingMode == BillingModeImage {
		if !p.PerRequestPrice.Valid && len(p.Intervals) == 0 {
			return infraerrors.BadRequest(
				"BILLING_MODE_MISSING_PRICE",
				"per-request price or intervals required for per_request/image billing mode",
			)
		}
	}
	return nil
}

func checkPricesNotNegative(p ChannelModelPricing) error {
	checks := []struct {
		field string
		val   decimal.NullDecimal
	}{
		{"input_price", p.InputPrice},
		{"output_price", p.OutputPrice},
		{"cache_write_price", p.CacheWritePrice},
		{"cache_read_price", p.CacheReadPrice},
		{"image_output_price", p.ImageOutputPrice},
		{"per_request_price", p.PerRequestPrice},
	}
	for _, c := range checks {
		if decimalx.IsNegative(c.val) {
			return infraerrors.BadRequest("NEGATIVE_PRICE", fmt.Sprintf("%s must be >= 0", c.field))
		}
	}
	return nil
}

func checkIntervalsHavePrices(p ChannelModelPricing) error {
	for _, iv := range p.Intervals {
		if !iv.InputPrice.Valid && !iv.OutputPrice.Valid &&
			!iv.CacheWritePrice.Valid && !iv.CacheReadPrice.Valid &&
			!iv.ImageOutputPrice.Valid && !iv.PerRequestPrice.Valid {
			return infraerrors.BadRequest(
				"INTERVAL_MISSING_PRICE",
				fmt.Sprintf("interval [%d, %s] has no price fields set for model %v",
					iv.MinTokens, formatMaxTokens(iv.MaxTokens), p.Models),
			)
		}
	}
	return nil
}

func formatMaxTokens(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

// modelEntry is one model pattern used by the conflict detector.
type modelEntry struct {
	pattern  string
	prefix   string
	wildcard bool
}

func conflictsBetween(a, b modelEntry) bool {
	switch {
	case !a.wildcard && !b.wildcard:
		return a.prefix == b.prefix
	case a.wildcard && !b.wildcard:
		return strings.HasPrefix(b.prefix, a.prefix)
	case !a.wildcard && b.wildcard:
		return strings.HasPrefix(a.prefix, b.prefix)
	default:
		return strings.HasPrefix(a.prefix, b.prefix) ||
			strings.HasPrefix(b.prefix, a.prefix)
	}
}

func toModelEntry(pattern string) modelEntry {
	lower := strings.ToLower(pattern)
	prefix, isWild := wildcard.SplitSuffix(lower)
	return modelEntry{pattern: pattern, prefix: prefix, wildcard: isWild}
}

func validateNoConflictingModels(pricingList []ChannelModelPricing) error {
	byPlatform := make(map[string][]modelEntry)
	for _, p := range pricingList {
		for _, model := range p.Models {
			byPlatform[p.Platform] = append(byPlatform[p.Platform], toModelEntry(model))
		}
	}
	for platform, entries := range byPlatform {
		if err := detectConflicts(entries, platform, "MODEL_PATTERN_CONFLICT", "model patterns"); err != nil {
			return err
		}
	}
	return nil
}

func validateNoConflictingMappings(mapping map[string]map[string]string) error {
	for platform, platformMapping := range mapping {
		entries := make([]modelEntry, 0, len(platformMapping))
		for src := range platformMapping {
			entries = append(entries, toModelEntry(src))
		}
		if err := detectConflicts(entries, platform, "MAPPING_PATTERN_CONFLICT", "mapping source patterns"); err != nil {
			return err
		}
	}
	return nil
}

func validatePricingIntervals(pricingList []ChannelModelPricing) error {
	for _, pricing := range pricingList {
		if err := ValidateIntervals(pricing.Intervals); err != nil {
			return infraerrors.BadRequest(
				"INVALID_PRICING_INTERVALS",
				fmt.Sprintf("invalid pricing intervals for platform '%s' models %v: %v",
					pricing.Platform, pricing.Models, err),
			)
		}
	}
	return nil
}

// validateAccountStatsPricingRules enforces the same per-row checks as
// validatePricingBillingMode (negative prices, billing-mode requirements,
// interval price coverage) on every Pricing entry inside every rule.
// Mirrors the host's `validatePricingEntries` helper introduced by
// release commit 9c09bd19b.
func validateAccountStatsPricingRules(rules []AccountStatsPricingRule) error {
	for i := range rules {
		if err := validatePricingBillingMode(rules[i].Pricing); err != nil {
			return err
		}
	}
	return nil
}

func detectConflicts(entries []modelEntry, platform, errCode, label string) error {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if conflictsBetween(entries[i], entries[j]) {
				return infraerrors.BadRequest(errCode,
					fmt.Sprintf("%s '%s' and '%s' conflict in platform '%s': overlapping match range",
						label, entries[i].pattern, entries[j].pattern, platform))
			}
		}
	}
	return nil
}
