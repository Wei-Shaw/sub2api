// Package idempotency contains the port interface for the idempotency
// bounded context: the SQL-backed store of idempotent write records. The
// contract references only domain types so the repository layer can implement
// it without importing internal/service. The service package keeps a type
// alias to the interface so existing call sites and test stubs continue to
// satisfy the contract.
package idempotency

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// IdempotencyRepository persists idempotency records.
type IdempotencyRepository interface {
	CreateProcessing(ctx context.Context, record *domain.IdempotencyRecord) (bool, error)
	GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*domain.IdempotencyRecord, error)
	TryReclaim(ctx context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error)
	ExtendProcessingLock(ctx context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error)
	MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error
	MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error
	DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error)
}
