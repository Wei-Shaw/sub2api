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
RETURNING id, created_at
`
	var id int64
	var createdAt time.Time
	if err := r.db.QueryRowContext(ctx, q,
		m.UserID, m.FileName, m.CosKey, m.CosURL,
		m.ContentType, m.SizeBytes, m.Kind, m.Source,
	).Scan(&id, &createdAt); err != nil {
		return 0, fmt.Errorf("insert user_material: %w", err)
	}
	m.ID = id
	m.CreatedAt = createdAt
	return id, nil
}

// GetByID 按 id 查一条；找不到（含已软删）返回 nil, nil。
func (r *userMaterialRepository) GetByID(ctx context.Context, id int64) (*service.UserMaterial, error) {
	const q = `
SELECT id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
FROM user_materials
WHERE id = $1 AND deleted_at IS NULL
`
	row := r.db.QueryRowContext(ctx, q, id)
	m := &service.UserMaterial{}
	if err := row.Scan(&m.ID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
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

	listQ := "SELECT id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at FROM user_materials " +
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
		if err := rows.Scan(&m.ID, &m.UserID, &m.FileName, &m.CosKey, &m.CosURL,
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
