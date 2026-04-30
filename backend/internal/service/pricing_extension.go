// Package service — pricing_extension.go
//
// PricingExtension types live in the service package to avoid an import
// cycle: BillingService consumes the PricingAdjuster interface, and the
// plugin layer (backend/internal/plugin) supplies the concrete
// implementation. The plugin package already imports service for
// PricingOverrideCache, so all types stay here on the service side.
//
// The interface is intentionally thin — only AdjustCost is exposed —
// because the data-layer (List + Watch) already reaches the service
// layer through PricingOverrideCache. BillingService never needs to
// talk to the plugin directly for cache contents.

package service

import "context"

// PricingAdjuster is the host-facing surface BillingService consults
// after computing a base cost. The implementation is allowed to error
// out or return Modified=false; in either case BillingService keeps its
// own cost. See plugin-sdk/proto/sdk.proto::PricingExtension.AdjustCost
// for the underlying RPC contract.
//
// Implementations must be safe for concurrent use.
type PricingAdjuster interface {
	AdjustCost(ctx context.Context, in AdjustCostInput) (AdjustCostResult, error)
}

// AdjustCostInput carries the raw fields BillingService passes in. It
// mirrors the proto AdjustCostRequest 1:1 but stays plain Go so callers
// in the service package do not need to import pluginsdk.
type AdjustCostInput struct {
	RequestID   string
	Model       string
	GroupID     int64
	UserID      string
	Platform    string
	ServiceTier string

	// CoreCost is the cost the host computed before consulting the
	// plugin. Treated as the authoritative answer when the plugin
	// returns Modified=false or errors out.
	CoreCost AdjustCostBreakdown
	Tokens   AdjustCostTokens
}

// AdjustCostBreakdown mirrors proto PricingCostBreakdown.
type AdjustCostBreakdown struct {
	Currency       string
	Total          float64
	InputCost      float64
	OutputCost     float64
	CacheWriteCost float64
	CacheReadCost  float64
	ImageCost      float64
	BillingMode    string
}

// AdjustCostTokens mirrors proto PricingUsageTokens.
type AdjustCostTokens struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ImageCount          int64
}

// AdjustCostResult is what BillingService receives back. Modified=false
// means the host should keep its own cost.
type AdjustCostResult struct {
	Modified         bool
	FinalCost        AdjustCostBreakdown
	AdjustmentReason string
}
