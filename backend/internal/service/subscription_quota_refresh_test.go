package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type quotaRefreshUserSubRepoStub struct {
	userSubRepoNoop
	sub       *UserSubscription
	refreshed bool
	period    string
	newExpiry time.Time
}

func (r *quotaRefreshUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *quotaRefreshUserSubRepoStub) RefreshQuotaAndShorten(_ context.Context, _ int64, period string, newExpiresAt, newWindowStart time.Time) error {
	r.refreshed = true
	r.period = period
	r.newExpiry = newExpiresAt
	r.sub.ExpiresAt = newExpiresAt
	r.sub.MonthlyUsageUSD = 0
	r.sub.MonthlyWindowStart = &newWindowStart
	return nil
}

func TestRefreshQuotaAndShortenKeepsRenewalLayer(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-6 * 24 * time.Hour)
	repo := &quotaRefreshUserSubRepoStub{}
	repo.sub = &UserSubscription{
		ID:                 99,
		UserID:             31,
		GroupID:            7,
		Status:             SubscriptionStatusActive,
		StartsAt:           windowStart,
		ExpiresAt:          now.Add(54 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    39,
		Group:              &Group{ID: 7, MonthlyLimitUSD: func() *float64 { v := 39.0; return &v }()},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	result, err := svc.RefreshQuotaAndShorten(context.Background(), 99, "monthly")

	require.NoError(t, err)
	require.True(t, repo.refreshed)
	require.Equal(t, "monthly", repo.period)
	require.InDelta(t, now.Add(30*24*time.Hour).Unix(), repo.newExpiry.Unix(), 3)
	require.Equal(t, float64(0), result.MonthlyUsageUSD)
}

func TestRefreshQuotaAndShortenRejectsQuotaThatIsNotFull(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-6 * 24 * time.Hour)
	limit := 39.0
	repo := &quotaRefreshUserSubRepoStub{sub: &UserSubscription{
		ID:                 99,
		UserID:             31,
		GroupID:            7,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(54 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    38.99,
		Group:              &Group{ID: 7, MonthlyLimitUSD: &limit},
	}}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	_, err := svc.RefreshQuotaAndShorten(context.Background(), 99, "monthly")

	require.Error(t, err)
	require.False(t, repo.refreshed)
}
