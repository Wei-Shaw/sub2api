package domain

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Subscription-related shared error sentinels. The service package re-exports
// each via var alias so existing call sites and test stubs keep compiling.
var ErrSubscriptionNotFound = infraerrors.NotFound("SUBSCRIPTION_NOT_FOUND", "subscription not found")
