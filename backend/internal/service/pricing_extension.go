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

// AccountStatsCostResolver is the host-facing surface GatewayService
// consults after computing the customer cost. The implementation is
// allowed to error out or return HasCost=false; in either case the host
// keeps usage_logs.account_stats_cost NULL and aggregation queries fall
// back to total_cost via COALESCE(account_stats_cost, total_cost). See
// plugin-sdk/proto/sdk.proto::PricingExtension.ResolveAccountStatsCost
// for the underlying RPC contract.
//
// Implementations must be safe for concurrent use.
type AccountStatsCostResolver interface {
	ResolveAccountStatsCost(ctx context.Context, in AccountStatsCostInput) (AccountStatsCostResult, error)
}

// AccountStatsCostInput carries the per-request fields GatewayService
// passes in. Mirrors the proto ResolveAccountStatsCostRequest 1:1 but
// stays plain Go so callers in the service package do not need to
// import pluginsdk.
type AccountStatsCostInput struct {
	RequestID     string
	ChannelID     int64
	AccountID     int64
	GroupID       int64
	UpstreamModel string
	Tokens        AccountStatsCostTokens
	RequestCount  int
	TotalCost     float64
}

// AccountStatsCostTokens is the per-request token / image count
// breakdown forwarded to the resolver. Field semantics match the proto
// PricingUsageTokens message so the client can copy values verbatim.
type AccountStatsCostTokens struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ImageOutputTokens   int64
}

// AccountStatsCostResult is what GatewayService receives back.
// HasCost=false means the host should keep account_stats_cost NULL.
type AccountStatsCostResult struct {
	HasCost          bool
	Cost             float64
	ResolutionReason string
}
