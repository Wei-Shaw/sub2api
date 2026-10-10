package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type geminiCachedContentSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type geminiCachedContentRepository struct {
	sql geminiCachedContentSQLExecutor
}

func NewGeminiCachedContentRepository(db *sql.DB) service.GeminiCachedContentRepository {
	return &geminiCachedContentRepository{sql: db}
}

const geminiCachedContentColumns = `id, public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time, created_at, updated_at`

func (r *geminiCachedContentRepository) Create(ctx context.Context, record *service.GeminiCachedContent) error {
	row := r.sql.QueryRowContext(ctx, `
INSERT INTO gemini_cached_contents (public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at, updated_at`,
		record.PublicID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID, record.UpstreamName,
		record.Model, record.UpstreamModel, record.DisplayName, record.TotalTokenCount, record.ExpireTime)
	if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
	}
	return nil
}

func (r *geminiCachedContentRepository) GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE public_id = $1 AND api_key_id = $2 AND deleted_at IS NULL`, publicID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return record, nil
}

func (r *geminiCachedContentRepository) ListForOwner(ctx context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	if limit <= 0 {
		limit = service.GeminiCachedContentListDefaultPageSize
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE api_key_id = $1 AND group_id = $2 AND deleted_at IS NULL AND expire_time > $3 AND ($4 <= 0 OR id < $4)
 ORDER BY id DESC
 LIMIT $5`, apiKeyID, groupID, activeAt, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.GeminiCachedContent
	for rows.Next() {
		record, err := scanGeminiCachedContent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (r *geminiCachedContentRepository) UpdateExpireTime(ctx context.Context, id int64, expireTime time.Time) (time.Time, error) {
	var previous time.Time
	err := r.sql.QueryRowContext(ctx, `WITH prev AS (
 SELECT id, expire_time FROM gemini_cached_contents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE
)
UPDATE gemini_cached_contents AS g
 SET expire_time = $2, updated_at = NOW()
 FROM prev
 WHERE g.id = prev.id
 RETURNING prev.expire_time`, id, expireTime).Scan(&previous)
	if err != nil {
		return time.Time{}, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return previous, nil
}

func (r *geminiCachedContentRepository) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET deleted_at = NOW(), updated_at = NOW()
 WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

type geminiCachedContentScanner interface {
	Scan(dest ...any) error
}

func scanGeminiCachedContent(row geminiCachedContentScanner) (*service.GeminiCachedContent, error) {
	var record service.GeminiCachedContent
	if err := row.Scan(
		&record.ID,
		&record.PublicID,
		&record.UserID,
		&record.APIKeyID,
		&record.GroupID,
		&record.AccountID,
		&record.UpstreamName,
		&record.Model,
		&record.UpstreamModel,
		&record.DisplayName,
		&record.TotalTokenCount,
		&record.ExpireTime,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &record, nil
}
