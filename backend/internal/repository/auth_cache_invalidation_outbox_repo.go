package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type authCacheInvalidationOutboxRepository struct {
	db *sql.DB
}

func NewAuthCacheInvalidationOutboxRepository(db *sql.DB) service.AuthCacheInvalidationOutboxRepository {
	return &authCacheInvalidationOutboxRepository{db: db}
}

func (r *authCacheInvalidationOutboxRepository) Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]service.AuthCacheInvalidationEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil auth cache invalidation outbox database")
	}
	if limit <= 0 {
		limit = 100
	}
	leaseSeconds := int64(lease / time.Second)
	if leaseSeconds < 1 {
		leaseSeconds = 30
	}
	if IsMySQL() {
		return r.claimMySQL(ctx, workerID, limit, leaseSeconds)
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM auth_cache_invalidation_outbox
			WHERE available_at <= NOW()
			  AND (claimed_at IS NULL OR claimed_at < NOW() - ($3 * INTERVAL '1 second'))
			ORDER BY id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE auth_cache_invalidation_outbox AS o
		SET claimed_at = NOW(), claimed_by = $1
		FROM candidates AS c
		WHERE o.id = c.id
		RETURNING o.id, o.cache_key, o.attempts, o.delivery_stage, o.created_at
	`, workerID, limit, leaseSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]service.AuthCacheInvalidationEvent, 0, limit)
	for rows.Next() {
		var event service.AuthCacheInvalidationEvent
		if err := rows.Scan(&event.ID, &event.CacheKey, &event.Attempts, &event.Stage, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.CacheKey = strings.TrimSpace(event.CacheKey)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *authCacheInvalidationOutboxRepository) ScheduleSecondPass(ctx context.Context, id int64, workerID string, availableAt time.Time) error {
	if IsMySQL() {
		return r.scheduleSecondPassMySQL(ctx, id, workerID, availableAt)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE auth_cache_invalidation_outbox
		SET delivery_stage = 1,
			available_at = $3,
			last_error = NULL,
			claimed_at = NULL,
			claimed_by = NULL
		WHERE id = $1 AND claimed_by = $2 AND delivery_stage = 0
	`, id, workerID, availableAt)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d cannot schedule second pass", id)
	}
	return nil
}

func (r *authCacheInvalidationOutboxRepository) DeleteClaimed(ctx context.Context, id int64, workerID string) error {
	if IsMySQL() {
		return r.deleteClaimedMySQL(ctx, id, workerID)
	}
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM auth_cache_invalidation_outbox
		WHERE id = $1 AND claimed_by = $2
	`, id, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

func (r *authCacheInvalidationOutboxRepository) RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error {
	if IsMySQL() {
		return r.retryClaimedMySQL(ctx, id, workerID, availableAt, lastError)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE auth_cache_invalidation_outbox
		SET attempts = attempts + 1,
			available_at = $3,
			last_error = $4,
			claimed_at = NULL,
			claimed_by = NULL
		WHERE id = $1 AND claimed_by = $2
	`, id, workerID, availableAt, lastError)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

func (r *authCacheInvalidationOutboxRepository) Stats(ctx context.Context) (service.AuthCacheInvalidationOutboxStats, error) {
	var (
		stats     service.AuthCacheInvalidationOutboxStats
		oldest    sql.NullTime
		lastError sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(created_at), COALESCE(MAX(attempts), 0),
			(SELECT last_error
			 FROM auth_cache_invalidation_outbox
			 WHERE last_error IS NOT NULL
			 ORDER BY available_at DESC, id DESC
			 LIMIT 1)
		FROM auth_cache_invalidation_outbox
	`).Scan(&stats.Pending, &oldest, &stats.MaxAttempts, &lastError)
	if err != nil {
		return stats, err
	}
	if oldest.Valid {
		value := oldest.Time
		stats.OldestCreatedAt = &value
	}
	if lastError.Valid {
		stats.LastError = lastError.String
	}
	return stats, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。Stats 无占位符、语句双方言通用，无需分支。
// ---------------------------------------------------------------------------

// claimMySQL 是 Claim 的 MySQL 实现。PG 版为可写 CTE（candidates FOR UPDATE SKIP LOCKED
// + UPDATE..RETURNING）；MySQL 拆为同事务两步：先 SELECT FOR UPDATE SKIP LOCKED 取行集合，
// 再按主键 UPDATE 认领，返回先前读到的行。返回列（cache_key/attempts/delivery_stage/
// created_at）不被 UPDATE 修改，与 PG UPDATE..RETURNING（NEW 值）语义一致。
func (r *authCacheInvalidationOutboxRepository) claimMySQL(ctx context.Context, workerID string, limit int, leaseSeconds int64) ([]service.AuthCacheInvalidationEvent, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) ([]service.AuthCacheInvalidationEvent, error) {
		_ = tx.Rollback()
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, cache_key, attempts, delivery_stage, created_at
		FROM auth_cache_invalidation_outbox
		WHERE available_at <= NOW(3)
		  AND (claimed_at IS NULL OR claimed_at < NOW(3) - INTERVAL ? SECOND)
		ORDER BY id ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`, leaseSeconds, limit)
	if err != nil {
		return rollback(err)
	}

	events := make([]service.AuthCacheInvalidationEvent, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var event service.AuthCacheInvalidationEvent
		if err := rows.Scan(&event.ID, &event.CacheKey, &event.Attempts, &event.Stage, &event.CreatedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		event.CacheKey = strings.TrimSpace(event.CacheKey)
		events = append(events, event)
		ids = append(ids, event.ID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}

	if len(ids) > 0 {
		idArgs := make([]any, 0, len(ids)+1)
		idArgs = append(idArgs, workerID)
		for _, id := range ids {
			idArgs = append(idArgs, id)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE auth_cache_invalidation_outbox
			SET claimed_at = NOW(3), claimed_by = ?
			WHERE id IN (`+Placeholders(len(ids))+`)
		`, idArgs...); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

// scheduleSecondPassMySQL 是 ScheduleSecondPass 的 MySQL 实现（$n→?）。
func (r *authCacheInvalidationOutboxRepository) scheduleSecondPassMySQL(ctx context.Context, id int64, workerID string, availableAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE auth_cache_invalidation_outbox
		SET delivery_stage = 1,
			available_at = ?,
			last_error = NULL,
			claimed_at = NULL,
			claimed_by = NULL
		WHERE id = ? AND claimed_by = ? AND delivery_stage = 0
	`, id, workerID, availableAt)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d cannot schedule second pass", id)
	}
	return nil
}

// deleteClaimedMySQL 是 DeleteClaimed 的 MySQL 实现（$n→?）。
func (r *authCacheInvalidationOutboxRepository) deleteClaimedMySQL(ctx context.Context, id int64, workerID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM auth_cache_invalidation_outbox
		WHERE id = ? AND claimed_by = ?
	`, id, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

// retryClaimedMySQL 是 RetryClaimed 的 MySQL 实现（$n→?）。
func (r *authCacheInvalidationOutboxRepository) retryClaimedMySQL(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE auth_cache_invalidation_outbox
		SET attempts = attempts + 1,
			available_at = ?,
			last_error = ?,
			claimed_at = NULL,
			claimed_by = NULL
		WHERE id = ? AND claimed_by = ?
	`, id, workerID, availableAt, lastError)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("auth cache invalidation claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}
