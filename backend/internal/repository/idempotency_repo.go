package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type idempotencyRepository struct {
	sql sqlExecutor
}

func NewIdempotencyRepository(_ *dbent.Client, sqlDB *sql.DB) service.IdempotencyRepository {
	return &idempotencyRepository{sql: sqlDB}
}

func (r *idempotencyRepository) CreateProcessing(ctx context.Context, record *service.IdempotencyRecord) (bool, error) {
	if record == nil {
		return false, nil
	}
	if IsMySQL() {
		return r.createProcessingMySQL(ctx, record)
	}
	query := `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, locked_until, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (scope, idempotency_key_hash) DO NOTHING
		RETURNING id, created_at, updated_at
	`
	var createdAt time.Time
	var updatedAt time.Time
	err := scanSingleRow(ctx, r.sql, query, []any{
		record.Scope,
		record.IdempotencyKeyHash,
		record.RequestFingerprint,
		record.Status,
		record.LockedUntil,
		record.ExpiresAt,
	}, &record.ID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return true, nil
}

// createProcessingMySQL 是 CreateProcessing 的 MySQL 实现。
// PG 版 ON CONFLICT (scope, idempotency_key_hash) DO NOTHING RETURNING id, created_at,
// updated_at 拆为 ODKU no-op（幂等判定：RowsAffected 1=新插入、0=冲突）+ LastInsertId +
// 回查 created_at/updated_at（两列由数据库生成，应用侧不可知，对齐 PG 的 RETURNING）。
func (r *idempotencyRepository) createProcessingMySQL(ctx context.Context, record *service.IdempotencyRecord) (bool, error) {
	query := `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, locked_until, expires_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE idempotency_records.id = idempotency_records.id
	`
	res, err := r.sql.ExecContext(ctx, query,
		record.Scope,
		record.IdempotencyKeyHash,
		record.RequestFingerprint,
		record.Status,
		record.LockedUntil,
		record.ExpiresAt,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		return false, err
	}
	record.ID = id
	// created_at/updated_at 为数据库默认值（CURRENT_TIMESTAMP(3)），插入后回查补齐。
	if err := scanSingleRow(ctx, r.sql,
		"SELECT created_at, updated_at FROM idempotency_records WHERE id = ?",
		[]any{id}, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return false, err
	}
	return true, nil
}

func (r *idempotencyRepository) GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	query := `
		SELECT
			id, scope, idempotency_key_hash, request_fingerprint, status, response_status,
			response_body, error_reason, locked_until, expires_at, created_at, updated_at
		FROM idempotency_records
		WHERE scope = $1 AND idempotency_key_hash = $2
	`
	if IsMySQL() {
		query = `
		SELECT
			id, scope, idempotency_key_hash, request_fingerprint, status, response_status,
			response_body, error_reason, locked_until, expires_at, created_at, updated_at
		FROM idempotency_records
		WHERE scope = ? AND idempotency_key_hash = ?
		`
	}
	record := &service.IdempotencyRecord{}
	var responseStatus sql.NullInt64
	var responseBody sql.NullString
	var errorReason sql.NullString
	var lockedUntil sql.NullTime
	err := scanSingleRow(ctx, r.sql, query, []any{scope, keyHash},
		&record.ID,
		&record.Scope,
		&record.IdempotencyKeyHash,
		&record.RequestFingerprint,
		&record.Status,
		&responseStatus,
		&responseBody,
		&errorReason,
		&lockedUntil,
		&record.ExpiresAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if responseStatus.Valid {
		v := int(responseStatus.Int64)
		record.ResponseStatus = &v
	}
	if responseBody.Valid {
		v := responseBody.String
		record.ResponseBody = &v
	}
	if errorReason.Valid {
		v := errorReason.String
		record.ErrorReason = &v
	}
	if lockedUntil.Valid {
		v := lockedUntil.Time
		record.LockedUntil = &v
	}
	return record, nil
}

func (r *idempotencyRepository) TryReclaim(
	ctx context.Context,
	id int64,
	fromStatus string,
	now, newLockedUntil, newExpiresAt time.Time,
) (bool, error) {
	query := `
		UPDATE idempotency_records
		SET status = $2,
			locked_until = $3,
			error_reason = NULL,
			updated_at = NOW(),
			expires_at = $4
		WHERE id = $1
			AND status = $5
			AND (locked_until IS NULL OR locked_until <= $6)
	`
	if IsMySQL() {
		query = `
		UPDATE idempotency_records
		SET status = ?,
			locked_until = ?,
			error_reason = NULL,
			updated_at = NOW(3),
			expires_at = ?
		WHERE id = ?
			AND status = ?
			AND (locked_until IS NULL OR locked_until <= ?)
		`
	}
	// PG 占位符 $n 显式编号，参数按语义排列；MySQL ? 按出现顺序绑定，
	// SET 子句在 WHERE 前，需按 (status, locked_until, expires_at, id, from_status, now) 重排。
	args := []any{
		id,
		service.IdempotencyStatusProcessing,
		newLockedUntil,
		newExpiresAt,
		fromStatus,
		now,
	}
	if IsMySQL() {
		args = []any{
			service.IdempotencyStatusProcessing,
			newLockedUntil,
			newExpiresAt,
			id,
			fromStatus,
			now,
		}
	}
	res, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *idempotencyRepository) ExtendProcessingLock(
	ctx context.Context,
	id int64,
	requestFingerprint string,
	newLockedUntil,
	newExpiresAt time.Time,
) (bool, error) {
	query := `
		UPDATE idempotency_records
		SET locked_until = $2,
			expires_at = $3,
			updated_at = NOW()
		WHERE id = $1
			AND status = $4
			AND request_fingerprint = $5
	`
	if IsMySQL() {
		query = `
		UPDATE idempotency_records
		SET locked_until = ?,
			expires_at = ?,
			updated_at = NOW(3)
		WHERE id = ?
			AND status = ?
			AND request_fingerprint = ?
		`
	}
	// MySQL ? 按出现顺序绑定：SET 在 WHERE 前，重排为 (locked_until, expires_at, id, status, request_fingerprint)。
	args := []any{
		id,
		newLockedUntil,
		newExpiresAt,
		service.IdempotencyStatusProcessing,
		requestFingerprint,
	}
	if IsMySQL() {
		args = []any{
			newLockedUntil,
			newExpiresAt,
			id,
			service.IdempotencyStatusProcessing,
			requestFingerprint,
		}
	}
	res, err := r.sql.ExecContext(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *idempotencyRepository) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = $3,
			response_body = $4,
			error_reason = NULL,
			locked_until = NULL,
			expires_at = $5,
			updated_at = NOW()
		WHERE id = $1
	`
	if IsMySQL() {
		query = `
		UPDATE idempotency_records
		SET status = ?,
			response_status = ?,
			response_body = ?,
			error_reason = NULL,
			locked_until = NULL,
			expires_at = ?,
			updated_at = NOW(3)
		WHERE id = ?
		`
	}
	// MySQL ? 按出现顺序绑定：SET 在 WHERE 前，重排为 (status, response_status, response_body, expires_at, id)。
	args := []any{
		id,
		service.IdempotencyStatusSucceeded,
		responseStatus,
		responseBody,
		expiresAt,
	}
	if IsMySQL() {
		args = []any{
			service.IdempotencyStatusSucceeded,
			responseStatus,
			responseBody,
			expiresAt,
			id,
		}
	}
	_, err := r.sql.ExecContext(ctx, query, args...)
	return err
}

func (r *idempotencyRepository) MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2,
			error_reason = $3,
			locked_until = $4,
			expires_at = $5,
			updated_at = NOW()
		WHERE id = $1
	`
	if IsMySQL() {
		query = `
		UPDATE idempotency_records
		SET status = ?,
			error_reason = ?,
			locked_until = ?,
			expires_at = ?,
			updated_at = NOW(3)
		WHERE id = ?
		`
	}
	// MySQL ? 按出现顺序绑定：SET 在 WHERE 前，重排为 (status, error_reason, locked_until, expires_at, id)。
	args := []any{
		id,
		service.IdempotencyStatusFailedRetryable,
		errorReason,
		lockedUntil,
		expiresAt,
	}
	if IsMySQL() {
		args = []any{
			service.IdempotencyStatusFailedRetryable,
			errorReason,
			lockedUntil,
			expiresAt,
			id,
		}
	}
	_, err := r.sql.ExecContext(ctx, query, args...)
	return err
}

func (r *idempotencyRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	if IsMySQL() {
		return r.deleteExpiredMySQL(ctx, now, limit)
	}
	query := `
		WITH victims AS (
			SELECT id
			FROM idempotency_records
			WHERE expires_at <= $1
			ORDER BY expires_at ASC
			LIMIT $2
		)
		DELETE FROM idempotency_records
		WHERE id IN (SELECT id FROM victims)
	`
	res, err := r.sql.ExecContext(ctx, query, now, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteExpiredMySQL 是 DeleteExpired 的 MySQL 实现：可写/带 LIMIT 的 CTE DELETE 拆为
// 「派生表选 victim + JOIN 删除」。MySQL 不支持 IN 子查询内直接 LIMIT，也不允许在
// 同一张表的 DELETE 子查询里直接引用目标表，经派生表物化后 JOIN 删除即可。
func (r *idempotencyRepository) deleteExpiredMySQL(ctx context.Context, now time.Time, limit int) (int64, error) {
	res, err := r.sql.ExecContext(ctx, `
		DELETE r FROM idempotency_records r
		JOIN (
			SELECT id
			FROM idempotency_records
			WHERE expires_at <= ?
			ORDER BY expires_at ASC
			LIMIT ?
		) victims ON victims.id = r.id
	`, now, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
