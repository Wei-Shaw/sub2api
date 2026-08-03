package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/ent/proxypool"
	"github.com/Wei-Shaw/sub2api/internal/service"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/lib/pq"
)

// proxyPoolRepository 代理池数据访问实现。
// 读取走 ent，写入（健康字段更新、账号改投）走原生 SQL，
// 与 proxyRepository 保持一致的事务边界风格。
type proxyPoolRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewProxyPoolRepository(client *dbent.Client, sqlDB *sql.DB) service.ProxyPoolRepository {
	return &proxyPoolRepository{client: client, sql: sqlDB}
}

func proxyPoolEntityToService(m *dbent.ProxyPool) *service.ProxyPool {
	if m == nil {
		return nil
	}
	return &service.ProxyPool{
		ID:                    m.ID,
		Name:                  m.Name,
		Description:           m.Description,
		Status:                m.Status,
		HealthIntervalSeconds: m.HealthIntervalSeconds,
		FailureThreshold:      m.FailureThreshold,
		AutoRebind:            m.AutoRebind,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
}

func (r *proxyPoolRepository) CreatePool(ctx context.Context, pool *service.ProxyPool) (*service.ProxyPool, error) {
	created, err := r.client.ProxyPool.Create().
		SetName(pool.Name).
		SetStatus(pool.Status).
		SetHealthIntervalSeconds(pool.HealthIntervalSeconds).
		SetFailureThreshold(pool.FailureThreshold).
		SetAutoRebind(pool.AutoRebind).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return proxyPoolEntityToService(created), nil
}

func (r *proxyPoolRepository) GetPoolByID(ctx context.Context, id int64) (*service.ProxyPool, error) {
	m, err := r.client.ProxyPool.Query().
		Where(proxypool.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return proxyPoolEntityToService(m), nil
}

func (r *proxyPoolRepository) ListPools(ctx context.Context) ([]service.ProxyPool, error) {
	pools, err := r.client.ProxyPool.Query().
		Order(dbent.Asc(proxypool.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.ProxyPool, 0, len(pools))
	for i := range pools {
		out = append(out, *proxyPoolEntityToService(pools[i]))
	}
	return out, nil
}

func (r *proxyPoolRepository) ListPoolsWithStats(ctx context.Context) ([]service.ProxyPoolWithStats, error) {
	pools, err := r.client.ProxyPool.Query().
		Order(dbent.Asc(proxypool.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.ProxyPoolWithStats, 0, len(pools))
	for i := range pools {
		base := proxyPoolEntityToService(pools[i])
		if base == nil {
			continue
		}
		out = append(out, service.ProxyPoolWithStats{ProxyPool: *base})
	}
	if len(out) == 0 {
		return out, nil
	}

	// 一次 SQL 汇总所有池的统计（代理健康分布 + 绑定账号数）
	rows, err := r.sql.QueryContext(ctx, `
		SELECT p.pool_id,
		       COUNT(*)::bigint                                              AS proxy_count,
		       COUNT(*) FILTER (WHERE p.pool_health = 'healthy')::bigint     AS healthy_count,
		       COUNT(*) FILTER (WHERE p.pool_health = 'unhealthy')::bigint   AS unhealthy_count,
		       COUNT(*) FILTER (WHERE p.pool_health = 'unknown')::bigint     AS unknown_count,
		       COALESCE(SUM(a.ct), 0)::bigint                                AS bound_account_sum
		FROM proxies p
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS ct FROM accounts a
			WHERE a.proxy_id = p.id AND a.deleted_at IS NULL
		) a ON TRUE
		WHERE p.pool_id IS NOT NULL AND p.deleted_at IS NULL
		GROUP BY p.pool_id`)
	if err != nil {
		return nil, fmt.Errorf("list proxy pool stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type poolStat struct {
		poolID          int64
		proxyCount      int64
		healthyCount    int64
		unhealthyCount  int64
		unknownCount    int64
		boundAccountSum int64
	}
	statsByPool := make(map[int64]poolStat)
	for rows.Next() {
		var s poolStat
		if err := rows.Scan(&s.poolID, &s.proxyCount, &s.healthyCount, &s.unhealthyCount, &s.unknownCount, &s.boundAccountSum); err != nil {
			return nil, err
		}
		statsByPool[s.poolID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if s, ok := statsByPool[out[i].ID]; ok {
			out[i].ProxyCount = s.proxyCount
			out[i].HealthyCount = s.healthyCount
			out[i].UnhealthyCount = s.unhealthyCount
			out[i].UnknownCount = s.unknownCount
			out[i].BoundAccountSum = s.boundAccountSum
		}
	}
	return out, nil
}

func (r *proxyPoolRepository) UpdatePool(ctx context.Context, pool *service.ProxyPool) error {
	builder := r.client.ProxyPool.UpdateOneID(pool.ID).
		SetName(pool.Name).
		SetStatus(pool.Status).
		SetHealthIntervalSeconds(pool.HealthIntervalSeconds).
		SetFailureThreshold(pool.FailureThreshold).
		SetAutoRebind(pool.AutoRebind)
	if pool.Description != nil {
		builder.SetDescription(*pool.Description)
	} else {
		builder.ClearDescription()
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *proxyPoolRepository) DeletePool(ctx context.Context, id int64) error {
	// 注意：ent 的 Delete() 走 SoftDeleteMixin，是软删除（UPDATE deleted_at），
	// 已删除行的 name 仍占用 UNIQUE 约束，重建同名池会违反唯一约束。
	// 管理端无回收站语义（列表自动过滤已删行），这里用原生 SQL 硬删；
	// proxies.pool_id 为 ON DELETE SET NULL、rebind_logs.pool_id 为 ON DELETE
	// CASCADE，硬删安全。
	_, err := r.sql.ExecContext(ctx, `DELETE FROM proxy_pools WHERE id = $1`, id)
	return err
}

func (r *proxyPoolRepository) ListPoolProxies(ctx context.Context, poolID int64) ([]service.Proxy, error) {
	proxies, err := r.client.Proxy.Query().
		Where(proxy.PoolID(poolID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *proxyEntityToService(proxies[i]))
	}
	return out, nil
}

func (r *proxyPoolRepository) AssignProxiesToPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if len(proxyIDs) == 0 {
		return 0, nil
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE proxies
		SET pool_id = $1, updated_at = NOW()
		WHERE id = ANY($2::bigint[]) AND deleted_at IS NULL`,
		poolID, pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *proxyPoolRepository) RemoveProxiesFromPool(ctx context.Context, proxyIDs []int64) (int64, error) {
	if len(proxyIDs) == 0 {
		return 0, nil
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE proxies
		SET pool_id = NULL, updated_at = NOW()
		WHERE id = ANY($1::bigint[]) AND deleted_at IS NULL`,
		pq.Array(proxyIDs))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *proxyPoolRepository) UpdateProxyPoolHealth(ctx context.Context, proxyID int64, health string, failures int, checkedAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE proxies
		SET pool_health = $1, pool_failures = $2, pool_checked_at = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL`,
		health, failures, checkedAt, proxyID)
	return err
}

func (r *proxyPoolRepository) CountAccountsByProxyIDs(ctx context.Context, proxyIDs []int64) (map[int64]int64, error) {
	if len(proxyIDs) == 0 {
		return map[int64]int64{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT proxy_id, COUNT(*)::bigint
		FROM accounts
		WHERE proxy_id = ANY($1::bigint[]) AND deleted_at IS NULL
		GROUP BY proxy_id`, pq.Array(proxyIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[int64]int64, len(proxyIDs))
	for rows.Next() {
		var proxyID, count int64
		if err := rows.Scan(&proxyID, &count); err != nil {
			return nil, err
		}
		counts[proxyID] = count
	}
	return counts, rows.Err()
}

// ListPoolUnassignedAccountIDs 返回绑定该池但 proxy_id 为空或 proxy 不属于该池的账号 ID。
func (r *proxyPoolRepository) ListPoolUnassignedAccountIDs(ctx context.Context, poolID int64) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT a.id
		FROM accounts a
		WHERE a.pool_id = $1 AND a.deleted_at IS NULL
		  AND (a.proxy_id IS NULL OR NOT EXISTS (
		      SELECT 1 FROM proxies p
		      WHERE p.id = a.proxy_id AND p.pool_id = $1 AND p.deleted_at IS NULL
		  ))
		ORDER BY a.id`, poolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AssignAccountToProxy 把单个账号改投到指定代理（池服务分配用）。
func (r *proxyPoolRepository) AssignAccountToProxy(ctx context.Context, accountID int64, proxyID int64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET proxy_id = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL`, proxyID, accountID)
	return err
}

// RebindAccountsOffProxy 把绑定在 fromProxyID 上的活跃账号改投到 toProxyID
// （nil 表示直连），并记录 fallback origin 供手动回切（已有 origin 的不覆盖）。
// 事务内完成改投 + 探针快照失效 + 调度器 outbox，返回受影响账号 ID 列表。
func (r *proxyPoolRepository) RebindAccountsOffProxy(ctx context.Context, fromProxyID int64, toProxyID *int64) ([]int64, error) {
	tx, txErr := r.client.Tx(ctx)
	if txErr != nil {
		if txErr != dbent.ErrTxStarted {
			return nil, txErr
		}
		return r.rebindAccountsOffProxyOnExec(ctx, r.sql, fromProxyID, toProxyID)
	}
	accountIDs, err := r.rebindAccountsOffProxyOnExec(ctx, tx, fromProxyID, toProxyID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

func (r *proxyPoolRepository) rebindAccountsOffProxyOnExec(ctx context.Context, exec sqlExecutor, fromProxyID int64, toProxyID *int64) ([]int64, error) {
	query := `
		UPDATE accounts SET proxy_id = $2,
			proxy_fallback_origin_id = COALESCE(proxy_fallback_origin_id, $1),
			extra = CASE
				WHEN platform='openai' AND type='apikey' AND extra ? 'upstream_billing_probe'
				THEN extra - 'upstream_billing_probe'
				ELSE extra
			END,
			updated_at = NOW()
		WHERE proxy_id = $1 AND deleted_at IS NULL
		RETURNING id`
	var rows *sql.Rows
	var err error
	if toProxyID == nil {
		rows, err = exec.QueryContext(ctx, query, fromProxyID, nil)
	} else {
		rows, err = exec.QueryContext(ctx, query, fromProxyID, *toProxyID)
	}
	if err != nil {
		return nil, err
	}
	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if len(accountIDs) > 0 {
		// 探针快照失效 + 调度器 outbox（与 SweepExpiredProxies 一致）
		probeAccountIDs, invErr := invalidateProxyProbeSnapshots(ctx, exec, fromProxyID)
		if invErr != nil {
			return nil, invErr
		}
		if enqErr := enqueueProxyProbeAccountChanges(ctx, exec, probeAccountIDs); enqErr != nil {
			return nil, enqErr
		}
		payload := map[string]any{"account_ids": sortedUniqueAccountIDs(accountIDs)}
		if enqErr := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); enqErr != nil {
			return nil, enqErr
		}
	}
	return accountIDs, nil
}

// RecordRebindLog 记录一次池重绑操作（审计/管理端展示）。
func (r *proxyPoolRepository) RecordRebindLog(ctx context.Context, entry *service.ProxyPoolRebindLog) error {
	if entry == nil {
		return nil
	}
	var toProxyID any
	if entry.ToProxyID != nil {
		toProxyID = *entry.ToProxyID
	}
	var fromProxyID any
	if entry.FromProxyID != nil {
		fromProxyID = *entry.FromProxyID
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO proxy_pool_rebind_logs (pool_id, from_proxy_id, to_proxy_id, account_count, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		entry.PoolID, fromProxyID, toProxyID, entry.AccountCount, entry.Reason)
	return err
}

// ListRebindLogs 返回池内最近的重绑日志（含代理名称，desc 排序）。
func (r *proxyPoolRepository) ListRebindLogs(ctx context.Context, poolID int64, limit int) ([]service.ProxyPoolRebindLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT l.id, l.pool_id, l.from_proxy_id, l.to_proxy_id, l.account_count, l.reason, l.created_at,
		       fp.name AS from_name, tp.name AS to_name
		FROM proxy_pool_rebind_logs l
		LEFT JOIN proxies fp ON fp.id = l.from_proxy_id
		LEFT JOIN proxies tp ON tp.id = l.to_proxy_id
		WHERE l.pool_id = $1
		ORDER BY l.id DESC
		LIMIT $2`, poolID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProxyPoolRebindLog, 0, limit)
	for rows.Next() {
		var (
			entry            service.ProxyPoolRebindLog
			fromID, toID     sql.NullInt64
			fromName, toName sql.NullString
		)
		if err := rows.Scan(&entry.ID, &entry.PoolID, &fromID, &toID, &entry.AccountCount, &entry.Reason, &entry.CreatedAt, &fromName, &toName); err != nil {
			return nil, err
		}
		if fromID.Valid {
			id := fromID.Int64
			entry.FromProxyID = &id
			entry.FromProxy = &service.Proxy{ID: id, Name: fromName.String}
		}
		if toID.Valid {
			id := toID.Int64
			entry.ToProxyID = &id
			entry.ToProxy = &service.Proxy{ID: id, Name: toName.String}
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}
