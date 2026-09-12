//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type subscriptionQuotaFixture struct {
	repo    service.SubscriptionQuotaRepository
	billing service.UsageBillingRepository
	sub     *service.UserSubscription
	keyID   int64
	at      time.Time
}

func newSubscriptionQuotaFixture(t *testing.T) subscriptionQuotaFixture {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: "quota-buckets-" + uuid.NewString() + "@example.com", PasswordHash: "hash"})
	group := mustCreateGroup(t, client, &service.Group{Name: "quota-buckets-" + uuid.NewString(), Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, GroupID: &group.ID, Key: "sk-quota-" + uuid.NewString(), Name: "quota"})
	at := time.Now().UTC().Truncate(time.Microsecond)
	start := at.Add(-time.Hour)
	sub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID, StartsAt: start, ExpiresAt: at.Add(60 * 24 * time.Hour), Status: service.SubscriptionStatusActive,
		DailyWindowStart: &start, WeeklyWindowStart: &start, MonthlyWindowStart: &start, DailyUsageUSD: 10, WeeklyUsageUSD: 20, MonthlyUsageUSD: 30})
	_, err := integrationDB.Exec(`UPDATE user_subscriptions SET daily_window_start=$2,weekly_window_start=$2,monthly_window_start=$2 WHERE id=$1`, sub.ID, start)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, query := range []string{"DELETE FROM usage_billing_dedup WHERE api_key_id=$1", "DELETE FROM usage_billing_dedup_archive WHERE api_key_id=$1"} {
			_, err := integrationDB.Exec(query, key.ID)
			require.NoError(t, err)
		}
		_, err := integrationDB.Exec("DELETE FROM user_subscriptions WHERE id=$1", sub.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM api_keys WHERE id=$1", key.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM groups WHERE id=$1", group.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM users WHERE id=$1", user.ID)
		require.NoError(t, err)
	})
	return subscriptionQuotaFixture{NewSubscriptionQuotaRepository(client), NewUsageBillingRepository(client, integrationDB), sub, key.ID, at}
}

func (f subscriptionQuotaFixture) enable(t *testing.T) *service.SubscriptionQuotaState {
	t.Helper()
	_, err := f.repo.EnableGroupQuota(context.Background(), f.sub.GroupID, f.at)
	require.NoError(t, err)
	state, err := f.repo.GetSubscriptionQuotaState(context.Background(), f.sub.ID)
	require.NoError(t, err)
	require.NotNil(t, state)
	state.AdmittedAt = f.at.Add(time.Microsecond)
	return state
}

func (f subscriptionQuotaFixture) command(quota *service.SubscriptionQuotaSnapshot, cost float64) *service.UsageBillingCommand {
	return &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: f.keyID, UserID: f.sub.UserID, SubscriptionID: &f.sub.ID, SubscriptionQuota: quota.Clone(), SubscriptionCost: cost}
}

func TestSubscriptionQuotaLateChargeRetainsUnselectedDimensions(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	at := f.at.Add(time.Minute)
	result, err := f.repo.RotateGroupQuota(ctx, service.RotateGroupQuotaInput{GroupID: f.sub.GroupID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true, Monthly: true}, Reason: "upstream_reset", At: at, DailyWindowStart: at.Truncate(24 * time.Hour)})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.AffectedSubscriptions)
	current, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.NotEqual(t, before.DailyBucketID, current.DailyBucketID)
	require.Equal(t, before.WeeklyBucketID, current.WeeklyBucketID)
	require.NotEqual(t, before.MonthlyBucketID, current.MonthlyBucketID)
	require.Equal(t, 0.0, current.DailyUsageUSD)
	require.Equal(t, 20.0, current.WeeklyUsageUSD)
	require.Equal(t, 0.0, current.MonthlyUsageUSD)
	cmd := f.command(&before.SubscriptionQuotaSnapshot, 2.5)
	charged, err := f.billing.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, charged.Applied)
	require.Equal(t, 0.0, charged.SubscriptionQuotaState.DailyUsageUSD)
	require.Equal(t, 22.5, charged.SubscriptionQuotaState.WeeklyUsageUSD)
	require.Equal(t, 0.0, charged.SubscriptionQuotaState.MonthlyUsageUSD)
	var daily, weekly, monthly float64
	require.NoError(t, integrationDB.QueryRow(`SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, f.sub.ID).Scan(&daily, &weekly, &monthly))
	require.Equal(t, []float64{0, 22.5, 0}, []float64{daily, weekly, monthly})
	again, err := f.billing.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, again.Applied)
	var oldUsage float64
	require.NoError(t, integrationDB.QueryRow(`SELECT used_usd FROM subscription_usage_buckets WHERE id=$1`, before.DailyBucketID).Scan(&oldUsage))
	require.Equal(t, 12.5, oldUsage)
	var charges int
	require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM subscription_quota_charges WHERE subscription_id=$1`, f.sub.ID).Scan(&charges))
	require.Equal(t, 1, charges)
}

func TestSubscriptionQuotaEnablePreservesAndRoutesLegacyAdmissions(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	ctx := context.Background()
	legacy := &service.SubscriptionQuotaSnapshot{SubscriptionID: f.sub.ID, UserID: f.sub.UserID, GroupID: f.sub.GroupID, AdmittedAt: f.at.Add(-time.Second), TermStartsAt: f.sub.StartsAt,
		DailyWindowStart: f.sub.DailyWindowStart, WeeklyWindowStart: f.sub.WeeklyWindowStart, MonthlyWindowStart: f.sub.MonthlyWindowStart}
	before := f.enable(t)
	require.Equal(t, 10.0, before.DailyUsageUSD)
	_, err := f.repo.EnableGroupQuota(ctx, f.sub.GroupID, f.at.Add(time.Second))
	require.NoError(t, err)
	charged, err := f.billing.Apply(ctx, f.command(legacy, 2))
	require.NoError(t, err)
	require.Equal(t, 12.0, charged.SubscriptionQuotaState.DailyUsageUSD)
	old := legacy.Clone()
	old.TermStartsAt = old.TermStartsAt.Add(-90 * 24 * time.Hour)
	charged, err = f.billing.Apply(ctx, f.command(old, 3))
	require.NoError(t, err)
	require.Equal(t, 12.0, charged.SubscriptionQuotaState.DailyUsageUSD, "older term must not affect initial current buckets")
	late := legacy.Clone()
	late.AdmittedAt = f.at.Add(time.Second)
	_, err = f.billing.Apply(ctx, f.command(late, 1))
	require.ErrorIs(t, err, service.ErrSubscriptionQuotaInvalid)
	_, err = f.billing.Apply(ctx, f.command(nil, 1))
	require.ErrorIs(t, err, service.ErrSubscriptionQuotaRequired)
}

func TestSubscriptionQuotaManualSameAnchorRejectsStaleMaintenance(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	input := service.RotateSubscriptionQuotaInput{SubscriptionID: f.sub.ID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "manual", At: f.at.Add(time.Minute), DailyWindowStart: f.sub.DailyWindowStart}
	reset, err := f.repo.RotateSubscriptionQuota(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, before.DailyBucketID, reset.DailyBucketID)
	reset.AdmittedAt = f.at.Add(2 * time.Minute)
	_, err = f.billing.Apply(ctx, f.command(&reset.SubscriptionQuotaSnapshot, 4))
	require.NoError(t, err)
	input.Reason = "automatic"
	input.Expected = &before.SubscriptionQuotaSnapshot
	stale, err := f.repo.RotateSubscriptionQuota(ctx, input)
	require.NoError(t, err)
	require.Equal(t, reset.DailyBucketID, stale.DailyBucketID)
	require.Equal(t, 4.0, stale.DailyUsageUSD)
}

func TestSubscriptionQuotaRenewedAndRevokedLateCharge(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	at := f.at.Add(time.Minute)
	_, err := f.repo.RotateSubscriptionQuota(ctx, service.RotateSubscriptionQuotaInput{SubscriptionID: f.sub.ID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true, Weekly: true, Monthly: true}, Reason: "renewal", NewTerm: true, TermStartsAt: at, At: at, DailyWindowStart: &at, WeeklyWindowStart: &at, MonthlyWindowStart: &at})
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE user_subscriptions SET deleted_at=NOW() WHERE id=$1`, f.sub.ID)
	require.NoError(t, err)
	charged, err := f.billing.Apply(ctx, f.command(&before.SubscriptionQuotaSnapshot, 2))
	require.NoError(t, err)
	require.EqualValues(t, 2, charged.SubscriptionQuotaState.TermEpoch)
	require.Zero(t, charged.SubscriptionQuotaState.DailyUsageUSD)
	require.NotNil(t, charged.SubscriptionQuotaState.DeletedAt)
}

func TestSubscriptionQuotaGroupFiltersAndRollback(t *testing.T) {
	for _, condition := range []string{"status='suspended'", "expires_at=NOW()-INTERVAL '1 hour'", "starts_at=NOW()+INTERVAL '1 hour'", "daily_window_start=NULL,weekly_window_start=NULL,monthly_window_start=NULL"} {
		t.Run(condition, func(t *testing.T) {
			f := newSubscriptionQuotaFixture(t)
			before := f.enable(t)
			ctx := context.Background()
			_, err := integrationDB.Exec(`UPDATE user_subscriptions SET `+condition+` WHERE id=$1`, f.sub.ID)
			require.NoError(t, err)
			result, err := f.repo.RotateGroupQuota(ctx, service.RotateGroupQuotaInput{GroupID: f.sub.GroupID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "upstream_reset", At: f.at.Add(time.Minute), DailyWindowStart: f.at})
			require.NoError(t, err)
			require.Zero(t, result.AffectedSubscriptions)
			after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
			require.NoError(t, err)
			require.Equal(t, before.DailyBucketID, after.DailyBucketID)
		})
	}
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	sentinel := errors.New("outbox failed")
	err := f.repo.WithGroupQuotaTx(ctx, f.sub.GroupID, true, func(txCtx context.Context) error {
		_, err := f.repo.RotateGroupQuota(txCtx, service.RotateGroupQuotaInput{GroupID: f.sub.GroupID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "upstream_reset", At: f.at.Add(time.Minute), DailyWindowStart: f.at})
		if err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, before.DailyBucketID, after.DailyBucketID)
	require.Equal(t, before.GroupRevision, after.GroupRevision)
}

func TestSubscriptionQuotaPublicationWaitsForAdmissionTransaction(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	locked := make(chan struct{})
	release := make(chan struct{})
	admissionDone := make(chan error, 1)
	go func() {
		admissionDone <- f.repo.WithGroupQuotaTx(ctx, f.sub.GroupID, false, func(txCtx context.Context) error {
			state, err := f.repo.GetSubscriptionQuotaState(txCtx, f.sub.ID)
			if err != nil {
				return err
			}
			if state.DailyBucketID != before.DailyBucketID {
				return fmt.Errorf("unexpected admission bucket")
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	published := make(chan error, 1)
	go func() {
		_, err := f.repo.RotateGroupQuota(ctx, service.RotateGroupQuotaInput{GroupID: f.sub.GroupID, Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "upstream_reset", At: f.at.Add(time.Minute), DailyWindowStart: f.at})
		published <- err
	}()
	select {
	case err := <-published:
		close(release)
		t.Fatalf("publication passed an admission lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-admissionDone)
	require.NoError(t, <-published)
}

func TestSubscriptionQuotaJoinsNativeObserverTransaction(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = f.repo.RotateGroupQuota(WithSubscriptionQuotaSQLTx(ctx, tx), service.RotateGroupQuotaInput{GroupID: f.sub.GroupID,
		Dimensions: service.SubscriptionQuotaDimensions{Weekly: true}, Reason: "upstream_reset", At: f.at.Add(time.Minute), DailyWindowStart: f.at})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, before.WeeklyBucketID, after.WeeklyBucketID)
	require.Equal(t, before.GroupRevision, after.GroupRevision)
}

func TestSubscriptionQuotaJoinsBoundEntClientTransaction(t *testing.T) {
	f := newSubscriptionQuotaFixture(t)
	before := f.enable(t)
	ctx := context.Background()
	tx, err := testEntClient(t).Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	repo := NewSubscriptionQuotaRepository(tx.Client())
	_, err = repo.RotateSubscriptionQuota(ctx, service.RotateSubscriptionQuotaInput{SubscriptionID: f.sub.ID,
		Dimensions: service.SubscriptionQuotaDimensions{Daily: true}, Reason: "manual", At: f.at.Add(time.Minute), DailyWindowStart: &f.at})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, before.DailyBucketID, after.DailyBucketID)
}
