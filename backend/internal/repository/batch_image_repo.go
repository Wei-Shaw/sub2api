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
	if IsMySQL() {
		return r.getBatchImageJobByBatchIDMySQL(ctx, batchID)
	}
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE batch_id = $1", batchID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) GetBatchImageJobByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*service.BatchImageJob, error) {
	if IsMySQL() {
		return r.getBatchImageJobByIdempotencyKeyMySQL(ctx, userID, apiKeyID, key)
	}
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE user_id = $1 AND api_key_id = $2 AND idempotency_key = $3
 ORDER BY id DESC LIMIT 1`, userID, apiKeyID, key))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) GetBatchImageJobByBatchIDForOwner(ctx context.Context, userID, apiKeyID int64, batchID string) (*service.BatchImageJob, error) {
	if IsMySQL() {
		return r.getBatchImageJobByBatchIDForOwnerMySQL(ctx, userID, apiKeyID, batchID)
	}
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE batch_id = $1 AND user_id = $2 AND api_key_id = $3 AND user_deleted_at IS NULL`, batchID, userID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

func (r *batchImageRepository) ListBatchImageJobsForOwner(ctx context.Context, userID, apiKeyID int64, filter service.BatchImageJobFilter) ([]*service.BatchImageJob, error) {
	if IsMySQL() {
		return r.listBatchImageJobsForOwnerMySQL(ctx, userID, apiKeyID, filter)
	}
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
	if IsMySQL() {
		return r.getBatchImageJobByIDMySQL(ctx, id)
	}
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
	if IsMySQL() {
		return r.touchBatchImageJobSubmittingMySQL(ctx, batchID)
	}
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET updated_at = $2
WHERE batch_id = $1
  AND status IN ('created', 'uploading')`, batchID, time.Now())
	return err
}

func (r *batchImageRepository) FailStaleUnsubmittedBatchImageJob(ctx context.Context, batchID string, cutoff time.Time, code, message string) (bool, error) {
	if IsMySQL() {
		return r.failStaleUnsubmittedBatchImageJobMySQL(ctx, batchID, cutoff, code, message)
	}
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
	if IsMySQL() {
		return r.updateBatchImageJobProviderOutputRefMySQL(ctx, batchID, providerOutputRef)
	}
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
	if IsMySQL() {
		return r.updateBatchImageJobProviderSubmitWithSQLMySQL(ctx, sqlq, params)
	}
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
	if IsMySQL() {
		return r.recordBatchImageJobSubmitFailureMySQL(ctx, batchID, code, message, markFailed)
	}
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
	if IsMySQL() {
		return r.markBatchImageJobSettledWithSQLMySQL(ctx, sqlq, params)
	}
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
	if IsMySQL() {
		return r.setBatchImageJobSettlementFailedMySQL(ctx, batchID, code, message)
	}
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
	if IsMySQL() {
		return r.transitionBatchImageJobStatusWithSQLMySQL(ctx, sqlq, batchID, toStatus, opts)
	}
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
	if IsMySQL() {
		return r.replaceBatchImageItemsForJobWithSQLMySQL(ctx, sqlq, batchID, items, counts)
	}
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
	if IsMySQL() {
		return batchImageItemPromptPreviewsMySQL(ctx, sqlq, batchID)
	}
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
	if IsMySQL() {
		return r.listBatchImageItemsMySQL(ctx, batchID, filter)
	}
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
	if IsMySQL() {
		return r.getBatchImageItemForDownloadMySQL(ctx, batchID, customID)
	}
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
	if IsMySQL() {
		return r.listBatchImageJobsDueForInputCleanupMySQL(ctx, cutoff, limit)
	}
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
	if IsMySQL() {
		return r.listBatchImageJobsDueForOutputCleanupMySQL(ctx, now, limit)
	}
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
	if IsMySQL() {
		return r.listStaleUnsubmittedBatchImageJobsMySQL(ctx, cutoff, limit)
	}
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
	if IsMySQL() {
		return r.markBatchImageInputDeletedMySQL(ctx, batchID, deletedAt)
	}
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
	if IsMySQL() {
		return r.markBatchImageOutputDeletedMySQL(ctx, batchID, deletedAt)
	}
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
	if IsMySQL() {
		return r.markBatchImageDownloadedMySQL(ctx, batchID, downloadedAt)
	}
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
	if IsMySQL() {
		return r.markBatchImageJobUserDeletedMySQL(ctx, userID, apiKeyID, batchID, deletedAt)
	}
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
	if IsMySQL() {
		return r.setBatchImageOutputExpiresAtMySQL(ctx, batchID, expiresAt)
	}
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
	if IsMySQL() {
		return r.recordBatchImageCleanupFailureMySQL(ctx, batchID, code, message)
	}
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
	if IsMySQL() {
		return createBatchImageJobMySQL(ctx, sqlq, params)
	}
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
    idempotency_key, request_hash, manifest_hash, retry_count, session_id, output_expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21,
    $22, $23, $24,
    $25, $26, $27, $28,
    $29,
    $30, $31,
    $32, $33, $34, $35, $36, $37
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
		params.IdempotencyKey, params.RequestHash, params.ManifestHash, params.RetryCount, params.SessionID, params.OutputExpiresAt,
	))
}

func createBatchImageItemWithSQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.CreateBatchImageItemParams) (*service.BatchImageItem, error) {
	if IsMySQL() {
		return createBatchImageItemMySQL(ctx, sqlq, params)
	}
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
	if IsMySQL() {
		return appendBatchImageEventMySQL(ctx, sqlq, batchID, eventType, payload)
	}
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
retry_count, version, session_id, output_expires_at, input_deleted_at, output_deleted_at, downloaded_at, user_deleted_at,
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
	var sessionID sql.NullString
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
		&job.RetryCount, &job.Version, &sessionID, &outputExpiresAt, &inputDeletedAt, &outputDeletedAt, &downloadedAt, &userDeletedAt,
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
	job.SessionID = batchImageNullStringPtr(sessionID)
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。PG 分支行为与字节保持原样。
// ---------------------------------------------------------------------------

// batchImageJobExistsMySQL 按 batch_id 回查 job 是否存在。
// 用于 MySQL RowsAffected（=值变化行）与 PG RowsAffected（=匹配行）语义差异的兜底判定。
func (r *batchImageRepository) batchImageJobExistsMySQL(ctx context.Context, batchID string) (bool, error) {
	var exists bool
	if err := r.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM batch_image_jobs WHERE batch_id = ?)`, batchID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// scanBatchImageJobByIDMySQL 在 sqlAtomicScope 上按主键回查单个 job 行。
func scanBatchImageJobByIDMySQL(ctx context.Context, scope *sqlAtomicScope, id int64) (*service.BatchImageJob, error) {
	rows, err := scope.query(ctx, batchImageJobSelectSQL+" WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	job, err := scanBatchImageJob(rows)
	if err != nil {
		return nil, err
	}
	return job, rows.Err()
}

// scanBatchImageItemByIDMySQL 在 sqlAtomicScope 上按主键回查单个 item 行。
func scanBatchImageItemByIDMySQL(ctx context.Context, scope *sqlAtomicScope, id int64) (*service.BatchImageItem, error) {
	rows, err := scope.query(ctx, batchImageItemSelectSQL+" WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	item, err := scanBatchImageItem(rows)
	if err != nil {
		return nil, err
	}
	return item, rows.Err()
}

// createBatchImageJobMySQL 是 createBatchImageJobWithSQL 的 MySQL 实现：
// PG 版 INSERT..RETURNING <全列> → 同一事务内 INSERT + LastInsertId + 按主键回查全列，
// 回查取回数据库生成的默认值（id/created_at/updated_at/version 等）。
// 唯一冲突由 MySQL 1062 经 translatePersistenceError 翻译，与 PG 23505 同路。
func createBatchImageJobMySQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.CreateBatchImageJobParams) (*service.BatchImageJob, error) {
	scope, err := beginSQLAtomicScope(ctx, nil, sqlq)
	if err != nil {
		return nil, err
	}
	defer scope.rollback()

	res, err := scope.exec(ctx, `
INSERT INTO batch_image_jobs (
    batch_id, user_id, api_key_id, account_id, provider, model, task_name, parent_batch_id, status,
    provider_job_name, provider_input_ref, provider_output_ref, gcs_input_uri, gcs_output_uri,
    item_count, success_count, fail_count, cancelled_count,
    estimated_cost, hold_amount, actual_cost,
    base_unit_price, group_rate_multiplier, account_rate_multiplier,
    batch_discount_multiplier, hold_multiplier, billable_unit_price, hold_unit_price,
    pricing_snapshot_version,
    currency, hold_id,
    idempotency_key, request_hash, manifest_hash, retry_count, session_id, output_expires_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?, ?,
    ?,
    ?, ?,
    ?, ?, ?, ?, ?, ?
)`,
		params.BatchID, params.UserID, params.APIKeyID, params.AccountID, params.Provider, params.Model, params.TaskName, params.ParentBatchID, params.Status,
		params.ProviderJobName, params.ProviderInputRef, params.ProviderOutputRef, params.GCSInputURI, params.GCSOutputURI,
		params.ItemCount, params.SuccessCount, params.FailCount, params.CancelledCount,
		params.EstimatedCost, params.HoldAmount, params.ActualCost,
		params.BaseUnitPrice, params.GroupRateMultiplier, params.AccountRateMultiplier,
		params.BatchDiscountMultiplier, params.HoldMultiplier, params.BillableUnitPrice, params.HoldUnitPrice,
		params.PricingSnapshotVersion,
		params.Currency, params.HoldID,
		params.IdempotencyKey, params.RequestHash, params.ManifestHash, params.RetryCount, params.SessionID, params.OutputExpiresAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	job, err := scanBatchImageJobByIDMySQL(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	if err := scope.commit(); err != nil {
		return nil, err
	}
	return job, nil
}

// createBatchImageItemMySQL 是 createBatchImageItemWithSQL 的 MySQL 实现：
// PG 版 INSERT..RETURNING <全列> → 同一事务内 INSERT + LastInsertId + 按主键回查全列。
func createBatchImageItemMySQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.CreateBatchImageItemParams) (*service.BatchImageItem, error) {
	scope, err := beginSQLAtomicScope(ctx, nil, sqlq)
	if err != nil {
		return nil, err
	}
	defer scope.rollback()

	res, err := scope.exec(ctx, `
INSERT INTO batch_image_items (
    job_id, custom_id, status, request_hash, prompt_preview, provider_source_object,
    source_line_number, source_byte_offset, source_byte_length,
    mime_type, file_extension, image_count,
    error_code, error_message, billed_amount, indexed_at
) VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?, ?
)`,
		params.JobID, params.CustomID, params.Status, params.RequestHash, params.PromptPreview, params.ProviderSourceObject,
		params.SourceLineNumber, params.SourceByteOffset, params.SourceByteLength,
		params.MimeType, params.FileExtension, params.ImageCount,
		params.ErrorCode, params.ErrorMessage, params.BilledAmount, params.IndexedAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	item, err := scanBatchImageItemByIDMySQL(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	if err := scope.commit(); err != nil {
		return nil, err
	}
	return item, nil
}

// appendBatchImageEventMySQL 是 appendBatchImageEventWithSQL 的 MySQL 实现（$n→?）。
func appendBatchImageEventMySQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID, eventType string, payload any) error {
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
VALUES (?, ?, ?)`, batchID, eventType, payloadArg)
	return err
}

// getBatchImageJobByBatchIDMySQL 是 GetBatchImageJobByBatchID 的 MySQL 实现（$1→?）。
func (r *batchImageRepository) getBatchImageJobByBatchIDMySQL(ctx context.Context, batchID string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE batch_id = ?", batchID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

// getBatchImageJobByIdempotencyKeyMySQL 是 GetBatchImageJobByIdempotencyKey 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) getBatchImageJobByIdempotencyKeyMySQL(ctx context.Context, userID, apiKeyID int64, key string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE user_id = ? AND api_key_id = ? AND idempotency_key = ?
 ORDER BY id DESC LIMIT 1`, userID, apiKeyID, key))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

// getBatchImageJobByBatchIDForOwnerMySQL 是 GetBatchImageJobByBatchIDForOwner 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) getBatchImageJobByBatchIDForOwnerMySQL(ctx context.Context, userID, apiKeyID int64, batchID string) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+`
 WHERE batch_id = ? AND user_id = ? AND api_key_id = ? AND user_deleted_at IS NULL`, batchID, userID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

// getBatchImageJobByIDMySQL 是 GetBatchImageJobByID 的 MySQL 实现（$1→?）。
func (r *batchImageRepository) getBatchImageJobByIDMySQL(ctx context.Context, id int64) (*service.BatchImageJob, error) {
	job, err := scanBatchImageJob(r.sql.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE id = ?", id))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	return job, nil
}

// listBatchImageJobsForOwnerMySQL 是 ListBatchImageJobsForOwner 的 MySQL 实现：
// $n→?；ILIKE→LIKE（utf8mb4_0900_ai_ci 天然大小写不敏感）。
func (r *batchImageRepository) listBatchImageJobsForOwnerMySQL(ctx context.Context, userID, apiKeyID int64, filter service.BatchImageJobFilter) ([]*service.BatchImageJob, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := batchImageJobSelectSQL + " WHERE user_id = ? AND api_key_id = ?"
	args := []any{userID, apiKeyID}
	if filter.ExcludeDeleted {
		query += " AND user_deleted_at IS NULL"
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.TaskNameLike != "" {
		query += " AND task_name LIKE ?"
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
		query += " AND created_at >= ?"
		args = append(args, *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query += " AND created_at < ?"
		args = append(args, *filter.CreatedBefore)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, filter.Offset)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

// touchBatchImageJobSubmittingMySQL 是 TouchBatchImageJobSubmitting 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) touchBatchImageJobSubmittingMySQL(ctx context.Context, batchID string) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET updated_at = ?
WHERE batch_id = ?
  AND status IN ('created', 'uploading')`, time.Now(), batchID)
	return err
}

// failStaleUnsubmittedBatchImageJobMySQL 是 FailStaleUnsubmittedBatchImageJob 的 MySQL 实现：
// $n→?（参数按占位符出现顺序展开，重复引用的 $4 展开两次）。
// WHERE 命中的行必然发生值变化（version+1、status 翻转），RowsAffected 语义与 PG 一致。
func (r *batchImageRepository) failStaleUnsubmittedBatchImageJobMySQL(ctx context.Context, batchID string, cutoff time.Time, code, message string) (bool, error) {
	now := time.Now()
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'failed',
    last_error_code = ?,
    last_error_message = ?,
    finished_at = CASE WHEN finished_at IS NULL THEN ? ELSE finished_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?
  AND status IN ('created', 'uploading')
  AND provider_job_name IS NULL
  AND updated_at <= ?`, code, message, now, now, batchID, cutoff)
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

// updateBatchImageJobProviderOutputRefMySQL 是 UpdateBatchImageJobProviderOutputRef 的 MySQL 实现。
// REVIEW: MySQL RowsAffected 统计"值变化的行"而 PG 统计"匹配的行"——行存在但新值与
// 旧值完全相同（同毫秒内重复提交同一 ref）时 MySQL 返回 0 而 PG 返回 1。
// affected==0 时回查存在性对齐 PG 行为（存在 → 成功；不存在 → ErrBatchImageJobNotFound）。
func (r *batchImageRepository) updateBatchImageJobProviderOutputRefMySQL(ctx context.Context, batchID, providerOutputRef string) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET provider_output_ref = ?, updated_at = ?
WHERE batch_id = ?`, providerOutputRef, time.Now(), batchID)
	if err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		exists, existsErr := r.batchImageJobExistsMySQL(ctx, batchID)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return service.ErrBatchImageJobNotFound
		}
	}
	return nil
}

// updateBatchImageJobProviderSubmitWithSQLMySQL 是 updateBatchImageJobProviderSubmitWithSQL
// 的 MySQL 实现：FOR UPDATE 锁定读（MySQL 8 支持）+ UPDATE（$n→?，重复参数按出现顺序展开）。
func (r *batchImageRepository) updateBatchImageJobProviderSubmitWithSQLMySQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.UpdateBatchImageJobProviderSubmitParams) error {
	var current string
	if err := sqlq.QueryRowContext(ctx, `SELECT status FROM batch_image_jobs WHERE batch_id = ? FOR UPDATE`, params.BatchID).Scan(&current); err != nil {
		return translatePersistenceError(err, service.ErrBatchImageJobNotFound, nil)
	}
	if !service.CanTransitionBatchImageJob(current, service.BatchImageJobStatusSubmitted) {
		return service.ErrBatchImageInvalidTransition
	}
	now := time.Now()
	if _, err := sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'submitted',
    provider_job_name = ?,
    provider_input_ref = NULLIF(?, ''),
    provider_output_ref = NULLIF(?, ''),
    gcs_input_uri = NULLIF(?, ''),
    gcs_output_uri = NULLIF(?, ''),
    submitted_at = CASE WHEN submitted_at IS NULL THEN ? ELSE submitted_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?`, params.ProviderJobName, params.ProviderInputRef, params.ProviderOutputRef, params.GCSInputURI, params.GCSOutputURI, now, now, params.BatchID); err != nil {
		return err
	}
	return appendBatchImageEventWithSQL(ctx, sqlq, params.BatchID, "provider_submitted", params.EventPayload)
}

// recordBatchImageJobSubmitFailureMySQL 是 RecordBatchImageJobSubmitFailure 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) recordBatchImageJobSubmitFailureMySQL(ctx context.Context, batchID, code, message string, markFailed bool) error {
	now := time.Now()
	statusSQL := "status"
	if markFailed {
		statusSQL = "'failed'"
	}
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = `+statusSQL+`,
    last_error_code = ?,
    last_error_message = ?,
    finished_at = CASE WHEN `+statusSQL+` = 'failed' AND finished_at IS NULL THEN ? ELSE finished_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?`, code, message, now, now, batchID)
	if err != nil {
		return err
	}
	eventType := "submit_failed"
	if !markFailed {
		eventType = "queue_failed"
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, eventType, map[string]any{"error_code": code})
}

// markBatchImageJobSettledWithSQLMySQL 是 markBatchImageJobSettledWithSQL 的 MySQL 实现（$n→?）。
// WHERE 命中的行必然发生值变化（version+1、settling→completed），RowsAffected 语义与 PG 一致。
func (r *batchImageRepository) markBatchImageJobSettledWithSQLMySQL(ctx context.Context, sqlq batchImageSQLExecutor, params service.MarkBatchImageJobSettledParams) error {
	now := time.Now()
	if params.Now != nil {
		now = *params.Now
	}
	outputExpiresAt := params.OutputExpiresAt

	res, err := sqlq.ExecContext(ctx, `
UPDATE batch_image_jobs
SET status = 'completed',
    actual_cost = ?,
    manifest_hash = ?,
    settled_at = CASE WHEN settled_at IS NULL THEN ? ELSE settled_at END,
    finished_at = CASE WHEN finished_at IS NULL THEN ? ELSE finished_at END,
    output_expires_at = CASE WHEN output_expires_at IS NULL THEN ? ELSE output_expires_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?
  AND status = 'settling'
  AND (manifest_hash IS NULL OR manifest_hash = '' OR manifest_hash = ?)`,
		params.ActualCost, params.ManifestHash, now, now, outputExpiresAt, now, params.BatchID, params.ManifestHash)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		job, getErr := scanBatchImageJob(sqlq.QueryRowContext(ctx, batchImageJobSelectSQL+" WHERE batch_id = ?", params.BatchID))
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

// setBatchImageJobSettlementFailedMySQL 是 SetBatchImageJobSettlementFailed 的 MySQL 实现：
// PG 版 UPDATE..RETURNING retry_count（返回更新后的值）→ 单事务内
// FOR UPDATE 读旧值 → UPDATE → 返回旧值+1。事件追加与 PG 版一致在 UPDATE 生效后独立执行。
func (r *batchImageRepository) setBatchImageJobSettlementFailedMySQL(ctx context.Context, batchID, code, message string) (int, error) {
	scope, err := beginSQLAtomicScope(ctx, nil, r.sql)
	if err != nil {
		return 0, err
	}
	defer scope.rollback()

	rows, err := scope.query(ctx, `SELECT retry_count FROM batch_image_jobs WHERE batch_id = ? FOR UPDATE`, batchID)
	if err != nil {
		return 0, err
	}
	var prevRetryCount int
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return 0, rowsErr
		}
		_ = rows.Close()
		return 0, service.ErrBatchImageJobNotFound
	}
	if err := rows.Scan(&prevRetryCount); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if _, err := scope.exec(ctx, `
UPDATE batch_image_jobs
SET last_error_code = ?,
    last_error_message = ?,
    retry_count = retry_count + 1,
    updated_at = ?
WHERE batch_id = ?`, code, message, time.Now(), batchID); err != nil {
		return 0, err
	}
	if err := scope.commit(); err != nil {
		return 0, err
	}
	return prevRetryCount + 1, appendBatchImageEventWithSQL(ctx, r.sql, batchID, "settlement_failed", map[string]any{
		"error_code": code,
	})
}

// transitionBatchImageJobStatusWithSQLMySQL 是 transitionBatchImageJobStatusWithSQL 的 MySQL 实现：
// FOR UPDATE 锁定读 + UPDATE（去掉 ::varchar 强转，$n→?，重复参数按出现顺序展开）。
func (r *batchImageRepository) transitionBatchImageJobStatusWithSQLMySQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID, toStatus string, opts service.BatchImageTransitionOptions) error {
	var current string
	if err := sqlq.QueryRowContext(ctx, `SELECT status FROM batch_image_jobs WHERE batch_id = ? FOR UPDATE`, batchID).Scan(&current); err != nil {
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
    status = ?,
    version = version + 1,
    updated_at = ?,
    last_error_code = CASE WHEN ? = 'failed' THEN ? ELSE last_error_code END,
    last_error_message = CASE WHEN ? = 'failed' THEN ? ELSE last_error_message END,
    submitted_at = CASE WHEN ? = 'submitted' AND submitted_at IS NULL THEN ? ELSE submitted_at END,
    started_at = CASE WHEN ? = 'running' AND started_at IS NULL THEN ? ELSE started_at END,
    finished_at = CASE WHEN ? IN ('completed', 'failed', 'cancelled') AND finished_at IS NULL THEN ? ELSE finished_at END,
    settled_at = CASE WHEN ? = 'completed' AND settled_at IS NULL THEN ? ELSE settled_at END,
    output_deleted_at = CASE WHEN ? = 'output_deleted' AND output_deleted_at IS NULL THEN ? ELSE output_deleted_at END
WHERE batch_id = ?`,
		toStatus, now,
		toStatus, opts.ErrorCode,
		toStatus, opts.ErrorMessage,
		toStatus, now,
		toStatus, now,
		toStatus, now,
		toStatus, now,
		toStatus, now,
		batchID); err != nil {
		return err
	}

	if opts.EventType != "" {
		return appendBatchImageEventWithSQL(ctx, sqlq, batchID, opts.EventType, opts.EventPayload)
	}
	return nil
}

// replaceBatchImageItemsForJobWithSQLMySQL 是 replaceBatchImageItemsForJobWithSQL 的 MySQL 实现：
// FOR UPDATE 锁定读 + DELETE/UPDATE（$n→?）。item 的重建走 createBatchImageItemWithSQL 的
// MySQL 分支（内部自带 ? 占位符与回查）。
func (r *batchImageRepository) replaceBatchImageItemsForJobWithSQLMySQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID string, items []service.CreateBatchImageItemParams, counts service.BatchImageCounts) error {
	var id int64
	var status string
	if err := sqlq.QueryRowContext(ctx, `SELECT id, status FROM batch_image_jobs WHERE batch_id = ? FOR UPDATE`, batchID).Scan(&id, &status); err != nil {
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
	if _, err := sqlq.ExecContext(ctx, `DELETE FROM batch_image_items WHERE job_id = ?`, batchID); err != nil {
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
SET success_count = ?,
    fail_count = ?,
    updated_at = ?
WHERE batch_id = ?`, counts.SuccessCount, counts.FailCount, time.Now(), batchID)
	return err
}

// batchImageItemPromptPreviewsMySQL 是 batchImageItemPromptPreviews 的 MySQL 实现（$1→?）。
func batchImageItemPromptPreviewsMySQL(ctx context.Context, sqlq batchImageSQLExecutor, batchID string) (map[string]string, error) {
	rows, err := sqlq.QueryContext(ctx, `SELECT custom_id, prompt_preview FROM batch_image_items WHERE job_id = ? AND prompt_preview IS NOT NULL`, batchID)
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

// listBatchImageItemsMySQL 是 ListBatchImageItems 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) listBatchImageItemsMySQL(ctx context.Context, batchID string, filter service.BatchImageItemFilter) ([]*service.BatchImageItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := batchImageItemSelectSQL + " WHERE job_id = ?"
	args := []any{batchID}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	query += " ORDER BY id ASC LIMIT ? OFFSET ?"
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

// getBatchImageItemForDownloadMySQL 是 GetBatchImageItemForDownload 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) getBatchImageItemForDownloadMySQL(ctx context.Context, batchID, customID string) (*service.BatchImageItem, error) {
	item, err := scanBatchImageItem(r.sql.QueryRowContext(ctx, batchImageItemSelectSQL+`
 WHERE job_id = ? AND custom_id = ?`, batchID, customID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBatchImageItemNotFound, nil)
	}
	return item, nil
}

// listBatchImageJobsDueForInputCleanupMySQL 是 ListBatchImageJobsDueForInputCleanup 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) listBatchImageJobsDueForInputCleanupMySQL(ctx context.Context, cutoff time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE input_deleted_at IS NULL
   AND provider_input_ref IS NOT NULL
   AND status IN ('completed', 'failed', 'cancelled', 'output_deleted')
   AND COALESCE(finished_at, settled_at, updated_at, created_at) <= ?
 ORDER BY id ASC
 LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

// listBatchImageJobsDueForOutputCleanupMySQL 是 ListBatchImageJobsDueForOutputCleanup 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) listBatchImageJobsDueForOutputCleanupMySQL(ctx context.Context, now time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE output_deleted_at IS NULL
   AND provider_output_ref IS NOT NULL
   AND status = 'completed'
   AND output_expires_at IS NOT NULL
   AND output_expires_at <= ?
 ORDER BY output_expires_at ASC, id ASC
 LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

// listStaleUnsubmittedBatchImageJobsMySQL 是 ListStaleUnsubmittedBatchImageJobs 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) listStaleUnsubmittedBatchImageJobsMySQL(ctx context.Context, cutoff time.Time, limit int) ([]*service.BatchImageJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, batchImageJobSelectSQL+`
 WHERE status IN ('created', 'uploading')
   AND provider_job_name IS NULL
   AND COALESCE(hold_amount, estimated_cost, 0) > 0
   AND updated_at <= ?
 ORDER BY updated_at ASC, id ASC
 LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatchImageJobs(rows)
}

// markBatchImageInputDeletedMySQL 是 MarkBatchImageInputDeleted 的 MySQL 实现（$n→?）。
// WHERE 命中的行必然发生值变化（version+1），RowsAffected 语义与 PG 一致。
func (r *batchImageRepository) markBatchImageInputDeletedMySQL(ctx context.Context, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET input_deleted_at = CASE WHEN input_deleted_at IS NULL THEN ? ELSE input_deleted_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?`, deletedAt, deletedAt, batchID)
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

// markBatchImageOutputDeletedMySQL 是 MarkBatchImageOutputDeleted 的 MySQL 实现（$n→?）。
// WHERE 命中的行必然发生值变化（version+1），RowsAffected 语义与 PG 一致。
// REVIEW: PG 的 SET 表达式一律读旧行值；MySQL 单表 UPDATE 的赋值从左到右求值，
// 后面的表达式读到的是本语句已更新的列值。故 finished_at 的 CASE（引用 status）
// 必须排在 status 重写之前，否则 status 已变 'output_deleted'，finished_at 永不回填。
func (r *batchImageRepository) markBatchImageOutputDeletedMySQL(ctx context.Context, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET finished_at = CASE WHEN status = 'completed' AND finished_at IS NULL THEN ? ELSE finished_at END,
    status = CASE WHEN status = 'completed' THEN 'output_deleted' ELSE status END,
    output_deleted_at = CASE WHEN output_deleted_at IS NULL THEN ? ELSE output_deleted_at END,
    updated_at = ?,
    version = version + 1
WHERE batch_id = ?`, deletedAt, deletedAt, deletedAt, batchID)
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

// markBatchImageDownloadedMySQL 是 MarkBatchImageDownloaded 的 MySQL 实现。
// REVIEW: 同 markBatchImageInputDeletedMySQL——幂等 CASE 写入在重复调用时
// MySQL RowsAffected=0 而 PG=1；回查存在性对齐 PG。
func (r *batchImageRepository) markBatchImageDownloadedMySQL(ctx context.Context, batchID string, downloadedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET downloaded_at = CASE WHEN downloaded_at IS NULL THEN ? ELSE downloaded_at END,
    updated_at = ?
WHERE batch_id = ?`, downloadedAt, downloadedAt, batchID)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		exists, existsErr := r.batchImageJobExistsMySQL(ctx, batchID)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return service.ErrBatchImageJobNotFound
		}
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "download_completed", map[string]any{
		"batch_id":      batchID,
		"downloaded_at": downloadedAt.UTC().Format(time.RFC3339),
	})
}

// markBatchImageJobUserDeletedMySQL 是 MarkBatchImageJobUserDeleted 的 MySQL 实现（$n→?）。
// WHERE user_deleted_at IS NULL 保证命中行必然发生值变化（NULL→值），RowsAffected 语义与 PG 一致。
func (r *batchImageRepository) markBatchImageJobUserDeletedMySQL(ctx context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET user_deleted_at = CASE WHEN user_deleted_at IS NULL THEN ? ELSE user_deleted_at END,
    updated_at = ?
WHERE batch_id = ?
  AND user_id = ?
  AND api_key_id = ?
  AND user_deleted_at IS NULL
  AND status IN ('completed', 'failed', 'cancelled', 'output_deleted')`, deletedAt, deletedAt, batchID, userID, apiKeyID)
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

// setBatchImageOutputExpiresAtMySQL 是 SetBatchImageOutputExpiresAt 的 MySQL 实现。
// REVIEW: 幂等 CASE 写入在 output_expires_at 已设置且同毫秒重复调用时
// MySQL RowsAffected=0 而 PG=1；回查存在性对齐 PG。
func (r *batchImageRepository) setBatchImageOutputExpiresAtMySQL(ctx context.Context, batchID string, expiresAt time.Time) error {
	res, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET output_expires_at = CASE WHEN output_expires_at IS NULL THEN ? ELSE output_expires_at END,
    updated_at = ?
WHERE batch_id = ?`, expiresAt, time.Now(), batchID)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		exists, existsErr := r.batchImageJobExistsMySQL(ctx, batchID)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return service.ErrBatchImageJobNotFound
		}
	}
	return nil
}

// recordBatchImageCleanupFailureMySQL 是 RecordBatchImageCleanupFailure 的 MySQL 实现（$n→?）。
func (r *batchImageRepository) recordBatchImageCleanupFailureMySQL(ctx context.Context, batchID, code, message string) error {
	_, err := r.sql.ExecContext(ctx, `
UPDATE batch_image_jobs
SET last_error_code = ?,
    last_error_message = ?,
    retry_count = retry_count + 1,
    updated_at = ?
WHERE batch_id = ?`, code, message, time.Now(), batchID)
	if err != nil {
		return err
	}
	return appendBatchImageEventWithSQL(ctx, r.sql, batchID, "output_cleanup_failed", map[string]any{"error_code": code})
}
