package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// UserSubscription lives in domain; re-export for existing call sites.
type UserSubscription = domain.UserSubscription

// subscriptionDayDuration re-export for existing unit tests.
const subscriptionDayDuration = domain.SubscriptionDayDuration
