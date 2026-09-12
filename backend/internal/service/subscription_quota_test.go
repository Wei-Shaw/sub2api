package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type managedQuotaFallbackRepo struct{ UserSubscriptionRepository }

func (*managedQuotaFallbackRepo) IncrementUsage(context.Context, int64, float64) error {
	return ErrSubscriptionQuotaRequired
}

func TestSubscriptionQuotaFallbackRejectsMissingAdmission(t *testing.T) {
	params := &postUsageBillingParams{Cost: &CostBreakdown{TotalCost: 1, ActualCost: 1}, User: &User{ID: 2}, APIKey: &APIKey{ID: 1}, Account: &Account{ID: 3},
		Subscription: &UserSubscription{ID: 4}, IsSubscriptionBill: true}
	applied, err := applyUsageBilling(context.Background(), "request", nil, params, &billingDeps{userSubRepo: &managedQuotaFallbackRepo{}}, nil)
	require.ErrorIs(t, err, ErrSubscriptionQuotaRequired)
	require.False(t, applied)
	params.Subscription.Quota = &SubscriptionQuotaSnapshot{DailyBucketID: 1, WeeklyBucketID: 2, MonthlyBucketID: 3}
	applied, err = applyUsageBilling(context.Background(), "request", nil, params, &billingDeps{}, nil)
	require.ErrorIs(t, err, ErrSubscriptionQuotaUnavailable)
	require.False(t, applied)
}

func TestSubscriptionQuotaFingerprintPreservesLegacyAndBindsBuckets(t *testing.T) {
	id := int64(4)
	base := UsageBillingCommand{RequestID: "request", APIKeyID: 1, UserID: 2, AccountID: 3, SubscriptionID: &id, SubscriptionCost: 1.25}
	legacy := base
	legacy.SubscriptionQuota = &SubscriptionQuotaSnapshot{SubscriptionID: id, AdmittedAt: time.Now()}
	require.Equal(t, buildUsageBillingFingerprint(&base), buildUsageBillingFingerprint(&legacy))
	managed := base
	managed.SubscriptionQuota = &SubscriptionQuotaSnapshot{DailyBucketID: 10, WeeklyBucketID: 11, MonthlyBucketID: 12}
	require.NotEqual(t, buildUsageBillingFingerprint(&base), buildUsageBillingFingerprint(&managed))
	frozen := buildUsageBillingFingerprint(&managed)
	managed.SubscriptionQuota.DailyBucketID = 13
	require.NotEqual(t, frozen, buildUsageBillingFingerprint(&managed))
}

func TestSubscriptionQuotaBuilderFreezesAdmissionSnapshot(t *testing.T) {
	start := time.Now().UTC()
	quota := &SubscriptionQuotaSnapshot{SubscriptionID: 4, UserID: 2, GroupID: 5, DailyBucketID: 10, WeeklyBucketID: 11, MonthlyBucketID: 12, DailyWindowStart: &start}
	params := &postUsageBillingParams{Cost: &CostBreakdown{TotalCost: 1, ActualCost: 1}, User: &User{ID: 2}, APIKey: &APIKey{ID: 1}, Account: &Account{ID: 3},
		Subscription: &UserSubscription{ID: 4, Quota: quota}, IsSubscriptionBill: true}
	cmd := buildUsageBillingCommand("request", nil, params)
	require.NotNil(t, cmd)
	require.Equal(t, quota, cmd.SubscriptionQuota)
	quota.DailyBucketID = 20
	*quota.DailyWindowStart = quota.DailyWindowStart.Add(time.Hour)
	require.EqualValues(t, 10, cmd.SubscriptionQuota.DailyBucketID)
	require.False(t, quota.DailyWindowStart.Equal(*cmd.SubscriptionQuota.DailyWindowStart))
}
