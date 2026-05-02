// pricing_extension_client_codec.go owns the proto<->service translation
// boundary for the PricingExtension RPCs (AdjustCost,
// ResolveAccountStatsCost) and the periodic cache snapshot
// (protoToOverride). The watch / lifecycle code in pricing_extension_client.go
// + pricing_extension_client_watch.go calls into these helpers; the codec
// stays in its own file so changes to the proto wire shape are localised
// to the encode/decode site rather than scattered across the lifecycle
// loops.
//
// T39: the decimal wire codec lives in plugin-sdk/decimalx; this file uses
// decimalx.FormatFloatString / decimalx.FromProtoString as the single
// source of truth for the host side. parseDecimalString below is a thin
// adapter that demotes NullDecimal -> float64 because the host still
// stores prices as float64 (BillingService / channel pricing layers).

package plugin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// AdjustCost invokes PricingExtension.AdjustCost on the plugin. It is
// safe to call from any goroutine. Errors / nil-stub state are surfaced
// as Modified=false; callers must treat a non-nil error as "no
// adjustment, keep host-computed cost" and continue serving the
// request.
//
// Input / result types live in the service package so BillingService can
// invoke this method via service.PricingAdjuster without importing the
// plugin package (which would create an import cycle).
func (c *PricingExtensionClient) AdjustCost(ctx context.Context, in service.AdjustCostInput) (service.AdjustCostResult, error) {
	if c == nil {
		return service.AdjustCostResult{}, nil
	}
	stub := c.stubSnapshot()
	if stub == nil {
		return service.AdjustCostResult{}, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, pricingAdjustTimeout)
	defer cancel()

	req := &pb.AdjustCostRequest{
		Model:       in.Model,
		GroupId:     in.GroupID,
		UserId:      in.UserID,
		Platform:    in.Platform,
		ServiceTier: in.ServiceTier,
		RequestId:   in.RequestID,
		CoreCost:    encodeCostBreakdown(in.CoreCost),
		Tokens:      encodeUsageTokensAdjust(in.Tokens),
	}

	resp, err := stub.AdjustCost(rpcCtx, req)
	if err != nil {
		return service.AdjustCostResult{}, err
	}
	if !resp.GetModified() {
		return service.AdjustCostResult{Modified: false, AdjustmentReason: resp.GetAdjustmentReason()}, nil
	}
	final := resp.GetFinalCost()
	if final == nil {
		// Plugin signalled modified=true but did not supply a cost. Treat
		// as a defective response and fall back to host cost.
		c.logger.Warn("adjust cost: modified=true but final_cost missing")
		return service.AdjustCostResult{Modified: false, AdjustmentReason: resp.GetAdjustmentReason()}, nil
	}
	return service.AdjustCostResult{
		Modified:         true,
		FinalCost:        c.decodeCostBreakdown(final),
		AdjustmentReason: resp.GetAdjustmentReason(),
	}, nil
}

// ResolveAccountStatsCost invokes PricingExtension.ResolveAccountStatsCost
// on the plugin. Returns HasCost=false on any failure (nil receiver,
// not-yet-started client, RPC error, timeout) so the gateway hot path
// can keep going with the default formula
// (total_cost × account_rate_multiplier).
//
// Input / result types live in the service package so GatewayService
// can invoke this method via service.AccountStatsCostResolver without
// importing the plugin package (avoids the existing import cycle).
func (c *PricingExtensionClient) ResolveAccountStatsCost(ctx context.Context, in service.AccountStatsCostInput) (service.AccountStatsCostResult, error) {
	if c == nil {
		return service.AccountStatsCostResult{}, nil
	}
	stub := c.stubSnapshot()
	if stub == nil {
		return service.AccountStatsCostResult{}, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, pricingAccountStatsTimeout)
	defer cancel()

	resp, err := stub.ResolveAccountStatsCost(rpcCtx, &pb.ResolveAccountStatsCostRequest{
		ChannelId:     in.ChannelID,
		AccountId:     in.AccountID,
		GroupId:       in.GroupID,
		UpstreamModel: in.UpstreamModel,
		Tokens:        encodeUsageTokensAccountStats(in.Tokens),
		RequestCount:  int32(in.RequestCount),
		TotalCost:     decimalx.FormatFloatString(in.TotalCost),
		RequestId:     in.RequestID,
	})
	if err != nil {
		return service.AccountStatsCostResult{}, err
	}
	if !resp.GetHasCost() {
		return service.AccountStatsCostResult{HasCost: false, ResolutionReason: resp.GetResolutionReason()}, nil
	}
	return service.AccountStatsCostResult{
		HasCost:          true,
		Cost:             c.parseDecimalString(resp.GetCost()),
		ResolutionReason: resp.GetResolutionReason(),
	}, nil
}

// encodeCostBreakdown converts the host's float64 cost breakdown into the
// proto wire form. Centralising the six FormatFloatString calls (rather
// than spelling them out at every RPC site) ensures any future field
// addition lands in exactly one place — the AdjustCost call site stays
// at <30 lines.
func encodeCostBreakdown(in service.AdjustCostBreakdown) *pb.PricingCostBreakdown {
	return &pb.PricingCostBreakdown{
		Currency:       in.Currency,
		Total:          decimalx.FormatFloatString(in.Total),
		InputCost:      decimalx.FormatFloatString(in.InputCost),
		OutputCost:     decimalx.FormatFloatString(in.OutputCost),
		CacheWriteCost: decimalx.FormatFloatString(in.CacheWriteCost),
		CacheReadCost:  decimalx.FormatFloatString(in.CacheReadCost),
		ImageCost:      decimalx.FormatFloatString(in.ImageCost),
		BillingMode:    in.BillingMode,
	}
}

// decodeCostBreakdown is the inverse of encodeCostBreakdown for the proto
// reply path. Method receiver is required because parseDecimalString logs
// per-client when it sees malformed input — operators want to know which
// plugin shipped the bad payload.
func (c *PricingExtensionClient) decodeCostBreakdown(in *pb.PricingCostBreakdown) service.AdjustCostBreakdown {
	if in == nil {
		return service.AdjustCostBreakdown{}
	}
	return service.AdjustCostBreakdown{
		Currency:       in.GetCurrency(),
		Total:          c.parseDecimalString(in.GetTotal()),
		InputCost:      c.parseDecimalString(in.GetInputCost()),
		OutputCost:     c.parseDecimalString(in.GetOutputCost()),
		CacheWriteCost: c.parseDecimalString(in.GetCacheWriteCost()),
		CacheReadCost:  c.parseDecimalString(in.GetCacheReadCost()),
		ImageCost:      c.parseDecimalString(in.GetImageCost()),
		BillingMode:    in.GetBillingMode(),
	}
}

// encodeUsageTokensAdjust maps a service.AdjustCostTokens onto the proto
// type for the AdjustCost RPC. AdjustCost uses the post-billing image_count
// (in.ImageCount) directly because the request has already counted the
// generated images.
func encodeUsageTokensAdjust(in service.AdjustCostTokens) *pb.PricingUsageTokens {
	return &pb.PricingUsageTokens{
		InputTokens:         in.InputTokens,
		OutputTokens:        in.OutputTokens,
		CacheCreationTokens: in.CacheCreationTokens,
		CacheReadTokens:     in.CacheReadTokens,
		ImageCount:          in.ImageCount,
	}
}

// encodeUsageTokensAccountStats mirrors encodeUsageTokensAdjust for the
// ResolveAccountStatsCost RPC. The image-count slot is fed by
// ImageOutputTokens because account-stats accounting buckets each
// generated image as a separate output token (matches the legacy
// host formula). Kept as a separate helper rather than a flag-based
// branch so the call site at the top of ResolveAccountStatsCost stays
// flat.
func encodeUsageTokensAccountStats(in service.AccountStatsCostTokens) *pb.PricingUsageTokens {
	return &pb.PricingUsageTokens{
		InputTokens:         in.InputTokens,
		OutputTokens:        in.OutputTokens,
		CacheCreationTokens: in.CacheCreationTokens,
		CacheReadTokens:     in.CacheReadTokens,
		ImageCount:          in.ImageOutputTokens,
	}
}

// parseDecimalString delegates to decimalx.FromProtoString (the single-source
// proto-wire codec since T39) and demotes the resulting NullDecimal into the
// host's float64 representation. Empty / unset wire values map to 0; parse
// failures are logged with the offending raw text so operators can audit the
// source plugin, then degraded to 0 so the gateway hot path never aborts on
// a single malformed payload.
//
// TODO(plugin-grpc, T24 follow-up): the host's service.PricingOverride still
// stores float64 because BillingService / channel pricing layers consume
// float64. Upgrading those to decimal.Decimal end-to-end is a much larger
// change and intentionally out of scope here. The single bounded loss
// happens at this site only — the proto wire and the plugin both speak
// canonical decimal, and the host cache lives behind a 12-digit
// NUMERIC(20,12) DB schema, so the 53-bit IEEE-754 mantissa still covers
// every price magnitude in production. Switch to a decimal-everywhere
// model only after the corresponding host-side cleanup ships.
func (c *PricingExtensionClient) parseDecimalString(s string) float64 {
	nd, err := decimalx.FromProtoString(s)
	if err != nil {
		c.logger.Warn("pricing override: bad decimal string", "raw", s, "error", err)
		return 0
	}
	if !nd.Valid {
		return 0
	}
	return nd.Decimal.InexactFloat64()
}

// protoToOverride converts a proto PricingOverride into the cache value
// type. Callers feed the result into PricingOverrideCache.Set / ReplaceAll.
//
// T24: prices arrive as decimal strings; we parse via decimalx
// (single-source codec, T39) and demote to float64 once at this boundary.
// See parseDecimalString for why the host still uses float64 internally.
func (c *PricingExtensionClient) protoToOverride(o *pb.PricingOverride) service.PricingOverride {
	if o == nil {
		return service.PricingOverride{}
	}
	out := service.PricingOverride{
		BillingMode:      o.GetBillingMode(),
		InputPrice:       c.parseDecimalString(o.GetInputPrice()),
		OutputPrice:      c.parseDecimalString(o.GetOutputPrice()),
		CacheWritePrice:  c.parseDecimalString(o.GetCacheWritePrice()),
		CacheReadPrice:   c.parseDecimalString(o.GetCacheReadPrice()),
		ImageOutputPrice: c.parseDecimalString(o.GetImageOutputPrice()),
		PerRequestPrice:  c.parseDecimalString(o.GetPerRequestPrice()),
		SourcePlugin:     c.pluginName,
	}
	if k := o.GetKey(); k != nil {
		out.Key = service.PricingOverrideKey{
			GroupID:  k.GetGroupId(),
			Platform: k.GetPlatform(),
			Model:    k.GetModel(),
		}
	}
	if intervals := o.GetIntervals(); len(intervals) > 0 {
		out.Intervals = make([]service.PricingOverrideInterval, 0, len(intervals))
		for _, iv := range intervals {
			out.Intervals = append(out.Intervals, c.protoToInterval(iv))
		}
	}
	return out
}

// protoToInterval translates one proto PricingInterval into the cache
// value. Extracted out of protoToOverride so adding a new price field
// touches only this 7-line table instead of replicating it inline.
func (c *PricingExtensionClient) protoToInterval(iv *pb.PricingInterval) service.PricingOverrideInterval {
	return service.PricingOverrideInterval{
		MinTokens:        iv.GetMinTokens(),
		MaxTokens:        iv.GetMaxTokens(),
		InputPrice:       c.parseDecimalString(iv.GetInputPrice()),
		OutputPrice:      c.parseDecimalString(iv.GetOutputPrice()),
		CacheWritePrice:  c.parseDecimalString(iv.GetCacheWritePrice()),
		CacheReadPrice:   c.parseDecimalString(iv.GetCacheReadPrice()),
		ImageOutputPrice: c.parseDecimalString(iv.GetImageOutputPrice()),
		PerRequestPrice:  c.parseDecimalString(iv.GetPerRequestPrice()),
	}
}
