package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SoraTaskRepository struct {
	db *sql.DB
}

func NewSoraTaskRepository(sqlDB *sql.DB) service.SoraTaskRepository {
	return &SoraTaskRepository{db: sqlDB}
}

func (r *SoraTaskRepository) Create(ctx context.Context, task *service.SoraTask) error {
	var charStr, reqStr *string
	if task.CharacterInfo != nil {
		b, _ := json.Marshal(task.CharacterInfo)
		s := string(b)
		charStr = &s
	}
	if len(task.RequestBody) > 0 && utf8.Valid(task.RequestBody) {
		s := string(task.RequestBody)
		reqStr = &s
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sora_tasks (
			id, account_id, api_key_id, upstream_task_id, object_type,
			model, prompt, status, progress, video_url, stored_key, storage_type,
			share_id, character_info, error_message, error_type,
			request_body, seconds, size, created_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		task.ID, task.AccountID, task.APIKeyID, task.UpstreamTaskID, task.ObjectType,
		task.Model, task.Prompt, task.Status, task.Progress, task.VideoURL,
		task.StoredKey, task.StorageType,
		task.ShareID, charStr, task.ErrorMessage, task.ErrorType,
		reqStr, task.Seconds, task.Size, task.CreatedAt, task.CompletedAt,
	)
	return err
}

func (r *SoraTaskRepository) GetByID(ctx context.Context, id string) (*service.SoraTask, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+soraTaskColumns+` FROM sora_tasks WHERE id = $1`, id)
	return scanTask(row)
}

func (r *SoraTaskRepository) GetByIDAndAPIKey(ctx context.Context, id string, apiKeyID int64) (*service.SoraTask, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+soraTaskColumns+` FROM sora_tasks WHERE id = $1 AND api_key_id = $2`,
		id, apiKeyID,
	)
	return scanTask(row)
}

func (r *SoraTaskRepository) Update(ctx context.Context, task *service.SoraTask) error {
	var charStr *string
	if task.CharacterInfo != nil {
		b, _ := json.Marshal(task.CharacterInfo)
		s := string(b)
		charStr = &s
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE sora_tasks SET
			upstream_task_id = $2, status = $3, progress = $4,
			video_url = $5, stored_key = $6, storage_type = $7,
			share_id = $8, character_info = $9,
			error_message = $10, error_type = $11,
			completed_at = $12, seconds = $13, size = $14, object_type = $15
		WHERE id = $1`,
		task.ID, task.UpstreamTaskID, task.Status, task.Progress,
		task.VideoURL, task.StoredKey, task.StorageType,
		task.ShareID, charStr,
		task.ErrorMessage, task.ErrorType,
		task.CompletedAt, task.Seconds, task.Size, task.ObjectType,
	)
	return err
}

func (r *SoraTaskRepository) ListPending(ctx context.Context) ([]*service.SoraTask, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+soraTaskColumns+` FROM sora_tasks WHERE status IN ('queued', 'in_progress') ORDER BY created_at ASC LIMIT 200`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tasks []*service.SoraTask
	for rows.Next() {
		t, err := scanTaskFromRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

const soraTaskColumns = `id, account_id, api_key_id, upstream_task_id, object_type,
	model, prompt, status, progress, video_url, stored_key, storage_type,
	share_id, character_info, error_message, error_type,
	request_body, seconds, size, created_at, completed_at`

func scanTask(s scannable) (*service.SoraTask, error) {
	var t service.SoraTask
	var charJSON, reqBody []byte
	err := s.Scan(
		&t.ID, &t.AccountID, &t.APIKeyID, &t.UpstreamTaskID, &t.ObjectType,
		&t.Model, &t.Prompt, &t.Status, &t.Progress, &t.VideoURL,
		&t.StoredKey, &t.StorageType,
		&t.ShareID, &charJSON, &t.ErrorMessage, &t.ErrorType,
		&reqBody, &t.Seconds, &t.Size, &t.CreatedAt, &t.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(charJSON) > 0 {
		var ch service.SoraCharacter
		if json.Unmarshal(charJSON, &ch) == nil && ch.Username != "" {
			t.CharacterInfo = &ch
		}
	}
	t.RequestBody = reqBody
	return &t, nil
}

func scanTaskFromRow(rows *sql.Rows) (*service.SoraTask, error) {
	return scanTask(rows)
}
