package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type imageStudioRepository struct {
	db *sql.DB
}

func NewImageStudioRepository(db *sql.DB) service.ImageStudioRepository {
	return &imageStudioRepository{db: db}
}

const imageStudioJobColumns = `id, user_id, api_key_id, status, kind, model, prompt, params,
	COALESCE(error_message, ''), COALESCE(warning, ''), image_count, duration_ms, created_at, started_at, completed_at`

const imageStudioAssetColumns = `id, job_id, user_id, storage_key, mime_type, bytes, COALESCE(revised_prompt, ''), created_at`

type imageStudioRowScanner interface {
	Scan(dest ...any) error
}

func scanImageStudioJob(row imageStudioRowScanner) (*service.ImageStudioJob, error) {
	var job service.ImageStudioJob
	var params []byte
	var duration sql.NullInt64
	var startedAt, completedAt sql.NullTime
	if err := row.Scan(&job.ID, &job.UserID, &job.APIKeyID, &job.Status, &job.Kind, &job.Model, &job.Prompt, &params,
		&job.ErrorMessage, &job.Warning, &job.ImageCount, &duration, &job.CreatedAt, &startedAt, &completedAt); err != nil {
		return nil, err
	}
	job.Params = json.RawMessage(params)
	if duration.Valid {
		job.DurationMs = &duration.Int64
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return &job, nil
}

func scanImageStudioAsset(row imageStudioRowScanner) (*service.ImageStudioAsset, error) {
	var asset service.ImageStudioAsset
	if err := row.Scan(&asset.ID, &asset.JobID, &asset.UserID, &asset.StorageKey, &asset.MimeType, &asset.Bytes, &asset.RevisedPrompt, &asset.CreatedAt); err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *imageStudioRepository) CreateJob(ctx context.Context, job *service.ImageStudioJob) error {
	return r.db.QueryRowContext(ctx, `INSERT INTO image_studio_jobs (user_id, api_key_id, status, kind, model, prompt, params)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		job.UserID, job.APIKeyID, job.Status, job.Kind, job.Model, job.Prompt, []byte(job.Params),
	).Scan(&job.ID, &job.CreatedAt)
}

// CreateJobWithinLimit 在按用户加的事务级咨询锁内计数并插入，并发提交也无法越过上限。
func (r *imageStudioRepository) CreateJobWithinLimit(ctx context.Context, job *service.ImageStudioJob, limit int) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryLockHash(fmt.Sprintf("image_studio_submit:%d", job.UserID))); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_studio_jobs WHERE user_id = $1 AND status IN ('queued', 'running')`, job.UserID).Scan(&count); err != nil {
		return err
	}
	if count >= limit {
		err = service.ErrImageStudioBusy
		return err
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO image_studio_jobs (user_id, api_key_id, status, kind, model, prompt, params)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		job.UserID, job.APIKeyID, job.Status, job.Kind, job.Model, job.Prompt, []byte(job.Params),
	).Scan(&job.ID, &job.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *imageStudioRepository) FailStaleJobs(ctx context.Context, userID int64, before time.Time, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE image_studio_jobs SET status = 'failed', error_message = $3, completed_at = NOW()
		WHERE user_id = $1 AND status IN ('queued', 'running') AND created_at < $2`, userID, before, message)
	return err
}

func (r *imageStudioRepository) CountUnfinishedJobs(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_studio_jobs WHERE user_id = $1 AND status IN ('queued', 'running')`, userID).Scan(&count)
	return count, err
}

func (r *imageStudioRepository) MarkJobRunning(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE image_studio_jobs SET status = 'running', started_at = NOW() WHERE id = $1 AND status = 'queued'`, id)
	return err
}

func (r *imageStudioRepository) FinishJob(ctx context.Context, job *service.ImageStudioJob, assets []service.ImageStudioAsset) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `UPDATE image_studio_jobs SET status = $2, error_message = NULLIF($3, ''), warning = NULLIF($4, ''),
		image_count = $5, duration_ms = $6, completed_at = NOW() WHERE id = $1`,
		job.ID, job.Status, job.ErrorMessage, job.Warning, job.ImageCount, job.DurationMs); err != nil {
		return err
	}
	for _, asset := range assets {
		if _, err = tx.ExecContext(ctx, `INSERT INTO image_studio_assets (job_id, user_id, storage_key, mime_type, bytes, revised_prompt)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))`,
			asset.JobID, asset.UserID, asset.StorageKey, asset.MimeType, asset.Bytes, asset.RevisedPrompt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *imageStudioRepository) GetJob(ctx context.Context, userID, id int64) (*service.ImageStudioJob, error) {
	job, err := scanImageStudioJob(r.db.QueryRowContext(ctx, `SELECT `+imageStudioJobColumns+` FROM image_studio_jobs WHERE id = $1 AND user_id = $2`, id, userID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrImageStudioNotFound, nil)
	}
	if err := r.attachAssets(ctx, []*service.ImageStudioJob{job}); err != nil {
		return nil, err
	}
	return job, nil
}

func (r *imageStudioRepository) ListJobs(ctx context.Context, userID int64, page, pageSize int) ([]service.ImageStudioJob, int64, error) {
	limit, offset := imageStudioPage(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_studio_jobs WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+imageStudioJobColumns+` FROM image_studio_jobs WHERE user_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]*service.ImageStudioJob, 0, limit)
	for rows.Next() {
		job, err := scanImageStudioJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := r.attachAssets(ctx, jobs); err != nil {
		return nil, 0, err
	}
	out := make([]service.ImageStudioJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, *job)
	}
	return out, total, nil
}

func (r *imageStudioRepository) attachAssets(ctx context.Context, jobs []*service.ImageStudioJob) error {
	if len(jobs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(jobs))
	byID := make(map[int64]*service.ImageStudioJob, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
		byID[job.ID] = job
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+imageStudioAssetColumns+` FROM image_studio_assets WHERE job_id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		asset, err := scanImageStudioAsset(rows)
		if err != nil {
			return err
		}
		if job := byID[asset.JobID]; job != nil {
			job.Assets = append(job.Assets, *asset)
		}
	}
	return rows.Err()
}

func (r *imageStudioRepository) DeleteJob(ctx context.Context, userID, id int64) (keys []string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM image_studio_jobs WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID).Scan(&status); err != nil {
		err = translatePersistenceError(err, service.ErrImageStudioNotFound, nil)
		return nil, err
	}
	if status == service.ImageStudioStatusQueued || status == service.ImageStudioStatusRunning {
		err = service.ErrImageStudioJobRunning
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT storage_key FROM image_studio_assets WHERE job_id = $1`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	_ = rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM image_studio_jobs WHERE id = $1`, id); err != nil {
		return nil, err
	}
	return keys, tx.Commit()
}

func (r *imageStudioRepository) ListAssets(ctx context.Context, userID int64, page, pageSize int) ([]service.ImageStudioAsset, int64, error) {
	limit, offset := imageStudioPage(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_studio_assets WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+imageStudioAssetColumns+` FROM image_studio_assets WHERE user_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	assets := make([]service.ImageStudioAsset, 0, limit)
	for rows.Next() {
		asset, err := scanImageStudioAsset(rows)
		if err != nil {
			return nil, 0, err
		}
		assets = append(assets, *asset)
	}
	return assets, total, rows.Err()
}

func (r *imageStudioRepository) GetAsset(ctx context.Context, userID, id int64) (*service.ImageStudioAsset, error) {
	asset, err := scanImageStudioAsset(r.db.QueryRowContext(ctx, `SELECT `+imageStudioAssetColumns+` FROM image_studio_assets WHERE id = $1 AND user_id = $2`, id, userID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrImageStudioNotFound, nil)
	}
	return asset, nil
}

func (r *imageStudioRepository) DeleteAsset(ctx context.Context, userID, id int64) (string, error) {
	var key string
	err := r.db.QueryRowContext(ctx, `DELETE FROM image_studio_assets WHERE id = $1 AND user_id = $2 RETURNING storage_key`, id, userID).Scan(&key)
	if err != nil {
		return "", translatePersistenceError(err, service.ErrImageStudioNotFound, nil)
	}
	return key, nil
}

func (r *imageStudioRepository) DeleteExpiredJobs(ctx context.Context, before time.Time, limit int) (keys []string, deleted int, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	// SKIP LOCKED：多副本同时清理时各取不同批次。
	rows, err := tx.QueryContext(ctx, `SELECT id FROM image_studio_jobs WHERE created_at < $1 AND status IN ('succeeded', 'failed')
		ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED`, before, limit)
	if err != nil {
		return nil, 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err = rows.Err(); err != nil || len(ids) == 0 {
		if err == nil {
			err = tx.Commit()
		}
		return nil, 0, err
	}
	keyRows, err := tx.QueryContext(ctx, `SELECT storage_key FROM image_studio_assets WHERE job_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, 0, err
	}
	for keyRows.Next() {
		var key string
		if err = keyRows.Scan(&key); err != nil {
			_ = keyRows.Close()
			return nil, 0, err
		}
		keys = append(keys, key)
	}
	_ = keyRows.Close()
	if err = keyRows.Err(); err != nil {
		return nil, 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM image_studio_jobs WHERE id = ANY($1)`, pq.Array(ids)); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return keys, len(ids), nil
}

func imageStudioPage(page, pageSize int) (limit, offset int) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page < 1 {
		page = 1
	}
	return pageSize, (page - 1) * pageSize
}
