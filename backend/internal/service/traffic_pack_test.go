//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPlanTrafficCreditDeductions_ConsumesEarliestExpiryThenOldestPurchase(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	batches := []TrafficCreditBatch{
		{ID: 30, RemainingUSD: 20, CreditedAt: base.Add(2 * time.Hour), ExpiresAt: base.AddDate(1, 0, 0)},
		{ID: 10, RemainingUSD: 5, CreditedAt: base, ExpiresAt: base.AddDate(1, 0, 0)},
		{ID: 20, RemainingUSD: 10, CreditedAt: base.Add(time.Hour), ExpiresAt: base.AddDate(1, 0, 7)},
	}

	plan, covered := PlanTrafficCreditDeductions(batches, 8)

	require.True(t, covered)
	require.Equal(t, []TrafficCreditDeduction{
		{CreditID: 10, AmountUSD: 5},
		{CreditID: 30, AmountUSD: 3},
	}, plan)
}

func TestPlanTrafficCreditDeductions_ReturnsUncoveredWhenBalanceInsufficient(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	plan, covered := PlanTrafficCreditDeductions([]TrafficCreditBatch{
		{ID: 1, RemainingUSD: 2, CreditedAt: base, ExpiresAt: base.AddDate(1, 0, 0)},
		{ID: 2, RemainingUSD: 3, CreditedAt: base.Add(time.Hour), ExpiresAt: base.AddDate(1, 0, 1)},
	}, 6)

	require.False(t, covered)
	require.Equal(t, []TrafficCreditDeduction{
		{CreditID: 1, AmountUSD: 2},
		{CreditID: 2, AmountUSD: 3},
	}, plan)
}

func TestBuildUsageBillingCommand_UsesTrafficPackInsteadOfBalance(t *testing.T) {
	t.Parallel()

	cmd := buildUsageBillingCommand("req-traffic-pack", &UsageLog{RequestID: "req-traffic-pack"}, &postUsageBillingParams{
		Cost:           &CostBreakdown{ActualCost: 0.25, TotalCost: 0.25},
		User:           &User{ID: 7},
		APIKey:         &APIKey{ID: 9},
		Account:        &Account{ID: 11},
		Platform:       PlatformOpenAI,
		UseTrafficPack: true,
	})

	require.NotNil(t, cmd)
	require.Equal(t, 0.25, cmd.TrafficPackCost)
	require.Zero(t, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
}

type trafficPackEligibilityRepo struct {
	TrafficPackRepository
	hasAvailable bool
}

func (r trafficPackEligibilityRepo) HasAvailableCredit(ctx context.Context, userID int64, now time.Time) (bool, error) {
	return r.hasAvailable, nil
}

type trafficPackEligibilityUserRepo struct {
	UserRepository
	balance float64
}

func (r trafficPackEligibilityUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	return &User{ID: id, Balance: r.balance}, nil
}

func TestBillingCacheServiceAllowsOpenAITrafficPackWhenBalanceEmpty(t *testing.T) {
	t.Parallel()

	svc := &BillingCacheService{
		cfg:                &config.Config{},
		userRepo:           trafficPackEligibilityUserRepo{balance: 0},
		trafficPackService: NewTrafficPackService(trafficPackEligibilityRepo{hasAvailable: true}),
	}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 7}, nil, nil, nil, PlatformOpenAI)
	require.NoError(t, err)
}

func TestBillingCacheServiceDoesNotUseTrafficPackForOtherPlatforms(t *testing.T) {
	t.Parallel()

	svc := &BillingCacheService{
		cfg:                &config.Config{},
		userRepo:           trafficPackEligibilityUserRepo{balance: 0},
		trafficPackService: NewTrafficPackService(trafficPackEligibilityRepo{hasAvailable: true}),
	}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 7}, nil, nil, nil, PlatformAnthropic)
	require.ErrorIs(t, err, ErrInsufficientBalance)
}
