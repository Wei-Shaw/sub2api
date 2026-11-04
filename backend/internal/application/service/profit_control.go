package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/shared/timezone"
)

const (
	profitControlRateEpsilon             = 1e-9
	profitControlFilterReasonThreshold   = "profit_threshold"
	profitControlFilterReasonInvalidRate = "profit_invalid_account_rate"
)

type profitControlGateCtxKey struct{}
type profitControlSuppressCtxKey struct{}
type tokenRequestPricingAtCtxKey struct{}
type tokenRequestBillingGroupCtxKey struct{}

type profitControlGate struct {
	groupID   int64
	platform  string
	threshold float64
	pricingAt time.Time
}

// WithGatewayTokenRequestPricing freezes token pricing for the request. Media-only
// and metadata endpoints deliberately do not call this function.
func WithGatewayTokenRequestPricing(ctx context.Context) (context.Context, time.Time) {
	if ctx == nil {
		ctx = context.Background()
	}
	pricingAt := timezone.Now()
	ctx = context.WithValue(ctx, tokenRequestPricingAtCtxKey{}, pricingAt)
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) {
		ctx = context.WithValue(ctx, tokenRequestBillingGroupCtxKey{}, group)
	}
	return ctx, pricingAt
}

func GatewayTokenRequestPricingAtFromContext(ctx context.Context) time.Time {
	pricingAt, _ := tokenRequestPricingAtFromContext(ctx)
	return pricingAt
}

func tokenRequestPricingAtFromContext(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	pricingAt, ok := ctx.Value(tokenRequestPricingAtCtxKey{}).(time.Time)
	return pricingAt, ok && !pricingAt.IsZero()
}

func tokenRequestBillingGroupFromContext(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	if ctx.Value(apiKeyGroupRoutingAttemptKey{}) != nil {
		if group, _ := ctx.Value(tokenRequestBillingGroupCtxKey{}).(*Group); IsGroupContextValid(group) {
			return group
		}
	}
	if group := currentAPIKeyRoutingGroup(ctx); IsGroupContextValid(group) {
		return group
	}
	group, _ := ctx.Value(tokenRequestBillingGroupCtxKey{}).(*Group)
	if IsGroupContextValid(group) {
		return group
	}
	return nil
}

// WithOpenAIProfitControlSuppressed keeps media, count-token, and live traffic
// outside token profit admission even if a defensive scheduler entry is used.
func WithOpenAIProfitControlSuppressed(ctx context.Context) context.Context {
	return context.WithValue(ctx, profitControlSuppressCtxKey{}, struct{}{})
}

func profitControlSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(profitControlSuppressCtxKey{}).(struct{})
	return ok
}

func clampProfitControlThreshold(threshold float64) float64 {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		return 0
	}
	return threshold
}

func profitControlOverThreshold(upstream, threshold float64) bool {
	return upstream-threshold > profitControlRateEpsilon*math.Max(1, math.Abs(threshold))
}

func resolveProfitControlGate(
	ctx context.Context,
	groupID *int64,
	loadGroup func(context.Context, int64) (*Group, error),
	resolveUserRate func(context.Context, int64, int64, float64) float64,
) *profitControlGate {
	if ctx == nil || groupID == nil || *groupID <= 0 || profitControlSuppressed(ctx) {
		return nil
	}
	pricingAt, ok := tokenRequestPricingAtFromContext(ctx)
	if !ok {
		return nil
	}

	var group *Group
	if current, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(current) && current.ID == *groupID {
		group = current
	} else if loadGroup != nil {
		loaded, err := loadGroup(ctx, *groupID)
		if err != nil {
			slog.Warn("profit_control_group_load_failed", "group_id", *groupID, "error", err)
			return nil
		}
		group = loaded
	}
	if group == nil || !group.ProfitControlEnabled || !profitControlPlatformSupported(group.Platform) {
		return nil
	}

	billingGroup := tokenRequestBillingGroupFromContext(ctx)
	if billingGroup == nil {
		if current, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(current) {
			billingGroup = current
		} else {
			billingGroup = group
		}
	}
	downstream := billingGroup.RateMultiplier
	if userID, _ := ctx.Value(ctxkey.UserID).(int64); userID > 0 && resolveUserRate != nil {
		downstream = resolveUserRate(ctx, userID, billingGroup.ID, billingGroup.RateMultiplier)
	}
	downstream *= billingGroup.PeakMultiplierAt(pricingAt)
	threshold := clampProfitControlThreshold(
		downstream * (1 - group.ProfitMinMargin - group.ProfitSafetyBuffer),
	)
	return &profitControlGate{
		groupID:   group.ID,
		platform:  group.Platform,
		threshold: threshold,
		pricingAt: pricingAt,
	}
}

func installProfitControlGate(ctx context.Context, groupID *int64, gate *profitControlGate) context.Context {
	existing, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate)
	if gate == nil {
		if existing != nil && groupID != nil && existing.groupID != *groupID {
			return context.WithValue(ctx, profitControlGateCtxKey{}, (*profitControlGate)(nil))
		}
		return ctx
	}
	if existing != nil && existing.groupID == gate.groupID {
		return ctx
	}
	return context.WithValue(ctx, profitControlGateCtxKey{}, gate)
}

func (s *GatewayService) withGatewayProfitControlGate(ctx context.Context, groupID *int64) context.Context {
	if s == nil {
		return ctx
	}
	gate := resolveProfitControlGate(
		ctx,
		groupID,
		func(loadCtx context.Context, id int64) (*Group, error) {
			if s.schedulerSnapshot != nil {
				return s.schedulerSnapshot.GetGroupByIDLite(loadCtx, id)
			}
			return s.resolveGroupByID(loadCtx, id)
		},
		s.ResolveUserGroupRateMultiplier,
	)
	return installProfitControlGate(ctx, groupID, gate)
}

func (s *OpenAIGatewayService) withOpenAIProfitControlGate(ctx context.Context, groupID *int64) context.Context {
	if s == nil {
		return ctx
	}
	gate := resolveProfitControlGate(
		ctx,
		groupID,
		func(loadCtx context.Context, id int64) (*Group, error) {
			if s.schedulerSnapshot != nil {
				return s.schedulerSnapshot.GetGroupByIDLite(loadCtx, id)
			}
			return nil, nil
		},
		s.ResolveUserGroupRateMultiplier,
	)
	return installProfitControlGate(ctx, groupID, gate)
}

func (s *OpenAIGatewayService) WithOpenAIRequestPricingContext(ctx context.Context, groupID *int64) (context.Context, time.Time) {
	ctx, pricingAt := WithGatewayTokenRequestPricing(ctx)
	return s.withOpenAIProfitControlGate(ctx, groupID), pricingAt
}

func (s *OpenAIGatewayService) WithOpenAITurnPricingContext(ctx context.Context, groupID *int64) (context.Context, time.Time) {
	pricingAt := timezone.Now()
	ctx = context.WithValue(ctx, tokenRequestPricingAtCtxKey{}, pricingAt)
	// A WS turn freezes the refreshed authenticated group without mutating the
	// connection's routing state. The profit gate must use the same sale price
	// as this turn's usage record, including multi-group connections.
	if group, _ := ctx.Value(ctxkey.Group).(*Group); IsGroupContextValid(group) &&
		groupID != nil && group.ID == *groupID {
		ctx = context.WithValue(ctx, tokenRequestBillingGroupCtxKey{}, group)
		ctx = context.WithValue(ctx, apiKeyGroupRoutingAttemptKey{}, group.ID)
	}
	if existing, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate); existing != nil {
		id := existing.groupID
		groupID = &id
		ctx = context.WithValue(ctx, profitControlGateCtxKey{}, (*profitControlGate)(nil))
	}
	return s.withOpenAIProfitControlGate(ctx, groupID), pricingAt
}

func OpenAIPricingAtFromContext(ctx context.Context) time.Time {
	return GatewayTokenRequestPricingAtFromContext(ctx)
}

func profitControlVetoReason(ctx context.Context, account *Account) (bool, string) {
	gate, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate)
	if gate == nil || account == nil {
		return false, ""
	}
	if account.RateMultiplier == nil || math.IsNaN(*account.RateMultiplier) ||
		math.IsInf(*account.RateMultiplier, 0) || *account.RateMultiplier < 0 {
		return true, profitControlFilterReasonInvalidRate
	}
	if profitControlOverThreshold(*account.RateMultiplier, gate.threshold) {
		return true, profitControlFilterReasonThreshold
	}
	return false, ""
}

func OpenAIProfitControlVeto(ctx context.Context, account *Account) (bool, string) {
	return profitControlVetoReason(ctx, account)
}

func profitControlVetoLatest(ctx context.Context, selected *Account, snapshot *SchedulerSnapshotService) (*Account, bool, string) {
	gate, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate)
	if gate == nil || selected == nil {
		return selected, false, ""
	}
	latest := selected
	if snapshot != nil {
		refreshed, err := snapshot.GetAccount(ctx, selected.ID)
		if err != nil {
			slog.Warn("profit_control_account_refresh_failed", "account_id", selected.ID, "error", err)
		} else if refreshed != nil && !refreshed.UpdatedAt.Before(selected.UpdatedAt) {
			latest = refreshed
		}
	}
	vetoed, reason := profitControlVetoReason(ctx, latest)
	return latest, vetoed, reason
}

func (s *GatewayService) GatewayProfitControlVetoLatest(ctx context.Context, selected *Account) (*Account, bool, string) {
	if s == nil {
		return selected, false, ""
	}
	return profitControlVetoLatest(ctx, selected, s.schedulerSnapshot)
}

func (s *OpenAIGatewayService) ProfitControlVetoLatest(ctx context.Context, selected *Account) (*Account, bool, string) {
	if s == nil {
		return selected, false, ""
	}
	return profitControlVetoLatest(ctx, selected, s.schedulerSnapshot)
}

func attachSelectionProfitGate(ctx context.Context, selection *AccountSelectionResult) *AccountSelectionResult {
	if selection == nil {
		return nil
	}
	if gate, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate); gate != nil {
		selection.profitGate = gate
	}
	return selection
}

func ContextWithSelectionProfitGate(ctx context.Context, selection *AccountSelectionResult) context.Context {
	if selection == nil || selection.profitGate == nil {
		return ctx
	}
	return context.WithValue(ctx, profitControlGateCtxKey{}, selection.profitGate)
}

func (s *GatewayService) isGatewayAccountProfitEligible(ctx context.Context, account *Account) bool {
	vetoed, _ := profitControlVetoReason(ctx, account)
	return !vetoed
}

func gatewayProfitControlGateActive(ctx context.Context) bool {
	gate, _ := ctx.Value(profitControlGateCtxKey{}).(*profitControlGate)
	return gate != nil
}

func (s *GatewayService) bindGatewayStickySessionDuringSelection(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if gatewayProfitControlGateActive(ctx) {
		return nil
	}
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
}

func (s *GatewayService) BindStickySessionAfterProfitAdmission(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if !gatewayProfitControlGateActive(ctx) {
		return s.BindStickySession(ctx, groupID, sessionHash, accountID)
	}
	existing, err := s.GetCachedSessionAccountID(ctx, groupID, sessionHash)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		return nil
	}
	if existing > 0 && existing != accountID {
		return nil
	}
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
}

func (s *OpenAIGatewayService) bindOpenAIStickySessionDuringSelection(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if gatewayProfitControlGateActive(ctx) || openAIImageSizeSpilloverPass(ctx) {
		return nil
	}
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
}

func (s *OpenAIGatewayService) BindStickySessionAfterProfitAdmission(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if !gatewayProfitControlGateActive(ctx) {
		return s.BindStickySession(ctx, groupID, sessionHash, accountID)
	}
	existing, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		return nil
	}
	if existing > 0 && existing != accountID {
		return nil
	}
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
}
