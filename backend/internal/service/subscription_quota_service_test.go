package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type managedQuotaGroupStub struct{ SubscriptionQuotaRepository }

func (managedQuotaGroupStub) GetGroupQuotaState(_ context.Context, id int64) (*SubscriptionGroupQuotaState, error) {
	return &SubscriptionGroupQuotaState{GroupID: id, Revision: 1}, nil
}

type currentSubscriptionQuotaStub struct {
	UserSubscriptionRepository
	current *UserSubscription
	reads   int
}

func (r *currentSubscriptionQuotaStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	r.reads++
	copy := *r.current
	return &copy, nil
}

func TestSubscriptionQuotaManagedGroupBypassesDelayedOldL1Fill(t *testing.T) {
	repo := &currentSubscriptionQuotaStub{current: &UserSubscription{ID: 3, UserID: 1, GroupID: 2, Status: SubscriptionStatusActive, DailyUsageUSD: 0}}
	s := NewSubscriptionService(nil, repo, nil, nil, &config.Config{SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 100, L1TTLSeconds: 60}})
	t.Cleanup(s.Stop)
	t.Cleanup(s.subCacheL1.Close)
	s.SetSubscriptionQuotaRepository(managedQuotaGroupStub{})
	old := &UserSubscription{ID: 3, UserID: 1, GroupID: 2, Status: SubscriptionStatusActive, DailyUsageUSD: 100}
	// Simulate an asynchronous old DB read filling L1 after reset committed.
	require.True(t, s.subCacheL1.SetWithTTL(subCacheKey(1, 2), old, 1, time.Minute))
	s.subCacheL1.Wait()
	_, present := s.subCacheL1.Get(subCacheKey(1, 2))
	require.True(t, present)
	got, err := s.GetActiveSubscription(context.Background(), 1, 2)
	require.NoError(t, err)
	require.Equal(t, 1, repo.reads)
	require.Zero(t, got.DailyUsageUSD)
	got.DailyUsageUSD = 99
	require.Zero(t, repo.current.DailyUsageUSD)
}

func TestSubscriptionQuotaAdmissionRejectsZeroRemainingAllowance(t *testing.T) {
	limit, zero := 10.0, 0.0
	for _, test := range []struct {
		name  string
		group Group
		sub   UserSubscription
		want  error
	}{
		{"daily", Group{DailyLimitUSD: &limit}, UserSubscription{DailyUsageUSD: 10}, ErrDailyLimitExceeded},
		{"weekly", Group{WeeklyLimitUSD: &limit}, UserSubscription{WeeklyUsageUSD: 10}, ErrWeeklyLimitExceeded},
		{"monthly", Group{MonthlyLimitUSD: &limit}, UserSubscription{MonthlyUsageUSD: 10}, ErrMonthlyLimitExceeded},
		{"remaining", Group{DailyLimitUSD: &limit}, UserSubscription{DailyUsageUSD: 9.999}, nil},
		{"unlimited_zero_configuration", Group{DailyLimitUSD: &zero}, UserSubscription{DailyUsageUSD: 100}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, checkSubscriptionQuotaAdmissionLimits(&test.sub, &test.group), test.want)
		})
	}
}
