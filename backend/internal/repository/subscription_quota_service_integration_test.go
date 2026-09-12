//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func quotaServiceFixture(t *testing.T) (subscriptionQuotaFixture, *service.SubscriptionService, *service.Group) {
	t.Helper()
	f := newSubscriptionQuotaFixture(t)
	client := testEntClient(t)
	groups := NewGroupRepository(client, integrationDB)
	s := service.NewSubscriptionService(groups, NewUserSubscriptionRepository(client), nil, client, nil)
	s.SetSubscriptionQuotaRepository(f.repo)
	t.Cleanup(s.Stop)
	g, err := groups.GetByID(context.Background(), f.sub.GroupID)
	require.NoError(t, err)
	return f, s, g
}

func TestSubscriptionQuotaServiceAdmissionManualResetAndLateCharge(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	f.enable(t)
	ctx := context.Background()
	first, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.NotNil(t, first.Quota)
	before := first.Quota.Clone()
	_, err = s.AdminResetQuota(ctx, f.sub.ID, true, false, false)
	require.NoError(t, err)
	next, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.NotEqual(t, before.DailyBucketID, next.Quota.DailyBucketID)
	require.Equal(t, before.WeeklyBucketID, next.Quota.WeeklyBucketID)
	require.Equal(t, before.MonthlyBucketID, next.Quota.MonthlyBucketID)
	require.True(t, first.ExpiresAt.Equal(next.ExpiresAt))
	_, err = f.billing.Apply(ctx, f.command(before, 3))
	require.NoError(t, err)
	fresh, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Zero(t, fresh.DailyUsageUSD)
	require.Equal(t, 23.0, fresh.WeeklyUsageUSD)
	// Two manual resets in one calendar day still create distinct generations.
	_, err = s.AdminResetQuota(ctx, f.sub.ID, true, false, false)
	require.NoError(t, err)
	after, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.True(t, next.DailyWindowStart.Equal(*after.DailyWindowStart))
	require.NotEqual(t, next.Quota.DailyBucketID, after.Quota.DailyBucketID)
	// Old service objects cannot replay an expired maintenance decision.
	stale := *first
	oldAnchor := time.Now().Add(-48 * time.Hour)
	stale.DailyWindowStart = &oldAnchor
	require.NoError(t, s.CheckAndResetWindows(ctx, &stale))
	require.Equal(t, after.Quota.DailyBucketID, stale.Quota.DailyBucketID)
	current, err := s.GetActiveSubscription(ctx, f.sub.UserID, f.sub.GroupID)
	require.NoError(t, err)
	require.Zero(t, current.DailyUsageUSD)
}

func TestSubscriptionQuotaServiceLegacyAdmissionTransitionsAtEnable(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	ctx := context.Background()
	legacy, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.True(t, legacy.Quota.IsLegacy())
	groupState, err := f.repo.EnableGroupQuota(ctx, g.ID, time.Time{})
	require.NoError(t, err)
	require.True(t, legacy.Quota.AdmittedAt.Before(groupState.EnabledAt))
	_, err = f.billing.Apply(ctx, f.command(legacy.Quota, 2))
	require.NoError(t, err)
	managed, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.False(t, managed.Quota.IsLegacy())
	require.Equal(t, 12.0, managed.DailyUsageUSD)
	// Missing-token legacy paths cannot charge current managed counters.
	repo := NewUserSubscriptionRepository(testEntClient(t))
	require.ErrorIs(t, repo.IncrementUsage(ctx, f.sub.ID, 9), service.ErrSubscriptionQuotaRequired)
	require.ErrorIs(t, repo.ResetUsageWindows(ctx, f.sub.ID, true, true, true, time.Now(), time.Now()), service.ErrSubscriptionQuotaRequired)
	require.ErrorIs(t, repo.ResetDailyUsage(ctx, f.sub.ID, managed.DailyWindowStart, time.Now()), service.ErrSubscriptionQuotaRequired)
	copy := *managed
	copy.DailyUsageUSD = 0
	require.NoError(t, repo.Update(ctx, &copy))
	state, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, 12.0, state.DailyUsageUSD)
	require.Equal(t, 12.0, copy.DailyUsageUSD, "generic metadata updates preserve managed projection")
}

func TestSubscriptionQuotaServiceTermLifecyclePreservesHistoricalCharges(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	f.enable(t)
	ctx := context.Background()
	before, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	_, err = s.ExtendSubscription(ctx, f.sub.ID, 5)
	require.NoError(t, err)
	extended, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Equal(t, before.Quota.DailyBucketID, extended.Quota.DailyBucketID)
	require.Equal(t, before.DailyUsageUSD, extended.DailyUsageUSD)
	require.NoError(t, s.RevokeSubscription(ctx, f.sub.ID))
	_, err = s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.Error(t, err)
	_, err = f.billing.Apply(ctx, f.command(before.Quota, 2))
	require.NoError(t, err)
	_, err = s.RestoreSubscription(ctx, f.sub.ID)
	require.NoError(t, err)
	restored, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Equal(t, before.Quota.DailyBucketID, restored.Quota.DailyBucketID)
	require.Equal(t, 12.0, restored.DailyUsageUSD)
	// An expired renewal advances the term and all three bucket identities.
	_, err = integrationDB.Exec(`UPDATE user_subscriptions SET expires_at=NOW()-INTERVAL '1 minute',status='expired' WHERE id=$1`, f.sub.ID)
	require.NoError(t, err)
	_, _, err = s.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{UserID: f.sub.UserID, GroupID: g.ID, ValidityDays: 30})
	require.NoError(t, err)
	renewed, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Equal(t, before.Quota.TermEpoch+1, renewed.Quota.TermEpoch)
	require.NotEqual(t, before.Quota.MonthlyBucketID, renewed.Quota.MonthlyBucketID)
	require.Zero(t, renewed.DailyUsageUSD)
	_, err = f.billing.Apply(ctx, f.command(restored.Quota, 4))
	require.NoError(t, err)
	current, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Zero(t, current.DailyUsageUSD)
}

func TestSubscriptionQuotaServiceFirstUseAndInvalidTermAdmission(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	ctx := context.Background()
	_, err := integrationDB.Exec(`UPDATE user_subscriptions SET daily_window_start=NULL,weekly_window_start=NULL,monthly_window_start=NULL WHERE id=$1`, f.sub.ID)
	require.NoError(t, err)
	state := f.enable(t)
	require.Nil(t, state.DailyWindowStart)
	first, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.NotNil(t, first.DailyWindowStart)
	require.NotEqual(t, state.DailyBucketID, first.Quota.DailyBucketID)
	require.Equal(t, 10.0, first.DailyUsageUSD, "first activation preserves any usage already accounted before an anchor existed")
	again, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
	require.NoError(t, err)
	require.Equal(t, first.Quota.DailyBucketID, again.Quota.DailyBucketID)
	for _, condition := range []string{"starts_at=NOW()+INTERVAL '1 hour'", "starts_at=NOW()-INTERVAL '1 hour',status='suspended'", "status='active',expires_at=NOW()-INTERVAL '1 hour'"} {
		_, err = integrationDB.Exec(`UPDATE user_subscriptions SET `+condition+` WHERE id=$1`, f.sub.ID)
		require.NoError(t, err)
		_, err = s.AdmitSubscriptionQuota(ctx, f.sub.ID, g)
		require.Error(t, err)
	}
}

func TestSubscriptionQuotaServiceExpiredExtensionStartsNewTerm(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	before := f.enable(t)
	_, err := integrationDB.Exec(`UPDATE user_subscriptions SET expires_at=NOW()-INTERVAL '1 hour',status='expired' WHERE id=$1`, f.sub.ID)
	require.NoError(t, err)
	_, err = s.ExtendSubscription(context.Background(), f.sub.ID, 7)
	require.NoError(t, err)
	after, err := s.AdmitSubscriptionQuota(context.Background(), f.sub.ID, g)
	require.NoError(t, err)
	require.Equal(t, before.TermEpoch+1, after.Quota.TermEpoch)
	require.Zero(t, after.MonthlyUsageUSD)
}

func TestSubscriptionQuotaServiceAdmissionWaitsForGroupPublication(t *testing.T) {
	f, s, g := quotaServiceFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	locked, release := make(chan struct{}), make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- f.repo.WithGroupQuotaTx(ctx, g.ID, true, func(txCtx context.Context) error {
			now, err := f.repo.QuotaNow(txCtx)
			if err != nil {
				return err
			}
			_, err = f.repo.RotateGroupQuota(txCtx, service.RotateGroupQuotaInput{GroupID: g.ID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "upstream_reset", At: now, DailyWindowStart: now.Truncate(24 * time.Hour)})
			close(locked)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	<-locked
	type admissionResult struct {
		sub *service.UserSubscription
		err error
	}
	admitted := make(chan admissionResult, 1)
	go func() { sub, err := s.AdmitSubscriptionQuota(ctx, f.sub.ID, g); admitted <- admissionResult{sub, err} }()
	select {
	case result := <-admitted:
		close(release)
		t.Fatalf("admission escaped uncommitted publication: %+v", result)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-committed)
	result := <-admitted
	require.NoError(t, result.err)
	require.NotEqual(t, before.DailyBucketID, result.sub.Quota.DailyBucketID)
	require.Equal(t, before.WeeklyBucketID, result.sub.Quota.WeeklyBucketID)
	require.Zero(t, result.sub.DailyUsageUSD)
}
