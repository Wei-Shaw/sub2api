package handler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func quotaTurnSubscription(bucket int64) *service.UserSubscription {
	start := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	return &service.UserSubscription{ID: 7, UserID: 1, GroupID: 2, Quota: &service.SubscriptionQuotaSnapshot{
		SubscriptionID: 7, UserID: 1, GroupID: 2, TermEpoch: 1, DailyBucketID: bucket,
		WeeklyBucketID: bucket + 1, MonthlyBucketID: bucket + 2, DailyWindowStart: &start,
	}}
}

func TestOpenAIWSTurnSubscriptionFinalAdmissionAndAsyncIsolation(t *testing.T) {
	base := quotaTurnSubscription(10)
	var turns openAIWSTurnSubscription
	turns.startAttempt()
	require.NoError(t, turns.admit(1, base, func(sub *service.UserSubscription) error {
		sub.Quota.DailyBucketID = 20 // A reset completed during concurrency admission.
		return nil
	}))
	firstBill := turns.finish(1, nil)
	require.EqualValues(t, 20, firstBill.Quota.DailyBucketID)
	require.EqualValues(t, 10, base.Quota.DailyBucketID, "shared auth state must remain untouched")
	require.NoError(t, turns.admit(2, base, func(sub *service.UserSubscription) error {
		sub.Quota.DailyBucketID = 30
		*sub.Quota.DailyWindowStart = sub.Quota.DailyWindowStart.Add(24 * time.Hour)
		return nil
	}))
	secondBill := turns.finish(2, nil)
	require.EqualValues(t, 30, secondBill.Quota.DailyBucketID)
	require.EqualValues(t, 20, firstBill.Quota.DailyBucketID, "a delayed usage task must retain the first turn's bucket")
	require.Equal(t, *base.Quota.DailyWindowStart, *firstBill.Quota.DailyWindowStart)
	require.NotEqual(t, *firstBill.Quota.DailyWindowStart, *secondBill.Quota.DailyWindowStart)
}

func TestOpenAIWSTurnSubscriptionPreservesQuotaAcrossFailoverRenumbering(t *testing.T) {
	var turns openAIWSTurnSubscription
	base := quotaTurnSubscription(10)
	checks := 0
	check := func(sub *service.UserSubscription) error {
		checks++
		sub.Quota.DailyBucketID = int64(checks * 100)
		return nil
	}
	require.NoError(t, turns.admit(1, base, check))
	require.NotNil(t, turns.finish(1, nil))
	require.NoError(t, turns.admit(2, base, check))
	failedBill := turns.finish(2, errors.New("upstream failover"))
	require.EqualValues(t, 200, failedBill.Quota.DailyBucketID)
	failedBill.Quota.DailyBucketID = 999 // Even a consumer cannot rewrite the retry snapshot.
	turns.startAttempt()                 // The retry transport calls the same logical turn "1".
	require.NoError(t, turns.admit(1, base, check))
	require.Equal(t, 2, checks, "failover must neither re-admit nor consume another RPM")
	require.EqualValues(t, 200, turns.finish(1, nil).Quota.DailyBucketID)
	require.NoError(t, turns.admit(2, base, check))
	require.EqualValues(t, 300, turns.finish(2, nil).Quota.DailyBucketID)
}

func TestOpenAIWSTurnSubscriptionRejectsOverlapAndIgnoresOldCompletion(t *testing.T) {
	var turns openAIWSTurnSubscription
	base := quotaTurnSubscription(10)
	check := func(*service.UserSubscription) error { return nil }
	require.NoError(t, turns.admit(1, base, check))
	require.ErrorIs(t, turns.admit(2, base, check), errOpenAIWSQuotaTurnActive)
	require.NotNil(t, turns.finish(1, nil))
	require.NoError(t, turns.admit(2, base, check))
	require.Nil(t, turns.finish(1, nil))
	require.EqualValues(t, 10, turns.finish(2, nil).Quota.DailyBucketID)
}

func TestOpenAIWSTurnSubscriptionRejectedAdmissionPublishesNoQuota(t *testing.T) {
	var turns openAIWSTurnSubscription
	base := quotaTurnSubscription(10)
	denied := errors.New("subscription expired")
	require.ErrorIs(t, turns.admit(1, base, func(sub *service.UserSubscription) error {
		sub.Quota.DailyBucketID = 999
		return denied
	}), denied)
	require.Nil(t, turns.finish(1, denied))
	require.EqualValues(t, 10, base.Quota.DailyBucketID)
	require.NoError(t, turns.admit(1, base, func(*service.UserSubscription) error { return nil }))
	require.EqualValues(t, 10, turns.finish(1, nil).Quota.DailyBucketID)
}

func TestOpenAIWSTurnSubscriptionConcurrentFailedAttemptSnapshots(t *testing.T) {
	var turns openAIWSTurnSubscription
	require.NoError(t, turns.admit(1, quotaTurnSubscription(10), func(*service.UserSubscription) error { return nil }))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := turns.finish(1, errors.New("retry"))
			sub.Quota.DailyBucketID++
			*sub.Quota.DailyWindowStart = sub.Quota.DailyWindowStart.Add(time.Hour)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 10, turns.finish(1, nil).Quota.DailyBucketID)
}
