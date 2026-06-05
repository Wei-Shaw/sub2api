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

// HostBalanceService is the host-side surface HostServiceServer needs to
// satisfy CreditBalance / DeductBalance. The concrete adapter wraps the
// host UserRepository plus an idempotency store (see
// host_service_adapters.go); injecting only this small interface keeps
// the gRPC handler decoupled from the wider service graph.
type HostBalanceService interface {
	CreditBalance(ctx context.Context, userID int64, amount decimal.Decimal,
		reason, idempotencyKey, source string) (newBalance decimal.Decimal, alreadyApplied bool, err error)
	DeductBalance(ctx context.Context, userID int64, amount decimal.Decimal,
		reason, idempotencyKey, source string, allowNegative bool) (newBalance decimal.Decimal, alreadyApplied bool, err error)
}

// HostSubscriptionAssigner is the host-side surface for the
// AssignSubscription / RevokeSubscriptionDays RPCs.
type HostSubscriptionAssigner interface {
	AssignSubscription(ctx context.Context, userID, planID, groupID int64, days int,
		source, idempotencyKey string) (subscriptionID, expiresAtUnix int64, alreadyApplied bool, err error)
	RevokeSubscriptionDays(ctx context.Context, userID, groupID int64, days int,
		reason, idempotencyKey string) (expiresAtUnix int64, alreadyApplied bool, err error)
}

// HostAffiliateAccruer wraps the affiliate-rebate flow for the
// AccrueRebate RPC.
type HostAffiliateAccruer interface {
	AccrueRebate(ctx context.Context, inviteeUserID int64, orderAmount decimal.Decimal,
		idempotencyKey string) (rebateAmount decimal.Decimal, inviterUserID int64, alreadyApplied bool, err error)
}

// HostUserLookup serves GetUserByID. Returning (nil, nil) signals
// not-found so the handler can map it to found=false; any other error
// surfaces as Internal.
type HostUserLookup interface {
	GetUserByID(ctx context.Context, userID int64) (*HostUserInfo, error)
}

// HostUserInfo is the projection HostService.GetUserByID returns. Kept
// minimal so the wire contract doesn't drag in unrelated user fields.
type HostUserInfo struct {
	ID        int64
	InviterID int64
	Email     string
	Username  string
	Role      string
	Balance   decimal.Decimal
}

// HostServiceServer implements pluginsdk.HostServiceServer. One
// instance is shared across all plugins — the narrow RPC surface
// requires no per-plugin state. Construct with NewHostServiceServer;
// pass a nil resolver to indicate "no host pricing backend" which
// makes ResolveModelPricing return Unimplemented (same gRPC behaviour
// as not registering the service at all, but lets the manager decide
// once at wire time rather than on every call).
//
// All dependencies are optional: a nil field makes the corresponding
// RPC return codes.Unimplemented (the embedded
// UnimplementedHostServiceServer base behaviour). The manager rebuilds
// the server via NewHostServiceServer whenever a setter is called so
// the field set is always coherent.
type HostServiceServer struct {
	pluginsdk.UnimplementedHostServiceServer

	resolver     HostPricingResolver
	balance      HostBalanceService
	subscription HostSubscriptionAssigner
	affiliate    HostAffiliateAccruer
	userLookup   HostUserLookup
}

// HostServiceDeps groups the optional host-side dependencies the gRPC
// HostServiceServer wraps. All fields are optional; nil values cause
// the corresponding RPC to return codes.Unimplemented.
type HostServiceDeps struct {
	Pricing      HostPricingResolver
	Balance      HostBalanceService
	Subscription HostSubscriptionAssigner
	Affiliate    HostAffiliateAccruer
	UserLookup   HostUserLookup
}

// NewHostServiceServer constructs a server bound to the given pricing
// resolver. Backwards-compatible with the original single-arg signature;
// other dependencies stay nil and their RPCs return Unimplemented. Use
// NewHostServiceServerWithDeps to wire the full payment-plugin surface.
func NewHostServiceServer(resolver HostPricingResolver) *HostServiceServer {
	return &HostServiceServer{resolver: resolver}
}

// NewHostServiceServerWithDeps constructs a server with every
// dependency declared via HostServiceDeps. A zero-value Deps produces
// a server whose every RPC returns Unimplemented — semantically
// identical to not registering the service at all.
func NewHostServiceServerWithDeps(deps HostServiceDeps) *HostServiceServer {
	return &HostServiceServer{
		resolver:     deps.Pricing,
		balance:      deps.Balance,
		subscription: deps.Subscription,
		affiliate:    deps.Affiliate,
		userLookup:   deps.UserLookup,
	}
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

// parsePositiveAmount validates and parses a decimal-string amount. The
// helper is used by CreditBalance / DeductBalance / AccrueRebate which
// all reject zero or negative values via InvalidArgument.
func parsePositiveAmount(s, fieldName string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Decimal{}, status.Errorf(codes.InvalidArgument,
			"%s: invalid decimal: %v", fieldName, err)
	}
	if d.Sign() <= 0 {
		return decimal.Decimal{}, status.Errorf(codes.InvalidArgument,
			"%s must be > 0", fieldName)
	}
	return d, nil
}

// CreditBalance implements HostService.CreditBalance.
func (s *HostServiceServer) CreditBalance(
	ctx context.Context, req *pluginsdk.CreditBalanceRequest,
) (*pluginsdk.CreditBalanceResponse, error) {
	if s.balance == nil {
		return nil, status.Error(codes.Unimplemented, "host balance service not configured")
	}
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id must be > 0")
	}
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	amount, err := parsePositiveAmount(req.GetAmount(), "amount")
	if err != nil {
		return nil, err
	}
	newBal, applied, err := s.balance.CreditBalance(ctx, req.GetUserId(), amount,
		req.GetReason(), req.GetIdempotencyKey(), req.GetSource())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	return &pluginsdk.CreditBalanceResponse{
		NewBalance:     newBal.String(),
		AlreadyApplied: applied,
	}, nil
}

// DeductBalance implements HostService.DeductBalance. AllowNegative=true
// means "refund flow — let balance go negative if necessary"; otherwise
// the host returns FailedPrecondition when amount > current balance.
func (s *HostServiceServer) DeductBalance(
	ctx context.Context, req *pluginsdk.DeductBalanceRequest,
) (*pluginsdk.DeductBalanceResponse, error) {
	if s.balance == nil {
		return nil, status.Error(codes.Unimplemented, "host balance service not configured")
	}
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id must be > 0")
	}
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	amount, err := parsePositiveAmount(req.GetAmount(), "amount")
	if err != nil {
		return nil, err
	}
	newBal, applied, err := s.balance.DeductBalance(ctx, req.GetUserId(), amount,
		req.GetReason(), req.GetIdempotencyKey(), req.GetSource(), req.GetAllowNegative())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	return &pluginsdk.DeductBalanceResponse{
		NewBalance:     newBal.String(),
		AlreadyApplied: applied,
	}, nil
}

// AssignSubscription implements HostService.AssignSubscription.
func (s *HostServiceServer) AssignSubscription(
	ctx context.Context, req *pluginsdk.AssignSubscriptionRequest,
) (*pluginsdk.AssignSubscriptionResponse, error) {
	if s.subscription == nil {
		return nil, status.Error(codes.Unimplemented, "host subscription service not configured")
	}
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id must be > 0")
	}
	if req.GetGroupId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "group_id must be > 0")
	}
	if req.GetDays() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "days must be > 0")
	}
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	subID, expiresAt, applied, err := s.subscription.AssignSubscription(ctx,
		req.GetUserId(), req.GetPlanId(), req.GetGroupId(), int(req.GetDays()),
		req.GetSource(), req.GetIdempotencyKey())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	return &pluginsdk.AssignSubscriptionResponse{
		SubscriptionId: subID,
		ExpiresAtUnix:  expiresAt,
		AlreadyApplied: applied,
	}, nil
}

// RevokeSubscriptionDays implements HostService.RevokeSubscriptionDays.
func (s *HostServiceServer) RevokeSubscriptionDays(
	ctx context.Context, req *pluginsdk.RevokeSubscriptionDaysRequest,
) (*pluginsdk.RevokeSubscriptionDaysResponse, error) {
	if s.subscription == nil {
		return nil, status.Error(codes.Unimplemented, "host subscription service not configured")
	}
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id must be > 0")
	}
	if req.GetGroupId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "group_id must be > 0")
	}
	if req.GetDays() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "days must be > 0")
	}
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	expiresAt, applied, err := s.subscription.RevokeSubscriptionDays(ctx,
		req.GetUserId(), req.GetGroupId(), int(req.GetDays()),
		req.GetReason(), req.GetIdempotencyKey())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	return &pluginsdk.RevokeSubscriptionDaysResponse{
		ExpiresAtUnix:  expiresAt,
		AlreadyApplied: applied,
	}, nil
}

// AccrueRebate implements HostService.AccrueRebate. The handler returns
// success with rebate_amount="" and inviter_user_id=0 when the invitee
// has no inviter or rebates are disabled — those are not RPC errors.
func (s *HostServiceServer) AccrueRebate(
	ctx context.Context, req *pluginsdk.AccrueRebateRequest,
) (*pluginsdk.AccrueRebateResponse, error) {
	if s.affiliate == nil {
		return nil, status.Error(codes.Unimplemented, "host affiliate service not configured")
	}
	if req.GetInviteeUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invitee_user_id must be > 0")
	}
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	amount, err := parsePositiveAmount(req.GetOrderAmount(), "order_amount")
	if err != nil {
		return nil, err
	}
	rebate, inviterID, applied, err := s.affiliate.AccrueRebate(ctx,
		req.GetInviteeUserId(), amount, req.GetIdempotencyKey())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	resp := &pluginsdk.AccrueRebateResponse{
		InviterUserId:  inviterID,
		AlreadyApplied: applied,
	}
	if rebate.Sign() > 0 {
		resp.RebateAmount = rebate.String()
	}
	return resp, nil
}

// GetUserByID implements HostService.GetUserByID. A nil HostUserInfo
// from the underlying lookup signals "not found" and produces a
// found=false response (the wire contract for soft-deletes).
func (s *HostServiceServer) GetUserByID(
	ctx context.Context, req *pluginsdk.GetUserByIDRequest,
) (*pluginsdk.GetUserByIDResponse, error) {
	if s.userLookup == nil {
		return nil, status.Error(codes.Unimplemented, "host user lookup not configured")
	}
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id must be > 0")
	}
	info, err := s.userLookup.GetUserByID(ctx, req.GetUserId())
	if err != nil {
		return nil, mapHostServiceError(err)
	}
	if info == nil {
		return &pluginsdk.GetUserByIDResponse{Found: false}, nil
	}
	return &pluginsdk.GetUserByIDResponse{
		Found:     true,
		UserId:    info.ID,
		Email:     info.Email,
		Username:  info.Username,
		Balance:   info.Balance.String(),
		InviterId: info.InviterID,
		Role:      info.Role,
	}, nil
}

// mapHostServiceError converts host-side errors to gRPC status codes.
// Insufficient-balance is the only recognised FailedPrecondition; other
// errors surface as Internal so plugins can log + retry without leaking
// host stack traces.
func mapHostServiceError(err error) error {
	if err == nil {
		return nil
	}
	// Already a status — pass through (adapters may pre-classify).
	if _, ok := status.FromError(err); ok {
		return err
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "insufficient") || strings.Contains(lower, "negative balance"):
		return status.Error(codes.FailedPrecondition, msg)
	case strings.Contains(lower, "not found"):
		return status.Error(codes.NotFound, msg)
	default:
		return status.Error(codes.Internal, msg)
	}
}
