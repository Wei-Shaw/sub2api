package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// cacheInvalidationOutboxRepository implements service.CacheInvalidationOutboxRepository.
// Writes use txAwareSQLExecutor so they automatically participate in any active
// Ent transaction stored in ctx (e.g. by UserPoolRepository operations).
type cacheInvalidationOutboxRepository struct {
	db     *sql.DB
	client *dbent.Client
}

// NewCacheInvalidationOutboxRepository constructs a new outbox repository.
func NewCacheInvalidationOutboxRepository(db *sql.DB, client *dbent.Client) service.CacheInvalidationOutboxRepository {
	return &cacheInvalidationOutboxRepository{db: db, client: client}
}

// execFrom returns the SQL executor to use: the transaction from ctx if one is active
// (via dbent.TxFromContext), otherwise the raw *sql.DB.
func (r *cacheInvalidationOutboxRepository) execFrom(ctx context.Context) sqlQueryExecutor {
	return txAwareSQLExecutor(ctx, r.db, r.client)
}

// Enqueue writes a CacheInvalidationEvent row into the outbox using the transaction
// already bound to ctx (if any).  Failure propagates to the caller, causing any
// surrounding business transaction to roll back.
func (r *cacheInvalidationOutboxRepository) Enqueue(ctx context.Context, event service.CacheInvalidationEvent) error {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("cache_invalidation_outbox: marshal payload: %w", err)
	}

	exec := r.execFrom(ctx)

	var aggregateID *int64
	if event.AggregateID != nil {
		aggregateID = event.AggregateID
	}

	var idempotencyKey *string
	if event.IdempotencyKey != "" {
		k := event.IdempotencyKey
		idempotencyKey = &k
	}

	maxAttempts := event.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 12
	}

	_, err = exec.ExecContext(ctx, `
INSERT INTO cache_invalidation_outbox
    (event_type, aggregate_type, aggregate_id, reason, cache_types, payload,
     status, attempts, max_attempts, next_attempt_at, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6,
        'pending', 0, $7, NOW(), $8)
ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		event.EventType,
		event.AggregateType,
		aggregateID,
		event.Reason,
		pq.Array(event.CacheTypes),
		payloadJSON,
		maxAttempts,
		idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("cache_invalidation_outbox: enqueue: %w", err)
	}
	return nil
}

// ClaimReady locks up to `limit` ready rows (FOR UPDATE SKIP LOCKED), transitions them
// to "processing", and returns the claimed events.
func (r *cacheInvalidationOutboxRepository) ClaimReady(ctx context.Context, workerID string, limit int, lockTimeout time.Duration) ([]service.CacheInvalidationEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	lockedAt := now

	rows, err := r.db.QueryContext(ctx, `
UPDATE cache_invalidation_outbox
   SET status     = 'processing',
       locked_at  = $1,
       locked_by  = $2,
       updated_at = NOW()
 WHERE id IN (
     SELECT id
       FROM cache_invalidation_outbox
      WHERE status IN ('pending', 'failed')
        AND next_attempt_at <= NOW()
      ORDER BY id ASC
      LIMIT $3
      FOR UPDATE SKIP LOCKED
 )
RETURNING id, event_type, aggregate_type, aggregate_id, reason, cache_types, payload,
          status, attempts, max_attempts, next_attempt_at,
          locked_at, locked_by, processed_at, last_error,
          COALESCE(idempotency_key, ''), created_at, updated_at`,
		lockedAt, workerID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("cache_invalidation_outbox: claim_ready: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []service.CacheInvalidationEvent
	for rows.Next() {
		ev, err := scanOutboxRow(rows)
		if err != nil {
			return nil, fmt.Errorf("cache_invalidation_outbox: scan: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cache_invalidation_outbox: rows: %w", err)
	}
	return events, nil
}

// MarkSucceeded marks the row as "succeeded" and records processed_at.
func (r *cacheInvalidationOutboxRepository) MarkSucceeded(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE cache_invalidation_outbox
   SET status       = 'succeeded',
       processed_at = NOW(),
       updated_at   = NOW()
 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("cache_invalidation_outbox: mark_succeeded: %w", err)
	}
	return nil
}

// MarkFailed marks the row as "failed" and schedules the next retry.
func (r *cacheInvalidationOutboxRepository) MarkFailed(ctx context.Context, id int64, procErr error, nextAttemptAt time.Time) error {
	var lastError string
	if procErr != nil {
		lastError = procErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE cache_invalidation_outbox
   SET status         = 'failed',
       attempts       = attempts + 1,
       last_error     = $2,
       next_attempt_at = $3,
       locked_at      = NULL,
       locked_by      = NULL,
       updated_at     = NOW()
 WHERE id = $1`, id, lastError, nextAttemptAt)
	if err != nil {
		return fmt.Errorf("cache_invalidation_outbox: mark_failed: %w", err)
	}
	return nil
}

// MarkDead marks the row as "dead" (exceeded max_attempts).
func (r *cacheInvalidationOutboxRepository) MarkDead(ctx context.Context, id int64, procErr error) error {
	var lastError string
	if procErr != nil {
		lastError = procErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE cache_invalidation_outbox
   SET status     = 'dead',
       attempts   = attempts + 1,
       last_error = $2,
       locked_at  = NULL,
       locked_by  = NULL,
       updated_at = NOW()
 WHERE id = $1`, id, lastError)
	if err != nil {
		return fmt.Errorf("cache_invalidation_outbox: mark_dead: %w", err)
	}
	return nil
}

// RequeueStaleProcessing returns timed-out "processing" rows to "pending" so they can be
// retried.  olderThan should be NOW() - lockTimeout.
func (r *cacheInvalidationOutboxRepository) RequeueStaleProcessing(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
UPDATE cache_invalidation_outbox
   SET status     = 'pending',
       locked_at  = NULL,
       locked_by  = NULL,
       updated_at = NOW()
 WHERE status    = 'processing'
   AND locked_at <= $1`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("cache_invalidation_outbox: requeue_stale: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ── scan helper ─────────────────────────────────────────────────────────────

type outboxRowScanner interface {
	Scan(dest ...any) error
}

func scanOutboxRow(s outboxRowScanner) (service.CacheInvalidationEvent, error) {
	var (
		ev             service.CacheInvalidationEvent
		aggregateID    sql.NullInt64
		cacheTypes     pq.StringArray
		payloadRaw     []byte
		lockedAt       sql.NullTime
		lockedBy       sql.NullString
		processedAt    sql.NullTime
		lastError      sql.NullString
		idempotencyKey string
	)
	if err := s.Scan(
		&ev.ID, &ev.EventType, &ev.AggregateType, &aggregateID, &ev.Reason,
		&cacheTypes, &payloadRaw,
		&ev.Status, &ev.Attempts, &ev.MaxAttempts, &ev.NextAttemptAt,
		&lockedAt, &lockedBy, &processedAt, &lastError,
		&idempotencyKey, &ev.CreatedAt, &ev.UpdatedAt,
	); err != nil {
		return ev, err
	}
	if aggregateID.Valid {
		v := aggregateID.Int64
		ev.AggregateID = &v
	}
	ev.CacheTypes = []string(cacheTypes)
	if lockedAt.Valid {
		v := lockedAt.Time
		ev.LockedAt = &v
	}
	ev.LockedBy = lockedBy.String
	if processedAt.Valid {
		v := processedAt.Time
		ev.ProcessedAt = &v
	}
	ev.LastError = lastError.String
	ev.IdempotencyKey = idempotencyKey
	if len(payloadRaw) > 0 {
		if err := json.Unmarshal(payloadRaw, &ev.Payload); err != nil {
			return ev, fmt.Errorf("unmarshal payload: %w", err)
		}
	}
	return ev, nil
}
