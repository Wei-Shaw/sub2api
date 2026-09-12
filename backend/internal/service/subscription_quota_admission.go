package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// SetSubscriptionQuotaAdmission is called once during dependency construction,
// before serving traffic. Keeping this callback avoids a constructor cycle with
// SubscriptionService, which also invalidates the billing cache.
func (s *BillingCacheService) SetSubscriptionQuotaAdmission(admit func(context.Context, int64, *Group) (*UserSubscription, error)) {
	s.subscriptionQuotaAdmission = admit
}

// RefreshSubscriptionQuotaAdmission freezes authoritative quota identity at
// final admission. Subscription Redis entries never authorize bucket-managed
// traffic, so a delayed old fill/increment cannot restore an obsolete allowance.
// The caller owns subscription (never a shared L1 object). This method does not
// increment RPM and is suitable for rechecking the first WebSocket turn after
// its concurrency wait.
func (s *BillingCacheService) RefreshSubscriptionQuotaAdmission(ctx context.Context, userID int64, group *Group, subscription *UserSubscription) error {
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	if subscription == nil || group == nil || subscription.UserID != userID || subscription.GroupID != group.ID {
		return ErrSubscriptionInvalid
	}
	if s.subscriptionQuotaAdmission == nil {
		return s.checkSubscriptionEligibility(ctx, userID, group, subscription)
	}
	fresh, err := s.subscriptionQuotaAdmission(ctx, subscription.ID, group)
	if err != nil {
		return err
	}
	if fresh == nil || fresh.ID != subscription.ID || fresh.UserID != userID || fresh.GroupID != group.ID {
		return ErrSubscriptionInvalid
	}
	*subscription = *CloneSubscriptionForRequest(fresh)
	return nil
}

// CloneSubscriptionForRequest keeps per-request admission updates out of shared
// service caches and preserves a turn's quota token for asynchronous billing.
func CloneSubscriptionForRequest(subscription *UserSubscription) *UserSubscription {
	if subscription == nil {
		return nil
	}
	copy := *subscription
	cloneTime := func(value *time.Time) *time.Time {
		if value == nil {
			return nil
		}
		v := *value
		return &v
	}
	copy.DailyWindowStart = cloneTime(subscription.DailyWindowStart)
	copy.WeeklyWindowStart = cloneTime(subscription.WeeklyWindowStart)
	copy.MonthlyWindowStart = cloneTime(subscription.MonthlyWindowStart)
	if subscription.Quota != nil {
		quota := *subscription.Quota
		quota.DailyWindowStart = cloneTime(subscription.Quota.DailyWindowStart)
		quota.WeeklyWindowStart = cloneTime(subscription.Quota.WeeklyWindowStart)
		quota.MonthlyWindowStart = cloneTime(subscription.Quota.MonthlyWindowStart)
		copy.Quota = &quota
	}
	return &copy
}
