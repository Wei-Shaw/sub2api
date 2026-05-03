// grpc_server_host.go implements HostService — the plugin→host reverse
// RPC surface carrying narrow host-side lookups (see plugin-sdk/proto/
// sdk.proto HostService block and ADR-UPSTREAM-SYNC-115-121 §4).
//
// Scope and non-goals
// -------------------
// HostService is intentionally small. The only RPC today is
// ResolveModelPricing, which proxies BillingService.GetModelPricing so
// the channel-management plugin can render a global-pricing fallback
// for "未配置模型" popovers after channel_available.go moved into the
// plugin (V5 W8 deliberately dropped the fallback pending this wire).
//
// New RPCs may be appended here when a plugin has a similarly narrow
// read-only need. Anything that mutates host state, requires per-
// plugin authorization, or introduces a new dependency graph belongs
// in its own extension service — HostService is the "utility belt"
// catch-all, not a kitchen sink.
//
// Decoupling
// ----------
// HostServiceServer talks to the host business layer through the
// HostPricingResolver interface, not a concrete *service.BillingService.
// This keeps the plugin package's dependency graph narrow (BillingService
// transitively imports large portions of the service layer) and makes
// the server trivially stubbable in unit tests.

package plugin

import (
	"context"
	"log/slog"
	"strings"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// HostPricingResolver is the minimal host-side surface HostServiceServer
// needs to satisfy ResolveModelPricing. *service.BillingService
// satisfies it directly via GetModelPricing; tests inject a stub.
//
// Kept private to this package is tempting but the interface is also
// the seam PluginManager exposes via SetHostPricingResolver, so
// exporting it is the cleaner split — callers see one contract, not
// a setter-type + unexported-interface pair.
type HostPricingResolver interface {
	// GetModelPricing mirrors service.BillingService.GetModelPricing.
	// Returns an error when pricing is not found for the model; the
	// server converts that into a has_pricing=false response rather
	// than surfacing the error over the wire because "model not
	// configured" is an expected outcome, not an RPC failure.
	GetModelPricing(model string) (*service.ModelPricing, error)
}

// HostServiceServer implements pluginsdk.HostServiceServer. One
// instance is shared across all plugins — the narrow RPC surface
// requires no per-plugin state. Construct with NewHostServiceServer;
// pass a nil resolver to indicate "no host pricing backend" which
// makes ResolveModelPricing return Unimplemented (same gRPC behaviour
// as not registering the service at all, but lets the manager decide
// once at wire time rather than on every call).
type HostServiceServer struct {
	pluginsdk.UnimplementedHostServiceServer

	resolver HostPricingResolver
}

// NewHostServiceServer constructs a server bound to the given
// resolver. A nil resolver is accepted and produces a server whose
// ResolveModelPricing returns codes.Unimplemented — semantically
// identical to not registering the service at all, but kept as an
// explicit option so PluginManager can still wire the service for
// tests without a full BillingService.
func NewHostServiceServer(resolver HostPricingResolver) *HostServiceServer {
	return &HostServiceServer{resolver: resolver}
}

// RegisterServices wires the server onto a gRPC server. Separate from
// the constructor so manager_sdk.go can lazily attach it alongside the
// other extensions.
func (s *HostServiceServer) RegisterServices(g *grpc.Server) {
	pluginsdk.RegisterHostServiceServer(g, s)
}

// ResolveModelPricing returns the host's globally-known pricing for
// the requested model. Missing pricing is reported as has_pricing=false
// (not an RPC error) so plugins can degrade gracefully without
// branching on gRPC status codes.
func (s *HostServiceServer) ResolveModelPricing(
	_ context.Context,
	req *pluginsdk.ResolveModelPricingRequest,
) (*pluginsdk.ResolveModelPricingResponse, error) {
	if s.resolver == nil {
		// Explicit "feature not available" signal — plugin SDK
		// translates it to ErrHostPricingUnavailable so the caller
		// can log once and move on.
		return nil, status.Error(codes.Unimplemented, "host pricing resolver not configured")
	}
	model := strings.TrimSpace(req.GetModel())
	if model == "" {
		// Empty model is a client-side bug — surface it instead of
		// silently returning has_pricing=false so the plugin author
		// notices during development.
		return nil, status.Error(codes.InvalidArgument, "model is required")
	}

	pricing, err := s.resolver.GetModelPricing(model)
	if err != nil || pricing == nil {
		// "no pricing found" is the happy path for unknown models;
		// return has_pricing=false and let the plugin render "未配置".
		return &pluginsdk.ResolveModelPricingResponse{HasPricing: false}, nil
	}

	return &pluginsdk.ResolveModelPricingResponse{
		HasPricing:               true,
		InputPricePerToken:       formatHostPrice(pricing.InputPricePerToken),
		OutputPricePerToken:      formatHostPrice(pricing.OutputPricePerToken),
		CacheWritePricePerToken:  formatHostPrice(pricing.CacheCreationPricePerToken),
		CacheReadPricePerToken:   formatHostPrice(pricing.CacheReadPricePerToken),
		ImageOutputPricePerToken: formatHostPrice(pricing.ImageOutputPricePerToken),
	}, nil
}

// formatHostPrice encodes a float64 price into the T24 decimal-string
// contract (empty string == "not set"). Zero values round-trip as "" so
// the plugin SDK decodes them as nil *decimal.Decimal, matching the
// upstream "0 means unconfigured" convention LiteLLM uses.
//
// decimal.NewFromFloat avoids the IEEE-754 tail that naive
// strconv.FormatFloat would emit (e.g. 3e-06 → "0.000003000000000001"):
// LiteLLM values are decimal in source, so decimal.NewFromFloat's
// shortest-repr round-trip preserves them cleanly enough for display.
func formatHostPrice(v float64) string {
	if v < 0 {
		slog.Warn("negative price value in host pricing response", "value", v)
		return ""
	}
	if v == 0 {
		return ""
	}
	return decimal.NewFromFloat(v).String()
}
