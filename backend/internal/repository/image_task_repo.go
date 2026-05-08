package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type imageTaskRepository struct {
	sql sqlExecutor
}

func NewImageTaskRepository(sqlDB *sql.DB) service.ImageTaskRepository {
	return &imageTaskRepository{sql: sqlDB}
}

func (r *imageTaskRepository) Create(ctx context.Context, task *service.ImageTask) error {
	query := `
		INSERT INTO image_tasks (
			task_id, user_id, api_key_id, status, endpoint, model, prompt,
			file_path, mime_type, byte_size, error_message, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, '', '', 0, '', $8, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	return scanSingleRow(ctx, r.sql, query, []any{
		task.TaskID, task.UserID, task.APIKeyID, task.Status, task.Endpoint, task.Model, task.Prompt, task.ExpiresAt,
	}, &task.CreatedAt, &task.UpdatedAt)
}

func (r *imageTaskRepository) GetByTaskID(ctx context.Context, taskID string) (*service.ImageTask, error) {
	query := `
		SELECT task_id, user_id, api_key_id, status, endpoint, model, prompt,
			file_path, mime_type, byte_size, error_message, created_at, updated_at, expires_at
		FROM image_tasks
		WHERE task_id = $1
	`
	var task service.ImageTask
	if err := scanSingleRow(ctx, r.sql, query, []any{taskID},
		&task.TaskID, &task.UserID, &task.APIKeyID, &task.Status, &task.Endpoint, &task.Model, &task.Prompt,
		&task.FilePath, &task.MimeType, &task.ByteSize, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt, &task.ExpiresAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrImageTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (r *imageTaskRepository) MarkRunning(ctx context.Context, taskID string) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_tasks
		SET status = $1, updated_at = NOW()
		WHERE task_id = $2
	`, service.ImageTaskStatusRunning, taskID)
	return err
}

func (r *imageTaskRepository) MarkSucceeded(ctx context.Context, taskID, filePath, mimeType string, byteSize int64, expiresAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_tasks
		SET status = $1, file_path = $2, mime_type = $3, byte_size = $4, error_message = '', expires_at = $5, updated_at = NOW()
		WHERE task_id = $6
	`, service.ImageTaskStatusSucceeded, filePath, mimeType, byteSize, expiresAt, taskID)
	return err
}

func (r *imageTaskRepository) MarkFailed(ctx context.Context, taskID, message string, expiresAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_tasks
		SET status = $1, error_message = $2, expires_at = $3, updated_at = NOW()
		WHERE task_id = $4
	`, service.ImageTaskStatusFailed, message, expiresAt, taskID)
	return err
}

func (r *imageTaskRepository) DeleteExpired(ctx context.Context, now time.Time) ([]service.ImageTask, error) {
	rows, err := r.sql.QueryContext(ctx, `
		DELETE FROM image_tasks
		WHERE expires_at <= $1
		RETURNING task_id, user_id, api_key_id, status, endpoint, model, prompt,
			file_path, mime_type, byte_size, error_message, created_at, updated_at, expires_at
	`, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tasks []service.ImageTask
	for rows.Next() {
		var task service.ImageTask
		if err := rows.Scan(
			&task.TaskID, &task.UserID, &task.APIKeyID, &task.Status, &task.Endpoint, &task.Model, &task.Prompt,
			&task.FilePath, &task.MimeType, &task.ByteSize, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt, &task.ExpiresAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *imageTaskRepository) MarkStaleRunningFailed(ctx context.Context, message string) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_tasks
		SET status = $1, error_message = $2, updated_at = NOW()
		WHERE status IN ($3, $4)
	`, service.ImageTaskStatusFailed, message, service.ImageTaskStatusPending, service.ImageTaskStatusRunning)
	return err
}
