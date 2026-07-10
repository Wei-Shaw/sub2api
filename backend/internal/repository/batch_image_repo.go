package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type batchImageSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type batchImageRepository struct {
	db  *sql.DB
	sql batchImageSQLExecutor
}

func NewBatchImageRepository(db *sql.DB) service.BatchImageRepository {
	return &batchImageRepository{db: db, sql: db}
}

func (r *batchImageRepository) CreateBatchImageJob(ctx context.Context, params service.CreateBatchImageJobParams) (*service.BatchImageJob, error) {
	if !service.IsSupportedBatchImageProvider(params.Provider) {
		return nil, service.ErrBatchImageInvalidProvider
	}
	if params.BatchID == "" {
		batchID, err := service.NewBatchImageID()
		if err != nil {
			return nil, err
		}
		params.BatchID = batchID
	}
	if params.Status == "" {
		params.Status = service.BatchImageJobStatusCreated
	}
	if params.Currency == "" {
		params.Currency = "USD"
	}

	job, err := createBatchImageJobWithSQL(ctx, r.sql, params)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrBatchImageJobExists)
	}
	return job, nil
}

func (r *batchImageRepository) GetBatchImageJobByBatchID(ctx context.Context, batchID string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE batch_id = $1", batchID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) GetBatchImageJobByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE user_id = $1 AND api_key_id = $2 AND idempotency_key = $3
 ORDER BY id DESC LIMIT 1`, userID, apiKeyID, key))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) GetBatchImageJobByBatchIDForOwner(ctx context.Context, userID, apiKeyID int64, batchID string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE batch_id = $1 AND user_id = $2 AND api_key_id = $3 AND user_deleted_at IS NULL`, batchID, userID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) ListBatchImageJobsForOwner(ctx context.Context, userID, apiKeyID int64, filter service.BatchImageJobFilter) ([]*service.BatchImageJob, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := batchImageJobSelectSQL + " WHERE user_id = $1 AND api_key_id = $2"
	args := []any{userID, apiKeyID}
	if filter.ExcludeDeleted {
		query += " AND user_deleted_at IS NULL"
	}
	if filter.Status != "" {
		query += " AND status = $" + strconv.Itoa(len(args)+1)
		args = append(args, filter.Status)
	}
	if filter.TaskNameLike != "" {
		query += " AND task_name ILIKE $" + strconv.Itoa(len(args)+1)
		args = append(args, "%"+filter.TaskNameLike+"%")
	}
	if filter.Downloaded != nil {
		if *filter.Downloaded {
			query += " AND downloaded_at IS NOT NULL"
		} else {
			query += " AND downloaded_at IS NULL"
		}
	}
	if filter.CreatedAfter != nil {
		query += " AND created_at >= $" + strconv.Itoa(len(args)+1)
		args = append(args, *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query += " AND created_at < $" + strconv.Itoa(len(args)+1)
		args = append(args, *filter.CreatedBefore)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	args = append(args, limit, filter.Offset)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

func (r *batchImageRepository) GetBatchImageJobByID(ctx context.Context, id int64) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE id = $1", id))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) TransitionBatchImageJobStatus(ctx context.Context, batchID, toStatus string, opts service.BatchImageTransitionOptions) error {
	if r.db == nil {
		return r.transitionBatchImageJobStatusWithSQL(ctx, r.sql, batchID, toStatus, opts)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.transitionBatchImageJobStatusWithSQL(ctx, tx, batchID, toStatus, opts); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *batchImageRepository) TouchBatchImageJobSubmitting(ctx context.Context, batchID string) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET updated_at = $2
WHERE batch_id = $1
  AND status IN ('created', 'uploading')`, batchID, time.Now())
	return err
}

func (r *batchImageRepository) FailStaleUnsubmittedBatchImageJob(ctx context.Context, batchID string, cutoff time.Time, code, message string) (bool, error) {
	now := time.Now()
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'failed',
    last_error_code = $2,
    last_error_message = $3,
    finished_at = CASE WHEN finished_at IS NULL THEN $4 ELSE finished_at END,
    updated_at = $4,
    version = version + 1
WHERE batch_id = $1
  AND status IN ('created', 'uploading')
  AND provider_job_name IS NULL
  AND updated_at <= $5`, batchID, code, message, now, cutoff)
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
	return true, appendBatchImageEventWithSQL(ctx, r.sql, batchID, "billing_hold_recovery_failed_unsubmitted", map[string]any{
		"batch_id":   batchID,
		"error_code": code,
	})
}

func (r *batchImageRepository) UpdateBatchImageJobProviderOutputRef(ctx context.Context, batchID, providerOutputRef string) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET provider_output_ref = $2, updated_at = $3
WHERE batch_id = $1`, batchID, providerOutputRef, time.Now())
	if err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return nil
}

func (r *batchImageRepository) UpdateBatchImageJobProviderSubmit(ctx context.Context, params service.UpdateBatchImageJobProviderSubmitParams) error {
	if r.db == nil {
		return r.updateBatchImageJobProviderSubmitWithSQL(ctx, r.sql, params)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := r.updateBatchImageJobProviderSubmitWithSQL(ctx, tx, params); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *batchImageRepository) updateBatchImageJobProviderSubmitWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.UpdateBatchImageJobProviderSubmitParams) error {
	var current string
	if err := sqlq.QueryRowContext(ctx, `SELECT status FROM batch_image_jobs WHERE batch_id = $1 FOR UPDATE`, params.BatchID).Scan(&current); err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	if !service.CanTransitionBatchImageJob(current, service.BatchImageJobStatusSubmitted) {
		return service.ErrBatchImageInvalidTransition
	}
	now := time.Now()
	if _, err := sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'submitted',
    provider_job_name = $2,
    provider_input_ref = NULLIF($3, ''),
    provider_output_ref = NULLIF($4, ''),
    gcs_input_uri = NULLIF($5, ''),
    gcs_output_uri = NULLIF($6, ''),
    submitted_at = CASE WHEN submitted_at IS NULL THEN $7 ELSE submitted_at END,
    updated_at = $7,
    version = version + 1
WHERE batch_id = $1`, params.BatchID, params.ProviderJobName, params.ProviderInputRef, params.ProviderOutputRef, params.GCSInputURI, params.GCSOutputURI, now); err != nil {
		return err
	}
	return appendBatchImageEventWithSQL(ctx, sqlq, params.BatchID, "provider_submitted", params.EventPayload)
}

func (r *batchImageRepository) RecordBatchImageJobSubmitFailure(ctx context.Context, batchID, code, message string, markFailed bool) error {
	now := time.Now()
	statusSQL := "status"
	if markFailed {
		statusSQL = "'failed'"
	}
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = `+statusSQL+`,
    last_error_code = $2,
    last_error_message = $3,
    finished_at = CASE WHEN `+statusSQL+` = 'failed' AND finished_at IS NULL THEN $4 ELSE finished_at END,
    updated_at = $4,
    version = version + 1
WHERE batch_id = $1`, batchID, code, message, now)
	if err != nil {
		return err
	}
	eventType := "submit_failed"
	if !markFailed {
		eventType = "queue_failed"
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, eventType, map[string]any{"error_code": code})
}

func (r *batchImageRepository) MarkBatchImageJobSettled(ctx context.Context, params service.MarkBatchImageJobSettledParams) error {
	if r.db == nil {
		return r.markBatchImageJobSettledWithSQL(ctx, r.sql, params)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.markBatchImageJobSettledWithSQL(ctx, tx, params); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *batchImageRepository) markBatchImageJobSettledWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.MarkBatchImageJobSettledParams) error {
	now := time.Now()
	if params.Now != nil {
		now = *params.Now
	}
	outputExpiresAt := params.OutputExpiresAt

	res, err := sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'completed',
    actual_cost = $2,
    manifest_hash = $3,
    settled_at = CASE WHEN settled_at IS NULL THEN $4 ELSE settled_at END,
    finished_at = CASE WHEN finished_at IS NULL THEN $4 ELSE finished_at END,
    output_expires_at = CASE WHEN output_expires_at IS NULL THEN $5 ELSE output_expires_at END,
    updated_at = $4,
    version = version + 1
WHERE batch_id = $1
  AND status = 'settling'
  AND (manifest_hash IS NULL OR manifest_hash = '' OR manifest_hash = $3)`, params.BatchID, params.ActualCost, params.ManifestHash, now, outputExpiresAt)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		job, getErr := scanBatchImageJob(sqlq.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE batch_id = $1", params.BatchID))
		if getErr != nil {
			return translatePersistenceError(getErr, service.ErrBatchImageJobNotFound, nil)
		}
		if job.Status != service.BatchImageJobStatusSettling {
			if job.Status == service.BatchImageJobStatusCompleted {
				return service.ErrBatchImageAlreadySettled
			}
			return service.ErrBatchImageSettlementInvalidStatus
		}
		return service.ErrBatchImageSettlementManifestConflict
	}
	return appendBatchImageEventWithSQL(ctx, sqlq, params.BatchID, "settlement_completed", params.EventPayload)
}

func (r *batchImageRepository) SetBatchImageJobSettlementFailed(ctx context.Context, batchID, code, message string) (int, error) {
	var retryCount int
	err := r.sql.QueryRowContext(ctx, `
UPDATE batch_image_jobs
SET last_error_code = $2,
    last_error_message = $3,
    retry_count = retry_count + 1,
    updated_at = $4
WHERE batch_id = $1
RETURNING retry_count`, batchID, code, message, time.Now()).Scan(&retryCount)
	if err != nil {
		return 0, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return retryCount, appendBatchImageEventWithSQL(ctx, r.sql, batchID, "settlement_failed", map[string]any{
		"error_code": code,
	})
}

func (r *batchImageRepository) transitionBatchImageJobStatusWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID, toStatus string, opts service.BatchImageTransitionOptions) error {
	var current string
	if err := sqlq.QueryRowContext(ctx, `SELECT status FROM batch_image_jobs WHERE batch_id = $1 FOR UPDATE`, batchID).Scan(&current); err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	if !service.CanTransitionBatchImageJob(current, toStatus) {
		return service.ErrBatchImageInvalidTransition
	}

	now := time.Now()
	if opts.Now != nil {
		now = *opts.Now
	}

	if _, err := sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET
    status = $2::varchar,
    version = version + 1,
    updated_at = $3,
    last_error_code = CASE WHEN $2::varchar = 'failed' THEN $4 ELSE last_error_code END,
    last_error_message = CASE WHEN $2::varchar = 'failed' THEN $5 ELSE last_error_message END,
    submitted_at = CASE WHEN $2::varchar = 'submitted' AND submitted_at IS NULL THEN $3 ELSE submitted_at END,
    started_at = CASE WHEN $2::varchar = 'running' AND started_at IS NULL THEN $3 ELSE started_at END,
    finished_at = CASE WHEN $2::varchar IN ('completed', 'failed', 'cancelled') AND finished_at IS NULL THEN $3 ELSE finished_at END,
    settled_at = CASE WHEN $2::varchar = 'completed' AND settled_at IS NULL THEN $3 ELSE settled_at END,
    output_deleted_at = CASE WHEN $2::varchar = 'output_deleted' AND output_deleted_at IS NULL THEN $3 ELSE output_deleted_at END
WHERE batch_id = $1`, batchID, toStatus, now, opts.ErrorCode, opts.ErrorMessage); err != nil {
		return err
	}

	if opts.EventType != "" {
		return appendBatchImageEventWithSQL(ctx, sqlq, batchID, opts.EventType, opts.EventPayload)
	}
	return nil
}

func (r *batchImageRepository) CreateBatchImageItem(ctx context.Context, params service.CreateBatchImageItemParams) (*service.BatchImageItem, error) {
	item, err := createBatchImageItemWithSQL(ctx, r.sql, params)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrBatchImageItemExists)
	}
	return item, nil
}

func (r *batchImageRepository) BulkCreateBatchImageItems(ctx context.Context, params []service.CreateBatchImageItemParams) error {
	if len(params) == 0 {
		return nil
	}
	if r.db == nil {
		for _, param := range params {
			if _, err := createBatchImageItemWithSQL(ctx, r.sql, param); err != nil {
				return translatePersistenceError(err, nil, service.ErrBatchImageItemExists)
			}
		}
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, param := range params {
		if _, err := createBatchImageItemWithSQL(ctx, tx, param); err != nil {
			return translatePersistenceError(err, nil, service.ErrBatchImageItemExists)
		}
	}
	return tx.Commit()
}

func (r *batchImageRepository) ReplaceBatchImageItemsForJob(ctx context.Context, batchID string, items []service.CreateBatchImageItemParams, counts service.BatchImageCounts) error {
	if r.db == nil {
		return r.replaceBatchImageItemsForJobWithSQL(ctx, r.sql, batchID, items, counts)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.replaceBatchImageItemsForJobWithSQL(ctx, tx, batchID, items, counts); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *batchImageRepository) replaceBatchImageItemsForJobWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID string, items []service.CreateBatchImageItemParams, counts service.BatchImageCounts) error {
	var id int64
	var status string
	if err := sqlq.QueryRowContext(ctx, `SELECT id, status FROM batch_image_jobs WHERE batch_id = $1 FOR UPDATE`, batchID).Scan(&id, &status); err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	// 仅允许 indexing 状态重建 item 表：防止锁过期后掉队的 worker
	// 重写已完成/已结算 job 的条目，造成账目与结果漂移。
	if status != service.BatchImageJobStatusIndexing {
		return service.ErrBatchImageIndexStateConflict
	}
	promptPreviews, err := r.batchImageItemPromptPreviews(ctx, sqlq, batchID)
	if err != nil {
		return err
	}
	if _, err := sqlq.ExecContext(ctx, `DELETE FROM batch_image_items WHERE job_id = $1`, batchID); err != nil {
		return err
	}
	for _, item := range items {
		item.JobID = batchID
		if item.PromptPreview == nil {
			if preview := promptPreviews[item.CustomID]; preview != "" {
				item.PromptPreview = &preview
			}
		}
		if _, err := createBatchImageItemWithSQL(ctx, sqlq, item); err != nil {
			return translatePersistenceError(err, nil, service.ErrBatchImageItemExists)
		}
	}
	_, err = sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET success_count = $2,
    fail_count = $3,
    updated_at = $4
WHERE batch_id = $1`, batchID, counts.SuccessCount, counts.FailCount, time.Now())
	return err
}

func (r *batchImageRepository) batchImageItemPromptPreviews(ctx context.Context, sqlq batchImageSQLExecutor, batchID string) (map[string]string, error) {
	rows, err := sqlq.QueryContext(ctx, `SELECT custom_id, prompt_preview FROM batch_image_items WHERE job_id = $1 AND prompt_preview IS NOT NULL`, batchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var customID string
		var preview sql.NullString
		if err := rows.Scan(&customID, &preview); err != nil {
			return nil, err
		}
		if preview.Valid && preview.String != "" {
			out[customID] = preview.String
		}
	}
	return out, rows.Err()
}

func (r *batchImageRepository) ListBatchImageItems(ctx context.Context, batchID string, filter service.BatchImageItemFilter) ([]*service.BatchImageItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := batchImageItemSelectSQL + " WHERE job_id = $1"
	args := []any{batchID}
	if filter.Status != "" {
		query += " AND status = $2"
		args = append(args, filter.Status)
	}
	query += " ORDER BY id ASC LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	args = append(args, limit, filter.Offset)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []*service.BatchImageItem
	for rows.Next() {
		item, err := scanBatchImageItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *batchImageRepository) ListBatchImageItemsForOwner(ctx context.Context, userID, apiKeyID int64, batchID string, filter service.BatchImageItemFilter) ([]*service.BatchImageItem, error) {
	if _, err := r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID); err != nil {
		return nil, err
	}
	return r.ListBatchImageItems(ctx, batchID, filter)
}

func (r *batchImageRepository) GetBatchImageJobForDownload(ctx context.Context, userID, apiKeyID int64, batchID string) (*service.BatchImageJob, error) {
	return r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID)
}

func (r *batchImageRepository) GetBatchImageItemForDownload(ctx context.Context, batchID, customID string) (*service.BatchImageItem, error) {
	item, err := scanBatchImageItem(r.sql.QueryRowContext(ctx, batchImageItemSelectSQL+`
 WHERE job_id = $1 AND custom_id = $2`, batchID, customID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageItemNotFound, nil)
	}
	return item, nil
}

func (r *batchImageRepository) ListBatchImageItemsForDownload(ctx context.Context, batchID string, status string, limit int) ([]*service.BatchImageItem, error) {
	return r.ListBatchImageItems(ctx, batchID, service.BatchImageItemFilter{Status: status, Limit: limit})
}

func (r *batchImageRepository) ListBatchImageJobsDueForInputCleanup(ctx context.Context, cutoff time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE input_deleted_at IS NULL
   AND provider_input_ref IS NOT NULL
   AND status IN ('completed', 'failed', 'cancelled', 'output_deleted')
   AND COALESCE(finished_at, settled_at, updated_at, created_at) <= $1
 ORDER BY id ASC
 LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

func (r *batchImageRepository) ListBatchImageJobsDueForOutputCleanup(ctx context.Context, now time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE output_deleted_at IS NULL
   AND provider_output_ref IS NOT NULL
   AND status = 'completed'
   AND output_expires_at IS NOT NULL
   AND output_expires_at <= $1
 ORDER BY output_expires_at ASC, id ASC
 LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

func (r *batchImageRepository) ListStaleUnsubmittedBatchImageJobs(ctx context.Context, cutoff time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE status IN ('created', 'uploading')
   AND provider_job_name IS NULL
   AND COALESCE(hold_amount, estimated_cost, 0) > 0
   AND updated_at <= $1
 ORDER BY updated_at ASC, id ASC
 LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

func (r *batchImageRepository) MarkBatchImageInputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET input_deleted_at = CASE WHEN input_deleted_at IS NULL THEN $2 ELSE input_deleted_at END,
    updated_at = $2,
    version = version + 1
WHERE batch_id = $1`, batchID, deletedAt)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "input_cleanup_completed", map[string]any{
		"batch_id":       batchID,
		"cleanup_target": "input",
		"deleted_at":     deletedAt.UTC().Format(time.RFC3339),
	})
}

func (r *batchImageRepository) MarkBatchImageOutputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = CASE WHEN status = 'completed' THEN 'output_deleted' ELSE status END,
    output_deleted_at = CASE WHEN output_deleted_at IS NULL THEN $2 ELSE output_deleted_at END,
    finished_at = CASE WHEN status = 'completed' AND finished_at IS NULL THEN $2 ELSE finished_at END,
    updated_at = $2,
    version = version + 1
WHERE batch_id = $1`, batchID, deletedAt)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "output_cleanup_completed", map[string]any{
		"batch_id":       batchID,
		"cleanup_target": "output",
		"deleted_at":     deletedAt.UTC().Format(time.RFC3339),
	})
}

func (r *batchImageRepository) MarkBatchImageDownloaded(ctx context.Context, batchID string, downloadedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET downloaded_at = CASE WHEN downloaded_at IS NULL THEN $2 ELSE downloaded_at END,
    updated_at = $2
WHERE batch_id = $1`, batchID, downloadedAt)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "download_completed", map[string]any{
		"batch_id":      batchID,
		"downloaded_at": downloadedAt.UTC().Format(time.RFC3339),
	})
}

func (r *batchImageRepository) MarkBatchImageJobUserDeleted(ctx context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET user_deleted_at = CASE WHEN user_deleted_at IS NULL THEN $4 ELSE user_deleted_at END,
    updated_at = $4
WHERE batch_id = $1
  AND user_id = $2
  AND api_key_id = $3
  AND user_deleted_at IS NULL
  AND status IN ('completed', 'failed', 'cancelled', 'output_deleted')`, batchID, userID, apiKeyID, deletedAt)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageRecordDeleteNotReady
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "user_record_deleted", map[string]any{
		"batch_id":   batchID,
		"deleted_at": deletedAt.UTC().Format(time.RFC3339),
		"user_id":    userID,
		"api_key_id": apiKeyID,
	})
}

func (r *batchImageRepository) SetBatchImageOutputExpiresAt(ctx context.Context, batchID string, expiresAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET output_expires_at = CASE WHEN output_expires_at IS NULL THEN $2 ELSE output_expires_at END,
    updated_at = $3
WHERE batch_id = $1`, batchID, expiresAt, time.Now())
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return nil
}

func (r *batchImageRepository) RecordBatchImageCleanupFailure(ctx context.Context, batchID, code, message string) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET last_error_code = $2,
    last_error_message = $3,
    retry_count = retry_count + 1,
    updated_at = $4
WHERE batch_id = $1`, batchID, code, message, time.Now())
	if err != nil {
		return err
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "output_cleanup_failed", map[string]any{"error_code": code})
}

func (r *batchImageRepository) AppendBatchImageEvent(ctx context.Context, batchID, eventType string, payload any) error {
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, eventType, payload)
}

func createBatchImageJobWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.CreateBatchImageJobParams) (*service.BatchImageJob, error) {
	return scanBatchImageJob(sqlq.QueryRowContext(ctx, `
INSERT INTO batch_image_jobs (
    batch_id, user_id, api_key_id, account_id, provider, model, task_name, parent_batch_id, status,
    provider_job_name, provider_input_ref, provider_output_ref, gcs_input_uri, gcs_output_uri,
    item_count, success_count, fail_count, cancelled_count,
    estimated_cost, hold_amount, actual_cost,
    base_unit_price, group_rate_multiplier, account_rate_multiplier,
    batch_discount_multiplier, hold_multiplier, billable_unit_price, hold_unit_price,
    pricing_snapshot_version,
    currency, hold_id,
    idempotency_key, request_hash, manifest_hash, retry_count, output_expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21,
    $22, $23, $24,
    $25, $26, $27, $28,
    $29,
    $30, $31,
    $32, $33, $34, $35, $36
)
RETURNING `+batchImageJobColumns,
		params.BatchID, params.UserID, params.APIKeyID, params.AccountID, params.Provider, params.Model, params.TaskName, params.ParentBatchID, params.Status,
		params.ProviderJobName, params.ProviderInputRef, params.ProviderOutputRef, params.GCSInputURI, params.GCSOutputURI,
		params.ItemCount, params.SuccessCount, params.FailCount, params.CancelledCount,
		params.EstimatedCost, params.HoldAmount, params.ActualCost,
		params.BaseUnitPrice, params.GroupRateMultiplier, params.AccountRateMultiplier,
		params.BatchDiscountMultiplier, params.HoldMultiplier, params.BillableUnitPrice, params.HoldUnitPrice,
		params.PricingSnapshotVersion,
		params.Currency, params.HoldID,
		params.IdempotencyKey, params.RequestHash, params.ManifestHash, params.RetryCount, params.OutputExpiresAt,
	))
}

func createBatchImageItemWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.CreateBatchImageItemParams) (*service.BatchImageItem, error) {
	return scanBatchImageItem(sqlq.QueryRowContext(ctx, `
INSERT INTO batch_image_items (
    job_id, custom_id, status, request_hash, prompt_preview, provider_source_object,
    source_line_number, source_byte_offset, source_byte_length,
    mime_type, file_extension, image_count,
    error_code, error_message, billed_amount, indexed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12,
    $13, $14, $15, $16
)
RETURNING `+batchImageItemColumns,
		params.JobID, params.CustomID, params.Status, params.RequestHash, params.PromptPreview, params.ProviderSourceObject,
		params.SourceLineNumber, params.SourceByteOffset, params.SourceByteLength,
		params.MimeType, params.FileExtension, params.ImageCount,
		params.ErrorCode, params.ErrorMessage, params.BilledAmount, params.IndexedAt,
	))
}

func appendBatchImageEventWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID, eventType string, payload any) error {
	var payloadArg any
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		payloadArg = string(payloadBytes)
	}
	_, err := sqlq.ExecContext(ctx, `
INSERT INTO batch_image_events (job_id, event_type, payload)
VALUES ($1, $2, $3)`, batchID, eventType, payloadArg)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

const batchImageJobColumns = `
id, batch_id, user_id, api_key_id, account_id, provider, model, task_name, parent_batch_id, status,
provider_job_name, provider_input_ref, provider_output_ref, gcs_input_uri, gcs_output_uri,
item_count, success_count, fail_count, cancelled_count,
estimated_cost, hold_amount, actual_cost,
base_unit_price, group_rate_multiplier, account_rate_multiplier,
batch_discount_multiplier, hold_multiplier, billable_unit_price, hold_unit_price,
pricing_snapshot_version,
currency, hold_id,
idempotency_key, request_hash, manifest_hash,
retry_count, version, output_expires_at, input_deleted_at, output_deleted_at, downloaded_at, user_deleted_at,
last_error_code, last_error_message,
created_at, updated_at, submitted_at, started_at, finished_at, settled_at`

const batchImageJobSelectSQL = `SELECT ` + batchImageJobColumns + ` FROM batch_image_jobs`

func scanBatchImageJob(row rowScanner) (*service.BatchImageJob, error) {
	var job service.BatchImageJob
	var apiKeyID, accountID sql.NullInt64
	var providerJobName, providerInputRef, providerOutputRef, gcsInputURI, gcsOutputURI sql.NullString
	var parentBatchID sql.NullString
	var holdAmount, actualCost sql.NullFloat64
	var holdID, idempotencyKey, requestHash, manifestHash sql.NullString
	var outputExpiresAt, inputDeletedAt, outputDeletedAt, downloadedAt, userDeletedAt sql.NullTime
	var lastErrorCode, lastErrorMessage sql.NullString
	var submittedAt, startedAt, finishedAt, settledAt sql.NullTime

	err := row.Scan(
		&job.ID, &job.BatchID, &job.UserID, &apiKeyID, &accountID, &job.Provider, &job.Model, &job.TaskName, &parentBatchID, &job.Status,
		&providerJobName, &providerInputRef, &providerOutputRef, &gcsInputURI, &gcsOutputURI,
		&job.ItemCount, &job.SuccessCount, &job.FailCount, &job.CancelledCount,
		&job.EstimatedCost, &holdAmount, &actualCost,
		&job.BaseUnitPrice, &job.GroupRateMultiplier, &job.AccountRateMultiplier,
		&job.BatchDiscountMultiplier, &job.HoldMultiplier, &job.BillableUnitPrice, &job.HoldUnitPrice,
		&job.PricingSnapshotVersion,
		&job.Currency, &holdID,
		&idempotencyKey, &requestHash, &manifestHash,
		&job.RetryCount, &job.Version, &outputExpiresAt, &inputDeletedAt, &outputDeletedAt, &downloadedAt, &userDeletedAt,
		&lastErrorCode, &lastErrorMessage,
		&job.CreatedAt, &job.UpdatedAt, &submittedAt, &startedAt, &finishedAt, &settledAt,
	)
	if err != nil {
		return nil, err
	}

	job.APIKeyID = batchImageNullInt64Ptr(apiKeyID)
	job.AccountID = batchImageNullInt64Ptr(accountID)
	job.ProviderJobName = batchImageNullStringPtr(providerJobName)
	job.ProviderInputRef = batchImageNullStringPtr(providerInputRef)
	job.ProviderOutputRef = batchImageNullStringPtr(providerOutputRef)
	job.ParentBatchID = batchImageNullStringPtr(parentBatchID)
	job.GCSInputURI = batchImageNullStringPtr(gcsInputURI)
	job.GCSOutputURI = batchImageNullStringPtr(gcsOutputURI)
	job.HoldAmount = batchImageNullFloat64Ptr(holdAmount)
	job.ActualCost = batchImageNullFloat64Ptr(actualCost)
	job.HoldID = batchImageNullStringPtr(holdID)
	job.IdempotencyKey = batchImageNullStringPtr(idempotencyKey)
	job.RequestHash = batchImageNullStringPtr(requestHash)
	job.ManifestHash = batchImageNullStringPtr(manifestHash)
	job.OutputExpiresAt = batchImageNullTimePtr(outputExpiresAt)
	job.InputDeletedAt = batchImageNullTimePtr(inputDeletedAt)
	job.OutputDeletedAt = batchImageNullTimePtr(outputDeletedAt)
	job.DownloadedAt = batchImageNullTimePtr(downloadedAt)
	job.UserDeletedAt = batchImageNullTimePtr(userDeletedAt)
	job.LastErrorCode = batchImageNullStringPtr(lastErrorCode)
	job.LastErrorMessage = batchImageNullStringPtr(lastErrorMessage)
	job.SubmittedAt = batchImageNullTimePtr(submittedAt)
	job.StartedAt = batchImageNullTimePtr(startedAt)
	job.FinishedAt = batchImageNullTimePtr(finishedAt)
	job.SettledAt = batchImageNullTimePtr(settledAt)
	return &job, nil
}

func scanBatchImageJobs(rows *sql.Rows) ([]*service.BatchImageJob, error) {
	var jobs []*service.BatchImageJob
	for rows.Next() {
		job, err := scanBatchImageJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

const batchImageItemColumns = `
id, job_id, custom_id, status, request_hash, prompt_preview, provider_source_object,
source_line_number, source_byte_offset, source_byte_length,
mime_type, file_extension, image_count,
error_code, error_message, billed_amount,
created_at, indexed_at`

const batchImageItemSelectSQL = `SELECT ` + batchImageItemColumns + ` FROM batch_image_items`

func scanBatchImageItem(row rowScanner) (*service.BatchImageItem, error) {
	var item service.BatchImageItem
	var requestHash, promptPreview, providerSourceObject sql.NullString
	var sourceLineNumber sql.NullInt64
	var sourceByteOffset, sourceByteLength sql.NullInt64
	var mimeType, fileExtension, errorCode, errorMessage sql.NullString
	var billedAmount sql.NullFloat64
	var indexedAt sql.NullTime

	err := row.Scan(
		&item.ID, &item.JobID, &item.CustomID, &item.Status, &requestHash, &promptPreview, &providerSourceObject,
		&sourceLineNumber, &sourceByteOffset, &sourceByteLength,
		&mimeType, &fileExtension, &item.ImageCount,
		&errorCode, &errorMessage, &billedAmount,
		&item.CreatedAt, &indexedAt,
	)
	if err != nil {
		return nil, err
	}

	item.RequestHash = batchImageNullStringPtr(requestHash)
	item.PromptPreview = batchImageNullStringPtr(promptPreview)
	item.ProviderSourceObject = batchImageNullStringPtr(providerSourceObject)
	item.SourceLineNumber = batchImageNullIntPtr(sourceLineNumber)
	item.SourceByteOffset = batchImageNullInt64Ptr(sourceByteOffset)
	item.SourceByteLength = batchImageNullInt64Ptr(sourceByteLength)
	item.MimeType = batchImageNullStringPtr(mimeType)
	item.FileExtension = batchImageNullStringPtr(fileExtension)
	item.ErrorCode = batchImageNullStringPtr(errorCode)
	item.ErrorMessage = batchImageNullStringPtr(errorMessage)
	item.BilledAmount = batchImageNullFloat64Ptr(billedAmount)
	item.IndexedAt = batchImageNullTimePtr(indexedAt)
	return &item, nil
}

func batchImageNullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func batchImageNullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func batchImageNullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

func batchImageNullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func batchImageNullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

var _ service.BatchImageRepository = (*batchImageRepository)(nil)
