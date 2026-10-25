package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
 upstream_model, display_name, total_token_count, expire_time, channel_usage, created_at, updated_at`

func (r *geminiCachedContentRepository) Create(ctx context.Context, record *service.GeminiCachedContent) error {
	channelUsage, err := json.Marshal(record.ChannelUsage)
	if err != nil {
		return err
	}
	if IsMySQL() {
		return r.createMySQL(ctx, record, channelUsage)
	}
	row := r.sql.QueryRowContext(ctx, `
INSERT INTO gemini_cached_contents (public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time, channel_usage)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, created_at, updated_at`,
		record.PublicID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID, record.UpstreamName,
		record.Model, record.UpstreamModel, record.DisplayName, record.TotalTokenCount, record.ExpireTime, channelUsage)
	if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
	}
	return nil
}

func (r *geminiCachedContentRepository) GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	if IsMySQL() {
		return r.getForOwnerMySQL(ctx, apiKeyID, publicID)
	}
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE public_id = $1 AND api_key_id = $2 AND deleted_at IS NULL`, publicID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return record, nil
}

func (r *geminiCachedContentRepository) ListForOwner(ctx context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	if IsMySQL() {
		return r.listForOwnerMySQL(ctx, apiKeyID, groupID, activeAt, beforeID, limit)
	}
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

func (r *geminiCachedContentRepository) BeginMutation(ctx context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	if IsMySQL() {
		return r.beginMutationMySQL(ctx, id, baseline, token, lease)
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NULL AND expire_time = $4`,
		id, token, lease.Seconds(), baseline)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	if err := r.sql.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM gemini_cached_contents WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, service.ErrGeminiCachedContentNotFound
	}
	return false, nil
}

func (r *geminiCachedContentRepository) TakeOverStaleMutation(ctx context.Context, id int64, token string, lease time.Duration) (*service.GeminiCachedContent, error) {
	if IsMySQL() {
		return r.takeOverStaleMutationMySQL(ctx, id, token, lease)
	}
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NOT NULL AND mutation_lease_until < NOW()
 RETURNING `+geminiCachedContentColumns, id, token, lease.Seconds()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *geminiCachedContentRepository) CommitMutation(ctx context.Context, id int64, token string, expireTime time.Time) error {
	if IsMySQL() {
		return r.commitMutationMySQL(ctx, id, token, expireTime)
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET expire_time = $3, updated_at = NOW(), mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token = $2`, id, token, expireTime)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return nil
}

func (r *geminiCachedContentRepository) ReleaseMutation(ctx context.Context, id int64, token string) error {
	if IsMySQL() {
		return r.releaseMutationMySQL(ctx, id, token)
	}
	_, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = $1 AND mutation_token = $2`, id, token)
	return err
}

func (r *geminiCachedContentRepository) ListStaleMutations(ctx context.Context, limit int) ([]int64, error) {
	if IsMySQL() {
		return r.listStaleMutationsMySQL(ctx, limit)
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT id FROM gemini_cached_contents
 WHERE mutation_token IS NOT NULL AND deleted_at IS NULL AND mutation_lease_until < NOW()
 ORDER BY mutation_lease_until
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *geminiCachedContentRepository) SoftDelete(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.softDeleteMySQL(ctx, id)
	}
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
	var channelUsage []byte
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
		&channelUsage,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(channelUsage) > 0 {
		if err := json.Unmarshal(channelUsage, &record.ChannelUsage); err != nil {
			return nil, err
		}
	}
	return &record, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。PG 分支行为与字节保持原样。
// ---------------------------------------------------------------------------

// createMySQL 是 Create 的 MySQL 实现：PG 版 INSERT..RETURNING id, created_at, updated_at
// → 同一事务内 INSERT + LastInsertId + 按主键回查（created_at/updated_at 为数据库默认值，
// 应用侧未知）。public_id 唯一冲突（1062）经 translatePersistenceError 翻译，与 PG 23505 同路。
func (r *geminiCachedContentRepository) createMySQL(ctx context.Context, record *service.GeminiCachedContent, channelUsage []byte) error {
	scope, err := beginSQLAtomicScope(ctx, nil, r.sql)
	if err != nil {
		return err
	}
	defer scope.rollback()

	res, err := scope.exec(ctx, `
INSERT INTO gemini_cached_contents (public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time, channel_usage)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.PublicID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID, record.UpstreamName,
		record.Model, record.UpstreamModel, record.DisplayName, record.TotalTokenCount, record.ExpireTime, channelUsage)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rows, err := scope.query(ctx, `SELECT id, created_at, updated_at FROM gemini_cached_contents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return rowsErr
		}
		_ = rows.Close()
		return sql.ErrNoRows
	}
	if err := rows.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return scope.commit()
}

// getForOwnerMySQL 是 GetForOwner 的 MySQL 实现（$n→?）。
func (r *geminiCachedContentRepository) getForOwnerMySQL(ctx context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE public_id = ? AND api_key_id = ? AND deleted_at IS NULL`, publicID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return record, nil
}

// listForOwnerMySQL 是 ListForOwner 的 MySQL 实现（$n→?，重复引用的 $4 展开两次）。
func (r *geminiCachedContentRepository) listForOwnerMySQL(ctx context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	if limit <= 0 {
		limit = service.GeminiCachedContentListDefaultPageSize
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE api_key_id = ? AND group_id = ? AND deleted_at IS NULL AND expire_time > ? AND (? <= 0 OR id < ?)
 ORDER BY id DESC
 LIMIT ?`, apiKeyID, groupID, activeAt, beforeID, beforeID, limit)
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

// beginMutationMySQL 是 BeginMutation 的 MySQL 实现：
// make_interval(secs => $3::double precision) → INTERVAL ? SECOND（预编译占位符合法）；
// NOW() → NOW(3)。WHERE mutation_token IS NULL 保证命中行必然发生值变化（NULL→token），
// RowsAffected 语义与 PG 一致。
// REVIEW: PG make_interval 的 secs 参数按整秒取整，MySQL INTERVAL ? SECOND 保留亚秒精度；
// 租约时长为秒级，差异不影响语义。
func (r *geminiCachedContentRepository) beginMutationMySQL(ctx context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = ?, mutation_lease_until = NOW(3) + INTERVAL ? SECOND
 WHERE id = ? AND deleted_at IS NULL AND mutation_token IS NULL AND expire_time = ?`,
		token, lease.Seconds(), id, baseline)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	if err := r.sql.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM gemini_cached_contents WHERE id = ? AND deleted_at IS NULL)`, id).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, service.ErrGeminiCachedContentNotFound
	}
	return false, nil
}

// takeOverStaleMutationMySQL 是 TakeOverStaleMutation 的 MySQL 实现。
// PG 版 UPDATE..RETURNING <全列>（更新后值）→ sqlAtomicScope 先 SELECT FOR UPDATE
// 取行集合再 UPDATE。RETURNING 列表不含 mutation_token / mutation_lease_until，
// 而这两个恰好是本 UPDATE 仅有的变更列，故先读到的行与 PG RETURNING 行完全一致。
func (r *geminiCachedContentRepository) takeOverStaleMutationMySQL(ctx context.Context, id int64, token string, lease time.Duration) (*service.GeminiCachedContent, error) {
	scope, err := beginSQLAtomicScope(ctx, nil, r.sql)
	if err != nil {
		return nil, err
	}
	defer scope.rollback()

	rows, err := scope.query(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE id = ? AND deleted_at IS NULL AND mutation_token IS NOT NULL AND mutation_lease_until < NOW(3)
 FOR UPDATE`, id)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return nil, rowsErr
		}
		_ = rows.Close()
		// 无匹配行 → (nil, nil)，对齐 PG 的 sql.ErrNoRows 分支
		return nil, nil
	}
	record, err := scanGeminiCachedContent(rows)
	if err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := scope.exec(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = ?, mutation_lease_until = NOW(3) + INTERVAL ? SECOND
 WHERE id = ?`, token, lease.Seconds(), id); err != nil {
		return nil, err
	}
	if err := scope.commit(); err != nil {
		return nil, err
	}
	return record, nil
}

// commitMutationMySQL 是 CommitMutation 的 MySQL 实现（$n→?，NOW()→NOW(3)）。
// WHERE mutation_token = ?（非 NULL）而 SET mutation_token = NULL，
// 命中行必然发生值变化，RowsAffected 语义与 PG 一致。
func (r *geminiCachedContentRepository) commitMutationMySQL(ctx context.Context, id int64, token string, expireTime time.Time) error {
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET expire_time = ?, updated_at = NOW(3), mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = ? AND deleted_at IS NULL AND mutation_token = ?`, expireTime, id, token)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return nil
}

// releaseMutationMySQL 是 ReleaseMutation 的 MySQL 实现（$n→?）。
func (r *geminiCachedContentRepository) releaseMutationMySQL(ctx context.Context, id int64, token string) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = ? AND mutation_token = ?`, id, token)
	return err
}

// listStaleMutationsMySQL 是 ListStaleMutations 的 MySQL 实现（$1→?，NOW()→NOW(3)）。
func (r *geminiCachedContentRepository) listStaleMutationsMySQL(ctx context.Context, limit int) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id FROM gemini_cached_contents
 WHERE mutation_token IS NOT NULL AND deleted_at IS NULL AND mutation_lease_until < NOW(3)
 ORDER BY mutation_lease_until
 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// softDeleteMySQL 是 SoftDelete 的 MySQL 实现（$1→?，NOW()→NOW(3)）。
func (r *geminiCachedContentRepository) softDeleteMySQL(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET deleted_at = NOW(3), updated_at = NOW(3)
 WHERE id = ? AND deleted_at IS NULL`, id)
	return err
}
