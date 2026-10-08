package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type proxyGroupRepository struct{ db *sql.DB }

func NewProxyGroupRepository(db *sql.DB) service.ProxyGroupRepository {
	return &proxyGroupRepository{db: db}
}

func (r *proxyGroupRepository) List(ctx context.Context, page, pageSize int, status, search string) ([]service.ProxyGroup, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	where, args := proxyGroupFilters(status, search)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM proxy_groups pg "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, proxyGroupListSQL+where+" ORDER BY pg.id DESC LIMIT $"+itoa(len(args)-1)+" OFFSET $"+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	groups, err := scanProxyGroups(rows)
	return groups, total, err
}

func (r *proxyGroupRepository) ListAll(ctx context.Context) ([]service.ProxyGroup, error) {
	rows, err := r.db.QueryContext(ctx, proxyGroupListSQL+" WHERE pg.deleted_at IS NULL ORDER BY pg.id DESC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProxyGroups(rows)
}

func (r *proxyGroupRepository) GetByID(ctx context.Context, id int64) (*service.ProxyGroup, error) {
	rows, err := r.db.QueryContext(ctx, proxyGroupListSQL+" WHERE pg.deleted_at IS NULL AND pg.id = $1", id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	groups, err := scanProxyGroups(rows)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, service.ErrProxyGroupNotFound
	}
	return &groups[0], nil
}

func (r *proxyGroupRepository) Create(ctx context.Context, group *service.ProxyGroup, proxyIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateProxyIDs(ctx, tx, proxyIDs); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO proxy_groups (name, description, status) VALUES ($1,$2,$3) RETURNING id, created_at, updated_at`, group.Name, group.Description, group.Status).Scan(&group.ID, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return err
	}
	if err := replaceProxyGroupMembers(ctx, tx, group.ID, proxyIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *proxyGroupRepository) Update(ctx context.Context, group *service.ProxyGroup, proxyIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateProxyIDs(ctx, tx, proxyIDs); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE proxy_groups SET name=$1, description=$2, status=$3, updated_at=NOW() WHERE id=$4 AND deleted_at IS NULL`, group.Name, group.Description, group.Status, group.ID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return service.ErrProxyGroupNotFound
	}
	if err := replaceProxyGroupMembers(ctx, tx, group.ID, proxyIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *proxyGroupRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE proxy_groups SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return service.ErrProxyGroupNotFound
	}
	return nil
}

func (r *proxyGroupRepository) CountAccountsByGroupID(ctx context.Context, id int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE proxy_group_id=$1 AND deleted_at IS NULL`, id).Scan(&count)
	return count, err
}

func (r *proxyGroupRepository) SelectRandomAvailableProxy(ctx context.Context, groupID int64) (*service.Proxy, error) {
	return r.SelectAvailableProxyExcluding(ctx, groupID, 0, nil)
}

func (r *proxyGroupRepository) SelectAvailableProxyExcluding(ctx context.Context, groupID, previousID int64, allowedIDs []int64) (*service.Proxy, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.name, p.protocol, p.host, p.port, COALESCE(p.username, ''), COALESCE(p.password, ''), p.status, p.created_at, p.updated_at, p.expires_at, p.fallback_mode, p.backup_proxy_id, p.expiry_warn_days
		FROM proxy_groups pg
		JOIN proxy_group_proxies pgp ON pgp.proxy_group_id = pg.id
		JOIN proxies p ON p.id = pgp.proxy_id
		WHERE pg.id=$1 AND pg.deleted_at IS NULL AND pg.status='active'
		  AND p.deleted_at IS NULL AND p.status='active'
		  AND p.id <> $2
		  AND ($3::bigint[] IS NULL OR p.id = ANY($3::bigint[]))
		  AND (p.expires_at IS NULL OR p.expires_at > NOW())
		ORDER BY random() LIMIT 1`, groupID, previousID, pq.Array(allowedIDs))
	proxy := &service.Proxy{}
	if err := row.Scan(&proxy.ID, &proxy.Name, &proxy.Protocol, &proxy.Host, &proxy.Port, &proxy.Username, &proxy.Password, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt, &proxy.ExpiresAt, &proxy.FallbackMode, &proxy.BackupProxyID, &proxy.ExpiryWarnDays); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return proxy, nil
}

const proxyGroupListSQL = `
	SELECT pg.id, pg.name, pg.description, pg.status, pg.created_at, pg.updated_at,
		COALESCE(pc.proxy_ids, ARRAY[]::bigint[]),
		COALESCE(pc.member_count, 0), COALESCE(pc.available_member_count, 0),
		COALESCE(ac.account_count, 0)
	FROM proxy_groups pg
	LEFT JOIN LATERAL (
		SELECT ARRAY_AGG(pgp.proxy_id ORDER BY pgp.proxy_id) AS proxy_ids,
			COUNT(*)::int AS member_count,
			COUNT(*) FILTER (WHERE p.status='active' AND p.deleted_at IS NULL AND (p.expires_at IS NULL OR p.expires_at > NOW()))::int AS available_member_count
		FROM proxy_group_proxies pgp
		LEFT JOIN proxies p ON p.id=pgp.proxy_id
		WHERE pgp.proxy_group_id=pg.id
	) pc ON TRUE
	LEFT JOIN LATERAL (
		SELECT COUNT(*)::bigint AS account_count FROM accounts a WHERE a.proxy_group_id=pg.id AND a.deleted_at IS NULL
	) ac ON TRUE`

func proxyGroupFilters(status, search string) (string, []any) {
	clauses := []string{"pg.deleted_at IS NULL"}
	args := make([]any, 0, 2)
	if status != "" {
		args = append(args, status)
		clauses = append(clauses, "pg.status=$"+itoa(len(args)))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		clauses = append(clauses, "(pg.name ILIKE $"+itoa(len(args))+" OR COALESCE(pg.description,'') ILIKE $"+itoa(len(args))+")")
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanProxyGroups(rows *sql.Rows) ([]service.ProxyGroup, error) {
	groups := make([]service.ProxyGroup, 0)
	for rows.Next() {
		var group service.ProxyGroup
		if err := rows.Scan(&group.ID, &group.Name, &group.Description, &group.Status, &group.CreatedAt, &group.UpdatedAt, pq.Array(&group.ProxyIDs), &group.MemberCount, &group.AvailableMemberCount, &group.AccountCount); err != nil {
			return nil, err
		}
		if group.ProxyIDs == nil {
			group.ProxyIDs = []int64{}
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func validateProxyIDs(ctx context.Context, tx *sql.Tx, proxyIDs []int64) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxies WHERE id = ANY($1) AND deleted_at IS NULL`, pq.Array(proxyIDs)).Scan(&count); err != nil {
		return err
	}
	if count != len(proxyIDs) {
		return infraerrors.BadRequest("PROXY_GROUP_PROXY_INVALID", "proxy group contains a missing or deleted proxy")
	}
	return nil
}

func replaceProxyGroupMembers(ctx context.Context, tx *sql.Tx, groupID int64, proxyIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM proxy_group_proxies WHERE proxy_group_id=$1`, groupID); err != nil {
		return err
	}
	for _, proxyID := range proxyIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO proxy_group_proxies (proxy_group_id, proxy_id) VALUES ($1,$2)`, groupID, proxyID); err != nil {
			return fmt.Errorf("insert proxy group member: %w", err)
		}
	}
	return nil
}
