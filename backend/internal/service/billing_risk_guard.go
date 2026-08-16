package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

var ErrBillingRiskLeaseLost = errors.New("billing risk lease lost")

type BillingRiskRequestKind string

const (
	BillingRiskRequestText      BillingRiskRequestKind = "text"
	BillingRiskRequestWebSocket BillingRiskRequestKind = "websocket"
	BillingRiskRequestSyncImage BillingRiskRequestKind = "sync_image"
	BillingRiskRequestVideo     BillingRiskRequestKind = "video"
	BillingRiskRequestAudio     BillingRiskRequestKind = "audio"
	BillingRiskRequestSearch    BillingRiskRequestKind = "search"
)

type BillingRiskRequest struct {
	UserID                int64
	Balance               float64
	MinimumBalanceReserve float64
	SubscriptionBilling   bool
	Kind                  BillingRiskRequestKind
	EstimatedCost         float64
	EstimateCertain       bool
	ForceProtect          bool
	LeaseID               string
}

type BillingRiskPermit struct {
	UserID            int64
	LeaseID           string
	RiskMicros        int64
	LeaseTTL          time.Duration
	IdleTTL           time.Duration
	UncertainCooldown time.Duration
	RefreshInterval   time.Duration
	guard             *BillingRiskGuard
}

type BillingRiskPermitSnapshot struct {
	UserID                   int64  `json:"user_id"`
	LeaseID                  string `json:"lease_id"`
	RiskMicros               int64  `json:"risk_micros"`
	LeaseTTLSeconds          int64  `json:"lease_ttl_seconds"`
	IdleTTLSeconds           int64  `json:"idle_ttl_seconds"`
	UncertainCooldownSeconds int64  `json:"uncertain_cooldown_seconds"`
	RefreshIntervalSeconds   int64  `json:"refresh_interval_seconds"`
}

type BillingRiskGuard struct {
	store    BillingRiskStore
	settings *SettingService
}

func NewBillingRiskGuard(store BillingRiskStore, settings *SettingService) *BillingRiskGuard {
	return &BillingRiskGuard{store: store, settings: settings}
}

func (g *BillingRiskGuard) currentSettings() BillingRiskSettings {
	if g == nil || g.settings == nil {
		return DefaultBillingRiskSettings()
	}
	return g.settings.GetBillingRiskSettings()
}

func (g *BillingRiskGuard) Acquire(ctx context.Context, request BillingRiskRequest) (*BillingRiskPermit, error) {
	settings := g.currentSettings()
	if !settings.Enabled || request.SubscriptionBilling {
		return nil, nil
	}
	if request.UserID <= 0 || math.IsNaN(request.Balance) || math.IsInf(request.Balance, 0) {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_REQUEST", "余额风险租约请求参数无效")
	}

	protected := request.ForceProtect || request.Kind == BillingRiskRequestVideo ||
		request.Balance <= settings.LowBalanceThreshold || !request.EstimateCertain
	if request.Kind != BillingRiskRequestText && request.Kind != BillingRiskRequestWebSocket {
		protected = protected || request.EstimatedCost >= settings.HighCostTrigger
	}
	if !protected {
		return nil, nil
	}
	if g == nil || g.store == nil {
		return nil, infraerrors.ServiceUnavailable("BILLING_RISK_GUARD_UNAVAILABLE", "余额风险保护暂时不可用")
	}

	balanceMicros, err := billingRiskMicrosFloor(request.Balance)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_REQUEST", err.Error())
	}
	minimumReserveMicros, err := billingRiskMicrosCeil(math.Max(request.MinimumBalanceReserve, 0))
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_REQUEST", err.Error())
	}
	overdraftMicros, err := billingRiskMicrosFloor(settings.OverdraftAllowance)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_REQUEST", err.Error())
	}

	minimumRiskMicros, err := billingRiskMicrosCeil(settings.MinimumRequestRisk)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_ESTIMATE", err.Error())
	}
	if minimumRiskMicros < 1 {
		minimumRiskMicros = 1
	}
	var riskMicros int64
	if request.EstimateCertain {
		if math.IsNaN(request.EstimatedCost) || math.IsInf(request.EstimatedCost, 0) || request.EstimatedCost < 0 {
			return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_ESTIMATE", "请求费用估算无效")
		}
		riskAmount := math.Max(request.EstimatedCost*settings.SafetyFactor, settings.MinimumRequestRisk)
		riskMicros, err = billingRiskMicrosCeil(riskAmount)
		if err != nil {
			return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_ESTIMATE", err.Error())
		}
	} else {
		riskMicros = balanceMicros - minimumReserveMicros
		if riskMicros < 0 {
			riskMicros = 0
		}
		riskMicros += overdraftMicros
		if riskMicros < minimumRiskMicros {
			riskMicros = minimumRiskMicros
		}
	}
	if riskMicros < minimumRiskMicros {
		riskMicros = minimumRiskMicros
	}

	leaseID := request.LeaseID
	if leaseID == "" {
		leaseID = uuid.NewString()
	}
	leaseTTL := time.Duration(settings.LeaseTTLSeconds) * time.Second
	if request.Kind == BillingRiskRequestVideo {
		leaseTTL = time.Duration(settings.VideoLeaseTTLSeconds) * time.Second
	}
	idleTTL := time.Duration(settings.IdleBalanceTTLSeconds) * time.Second
	result, err := g.store.Acquire(ctx, BillingRiskAcquireRequest{
		UserID:                   request.UserID,
		LeaseID:                  leaseID,
		RiskMicros:               riskMicros,
		BalanceMicros:            balanceMicros,
		MinimumReserveMicros:     minimumReserveMicros,
		OverdraftAllowanceMicros: overdraftMicros,
		LeaseTTL:                 leaseTTL,
		IdleTTL:                  idleTTL,
	})
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("BILLING_RISK_GUARD_UNAVAILABLE", "余额风险保护暂时不可用").WithCause(err)
	}
	if result == nil || !result.Acquired {
		return nil, infraerrors.TooManyRequests("BILLING_RISK_BUDGET_EXCEEDED", "账户可用风险额度已被其他在途请求占用，请稍后重试")
	}

	return &BillingRiskPermit{
		UserID:            request.UserID,
		LeaseID:           leaseID,
		RiskMicros:        riskMicros,
		LeaseTTL:          leaseTTL,
		IdleTTL:           idleTTL,
		UncertainCooldown: time.Duration(settings.UncertainCooldownSeconds) * time.Second,
		RefreshInterval:   time.Duration(settings.RefreshIntervalSeconds) * time.Second,
		guard:             g,
	}, nil
}

func (g *BillingRiskGuard) Refresh(ctx context.Context, permit *BillingRiskPermit) error {
	if g == nil || g.store == nil || permit == nil {
		return nil
	}
	refreshed, err := g.store.Refresh(ctx, permit.UserID, permit.LeaseID, permit.LeaseTTL, permit.IdleTTL)
	if err != nil || refreshed {
		return err
	}
	marked, markErr := g.store.MarkUncertain(
		ctx,
		permit.UserID,
		permit.LeaseID,
		permit.RiskMicros,
		permit.UncertainCooldown,
		permit.IdleTTL,
	)
	if markErr != nil {
		return fmt.Errorf("%w: restore uncertain cooldown: %v", ErrBillingRiskLeaseLost, markErr)
	}
	if !marked {
		return fmt.Errorf("%w: uncertain cooldown was not restored", ErrBillingRiskLeaseLost)
	}
	return ErrBillingRiskLeaseLost
}

func (g *BillingRiskGuard) ResetBalance(ctx context.Context, userID int64, balance float64, expectedVersion int64) (*BillingRiskBalanceResetResult, error) {
	if g == nil || g.store == nil {
		return nil, fmt.Errorf("billing risk store is unavailable")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("用户 ID 无效")
	}
	balanceMicros, err := billingRiskMicrosFloor(balance)
	if err != nil {
		return nil, err
	}
	idleTTL := time.Duration(g.currentSettings().IdleBalanceTTLSeconds) * time.Second
	return g.store.ResetBalance(ctx, userID, balanceMicros, expectedVersion, idleTTL)
}

func (g *BillingRiskGuard) Commit(ctx context.Context, permit *BillingRiskPermit, newBalance float64) error {
	if g == nil || g.store == nil || permit == nil {
		return nil
	}
	balanceMicros, err := billingRiskMicrosFloor(newBalance)
	if err != nil {
		return err
	}
	_, err = g.store.Commit(ctx, permit.UserID, permit.LeaseID, balanceMicros, permit.IdleTTL)
	return err
}

func (g *BillingRiskGuard) Release(ctx context.Context, permit *BillingRiskPermit) error {
	if g == nil || g.store == nil || permit == nil {
		return nil
	}
	_, err := g.store.Release(ctx, permit.UserID, permit.LeaseID, permit.IdleTTL)
	return err
}

func (g *BillingRiskGuard) MarkUncertain(ctx context.Context, permit *BillingRiskPermit) error {
	if g == nil || g.store == nil || permit == nil {
		return nil
	}
	_, err := g.store.MarkUncertain(ctx, permit.UserID, permit.LeaseID, permit.RiskMicros, permit.UncertainCooldown, permit.IdleTTL)
	return err
}

func (p *BillingRiskPermit) Refresh(ctx context.Context) error {
	if p == nil || p.guard == nil {
		return nil
	}
	return p.guard.Refresh(ctx, p)
}

func (p *BillingRiskPermit) Release(ctx context.Context) error {
	if p == nil || p.guard == nil {
		return nil
	}
	return p.guard.Release(ctx, p)
}

func (p *BillingRiskPermit) MarkUncertain(ctx context.Context) error {
	if p == nil || p.guard == nil {
		return nil
	}
	return p.guard.MarkUncertain(ctx, p)
}

func (p *BillingRiskPermit) Snapshot() *BillingRiskPermitSnapshot {
	if p == nil {
		return nil
	}
	return &BillingRiskPermitSnapshot{
		UserID:                   p.UserID,
		LeaseID:                  p.LeaseID,
		RiskMicros:               p.RiskMicros,
		LeaseTTLSeconds:          int64(p.LeaseTTL / time.Second),
		IdleTTLSeconds:           int64(p.IdleTTL / time.Second),
		UncertainCooldownSeconds: int64(p.UncertainCooldown / time.Second),
		RefreshIntervalSeconds:   int64(p.RefreshInterval / time.Second),
	}
}

func billingRiskMicrosCeil(amount float64) (int64, error) {
	return billingRiskMicros(amount, math.Ceil)
}

func billingRiskMicrosFloor(amount float64) (int64, error) {
	return billingRiskMicros(amount, math.Floor)
}

func billingRiskMicros(amount float64, round func(float64) float64) (int64, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("金额必须是有限数值")
	}
	micros := round(amount * 1_000_000)
	if micros > math.MaxInt64 || micros < math.MinInt64 {
		return 0, fmt.Errorf("金额超出可表示范围")
	}
	return int64(micros), nil
}
