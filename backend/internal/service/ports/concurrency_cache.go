package ports

import "context"

// ConcurrencyCache defines cache operations for concurrency service
// Uses independent keys per request slot with native Redis TTL for automatic cleanup
type ConcurrencyCache interface {
	// Account slot management - each slot is a separate key with independent TTL
	// Key format: concurrency:account:{accountID}:{requestID}
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error
	GetAccountConcurrency(ctx context.Context, accountID int64) (int, error)

	// User slot management - each slot is a separate key with independent TTL
	// Key format: concurrency:user:{userID}:{requestID}
	AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error
	GetUserConcurrency(ctx context.Context, userID int64) (int, error)

	// Wait queue - uses counter with TTL set only on creation
	IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error)
	DecrementWaitCount(ctx context.Context, userID int64) error
}
