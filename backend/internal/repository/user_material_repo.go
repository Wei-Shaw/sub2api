// Package repository: 用户素材库 repository。
//
// 与 async_media / async_video 走独立的表 user_materials（见 migrations/210_user_materials.sql）。
// 本 repo 只做 SQL 层，业务逻辑（上传字节 → 转存 COS → 落库）在 service.UserMaterialService。
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type userMaterialRepository struct {
	db *sql.DB
}

// NewUserMaterialRepository 构造。签名与其它 *sql.DB 风格 repo 一致。
func NewUserMaterialRepository(db *sql.DB) service.UserMaterialRepository {
	return &userMaterialRepository{db: db}
}

// Insert 插入一条素材记录，返回自增 id。
func (r *userMaterialRepository) Insert(ctx context.Context, m *service.UserMaterial) (int64, error) {
	if m == nil {
		return 0, errors.New("nil material")
	}
	const q = `
INSERT INTO user_materials (user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
RETURNING id, public_id, created_at
`
	var id int64
	var publicID string
	var createdAt time.Time
	if err := r.db.QueryRowContext(ctx, q,
		m.UserID, m.FileName, m.CosKey, m.CosURL,
		m.ContentType, m.SizeBytes, m.Kind, m.Source,
	).Scan(&id, &publicID, &createdAt); err != nil {
		return 0, fmt.Errorf("insert user_material: %w", err)
	}
	m.ID = id
	m.PublicID = publicID
	m.CreatedAt = createdAt
	return id, nil
}

func (r *userMaterialRepository) GetByPublicID(ctx context.Context, userID int64, publicID string) (*service.UserMaterial, error) {
	const q = `
SELECT id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
FROM user_materials
WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL
`
	row := r.db.QueryRowContext(ctx, q, publicID, userID)
	m := &service.UserMaterial{}
	if err := row.Scan(&m.ID, &m.PublicID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
		&m.ContentType, &m.SizeBytes, &m.Kind, &m.Source, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("get user_material by public id: %w", err)
	}
	return m, nil
}

func (r *userMaterialRepository) UpdateFileNameByID(ctx context.Context, userID, id int64, fileName string) (*service.UserMaterial, error) {
	const q = `
UPDATE user_materials
SET file_name = $3
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
`
	row := r.db.QueryRowContext(ctx, q, id, userID, fileName)
	m := &service.UserMaterial{}
	if err := row.Scan(&m.ID, &m.PublicID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
		&m.ContentType, &m.SizeBytes, &m.Kind, &m.Source, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("rename user_material by id: %w", err)
	}
	return m, nil
}

func (r *userMaterialRepository) UpdateFileNameByPublicID(ctx context.Context, userID int64, publicID, fileName string) (*service.UserMaterial, error) {
	const q = `
UPDATE user_materials
SET file_name = $3
WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
`
	row := r.db.QueryRowContext(ctx, q, publicID, userID, fileName)
	m := &service.UserMaterial{}
	if err := row.Scan(&m.ID, &m.PublicID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
		&m.ContentType, &m.SizeBytes, &m.Kind, &m.Source, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("rename user_material by public id: %w", err)
	}
	return m, nil
}

// GetByID 按 user_id + id 原子查询；归属不匹配与不存在使用相同结果。
func (r *userMaterialRepository) GetByID(ctx context.Context, userID, id int64) (*service.UserMaterial, error) {
	const q = `
SELECT id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
FROM user_materials
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
`
	row := r.db.QueryRowContext(ctx, q, id, userID)
	m := &service.UserMaterial{}
	if err := row.Scan(&m.ID, &m.PublicID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
		&m.ContentType, &m.SizeBytes, &m.Kind, &m.Source, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("get user_material: %w", err)
	}
	return m, nil
}

// List 按 user_id + 可选 kind/keyword 过滤，按 created_at 倒序分页。
// keyword 走 file_name ILIKE 前缀匹配（宽松），空则不过滤。
func (r *userMaterialRepository) List(ctx context.Context, userID int64, kind, keyword string, offset, limit int) ([]*service.UserMaterial, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where := []string{"user_id = $1", "deleted_at IS NULL"}
	args := []any{userID}
	idx := 2
	if k := strings.TrimSpace(kind); k != "" {
		where = append(where, fmt.Sprintf("kind = $%d", idx))
		args = append(args, k)
		idx++
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		where = append(where, fmt.Sprintf("file_name ILIKE $%d", idx))
		args = append(args, "%"+kw+"%")
		idx++
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	countQ := "SELECT COUNT(1) FROM user_materials " + whereSQL
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count user_materials: %w", err)
	}

	listQ := "SELECT id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at FROM user_materials " +
		whereSQL + fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query user_materials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.UserMaterial, 0, limit)
	for rows.Next() {
		m := &service.UserMaterial{}
		if err := rows.Scan(&m.ID, &m.PublicID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
			&m.ContentType, &m.SizeBytes, &m.Kind, &m.Source, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user_material: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iter user_materials: %w", err)
	}
	return out, total, nil
}

// SoftDelete 软删单条（不动 COS 对象；由后台清理任务在保留期后清理）。
// 只允许删自己的（业务层已校验 user_id）；本层再做一次 user_id 匹配防兜底。
func (r *userMaterialRepository) SoftDelete(ctx context.Context, userID, id int64) error {
	const q = `
UPDATE user_materials
SET deleted_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
`
	res, err := r.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("soft delete user_material: %w", err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *userMaterialRepository) SoftDeleteByPublicID(ctx context.Context, userID int64, publicID string) error {
	const q = `
UPDATE user_materials
SET deleted_at = NOW()
WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL
`
	res, err := r.db.ExecContext(ctx, q, publicID, userID)
	if err != nil {
		return fmt.Errorf("soft delete user_material by public id: %w", err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SoftDeleteByPublicIDs atomically soft-deletes all matching materials owned by
// the user and returns only the public IDs changed by this statement.
func (r *userMaterialRepository) SoftDeleteByPublicIDs(ctx context.Context, userID int64, publicIDs []string) ([]string, error) {
	if len(publicIDs) == 0 {
		return []string{}, nil
	}
	const q = `
UPDATE user_materials
SET deleted_at = NOW()
WHERE user_id = $1 AND public_id = ANY($2::uuid[]) AND deleted_at IS NULL
RETURNING public_id
`
	rows, err := r.db.QueryContext(ctx, q, userID, pq.Array(publicIDs))
	if err != nil {
		return nil, fmt.Errorf("batch soft delete user_materials by public id: %w", err)
	}
	defer func() { _ = rows.Close() }()

	deleted := make([]string, 0, len(publicIDs))
	for rows.Next() {
		var publicID string
		if err := rows.Scan(&publicID); err != nil {
			return nil, fmt.Errorf("scan batch deleted user_material public id: %w", err)
		}
		deleted = append(deleted, publicID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch deleted user_material public ids: %w", err)
	}
	return deleted, nil
}

// UsageByUser 统计该用户未删除素材的条数与总字节数，供服务层做配额校验。
//
// 只统计 deleted_at IS NULL 的行：软删后的对象虽然还在 COS 上（等后台清理），
// 但配额按"用户可见的素材"计，否则用户删了东西却发现还是传不上去，很反直觉。
// COALESCE 兜住"该用户一条都没有"时 SUM 返回 NULL 的情况。
func (r *userMaterialRepository) UsageByUser(ctx context.Context, userID int64) (int64, int64, error) {
	const q = `
SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
FROM user_materials
WHERE user_id = $1 AND deleted_at IS NULL
`
	var count, totalBytes int64
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&count, &totalBytes); err != nil {
		return 0, 0, fmt.Errorf("usage by user user_materials: %w", err)
	}
	return count, totalBytes, nil
}
