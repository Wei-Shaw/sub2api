package apikeycache

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// APIKeyCache defines cache operations for API key service
type APIKeyCache interface {
	GetCreateAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementCreateAttemptCount(ctx context.Context, userID int64) error
	DeleteCreateAttemptCount(ctx context.Context, userID int64) error

	IncrementDailyUsage(ctx context.Context, apiKey string) error
	SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error

	GetAuthCache(ctx context.Context, key string) (*domain.APIKeyAuthCacheEntry, error)
	SetAuthCache(ctx context.Context, key string, entry *domain.APIKeyAuthCacheEntry, ttl time.Duration) error
	DeleteAuthCache(ctx context.Context, key string) error

	// Pub/Sub for L1 cache invalidation across instances
	PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error
	SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error
}

type authCacheSubscriptionReadyKey struct{}

func WithAuthCacheSubscriptionReady(ctx context.Context, ready func()) context.Context {
	return context.WithValue(ctx, authCacheSubscriptionReadyKey{}, ready)
}

// NotifyAuthCacheSubscriptionReady lets cache implementations report that the
// server acknowledged the subscription without widening the public cache API.
func NotifyAuthCacheSubscriptionReady(ctx context.Context) {
	if ready, ok := ctx.Value(authCacheSubscriptionReadyKey{}).(func()); ok && ready != nil {
		ready()
	}
}
