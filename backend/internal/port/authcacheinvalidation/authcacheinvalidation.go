// Package authcacheinvalidation contains the port interface for the
// auth-cache-invalidation bounded context: the SQL-backed outbox that drives
// delayed two-pass cache invalidation. DTO/value types live in internal/domain;
// this package only owns the persistence port contract.
package authcacheinvalidation

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AuthCacheInvalidationOutboxRepository persists auth-cache invalidation events.
type AuthCacheInvalidationOutboxRepository interface {
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]domain.AuthCacheInvalidationEvent, error)
	DeleteClaimed(ctx context.Context, id int64, workerID string) error
	ScheduleSecondPass(ctx context.Context, id int64, workerID string, availableAt time.Time) error
	RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error
	Stats(ctx context.Context) (domain.AuthCacheInvalidationOutboxStats, error)
}
