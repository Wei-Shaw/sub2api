package domain

import "time"

// AuthCacheInvalidationEvent is a single outbox row representing a cache key
// awaiting invalidation across the auth-cache subscriber fleet.
type AuthCacheInvalidationEvent struct {
	ID        int64
	CacheKey  string
	Attempts  int
	Stage     int
	CreatedAt time.Time
}

// AuthCacheInvalidationOutboxStats is the operational snapshot of the outbox.
type AuthCacheInvalidationOutboxStats struct {
	Pending         int64
	OldestCreatedAt *time.Time
	MaxAttempts     int
	LastError       string
}
