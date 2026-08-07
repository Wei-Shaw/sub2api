package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type proxyPoolRepository struct {
	db *sql.DB
}

func NewProxyPoolRepository(db *sql.DB) service.ProxyPoolRepository {
	return &proxyPoolRepository{db: db}
}

type proxyPoolRowScanner interface {
	Scan(dest ...any) error
}

func scanProxyPool(row proxyPoolRowScanner) (*service.ProxyPool, error) {
	var (
		pool        service.ProxyPool
		description sql.NullString
		deletedAt   sql.NullTime
	)
	if err := row.Scan(
		&pool.ID,
		&pool.Name,
		&description,
		&pool.Status,
		&pool.HealthIntervalSeconds,
		&pool.FailureThreshold,
		&pool.AutoRebind,
		&pool.CreatedAt,
		&pool.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	if description.Valid {
		pool.Description = &description.String
	}
	if deletedAt.Valid {
		pool.DeletedAt = &deletedAt.Time
	}
	return &pool, nil
}

const proxyPoolColumns = `
	id, name, description, status, health_interval_seconds,
	failure_threshold, auto_rebind, created_at, updated_at, deleted_at
`

func (r *proxyPoolRepository) CreatePool(ctx context.Context, pool *service.ProxyPool) (*service.ProxyPool, error) {
	if r == nil || r.db == nil || pool == nil {
		return nil, errors.New("proxy pool repository unavailable")
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO proxy_pools (
			name, description, status, health_interval_seconds, failure_threshold, auto_rebind
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+proxyPoolColumns,
		pool.Name, pool.Description, pool.Status, pool.HealthIntervalSeconds, pool.FailureThreshold, pool.AutoRebind,
	)
	created, err := scanProxyPool(row)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrProxyPoolNameExists)
	}
	return created, nil
}

func (r *proxyPoolRepository) UpdatePool(ctx context.Context, pool *service.ProxyPool) error {
	if r == nil || r.db == nil || pool == nil {
		return errors.New("proxy pool repository unavailable")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE proxy_pools
		SET name = $2, description = $3, status = $4,
			health_interval_seconds = $5, failure_threshold = $6,
			auto_rebind = $7, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, pool.ID, pool.Name, pool.Description, pool.Status, pool.HealthIntervalSeconds, pool.FailureThreshold, pool.AutoRebind)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrProxyPoolNameExists)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrProxyPoolNotFound
	}
	return nil
}

func (r *proxyPoolRepository) DeletePool(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("proxy pool repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts
		SET pool_id = NULL, updated_at = NOW()
		WHERE pool_id = $1 AND deleted_at IS NULL
		RETURNING id
	`, id)
	if err != nil {
		return err
	}
	accountIDs, err := scanInt64Rows(rows)
	_ = rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE proxies
		SET pool_id = NULL, pool_health = 'unknown', pool_checked_at = NULL, pool_failures = 0,
			pool_grok_quality_status = 'unknown', pool_grok_quality_checked_at = NULL,
			pool_grok_quality_http_status = NULL, pool_grok_quality_message = NULL,
			updated_at = NOW()
		WHERE pool_id = $1
	`, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE proxy_pools SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrProxyPoolNotFound
	}
	if len(accountIDs) > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{"account_ids": accountIDs}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *proxyPoolRepository) GetPoolByID(ctx context.Context, id int64) (*service.ProxyPool, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("proxy pool repository unavailable")
	}
	pool, err := scanProxyPool(r.db.QueryRowContext(ctx, `SELECT `+proxyPoolColumns+` FROM proxy_pools WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProxyPoolNotFound
	}
	return pool, err
}

func (r *proxyPoolRepository) ListPools(ctx context.Context) ([]service.ProxyPool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+proxyPoolColumns+` FROM proxy_pools WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]service.ProxyPool, 0)
	for rows.Next() {
		pool, err := scanProxyPool(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *pool)
	}
	return result, rows.Err()
}

func (r *proxyPoolRepository) ListPoolsWithStats(ctx context.Context) ([]service.ProxyPoolWithStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pp.id, pp.name, pp.description, pp.status, pp.health_interval_seconds,
			pp.failure_threshold, pp.auto_rebind, pp.created_at, pp.updated_at, pp.deleted_at,
			COALESCE(ps.proxy_count, 0), COALESCE(ps.healthy_count, 0),
			COALESCE(ps.unhealthy_count, 0), COALESCE(ac.bound_account_count, 0)
		FROM proxy_pools pp
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS proxy_count,
				COUNT(*) FILTER (
					WHERE p.pool_health = 'healthy' AND p.status = 'active'
						AND p.pool_grok_quality_status = 'pass'
				) AS healthy_count,
				COUNT(*) FILTER (
					WHERE p.pool_health = 'unhealthy' OR p.status <> 'active'
						OR p.pool_grok_quality_status NOT IN ('pass', 'unknown')
				) AS unhealthy_count
			FROM proxies p
			WHERE p.pool_id = pp.id AND p.deleted_at IS NULL
		) ps ON TRUE
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS bound_account_count
			FROM accounts a
			WHERE a.pool_id = pp.id AND a.deleted_at IS NULL
		) ac ON TRUE
		WHERE pp.deleted_at IS NULL
		ORDER BY pp.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]service.ProxyPoolWithStats, 0)
	for rows.Next() {
		var (
			item        service.ProxyPoolWithStats
			description sql.NullString
			deletedAt   sql.NullTime
		)
		if err := rows.Scan(
			&item.ID, &item.Name, &description, &item.Status,
			&item.HealthIntervalSeconds, &item.FailureThreshold, &item.AutoRebind,
			&item.CreatedAt, &item.UpdatedAt, &deletedAt,
			&item.ProxyCount, &item.HealthyProxyCount, &item.UnhealthyProxyCount, &item.BoundAccountCount,
		); err != nil {
			return nil, err
		}
		if description.Valid {
			item.Description = &description.String
		}
		if deletedAt.Valid {
			item.DeletedAt = &deletedAt.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *proxyPoolRepository) ListPoolProxies(ctx context.Context, poolID int64) ([]service.ProxyPoolProxy, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.protocol, p.host, p.port, p.username, p.password, p.status,
			p.created_at, p.updated_at, p.pool_id, p.pool_health, p.pool_checked_at,
			p.pool_failures, p.pool_grok_quality_status, p.pool_grok_quality_checked_at,
			p.pool_grok_quality_http_status, p.pool_grok_quality_message, COUNT(a.id)
		FROM proxies p
		LEFT JOIN accounts a ON a.proxy_id = p.id AND a.pool_id = $1 AND a.deleted_at IS NULL
		WHERE p.pool_id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY p.id ASC
	`, poolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]service.ProxyPoolProxy, 0)
	for rows.Next() {
		var (
			item          service.ProxyPoolProxy
			username      sql.NullString
			password      sql.NullString
			checkedAt     sql.NullTime
			grokCheckedAt sql.NullTime
			grokHTTP      sql.NullInt64
			grokMessage   sql.NullString
		)
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Protocol, &item.Host, &item.Port,
			&username, &password, &item.Status, &item.CreatedAt, &item.UpdatedAt,
			&item.PoolID, &item.PoolHealth, &checkedAt, &item.PoolFailures,
			&item.GrokQualityStatus, &grokCheckedAt, &grokHTTP, &grokMessage, &item.AccountCount,
		); err != nil {
			return nil, err
		}
		if username.Valid {
			item.Username = username.String
		}
		if password.Valid {
			item.Password = password.String
		}
		if checkedAt.Valid {
			item.PoolCheckedAt = &checkedAt.Time
		}
		if grokCheckedAt.Valid {
			item.GrokQualityCheckedAt = &grokCheckedAt.Time
		}
		if grokHTTP.Valid {
			status := int(grokHTTP.Int64)
			item.GrokQualityHTTPStatus = &status
		}
		if grokMessage.Valid {
			item.GrokQualityMessage = grokMessage.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *proxyPoolRepository) AssignProxiesToPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if len(proxyIDs) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// If a proxy moves between pools, its old pool accounts must be reassigned.
	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts a
		SET proxy_id = NULL, updated_at = NOW()
		WHERE a.proxy_id = ANY($2) AND a.deleted_at IS NULL
			AND a.pool_id IS NOT NULL AND a.pool_id IS DISTINCT FROM $1
		RETURNING a.id
	`, poolID, pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	accountIDs, err := scanInt64Rows(rows)
	_ = rows.Close()
	if err != nil {
		return 0, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE proxies
		SET pool_id = $1, pool_health = 'unknown', pool_checked_at = NULL,
			pool_failures = 0, pool_grok_quality_status = 'unknown',
			pool_grok_quality_checked_at = NULL, pool_grok_quality_http_status = NULL,
			pool_grok_quality_message = NULL, updated_at = NOW()
		WHERE id = ANY($2) AND deleted_at IS NULL
	`, poolID, pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if len(accountIDs) > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{
			"account_ids": accountIDs,
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *proxyPoolRepository) RemoveProxiesFromPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if len(proxyIDs) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Accounts stay bound to the pool and are picked up by the next assignment pass.
	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts SET proxy_id = NULL, updated_at = NOW()
		WHERE pool_id = $1 AND proxy_id = ANY($2) AND deleted_at IS NULL
		RETURNING id
	`, poolID, pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	accountIDs, err := scanInt64Rows(rows)
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE proxies
		SET pool_id = NULL, pool_health = 'unknown', pool_checked_at = NULL,
			pool_failures = 0, pool_grok_quality_status = 'unknown',
			pool_grok_quality_checked_at = NULL, pool_grok_quality_http_status = NULL,
			pool_grok_quality_message = NULL, updated_at = NOW()
		WHERE pool_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`, poolID, pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if len(accountIDs) > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{
			"account_ids": accountIDs,
			"pool_id":     poolID,
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *proxyPoolRepository) UpdateProxyPoolHealth(ctx context.Context, poolID, proxyID int64, snapshot service.ProxyPoolHealthSnapshot) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE proxies
		SET pool_health = $3, pool_failures = $4, pool_checked_at = $5,
			pool_grok_quality_status = $6, pool_grok_quality_checked_at = $7,
			pool_grok_quality_http_status = $8, pool_grok_quality_message = $9,
			updated_at = NOW()
		WHERE id = $1 AND pool_id = $2 AND deleted_at IS NULL
	`, proxyID, poolID, snapshot.Health, snapshot.Failures, snapshot.CheckedAt,
		snapshot.GrokQualityStatus, snapshot.GrokQualityCheckedAt,
		snapshot.GrokQualityHTTPStatus, snapshot.GrokQualityMessage)
	return err
}

func (r *proxyPoolRepository) ListPoolUnassignedAccountIDs(ctx context.Context, poolID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id
		FROM accounts a
		LEFT JOIN proxies p ON p.id = a.proxy_id AND p.deleted_at IS NULL
		WHERE a.pool_id = $1 AND a.deleted_at IS NULL
			AND (a.proxy_id IS NULL OR p.pool_id IS DISTINCT FROM $1)
		ORDER BY a.id ASC
	`, poolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanInt64Rows(rows)
}

func (r *proxyPoolRepository) ListAccountIDsByProxy(ctx context.Context, poolID, proxyID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM accounts
		WHERE pool_id = $1 AND proxy_id = $2 AND deleted_at IS NULL
		ORDER BY id ASC
	`, poolID, proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanInt64Rows(rows)
}

func scanInt64Rows(rows *sql.Rows) ([]int64, error) {
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *proxyPoolRepository) CountAccountsByProxyIDs(ctx context.Context, proxyIDs []int64) (map[int64]int64, error) {
	counts := make(map[int64]int64, len(proxyIDs))
	if len(proxyIDs) == 0 {
		return counts, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT proxy_id, COUNT(*)
		FROM accounts
		WHERE proxy_id = ANY($1) AND deleted_at IS NULL
		GROUP BY proxy_id
	`, pq.Array(proxyIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var proxyID, count int64
		if err := rows.Scan(&proxyID, &count); err != nil {
			return nil, err
		}
		counts[proxyID] = count
	}
	return counts, rows.Err()
}

func (r *proxyPoolRepository) BindAccountsToPool(ctx context.Context, poolID int64, assignments []service.ProxyPoolAccountAssignment) ([]service.ProxyPoolAccountAssignment, error) {
	if len(assignments) == 0 {
		return []service.ProxyPoolAccountAssignment{}, nil
	}
	grouped := make(map[int64][]int64)
	for _, assignment := range assignments {
		grouped[assignment.ProxyID] = append(grouped[assignment.ProxyID], assignment.AccountID)
	}
	proxyIDs := make([]int64, 0, len(grouped))
	for proxyID := range grouped {
		proxyIDs = append(proxyIDs, proxyID)
	}
	sort.Slice(proxyIDs, func(i, j int) bool { return proxyIDs[i] < proxyIDs[j] })

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	applied := make([]service.ProxyPoolAccountAssignment, 0, len(assignments))
	for _, proxyID := range proxyIDs {
		rows, err := tx.QueryContext(ctx, `
			UPDATE accounts a
			SET pool_id = $1, proxy_id = $2, updated_at = NOW()
			WHERE a.id = ANY($3) AND a.deleted_at IS NULL
				AND EXISTS (
					SELECT 1 FROM proxies p
					WHERE p.id = $2 AND p.pool_id = $1 AND p.deleted_at IS NULL
						AND p.status = 'active' AND p.pool_health = 'healthy'
						AND p.pool_grok_quality_status = 'pass'
				)
			RETURNING a.id
		`, poolID, proxyID, pq.Array(grouped[proxyID]))
		if err != nil {
			return nil, err
		}
		ids, scanErr := scanInt64Rows(rows)
		_ = rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		for _, id := range ids {
			applied = append(applied, service.ProxyPoolAccountAssignment{AccountID: id, ProxyID: proxyID})
		}
	}
	if len(applied) > 0 {
		accountIDs := make([]int64, 0, len(applied))
		for _, assignment := range applied {
			accountIDs = append(accountIDs, assignment.AccountID)
		}
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{"account_ids": accountIDs, "pool_id": poolID}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	sort.Slice(applied, func(i, j int) bool { return applied[i].AccountID < applied[j].AccountID })
	return applied, nil
}

func (r *proxyPoolRepository) MarkAccountsPendingInPool(ctx context.Context, poolID int64, accountIDs []int64) ([]int64, error) {
	if len(accountIDs) == 0 {
		return []int64{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Keep the current proxy until a healthy pool member is available. The
	// regular assignment pass recognizes NULL or out-of-pool proxy IDs as pending.
	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts
		SET pool_id = $1, updated_at = NOW()
		WHERE id = ANY($2) AND deleted_at IS NULL
		RETURNING id
	`, poolID, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	appliedIDs, err := scanInt64Rows(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(appliedIDs) > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{
			"account_ids": appliedIDs,
			"pool_id":     poolID,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	sort.Slice(appliedIDs, func(i, j int) bool { return appliedIDs[i] < appliedIDs[j] })
	return appliedIDs, nil
}

func (r *proxyPoolRepository) UnbindAccountsFromPool(ctx context.Context, poolID int64, accountIDs []int64) (int64, error) {
	if len(accountIDs) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts
		SET pool_id = NULL, updated_at = NOW()
		WHERE pool_id = $1 AND id = ANY($2) AND deleted_at IS NULL
		RETURNING id
	`, poolID, pq.Array(accountIDs))
	if err != nil {
		return 0, err
	}
	appliedIDs, err := scanInt64Rows(rows)
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	if len(appliedIDs) > 0 {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, map[string]any{"account_ids": appliedIDs}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(appliedIDs)), nil
}

func (r *proxyPoolRepository) RecordRebindLog(ctx context.Context, entry *service.ProxyPoolRebindLog) error {
	if entry == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO proxy_pool_rebind_logs (pool_id, from_proxy_id, to_proxy_id, account_count, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, entry.PoolID, entry.FromProxyID, entry.ToProxyID, entry.AccountCount, entry.Reason)
	return err
}

func (r *proxyPoolRepository) ListRebindLogs(ctx context.Context, poolID int64, limit int) ([]service.ProxyPoolRebindLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT l.id, l.pool_id, l.from_proxy_id, l.to_proxy_id,
			COALESCE(fp.name, ''), COALESCE(tp.name, ''),
			l.account_count, l.reason, l.created_at
		FROM proxy_pool_rebind_logs l
		LEFT JOIN proxies fp ON fp.id = l.from_proxy_id
		LEFT JOIN proxies tp ON tp.id = l.to_proxy_id
		WHERE l.pool_id = $1
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $2
	`, poolID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]service.ProxyPoolRebindLog, 0)
	for rows.Next() {
		var (
			entry service.ProxyPoolRebindLog
			from  sql.NullInt64
			to    sql.NullInt64
		)
		if err := rows.Scan(
			&entry.ID, &entry.PoolID, &from, &to, &entry.FromProxyName, &entry.ToProxyName,
			&entry.AccountCount, &entry.Reason, &entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		if from.Valid {
			entry.FromProxyID = &from.Int64
		}
		if to.Valid {
			entry.ToProxyID = &to.Int64
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (r *proxyPoolRepository) String() string {
	return fmt.Sprintf("proxyPoolRepository(%p)", r)
}
