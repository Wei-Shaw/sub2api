package service

import (
	"github.com/Wei-Shaw/sub2api/internal/port/subscription"
)

// UserSubscriptionRepository interface lives in port/subscription.
// UserSubscription type and subscriptionDayDuration const are aliased in
// user_subscription.go; this file only re-exports the repository interface.
type UserSubscriptionRepository = subscription.UserSubscriptionRepository
