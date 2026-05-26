package pluginsdk

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// HostClient is the SDK-facing handle a plugin obtains via
// PluginContext.Host(). Every method routes to the host's
// HostServiceServer over the reverse gRPC channel.
//
// Unlike Secrets / Jobs / Settings the SDK wires HostClient
// unconditionally — no capability declaration is required. The
// availability model is inverted for this reason: callers MUST treat
// every error as "the host has no answer" and degrade gracefully
// rather than aborting. ErrHostPricingUnavailable is the stable
// sentinel surfaced when the host-side server is not registered
// (older host, or PricingResolver dependency not injected); any other
// error is a transient RPC failure the plugin can retry or ignore at
// its discretion.
//
// Keep the method set small: HostService is for narrow, side-effect-
// free lookups the host already computes for its own use. Large
// surface area belongs in a dedicated extension service.
type HostClient interface {
	// ResolveModelPricing returns the host's globally-known pricing
	// for the named model (LiteLLM data + host fallbacks). Returns
	// nil, nil when the host has no pricing for the model — ordinary
	// "model not configured" outcome, NOT an error.
	//
	// Returns a non-nil error only for genuine RPC failures (host
	// unavailable, context canceled, …). Callers should log at Debug
	// level and treat the request as if ResolveModelPricing had
	// returned nil, nil so missing pricing never aborts a request.
	ResolveModelPricing(ctx context.Context, model string) (*HostModelPricing, error)
}

// HostModelPricing mirrors the subset of host ModelPricing fields
// exposed by HostService.ResolveModelPricing. Nil-valued *decimal.Decimal
// fields mean "not set" — the host had no data for that particular
// dimension (e.g. a model without cache-breakdown pricing leaves the
// cache_*_price fields nil). Callers render nil as "unknown" rather
// than treating it as zero.
//
// Decimal is used instead of float64 to preserve exact LiteLLM values
// (prices like 0.000003 lose accuracy in IEEE-754 and that shows up
// in the user-facing popover as 0.00).
type HostModelPricing struct {
	InputPrice       *decimal.Decimal
	OutputPrice      *decimal.Decimal
	CacheWritePrice  *decimal.Decimal
	CacheReadPrice   *decimal.Decimal
	ImageOutputPrice *decimal.Decimal
}

// hostRPCTimeout caps per-call RPC time. HostService lookups are
// expected to be sub-millisecond (BillingService.GetModelPricing is an
// in-memory map read); anything beyond this usually means the host is
// degraded, and we prefer to surface the deadline so the plugin can
// fall back to nil pricing quickly rather than stall a user-facing
// page.
const hostRPCTimeout = 2 * time.Second

// ErrHostPricingUnavailable is the stable sentinel returned when the
// host-side HostService is not registered (older host, or the
// PricingResolver was not wired into PluginManager). Plugins may
// switch on this sentinel to log at Debug once rather than on every
// failing request.
var ErrHostPricingUnavailable = errors.New("pluginsdk: host pricing unavailable (HostService not registered)")

// hostClient is the SDK-side concrete implementation wrapping the
// generated gRPC client. It is always wired (see runner_init.go
// wireHost); when the host does not implement HostService the gRPC
// call surfaces codes.Unimplemented and we translate that into
// ErrHostPricingUnavailable so plugins can distinguish "no data" from
// "feature not available".
type hostClient struct {
	grpc pb.HostServiceClient
}

// newHostClient accepts any non-nil client so a future test harness
// can inject a stub. Returned value is safe for concurrent use;
// generated clients embed the grpc.ClientConn which already handles
// concurrency.
func newHostClient(c pb.HostServiceClient) *hostClient {
	return &hostClient{grpc: c}
}

func (h *hostClient) ResolveModelPricing(ctx context.Context, model string) (*HostModelPricing, error) {
	if model == "" {
		// No model name = no pricing. Avoid burning a round trip.
		return nil, nil
	}
	rpcCtx, cancel := context.WithTimeout(ctx, hostRPCTimeout)
	defer cancel()
	resp, err := h.grpc.ResolveModelPricing(rpcCtx, &pb.ResolveModelPricingRequest{Model: model})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostPricingUnavailable
		}
		return nil, err
	}
	if !resp.GetHasPricing() {
		return nil, nil
	}
	return &HostModelPricing{
		InputPrice:       parseOptionalDecimal(resp.GetInputPricePerToken()),
		OutputPrice:      parseOptionalDecimal(resp.GetOutputPricePerToken()),
		CacheWritePrice:  parseOptionalDecimal(resp.GetCacheWritePricePerToken()),
		CacheReadPrice:   parseOptionalDecimal(resp.GetCacheReadPricePerToken()),
		ImageOutputPrice: parseOptionalDecimal(resp.GetImageOutputPricePerToken()),
	}, nil
}

// parseOptionalDecimal mirrors the T24 "" = not set contract used by
// PricingExtension. Bad strings collapse to nil (treated as "not set")
// rather than surfacing an error because the caller has already
// committed to rendering whatever is returned — it's safer to drop a
// malformed field than to abort the whole popover.
func parseOptionalDecimal(s string) *decimal.Decimal {
	if s == "" {
		return nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil
	}
	return &d
}

// nilHostClient is returned from PluginContext.Host() when the SDK
// runner did not wire a real HostClient (shouldn't normally happen —
// Host is wired unconditionally — but covers test harnesses that
// construct a pluginCtx by hand). Every method returns
// ErrHostPricingUnavailable so plugins catch misconfiguration loudly.
type nilHostClient struct{}

func (nilHostClient) ResolveModelPricing(_ context.Context, _ string) (*HostModelPricing, error) {
	return nil, ErrHostPricingUnavailable
}
