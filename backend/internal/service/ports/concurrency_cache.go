package ports

import "context"

// ConcurrencyCache defines cache operations for concurrency service
type ConcurrencyCache interface {
	// Slot management
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (bool, error)
	ReleaseAccountSlot(ctx context.Context, accountID int64) error
	GetAccountConcurrency(ctx context.Context, accountID int64) (int, error)

	AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (bool, error)
	ReleaseUserSlot(ctx context.Context, userID int64) error
	GetUserConcurrency(ctx context.Context, userID int64) (int, error)

	// Wait queue
	IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error)
	DecrementWaitCount(ctx context.Context, userID int64) error
REDACTED
