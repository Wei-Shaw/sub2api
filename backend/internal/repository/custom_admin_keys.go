package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type scopedAdminKeyRepository struct{ db *sql.DB }

func NewScopedAdminKeyRepository(db *sql.DB) service.ScopedAdminKeyRepository {
	return &scopedAdminKeyRepository{db: db}
}

const scopedAdminKeyColumns = `id,name,description,key_prefix,key_hash,enabled,expires_at,allowed_ips,policy,created_by,created_at,updated_at,last_used_at,last_used_ip,revoked_at`

func scanScopedAdminKey(row interface{ Scan(...any) error }) (*service.ScopedAdminKey, error) {
	k := &service.ScopedAdminKey{}
	var ips, policy []byte
	err := row.Scan(&k.ID, &k.Name, &k.Description, &k.KeyPrefix, &k.Hash, &k.Enabled, &k.ExpiresAt, &ips, &policy, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt, &k.LastUsedAt, &k.LastUsedIP, &k.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, service.ErrScopedAdminKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(ips, &k.AllowedIPs); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(policy, &k.Policy); err != nil {
		return nil, err
	}
	return k, nil
}
func (r *scopedAdminKeyRepository) ListScopedAdminKeys(ctx context.Context) ([]*service.ScopedAdminKey, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT `+scopedAdminKeyColumns+` FROM scoped_admin_api_keys WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	items := []*service.ScopedAdminKey{}
	for rows.Next() {
		k, e := scanScopedAdminKey(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, k)
	}
	return items, rows.Err()
}
func (r *scopedAdminKeyRepository) GetScopedAdminKey(ctx context.Context, id string) (*service.ScopedAdminKey, error) {
	return scanScopedAdminKey(r.db.QueryRowContext(ctx, `SELECT `+scopedAdminKeyColumns+` FROM scoped_admin_api_keys WHERE id=$1`, id))
}
func (r *scopedAdminKeyRepository) InsertScopedAdminKey(ctx context.Context, k *service.ScopedAdminKey) error {
	ips, e := json.Marshal(k.AllowedIPs)
	if e != nil {
		return e
	}
	policy, e := json.Marshal(k.Policy)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, `INSERT INTO scoped_admin_api_keys (id,name,description,key_prefix,key_hash,enabled,expires_at,allowed_ips,policy,created_by,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, k.ID, k.Name, k.Description, k.KeyPrefix, k.Hash, k.Enabled, k.ExpiresAt, string(ips), string(policy), k.CreatedBy, k.CreatedAt, k.UpdatedAt)
	return e
}
func scopedAdminKeyResult(result sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return service.ErrScopedAdminKeyNotFound
	}
	return nil
}
func (r *scopedAdminKeyRepository) UpdateScopedAdminKey(ctx context.Context, k *service.ScopedAdminKey) error {
	ips, e := json.Marshal(k.AllowedIPs)
	if e != nil {
		return e
	}
	policy, e := json.Marshal(k.Policy)
	if e != nil {
		return e
	}
	return scopedAdminKeyResult(r.db.ExecContext(ctx, `UPDATE scoped_admin_api_keys SET name=$2,description=$3,enabled=$4,expires_at=$5,allowed_ips=$6,policy=$7,updated_at=$8 WHERE id=$1 AND revoked_at IS NULL`, k.ID, k.Name, k.Description, k.Enabled, k.ExpiresAt, string(ips), string(policy), k.UpdatedAt))
}
func (r *scopedAdminKeyRepository) TouchScopedAdminKey(ctx context.Context, id, expectedHash string, expectedUpdatedAt time.Time, ip string, now time.Time) error {
	return scopedAdminKeyResult(r.db.ExecContext(ctx, `UPDATE scoped_admin_api_keys SET last_used_at=$5,last_used_ip=$4 WHERE id=$1 AND key_hash=$2 AND updated_at=$3 AND revoked_at IS NULL AND enabled=true AND (expires_at IS NULL OR expires_at>$5)`, id, expectedHash, expectedUpdatedAt, ip, now))
}
func (r *scopedAdminKeyRepository) RevokeScopedAdminKey(ctx context.Context, id string, now time.Time) error {
	return scopedAdminKeyResult(r.db.ExecContext(ctx, `UPDATE scoped_admin_api_keys SET revoked_at=$2,enabled=false,updated_at=$2 WHERE id=$1 AND revoked_at IS NULL`, id, now))
}

// 删除仅隐藏列表项；原子检查撤销状态，不删除密钥记录或历史审计。
func (r *scopedAdminKeyRepository) DeleteScopedAdminKeyRecord(ctx context.Context, id string, now time.Time) error {
	return scopedAdminKeyResult(r.db.ExecContext(ctx, `UPDATE scoped_admin_api_keys SET deleted_at=$2,updated_at=$2 WHERE id=$1 AND revoked_at IS NOT NULL AND deleted_at IS NULL`, id, now))
}

// 轮换按读取时的摘要进行 CAS，同一旧版本只能成功轮换一次。
func (r *scopedAdminKeyRepository) RotateScopedAdminKeyHash(ctx context.Context, id, expectedHash, hash string, at time.Time) error {
	err := scopedAdminKeyResult(r.db.ExecContext(ctx, `UPDATE scoped_admin_api_keys SET key_hash=$2,updated_at=$3 WHERE id=$1 AND key_hash=$4 AND revoked_at IS NULL`, id, hash, at, expectedHash))
	if err == service.ErrScopedAdminKeyNotFound {
		return service.ErrScopedAdminKeyConflict
	}
	return err
}
