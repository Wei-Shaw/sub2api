package service

import (
	"context"
	"time"
)

type BillingRiskAcquireRequest struct {
	UserID                   int64
	LeaseID                  string
	RiskMicros               int64
	BalanceMicros            int64
	MinimumReserveMicros     int64
	OverdraftAllowanceMicros int64
	LeaseTTL                 time.Duration
	IdleTTL                  time.Duration
}

type BillingRiskAcquireResult struct {
	Acquired            bool
	WouldReject         bool
	ReservedTotalMicros int64
	KnownBalanceMicros  int64
}

type BillingRiskBalanceResetResult struct {
	Accepted           bool
	KnownBalanceMicros int64
}

// BillingRiskStore 只负责单用户 Redis 风险状态的原子变更，不包含触发或定价策略。
type BillingRiskStore interface {
	Acquire(ctx context.Context, request BillingRiskAcquireRequest) (*BillingRiskAcquireResult, error)
	GetBalanceVersion(ctx context.Context, userID int64) (int64, error)
	Refresh(ctx context.Context, userID int64, leaseID string, ttl, idleTTL time.Duration) (bool, error)
	Commit(ctx context.Context, userID int64, leaseID string, newBalanceMicros int64, idleTTL time.Duration) (bool, error)
	Release(ctx context.Context, userID int64, leaseID string, idleTTL time.Duration) (bool, error)
	MarkUncertain(ctx context.Context, userID int64, leaseID string, riskMicros int64, cooldown, idleTTL time.Duration) (bool, error)
	ResetBalance(ctx context.Context, userID int64, balanceMicros, expectedVersion int64, idleTTL time.Duration) (*BillingRiskBalanceResetResult, error)
}
