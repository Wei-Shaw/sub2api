// Package service — account_stats_pricing.go
//
// resolveAccountStatsCost implements the plugin-side decision for the
// account-statistics cost override that the host gateway records into
// usage_logs.account_stats_cost.
//
// Priority (first hit wins):
//
//  1. Custom rules (AccountStatsPricingRules). Always tried, independent
//     of the channel.ApplyPricingToAccountStats toggle. A rule matches when
//     accountID ∈ Rule.AccountIDs OR groupID ∈ Rule.GroupIDs and the
//     upstream model has a pricing entry under the rule.
//  2. ApplyPricingToAccountStats=true → use the request's actual customer
//     cost (totalCost passed from the host, before account_rate_multiplier).
//  3. Return AccountStatsCostResult{HasCost:false} so the host can fall
//     back to its LiteLLM lookup and ultimately the default formula
//     (total_cost × account_rate_multiplier).
//
// "Plugin returns nil → host applies LiteLLM → host applies default
// formula" matches the release/custom-0.1.120 layered behaviour, but
// keeps the LiteLLM dependency on the host side where it lives.

package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/platforms"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/wildcard"
	"github.com/shopspring/decimal"
)

// AccountStatsCostInput captures the fields the host gateway forwards
// when it asks the plugin to compute a custom account stats cost.
//
// T24: TotalCost is now decimal.Decimal end-to-end. The pricing gRPC
// server parses the proto string at the wire boundary
// (ResolveAccountStatsCostRequest.total_cost) and forwards a typed
// decimal here so the resolver never round-trips through IEEE-754.
type AccountStatsCostInput struct {
	ChannelID     int64
	AccountID     int64
	GroupID       int64
	UpstreamModel string // post-mapping model id (matches custom rules by model name)
	Tokens        UsageTokens
	RequestCount  int
	// TotalCost is the customer cost (before account_rate_multiplier).
	// Used only for priority 2 when ApplyPricingToAccountStats=true and
	// no custom rule matched.
	TotalCost decimal.Decimal
}

// UsageTokens is the per-request token / image count breakdown used by
// the account-stats pricing calculation. Mirrors the shape the host's
// service.UsageTokens uses; declared here so the plugin's service layer
// has no host import.
type UsageTokens struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ImageOutputTokens   int64
}

// AccountStatsCostResult is what the resolver returns. HasCost=false
// means "no opinion" — the host should proceed with its own pricing
// fallbacks (LiteLLM model file → default formula).
//
// Cost is a decimal.Decimal so the cost accumulator path stays inside the
// shopspring/decimal world end-to-end. The proto edge converts to float64
// once at the boundary (see internal/pricing/server.go).
type AccountStatsCostResult struct {
	HasCost bool
	Cost    decimal.Decimal
}

// ResolveAccountStatsCost is the plugin-facing entry point for the
// host's PricingExtension.ResolveAccountStatsCost RPC. It resolves the
// channel snapshot from the cache, applies the priority rules above,
// and returns a typed result the gRPC layer can serialise.
//
// channelID == 0 or an unknown / inactive channel returns HasCost=false
// so the host can short-circuit to its own fallback path without
// observing an error.
func (s *ChannelService) ResolveAccountStatsCost(ctx context.Context, in AccountStatsCostInput) (AccountStatsCostResult, error) {
	if in.UpstreamModel == "" {
		return AccountStatsCostResult{}, nil
	}

	channel, err := s.channelByIDFromCache(ctx, in.ChannelID, in.GroupID)
	if err != nil {
		return AccountStatsCostResult{}, err
	}
	if channel == nil {
		return AccountStatsCostResult{}, nil
	}

	platform := s.platformForRule(ctx, in.GroupID)

	// Priority 1: custom rules (always tried)
	if cost, ok := tryCustomRules(channel, in.AccountID, in.GroupID, platform, in.UpstreamModel, in.Tokens, in.RequestCount); ok {
		return AccountStatsCostResult{HasCost: true, Cost: cost}, nil
	}

	// Priority 2: ApplyPricingToAccountStats → use customer billing total.
	// TotalCost is decimal.Decimal end-to-end (T24); no float64 round-trip.
	if channel.ApplyPricingToAccountStats {
		if in.TotalCost.Sign() > 0 {
			return AccountStatsCostResult{HasCost: true, Cost: in.TotalCost}, nil
		}
		return AccountStatsCostResult{}, nil
	}

	// No opinion — host falls back to LiteLLM / default formula.
	return AccountStatsCostResult{}, nil
}

// channelByIDFromCache resolves the channel snapshot for the requested
// channelID. When channelID is 0 (legacy callers) the helper looks up by
// groupID instead so the resolver still works for old usage paths.
// Returns nil when the channel is unknown or inactive.
func (s *ChannelService) channelByIDFromCache(ctx context.Context, channelID, groupID int64) (*Channel, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}
	if channelID > 0 {
		if ch, ok := cache.byID[channelID]; ok && ch.IsActive() {
			return ch.Clone(), nil
		}
		return nil, nil
	}
	if ch, ok := cache.channelByGroupID[groupID]; ok && ch.IsActive() {
		return ch.Clone(), nil
	}
	return nil, nil
}

// platformForRule returns the platform string used to match Pricing.Platform
// inside a rule. Empty string is allowed when the group has no platform
// configured (rare); isPlatformMatch treats "" as "match anything".
func (s *ChannelService) platformForRule(ctx context.Context, groupID int64) string {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return ""
	}
	return cache.groupPlatform[groupID]
}

// tryCustomRules walks the rule list in SortOrder and returns the first
// match. A rule matches when (accountID ∈ AccountIDs OR groupID ∈ GroupIDs)
// and Pricing contains a row covering the upstream model. A rule whose
// AccountIDs and GroupIDs are both empty is skipped — operators creating
// blank rules should not accidentally cover every account.
//
// Returns (cost, true) when a rule produces a positive cost; (zero, false)
// otherwise so the caller can distinguish "no opinion" from a deliberate
// zero. (Negative or zero costs from a rule are treated as "no opinion"
// inside calculateAccountStatsCost so this contract holds.)
func tryCustomRules(
	channel *Channel, accountID, groupID int64,
	platform, model string, tokens UsageTokens, requestCount int,
) (decimal.Decimal, bool) {
	modelLower := strings.ToLower(model)
	for i := range channel.AccountStatsPricingRules {
		rule := &channel.AccountStatsPricingRules[i]
		if !matchAccountStatsRule(rule, accountID, groupID) {
			continue
		}
		pricing := findAccountStatsPricingForModel(rule.Pricing, platform, modelLower)
		if pricing == nil {
			// Rule scope matched but no model entry — keep walking; a
			// later rule may carry the model.
			continue
		}
		if cost, ok := calculateAccountStatsCost(pricing, tokens, requestCount); ok {
			return cost, true
		}
	}
	return decimal.Zero, false
}

// matchAccountStatsRule returns true when the rule's scope covers the
// supplied account or group. Empty scope is skipped to avoid catch-all
// rules matching unrelated requests.
func matchAccountStatsRule(rule *AccountStatsPricingRule, accountID, groupID int64) bool {
	if len(rule.AccountIDs) == 0 && len(rule.GroupIDs) == 0 {
		return false
	}
	for _, id := range rule.AccountIDs {
		if id == accountID {
			return true
		}
	}
	for _, id := range rule.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

// wildcardCandidate is one prefix-wildcard match candidate; sorted by
// prefix length to honour "longest prefix wins".
type wildcardCandidate struct {
	prefixLen int
	pricing   *ChannelModelPricing
}

// findAccountStatsPricingForModel resolves a pricing row within a rule
// for the supplied platform/model. Exact matches take precedence over
// wildcard matches; among wildcards the longest prefix wins.
func findAccountStatsPricingForModel(pricingList []ChannelModelPricing, platform, modelLower string) *ChannelModelPricing {
	for i := range pricingList {
		p := &pricingList[i]
		if !isAccountStatsPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			if strings.ToLower(m) == modelLower {
				return p
			}
		}
	}
	var matches []wildcardCandidate
	for i := range pricingList {
		p := &pricingList[i]
		if !isAccountStatsPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			ml := strings.ToLower(m)
			rawPrefix, isWild := wildcard.SplitSuffix(ml)
			if !isWild {
				continue
			}
			if strings.HasPrefix(modelLower, rawPrefix) {
				matches = append(matches, wildcardCandidate{prefixLen: len(rawPrefix), pricing: p})
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].prefixLen > matches[j].prefixLen
	})
	return matches[0].pricing
}

// isAccountStatsPlatformMatch treats an empty platform on either side as
// "match anything". A rule pricing row with Platform="" therefore covers
// every group platform; a request whose group has no resolved platform
// also matches every rule pricing row.
//
// 这是 account-stats 自定义规则的特例语义（与 channel pricing / cache /
// pricing.server 用的"严格相等"不同）。把"空串通配"集中在这一函数里，
// 通用的 platforms.Match 保持严格语义；一旦未来想去掉这条特例，只需删除
// 本函数的空串短路即可，不会牵连其他读路径。
func isAccountStatsPlatformMatch(queryPlatform, pricingPlatform string) bool {
	if queryPlatform == "" || pricingPlatform == "" {
		return true
	}
	return platforms.Match(queryPlatform, pricingPlatform)
}

// calculateAccountStatsCost computes the raw account stats cost for the
// supplied pricing row. Rate multipliers are NOT applied here — the host
// is responsible for layering on account_rate_multiplier when it
// records the final number into usage_logs.
//
// Returns (cost, true) on a positive cost; (zero, false) when the pricing
// row produced nothing meaningful (nil pricing, all token counts zero,
// no per-request price configured, etc.). The boolean lets the caller
// keep walking the rule list without confusing "0 cost configured" with
// "rule did not match this request shape".
func calculateAccountStatsCost(pricing *ChannelModelPricing, tokens UsageTokens, requestCount int) (decimal.Decimal, bool) {
	if pricing == nil {
		return decimal.Zero, false
	}
	switch pricing.BillingMode {
	case BillingModePerRequest, BillingModeImage:
		return calculatePerRequestAccountStatsCost(pricing, requestCount)
	default:
		return calculateTokenAccountStatsCost(pricing, tokens)
	}
}

func calculatePerRequestAccountStatsCost(pricing *ChannelModelPricing, requestCount int) (decimal.Decimal, bool) {
	if !decimalx.IsPositive(pricing.PerRequestPrice) {
		return decimal.Zero, false
	}
	if requestCount <= 0 {
		requestCount = 1
	}
	cost := pricing.PerRequestPrice.Decimal.Mul(decimal.NewFromInt(int64(requestCount)))
	return cost, true
}

// calculateTokenAccountStatsCost adds up the per-token contributions for
// each component (input / output / cache write / cache read / image
// output). Each addend stays in decimal land so we never round-trip
// through float64 between additions.
func calculateTokenAccountStatsCost(pricing *ChannelModelPricing, tokens UsageTokens) (decimal.Decimal, bool) {
	addends := []struct {
		count int64
		price decimal.NullDecimal
	}{
		{tokens.InputTokens, pricing.InputPrice},
		{tokens.OutputTokens, pricing.OutputPrice},
		{tokens.CacheCreationTokens, pricing.CacheWritePrice},
		{tokens.CacheReadTokens, pricing.CacheReadPrice},
		{tokens.ImageOutputTokens, pricing.ImageOutputPrice},
	}
	cost := decimal.Zero
	for _, a := range addends {
		if a.count == 0 || !a.price.Valid {
			continue
		}
		cost = cost.Add(decimal.NewFromInt(a.count).Mul(a.price.Decimal))
	}
	if cost.Sign() <= 0 {
		return decimal.Zero, false
	}
	return cost, true
}
