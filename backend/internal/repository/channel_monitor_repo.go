package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorhistory"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

// channelMonitorRepository 实现 service.ChannelMonitorRepository。
//
// 选型说明：
//   - CRUD 走 ent，复用项目的事务上下文支持
//   - 聚合查询（latest per model / availability）走原生 SQL，避免 ent 在 GROUP BY 上
//     的样板代码，并保证索引能被命中
type channelMonitorRepository struct {
	client *dbent.Client
	db     *sql.DB
}

// NewChannelMonitorRepository 创建仓储实例。
func NewChannelMonitorRepository(client *dbent.Client, db *sql.DB) service.ChannelMonitorRepository {
	return &channelMonitorRepository{client: client, db: db}
}

// ---------- CRUD ----------

func (r *channelMonitorRepository) Create(ctx context.Context, m *service.ChannelMonitor) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ChannelMonitor.Create().
		SetName(m.Name).
		SetProvider(channelmonitor.Provider(m.Provider)).
		SetAPIMode(defaultAPIModeRepo(m.APIMode)).
		SetEndpoint(m.Endpoint).
		SetAPIKeyEncrypted(m.APIKey). // 调用方传入的已是密文
		SetPrimaryModel(m.PrimaryModel).
		SetExtraModels(emptySliceIfNil(m.ExtraModels)).
		SetGroupName(m.GroupName).
		SetEnabled(m.Enabled).
		SetIntervalSeconds(m.IntervalSeconds).
		SetJitterSeconds(m.JitterSeconds).
		SetCreatedBy(m.CreatedBy).
		SetExtraHeaders(channelMonitorHeadersForPersistence(m)).
		SetBodyOverrideMode(defaultBodyModeRepo(m.BodyOverrideMode)).
		SetCheckMode(defaultCheckModeRepo(m.CheckMode))
	if m.TemplateID != nil {
		builder = builder.SetTemplateID(*m.TemplateID)
	}
	if m.AccountID != nil {
		builder = builder.SetAccountID(*m.AccountID)
	}
	if m.BodyOverride != nil {
		builder = builder.SetBodyOverride(m.BodyOverride)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	m.ID = created.ID
	m.CreatedAt = created.CreatedAt
	m.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *channelMonitorRepository) FindByDuplicateOperationID(ctx context.Context, operationID string) (*service.ChannelMonitor, error) {
	if strings.TrimSpace(operationID) == "" {
		return nil, nil
	}
	client := clientFromContext(ctx, r.client)
	row, err := client.ChannelMonitor.Query().
		Where(func(selector *entsql.Selector) {
			selector.Where(sqljson.ValueEQ(
				channelmonitor.FieldExtraHeaders,
				operationID,
				sqljson.Path(service.ChannelMonitorDuplicateOperationIDMetadataKey),
			))
		}).
		Order(dbent.Asc(channelmonitor.FieldID)).
		First(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find channel monitor duplicate operation: %w", err)
	}
	return entToServiceMonitor(row), nil
}

func (r *channelMonitorRepository) GetByID(ctx context.Context, id int64) (*service.ChannelMonitor, error) {
	row, err := r.client.ChannelMonitor.Query().
		Where(channelmonitor.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	return entToServiceMonitor(row), nil
}

func (r *channelMonitorRepository) Update(ctx context.Context, m *service.ChannelMonitor) error {
	client := clientFromContext(ctx, r.client)
	updater := client.ChannelMonitor.UpdateOneID(m.ID).
		SetName(m.Name).
		SetProvider(channelmonitor.Provider(m.Provider)).
		SetAPIMode(defaultAPIModeRepo(m.APIMode)).
		SetEndpoint(m.Endpoint).
		SetAPIKeyEncrypted(m.APIKey).
		SetPrimaryModel(m.PrimaryModel).
		SetExtraModels(emptySliceIfNil(m.ExtraModels)).
		SetGroupName(m.GroupName).
		SetEnabled(m.Enabled).
		SetIntervalSeconds(m.IntervalSeconds).
		SetJitterSeconds(m.JitterSeconds).
		SetExtraHeaders(channelMonitorHeadersForPersistence(m)).
		SetBodyOverrideMode(defaultBodyModeRepo(m.BodyOverrideMode)).
		SetCheckMode(defaultCheckModeRepo(m.CheckMode))
	if m.TemplateID != nil {
		updater = updater.SetTemplateID(*m.TemplateID)
	} else {
		updater = updater.ClearTemplateID()
	}
	if m.AccountID != nil {
		updater = updater.SetAccountID(*m.AccountID)
	} else {
		updater = updater.ClearAccountID()
	}
	if m.BodyOverride != nil {
		updater = updater.SetBodyOverride(m.BodyOverride)
	} else {
		updater = updater.ClearBodyOverride()
	}

	updated, err := updater.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	m.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *channelMonitorRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	if err := client.ChannelMonitor.DeleteOneID(id).Exec(ctx); err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	return nil
}

func (r *channelMonitorRepository) List(ctx context.Context, params service.ChannelMonitorListParams) ([]*service.ChannelMonitor, int64, error) {
	q := r.client.ChannelMonitor.Query()
	if params.Provider != "" {
		q = q.Where(channelmonitor.ProviderEQ(channelmonitor.Provider(params.Provider)))
	}
	if params.Enabled != nil {
		q = q.Where(channelmonitor.EnabledEQ(*params.Enabled))
	}
	if s := strings.TrimSpace(params.Search); s != "" {
		q = q.Where(channelmonitor.Or(
			channelmonitor.NameContainsFold(s),
			channelmonitor.GroupNameContainsFold(s),
			channelmonitor.PrimaryModelContainsFold(s),
		))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count monitors: %w", err)
	}

	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}

	rows, err := q.
		Order(dbent.Desc(channelmonitor.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list monitors: %w", err)
	}

	out := make([]*service.ChannelMonitor, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToServiceMonitor(row))
	}
	return out, int64(total), nil
}

// ---------- 调度器辅助 ----------

func (r *channelMonitorRepository) ListEnabled(ctx context.Context) ([]*service.ChannelMonitor, error) {
	rows, err := r.client.ChannelMonitor.Query().
		Where(channelmonitor.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors: %w", err)
	}
	out := make([]*service.ChannelMonitor, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToServiceMonitor(row))
	}
	return out, nil
}

func (r *channelMonitorRepository) MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	if err := client.ChannelMonitor.UpdateOneID(id).
		SetLastCheckedAt(checkedAt).
		Exec(ctx); err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	return nil
}

func (r *channelMonitorRepository) InsertHistoryBatch(ctx context.Context, rows []*service.ChannelMonitorHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	bulk := make([]*dbent.ChannelMonitorHistoryCreate, 0, len(rows))
	for _, row := range rows {
		c := client.ChannelMonitorHistory.Create().
			SetMonitorID(row.MonitorID).
			SetModel(row.Model).
			SetStatus(channelmonitorhistory.Status(row.Status)).
			SetMessage(row.Message).
			SetCheckedAt(row.CheckedAt)
		if row.LatencyMs != nil {
			c = c.SetLatencyMs(*row.LatencyMs)
		}
		if row.PingLatencyMs != nil {
			c = c.SetPingLatencyMs(*row.PingLatencyMs)
		}
		if row.Quota != nil {
			c = c.SetQuota(row.Quota)
		}
		bulk = append(bulk, c)
	}
	if _, err := client.ChannelMonitorHistory.CreateBulk(bulk...).Save(ctx); err != nil {
		return fmt.Errorf("insert history bulk: %w", err)
	}
	return nil
}

// DeleteHistoryBefore 物理删 checked_at < before 的明细，分批 channelMonitorPruneBatchSize 行一批，
// 避免单事务删除过多引起锁/WAL 压力。借助 (checked_at) 索引定位小批 id，再按 id 删。
func (r *channelMonitorRepository) DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error) {
	query := channelMonitorPruneHistorySQL
	if IsMySQL() {
		query = channelMonitorPruneHistorySQLMySQL
	}
	return deleteChannelMonitorBatched(ctx, r.db, query, before)
}

// ListHistory 按 checked_at 倒序返回某个监控的最近 N 条历史记录。
// model 为空时不过滤；非空时只返回该模型的记录。
func (r *channelMonitorRepository) ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*service.ChannelMonitorHistoryEntry, error) {
	q := r.client.ChannelMonitorHistory.Query().
		Where(channelmonitorhistory.MonitorIDEQ(monitorID))
	if strings.TrimSpace(model) != "" {
		q = q.Where(channelmonitorhistory.ModelEQ(model))
	}
	rows, err := q.
		Order(dbent.Desc(channelmonitorhistory.FieldCheckedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	out := make([]*service.ChannelMonitorHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entry := &service.ChannelMonitorHistoryEntry{
			ID:            row.ID,
			Model:         row.Model,
			Status:        string(row.Status),
			LatencyMs:     row.LatencyMs,
			PingLatencyMs: row.PingLatencyMs,
			Message:       row.Message,
			CheckedAt:     row.CheckedAt,
			Quota:         row.Quota,
		}
		out = append(out, entry)
	}
	return out, nil
}

// ---------- 用户视图聚合（原生 SQL） ----------

// ListLatestPerModel 用 DISTINCT ON 取每个 (monitor_id, model) 的最近一条记录。
// 借助 (monitor_id, model, checked_at DESC) 索引可走 Index Scan。
func (r *channelMonitorRepository) ListLatestPerModel(ctx context.Context, monitorID int64) ([]*service.ChannelMonitorLatest, error) {
	if IsMySQL() {
		return r.listLatestPerModelMySQL(ctx, monitorID)
	}
	const q = `
		SELECT DISTINCT ON (model)
		    model, status, latency_ms, ping_latency_ms, checked_at
		FROM channel_monitor_histories
		WHERE monitor_id = $1
		ORDER BY model, checked_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q, monitorID)
	if err != nil {
		return nil, fmt.Errorf("query latest per model: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.ChannelMonitorLatest, 0)
	for rows.Next() {
		l := &service.ChannelMonitorLatest{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&l.Model, &l.Status, &latency, &ping, &l.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest row: %w", err)
		}
		assignNullInt(&l.LatencyMs, latency)
		assignNullInt(&l.PingLatencyMs, ping)
		out = append(out, l)
	}
	return out, rows.Err()
}

// listLatestPerModelMySQL 是 ListLatestPerModel 的 MySQL 实现：
// DISTINCT ON (model) ... ORDER BY model, checked_at DESC →
// ROW_NUMBER() OVER (PARTITION BY model ORDER BY checked_at DESC) 取 rn=1（规范：DISTINCT ON → 窗口函数）。
func (r *channelMonitorRepository) listLatestPerModelMySQL(ctx context.Context, monitorID int64) ([]*service.ChannelMonitorLatest, error) {
	const q = `
		SELECT model, status, latency_ms, ping_latency_ms, checked_at
		FROM (
		    SELECT model, status, latency_ms, ping_latency_ms, checked_at,
		           ROW_NUMBER() OVER (PARTITION BY model ORDER BY checked_at DESC) AS rn
		    FROM channel_monitor_histories
		    WHERE monitor_id = ?
		) ranked
		WHERE rn = 1
	`
	rows, err := r.db.QueryContext(ctx, q, monitorID)
	if err != nil {
		return nil, fmt.Errorf("query latest per model: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.ChannelMonitorLatest, 0)
	for rows.Next() {
		l := &service.ChannelMonitorLatest{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&l.Model, &l.Status, &latency, &ping, &l.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest row: %w", err)
		}
		assignNullInt(&l.LatencyMs, latency)
		assignNullInt(&l.PingLatencyMs, ping)
		out = append(out, l)
	}
	return out, rows.Err()
}

// assignNullInt 把 sql.NullInt64 解包到 *int 指针目标（valid 才分配新 int）。
// 集中实现避免 latency / ping 两处重复 if latency.Valid { v := int(...) ... } 模板。
func assignNullInt(dst **int, n sql.NullInt64) {
	if !n.Valid {
		return
	}
	v := int(n.Int64)
	*dst = &v
}

// scanMonitorQuota 把裸 SQL 读出的 JSONB quota 列解包为配额快照。
// NULL（探活模式旧行）返回 nil；解析失败也返回 nil 并由调用方日志感知，
// 不阻断列表渲染（与聚合层"失败仅日志"的原则一致）。
func scanMonitorQuota(data []byte) *domain.MonitorQuotaSnapshot {
	if len(data) == 0 {
		return nil
	}
	snapshot := &domain.MonitorQuotaSnapshot{}
	if err := json.Unmarshal(data, snapshot); err != nil {
		return nil
	}
	return snapshot
}

// ComputeAvailability 计算指定窗口内每个模型的可用率与平均延迟。
// "可用" = status IN (operational, degraded)。
//
// 数据来源：明细表只保留 1 天；窗口前其余天数走聚合表。
// 明细保留 30 天（monitorHistoryRetentionDays），窗口 <= 30 天时直接扫 histories，
// 精度到秒，避免与聚合表 UNION 带来的 UTC 日切精度损失。
func (r *channelMonitorRepository) ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*service.ChannelMonitorAvailability, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	if IsMySQL() {
		return r.computeAvailabilityMySQL(ctx, monitorID, windowDays)
	}
	const q = `
		SELECT model,
		       COUNT(*)                                                             AS total,
		       COUNT(*) FILTER (WHERE status IN ('operational','degraded'))         AS ok,
		       CASE WHEN COUNT(latency_ms) > 0
		            THEN SUM(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)::float8 / COUNT(latency_ms)
		            ELSE NULL END                                                   AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id = $1
		  AND checked_at >= NOW() - ($2::int || ' days')::interval
		GROUP BY model
	`
	rows, err := r.db.QueryContext(ctx, q, monitorID, windowDays)
	if err != nil {
		return nil, fmt.Errorf("query availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.ChannelMonitorAvailability, 0)
	for rows.Next() {
		row, err := scanAvailabilityRow(rows, windowDays)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// computeAvailabilityMySQL 是 ComputeAvailability 的 MySQL 实现：
//   - COUNT(*) FILTER → SUM(CASE WHEN cond THEN 1 ELSE 0 END)；
//   - SUM(latency_ms) FILTER (...)::float8 / COUNT(latency_ms) →
//     SUM(CASE WHEN latency_ms IS NOT NULL THEN latency_ms END) / COUNT(latency_ms)
//     （整型相除 MySQL 返回 DECIMAL，database/sql 扫描到 NullFloat64 自动转换）；
//   - NOW() - ($2::int || ' days')::interval → NOW(3) - INTERVAL ? DAY；$n → ?。
func (r *channelMonitorRepository) computeAvailabilityMySQL(ctx context.Context, monitorID int64, windowDays int) ([]*service.ChannelMonitorAvailability, error) {
	const q = `
		SELECT model,
		       COUNT(*)                                                             AS total,
		       SUM(CASE WHEN status IN ('operational','degraded') THEN 1 ELSE 0 END) AS ok,
		       CASE WHEN COUNT(latency_ms) > 0
		            THEN SUM(CASE WHEN latency_ms IS NOT NULL THEN latency_ms END) / COUNT(latency_ms)
		            ELSE NULL END                                                   AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id = ?
		  AND checked_at >= NOW(3) - INTERVAL ? DAY
		GROUP BY model
	`
	rows, err := r.db.QueryContext(ctx, q, monitorID, windowDays)
	if err != nil {
		return nil, fmt.Errorf("query availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.ChannelMonitorAvailability, 0)
	for rows.Next() {
		row, err := scanAvailabilityRow(rows, windowDays)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// scanAvailabilityRow 把单行 (model, total, ok, avg_latency) 扫描为 ChannelMonitorAvailability。
// 仅服务于 ComputeAvailability（4 列）；批量版本因为多一列 monitor_id 直接 inline 调 finalizeAvailabilityRow。
func scanAvailabilityRow(rows interface{ Scan(...any) error }, windowDays int) (*service.ChannelMonitorAvailability, error) {
	row := &service.ChannelMonitorAvailability{WindowDays: windowDays}
	var avgLatency sql.NullFloat64
	if err := rows.Scan(&row.Model, &row.TotalChecks, &row.OperationalChecks, &avgLatency); err != nil {
		return nil, fmt.Errorf("scan availability row: %w", err)
	}
	finalizeAvailabilityRow(row, avgLatency)
	return row, nil
}

// finalizeAvailabilityRow 根据 OperationalChecks/TotalChecks 算出可用率，
// 并把 sql.NullFloat64 的平均延迟解包为 *int。两处复用避免维护漂移。
func finalizeAvailabilityRow(row *service.ChannelMonitorAvailability, avgLatency sql.NullFloat64) {
	if row.TotalChecks > 0 {
		row.AvailabilityPct = float64(row.OperationalChecks) * 100.0 / float64(row.TotalChecks)
	}
	if avgLatency.Valid {
		v := int(avgLatency.Float64)
		row.AvgLatencyMs = &v
	}
}

// ListLatestForMonitorIDs 一次性查询多个监控的"每个 (monitor_id, model) 最近一条"记录。
// 利用 PG 的 DISTINCT ON 特性，借助 (monitor_id, model, checked_at DESC) 索引可走 Index Scan。
func (r *channelMonitorRepository) ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*service.ChannelMonitorLatest, error) {
	out := make(map[int64][]*service.ChannelMonitorLatest, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if IsMySQL() {
		return r.listLatestForMonitorIDsMySQL(ctx, ids)
	}
	const q = `
		SELECT DISTINCT ON (monitor_id, model)
		    monitor_id, model, status, latency_ms, ping_latency_ms, checked_at, quota
		FROM channel_monitor_histories
		WHERE monitor_id = ANY($1)
		ORDER BY monitor_id, model, checked_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query latest batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		l := &service.ChannelMonitorLatest{}
		var latency, ping sql.NullInt64
		var quota []byte
		if err := rows.Scan(&monitorID, &l.Model, &l.Status, &latency, &ping, &l.CheckedAt, &quota); err != nil {
			return nil, fmt.Errorf("scan latest batch row: %w", err)
		}
		assignNullInt(&l.LatencyMs, latency)
		assignNullInt(&l.PingLatencyMs, ping)
		l.Quota = scanMonitorQuota(quota)
		out[monitorID] = append(out[monitorID], l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// listLatestForMonitorIDsMySQL 是 ListLatestForMonitorIDs 的 MySQL 实现：
// DISTINCT ON (monitor_id, model) → ROW_NUMBER() OVER (PARTITION BY monitor_id, model
// ORDER BY checked_at DESC) 取 rn=1；`= ANY($1)` + pq.Array → IN (?,?,...) 展开。
func (r *channelMonitorRepository) listLatestForMonitorIDsMySQL(ctx context.Context, ids []int64) (map[int64][]*service.ChannelMonitorLatest, error) {
	out := make(map[int64][]*service.ChannelMonitorLatest, len(ids))
	q := `
		SELECT monitor_id, model, status, latency_ms, ping_latency_ms, checked_at, quota
		FROM (
		    SELECT monitor_id, model, status, latency_ms, ping_latency_ms, checked_at, quota,
		           ROW_NUMBER() OVER (PARTITION BY monitor_id, model ORDER BY checked_at DESC) AS rn
		    FROM channel_monitor_histories
		    WHERE monitor_id IN (` + Placeholders(len(ids)) + `)
		) ranked
		WHERE rn = 1
		ORDER BY monitor_id, model, checked_at DESC
	`
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query latest batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		l := &service.ChannelMonitorLatest{}
		var latency, ping sql.NullInt64
		var quota []byte
		if err := rows.Scan(&monitorID, &l.Model, &l.Status, &latency, &ping, &l.CheckedAt, &quota); err != nil {
			return nil, fmt.Errorf("scan latest batch row: %w", err)
		}
		assignNullInt(&l.LatencyMs, latency)
		assignNullInt(&l.PingLatencyMs, ping)
		l.Quota = scanMonitorQuota(quota)
		out[monitorID] = append(out[monitorID], l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListRecentHistoryForMonitors 为多个 monitor 批量取各自"指定模型"最近 N 条历史（按 checked_at DESC，最新在前）。
// primaryModels[monitorID] 指定该监控要过滤的模型名；monitor 不在 primaryModels 中的记录不返回。
// 通过 CTE + unnest(两个 int8/text 数组) 构造 (monitor_id, model) 白名单，
// 再用 ROW_NUMBER() OVER (PARTITION BY monitor_id) 取各自前 N 条。
//
// 返回值：map[monitorID] -> []*ChannelMonitorHistoryEntry（不含 message，减少网络开销）。
// 空 ids / 空 primaryModels 返回空 map，不报错。
func (r *channelMonitorRepository) ListRecentHistoryForMonitors(
	ctx context.Context,
	ids []int64,
	primaryModels map[int64]string,
	perMonitorLimit int,
) (map[int64][]*service.ChannelMonitorHistoryEntry, error) {
	out := make(map[int64][]*service.ChannelMonitorHistoryEntry, len(ids))
	pairIDs, pairModels := buildMonitorModelPairs(ids, primaryModels)
	if len(pairIDs) == 0 {
		return out, nil
	}
	perMonitorLimit = clampTimelineLimit(perMonitorLimit)

	if IsMySQL() {
		return r.listRecentHistoryForMonitorsMySQL(ctx, pairIDs, pairModels, perMonitorLimit)
	}

	const q = `
		WITH targets AS (
		    SELECT unnest($1::bigint[]) AS monitor_id,
		           unnest($2::text[])   AS model
		),
		ranked AS (
		    SELECT h.monitor_id,
		           h.status,
		           h.latency_ms,
		           h.ping_latency_ms,
		           h.checked_at,
		           ROW_NUMBER() OVER (PARTITION BY h.monitor_id ORDER BY h.checked_at DESC) AS rn
		    FROM channel_monitor_histories h
		    JOIN targets t
		      ON t.monitor_id = h.monitor_id AND t.model = h.model
		)
		SELECT monitor_id, status, latency_ms, ping_latency_ms, checked_at
		FROM ranked
		WHERE rn <= $3
		ORDER BY monitor_id, checked_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(pairIDs), pq.Array(pairModels), perMonitorLimit)
	if err != nil {
		return nil, fmt.Errorf("query recent history batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		entry := &service.ChannelMonitorHistoryEntry{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&monitorID, &entry.Status, &latency, &ping, &entry.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan recent history row: %w", err)
		}
		assignNullInt(&entry.LatencyMs, latency)
		assignNullInt(&entry.PingLatencyMs, ping)
		out[monitorID] = append(out[monitorID], entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// listRecentHistoryForMonitorsMySQL 是 ListRecentHistoryForMonitors 的 MySQL 实现：
// 配对数组 unnest($1::bigint[]) + unnest($2::text[])（逐位配对语义）→
// JSON_TABLE 展开单参 JSON 数组 [{monitor_id, model}, ...]，保证 (monitor_id, model)
// 配对不被 zip 错位；$n → ?。
func (r *channelMonitorRepository) listRecentHistoryForMonitorsMySQL(ctx context.Context, pairIDs []int64, pairModels []string, perMonitorLimit int) (map[int64][]*service.ChannelMonitorHistoryEntry, error) {
	out := make(map[int64][]*service.ChannelMonitorHistoryEntry, len(pairIDs))

	pairs := make([]channelMonitorMonitorModelPair, 0, len(pairIDs))
	for i, id := range pairIDs {
		pairs = append(pairs, channelMonitorMonitorModelPair{MonitorID: id, Model: pairModels[i]})
	}
	pairsJSON, err := json.Marshal(pairs)
	if err != nil {
		return nil, fmt.Errorf("marshal monitor model pairs: %w", err)
	}

	const q = `
		WITH targets AS (
		    SELECT monitor_id, model
		    FROM JSON_TABLE(?, '$[*]' COLUMNS (
		        monitor_id BIGINT PATH '$.monitor_id',
		        model VARCHAR(200) PATH '$.model'
		    )) t
		),
		ranked AS (
		    SELECT h.monitor_id,
		           h.status,
		           h.latency_ms,
		           h.ping_latency_ms,
		           h.checked_at,
		           ROW_NUMBER() OVER (PARTITION BY h.monitor_id ORDER BY h.checked_at DESC) AS rn
		    FROM channel_monitor_histories h
		    JOIN targets t
		      ON t.monitor_id = h.monitor_id AND t.model = h.model
		)
		SELECT monitor_id, status, latency_ms, ping_latency_ms, checked_at
		FROM ranked
		WHERE rn <= ?
		ORDER BY monitor_id, checked_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q, string(pairsJSON), perMonitorLimit)
	if err != nil {
		return nil, fmt.Errorf("query recent history batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		entry := &service.ChannelMonitorHistoryEntry{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&monitorID, &entry.Status, &latency, &ping, &entry.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan recent history row: %w", err)
		}
		assignNullInt(&entry.LatencyMs, latency)
		assignNullInt(&entry.PingLatencyMs, ping)
		out[monitorID] = append(out[monitorID], entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// channelMonitorMonitorModelPair 是 (monitor_id, model) 配对参数的 JSON 载体。
type channelMonitorMonitorModelPair struct {
	MonitorID int64  `json:"monitor_id"`
	Model     string `json:"model"`
}

// buildMonitorModelPairs 基于 ids 过滤出有效的 (monitor_id, model) 对，model 为空时跳过。
// 保证两个数组长度一致且一一对应，供 unnest 展开。
func buildMonitorModelPairs(ids []int64, primaryModels map[int64]string) ([]int64, []string) {
	if len(ids) == 0 || len(primaryModels) == 0 {
		return nil, nil
	}
	pairIDs := make([]int64, 0, len(ids))
	pairModels := make([]string, 0, len(ids))
	for _, id := range ids {
		model, ok := primaryModels[id]
		if !ok || strings.TrimSpace(model) == "" {
			continue
		}
		pairIDs = append(pairIDs, id)
		pairModels = append(pairModels, model)
	}
	return pairIDs, pairModels
}

// timelineLimit* 批量 timeline 查询的 perMonitorLimit 夹紧范围。
// 下限 1 表示至少返回最近一条；上限 200 控制单次响应体与 SQL 内存占用（ROW_NUMBER 窗口上限）。
const (
	timelineLimitMin = 1
	timelineLimitMax = 200
)

// clampTimelineLimit 把 perMonitorLimit 夹紧到 [timelineLimitMin, timelineLimitMax]，避免非法值或超大查询。
func clampTimelineLimit(n int) int {
	if n < timelineLimitMin {
		return timelineLimitMin
	}
	if n > timelineLimitMax {
		return timelineLimitMax
	}
	return n
}

// ComputeAvailabilityForMonitors 一次性计算多个监控在某个窗口内的每模型可用率与平均延迟。
// 明细保留 30 天，直接扫 histories（窗口 <= 30 天时无需聚合）。
func (r *channelMonitorRepository) ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	out := make(map[int64][]*service.ChannelMonitorAvailability, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	if IsMySQL() {
		return r.computeAvailabilityForMonitorsMySQL(ctx, ids, windowDays)
	}
	const q = `
		SELECT monitor_id,
		       model,
		       COUNT(*)                                                             AS total,
		       COUNT(*) FILTER (WHERE status IN ('operational','degraded'))         AS ok,
		       CASE WHEN COUNT(latency_ms) > 0
		            THEN SUM(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)::float8 / COUNT(latency_ms)
		            ELSE NULL END                                                   AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id = ANY($1)
		  AND checked_at >= NOW() - ($2::int || ' days')::interval
		GROUP BY monitor_id, model
	`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids), windowDays)
	if err != nil {
		return nil, fmt.Errorf("query availability batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		row := &service.ChannelMonitorAvailability{WindowDays: windowDays}
		var avgLatency sql.NullFloat64
		if err := rows.Scan(&monitorID, &row.Model, &row.TotalChecks, &row.OperationalChecks, &avgLatency); err != nil {
			return nil, fmt.Errorf("scan availability batch row: %w", err)
		}
		// 批量查询多了首列 monitor_id；其余字段的可用率/平均延迟换算与单 monitor 版本一致，
		// 抽出 finalizeAvailabilityRow 复用，避免两处分别维护除法与 NullFloat 解包。
		finalizeAvailabilityRow(row, avgLatency)
		out[monitorID] = append(out[monitorID], row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// computeAvailabilityForMonitorsMySQL 是 ComputeAvailabilityForMonitors 的 MySQL 实现，
// 改写规则同 computeAvailabilityMySQL；`= ANY($1)` → IN (?,?,...) 展开。
func (r *channelMonitorRepository) computeAvailabilityForMonitorsMySQL(ctx context.Context, ids []int64, windowDays int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	out := make(map[int64][]*service.ChannelMonitorAvailability, len(ids))
	q := `
		SELECT monitor_id,
		       model,
		       COUNT(*)                                                             AS total,
		       SUM(CASE WHEN status IN ('operational','degraded') THEN 1 ELSE 0 END) AS ok,
		       CASE WHEN COUNT(latency_ms) > 0
		            THEN SUM(CASE WHEN latency_ms IS NOT NULL THEN latency_ms END) / COUNT(latency_ms)
		            ELSE NULL END                                                   AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id IN (` + Placeholders(len(ids)) + `)
		  AND checked_at >= NOW(3) - INTERVAL ? DAY
		GROUP BY monitor_id, model
	`
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, windowDays)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query availability batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var monitorID int64
		row := &service.ChannelMonitorAvailability{WindowDays: windowDays}
		var avgLatency sql.NullFloat64
		if err := rows.Scan(&monitorID, &row.Model, &row.TotalChecks, &row.OperationalChecks, &avgLatency); err != nil {
			return nil, fmt.Errorf("scan availability batch row: %w", err)
		}
		finalizeAvailabilityRow(row, avgLatency)
		out[monitorID] = append(out[monitorID], row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- 聚合维护 ----------

// UpsertDailyRollupsFor 把 targetDate 当天（[targetDate, targetDate+1d)）的明细
// 按 (monitor_id, model, bucket_date) 聚合写入 channel_monitor_daily_rollups。
//   - 用 ON CONFLICT (monitor_id, model, bucket_date) DO UPDATE 实现幂等回填，
//     重复执行只会用最新统计覆盖；
//   - $1::date 让 PG 自动把入参 truncate 到 UTC 日期，调用方不需要预处理 targetDate。
func (r *channelMonitorRepository) UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error) {
	if IsMySQL() {
		return r.upsertDailyRollupsForMySQL(ctx, targetDate)
	}
	const q = `
		INSERT INTO channel_monitor_daily_rollups (
		    monitor_id, model, bucket_date,
		    total_checks, ok_count,
		    operational_count, degraded_count, failed_count, error_count,
		    sum_latency_ms, count_latency,
		    sum_ping_latency_ms, count_ping_latency,
		    computed_at
		)
		SELECT
		    monitor_id,
		    model,
		    $1::date AS bucket_date,
		    COUNT(*)                                                         AS total_checks,
		    COUNT(*) FILTER (WHERE status IN ('operational','degraded'))     AS ok_count,
		    COUNT(*) FILTER (WHERE status = 'operational')                   AS operational_count,
		    COUNT(*) FILTER (WHERE status = 'degraded')                      AS degraded_count,
		    COUNT(*) FILTER (WHERE status = 'failed')                        AS failed_count,
		    COUNT(*) FILTER (WHERE status = 'error')                         AS error_count,
		    COALESCE(SUM(latency_ms) FILTER (WHERE latency_ms IS NOT NULL), 0)             AS sum_latency_ms,
		    COUNT(latency_ms)                                                AS count_latency,
		    COALESCE(SUM(ping_latency_ms) FILTER (WHERE ping_latency_ms IS NOT NULL), 0)   AS sum_ping_latency_ms,
		    COUNT(ping_latency_ms)                                           AS count_ping_latency,
		    NOW()
		FROM channel_monitor_histories
		WHERE checked_at >= $1::date
		  AND checked_at <  ($1::date + INTERVAL '1 day')
		GROUP BY monitor_id, model
		ON CONFLICT (monitor_id, model, bucket_date) DO UPDATE SET
		    total_checks        = EXCLUDED.total_checks,
		    ok_count            = EXCLUDED.ok_count,
		    operational_count   = EXCLUDED.operational_count,
		    degraded_count      = EXCLUDED.degraded_count,
		    failed_count        = EXCLUDED.failed_count,
		    error_count         = EXCLUDED.error_count,
		    sum_latency_ms      = EXCLUDED.sum_latency_ms,
		    count_latency       = EXCLUDED.count_latency,
		    sum_ping_latency_ms = EXCLUDED.sum_ping_latency_ms,
		    count_ping_latency  = EXCLUDED.count_ping_latency,
		    computed_at         = NOW()
	`
	res, err := r.db.ExecContext(ctx, q, targetDate)
	if err != nil {
		return 0, fmt.Errorf("upsert daily rollups for %s: %w", targetDate.Format("2006-01-02"), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected (upsert rollups): %w", err)
	}
	return n, nil
}

// upsertDailyRollupsForMySQL 是 UpsertDailyRollupsFor 的 MySQL 实现：
//   - COUNT/SUM ... FILTER → SUM(CASE WHEN ... ELSE 0 END) / SUM(CASE WHEN ... END)；
//   - $1::date → ?（DATE 列，driver 按 DSN Loc 格式化后取日期部分，语义与
//     PG 会话时区截断一致）；($1::date + INTERVAL '1 day') → 应用侧 AddDate 后以参数传入；
//   - NOW() → NOW(3)；ON CONFLICT ... DO UPDATE → ODKU（EXCLUDED.* → VALUES(...)）。
func (r *channelMonitorRepository) upsertDailyRollupsForMySQL(ctx context.Context, targetDate time.Time) (int64, error) {
	const q = `
		INSERT INTO channel_monitor_daily_rollups (
		    monitor_id, model, bucket_date,
		    total_checks, ok_count,
		    operational_count, degraded_count, failed_count, error_count,
		    sum_latency_ms, count_latency,
		    sum_ping_latency_ms, count_ping_latency,
		    computed_at
		)
		SELECT
		    monitor_id,
		    model,
		    ? AS bucket_date,
		    COUNT(*)                                                         AS total_checks,
		    SUM(CASE WHEN status IN ('operational','degraded') THEN 1 ELSE 0 END) AS ok_count,
		    SUM(CASE WHEN status = 'operational' THEN 1 ELSE 0 END)          AS operational_count,
		    SUM(CASE WHEN status = 'degraded' THEN 1 ELSE 0 END)             AS degraded_count,
		    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)               AS failed_count,
		    SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END)                AS error_count,
		    COALESCE(SUM(CASE WHEN latency_ms IS NOT NULL THEN latency_ms END), 0)         AS sum_latency_ms,
		    COUNT(latency_ms)                                                AS count_latency,
		    COALESCE(SUM(CASE WHEN ping_latency_ms IS NOT NULL THEN ping_latency_ms END), 0) AS sum_ping_latency_ms,
		    COUNT(ping_latency_ms)                                           AS count_ping_latency,
		    NOW(3)
		FROM channel_monitor_histories
		WHERE checked_at >= ?
		  AND checked_at < ?
		GROUP BY monitor_id, model
		ON DUPLICATE KEY UPDATE
		    total_checks        = VALUES(total_checks),
		    ok_count            = VALUES(ok_count),
		    operational_count   = VALUES(operational_count),
		    degraded_count      = VALUES(degraded_count),
		    failed_count        = VALUES(failed_count),
		    error_count         = VALUES(error_count),
		    sum_latency_ms      = VALUES(sum_latency_ms),
		    count_latency       = VALUES(count_latency),
		    sum_ping_latency_ms = VALUES(sum_ping_latency_ms),
		    count_ping_latency  = VALUES(count_ping_latency),
		    computed_at         = NOW(3)
	`
	res, err := r.db.ExecContext(ctx, q, targetDate, targetDate, targetDate.AddDate(0, 0, 1))
	if err != nil {
		return 0, fmt.Errorf("upsert daily rollups for %s: %w", targetDate.Format("2006-01-02"), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected (upsert rollups): %w", err)
	}
	return n, nil
}

// DeleteRollupsBefore 物理删 bucket_date < beforeDate 的聚合行，同样分批。
func (r *channelMonitorRepository) DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error) {
	query := channelMonitorPruneRollupSQL
	if IsMySQL() {
		query = channelMonitorPruneRollupSQLMySQL
	}
	return deleteChannelMonitorBatched(ctx, r.db, query, beforeDate)
}

// channelMonitorPruneBatchSize 单批删除上限。与 ops_cleanup_service 保持一致的 5000，
// 在大表上按 id 小批删可以避免长事务和 WAL 堆积。
const channelMonitorPruneBatchSize = 5000

// channelMonitorPruneHistorySQL 分批物理删明细表过期行。
const channelMonitorPruneHistorySQL = `
WITH batch AS (
    SELECT id FROM channel_monitor_histories
    WHERE checked_at < $1
    ORDER BY id
    LIMIT $2
)
DELETE FROM channel_monitor_histories
WHERE id IN (SELECT id FROM batch)
`

// channelMonitorPruneHistorySQLMySQL 是 channelMonitorPruneHistorySQL 的 MySQL 版：
// 可写 CTE（WITH batch AS (SELECT ... LIMIT) DELETE ...）无 MySQL 对应，
// 按规范改写为多表 DELETE JOIN 物化派生表（派生表先物化，规避同表 1093 限制）。
const channelMonitorPruneHistorySQLMySQL = `
DELETE h FROM channel_monitor_histories h
JOIN (
    SELECT id FROM channel_monitor_histories
    WHERE checked_at < ?
    ORDER BY id
    LIMIT ?
) batch ON batch.id = h.id
`

// channelMonitorPruneRollupSQL 分批物理删 rollup 表过期行。bucket_date 需要 ::date 转型
// 保证与 DATE 列一致比较。
const channelMonitorPruneRollupSQL = `
WITH batch AS (
    SELECT id FROM channel_monitor_daily_rollups
    WHERE bucket_date < $1::date
    ORDER BY id
    LIMIT $2
)
DELETE FROM channel_monitor_daily_rollups
WHERE id IN (SELECT id FROM batch)
`

// channelMonitorPruneRollupSQLMySQL 是 channelMonitorPruneRollupSQL 的 MySQL 版（同上说明）；
// bucket_date 为 DATE 列，DATE(?) 对参数截断到日期，对齐 PG `$1::date` 语义。
const channelMonitorPruneRollupSQLMySQL = `
DELETE r FROM channel_monitor_daily_rollups r
JOIN (
    SELECT id FROM channel_monitor_daily_rollups
    WHERE bucket_date < DATE(?)
    ORDER BY id
    LIMIT ?
) batch ON batch.id = r.id
`

// deleteChannelMonitorBatched 循环执行分批 DELETE，直到影响行为 0。返回累计删除行数。
// cutoff 由调用方按列类型传入（明细用 time.Time 对 TIMESTAMPTZ，rollup 用 time.Time SQL 侧 ::date 转型）。
func deleteChannelMonitorBatched(ctx context.Context, db *sql.DB, query string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx, query, cutoff, channelMonitorPruneBatchSize)
		if err != nil {
			return total, fmt.Errorf("channel_monitor prune batch: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("channel_monitor prune rows affected: %w", err)
		}
		total += affected
		if affected == 0 {
			break
		}
	}
	return total, nil
}

// LoadAggregationWatermark 读 watermark 表（id=1）。
// watermark 表不是 ent schema（只有一行），直接走原生 SQL。
//   - 行不存在或 last_aggregated_date IS NULL：返回 (nil, nil)，由调用方决定首次回填策略
func (r *channelMonitorRepository) LoadAggregationWatermark(ctx context.Context) (*time.Time, error) {
	const q = `SELECT last_aggregated_date FROM channel_monitor_aggregation_watermark WHERE id = 1`
	var t sql.NullTime
	if err := r.db.QueryRowContext(ctx, q).Scan(&t); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load aggregation watermark: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

// UpdateAggregationWatermark 更新 watermark（UPSERT 到 id=1）。
// $1::date 让 PG 把入参 truncate 到 UTC 日期，与 last_aggregated_date 列的 DATE 类型一致。
func (r *channelMonitorRepository) UpdateAggregationWatermark(ctx context.Context, date time.Time) error {
	query := `
		INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
		VALUES (1, $1::date, NOW())
		ON CONFLICT (id) DO UPDATE SET
		    last_aggregated_date = EXCLUDED.last_aggregated_date,
		    updated_at           = NOW()
	`
	if IsMySQL() {
		// MySQL 版：$1::date → ?（DATE 列取参数日期部分）；NOW() → NOW(3)；
		// ON CONFLICT DO UPDATE → ODKU（EXCLUDED.* → VALUES(...)）。
		query = `
		INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
		VALUES (1, ?, NOW(3))
		ON DUPLICATE KEY UPDATE
		    last_aggregated_date = VALUES(last_aggregated_date),
		    updated_at           = NOW(3)
	`
	}
	if _, err := r.db.ExecContext(ctx, query, date); err != nil {
		return fmt.Errorf("update aggregation watermark: %w", err)
	}
	return nil
}

// ---------- helpers ----------

func entToServiceMonitor(row *dbent.ChannelMonitor) *service.ChannelMonitor {
	if row == nil {
		return nil
	}
	extras := row.ExtraModels
	if extras == nil {
		extras = []string{}
	}
	headers := make(map[string]string, len(row.ExtraHeaders))
	for key, value := range row.ExtraHeaders {
		headers[key] = value
	}
	duplicateOperationID := headers[service.ChannelMonitorDuplicateOperationIDMetadataKey]
	delete(headers, service.ChannelMonitorDuplicateOperationIDMetadataKey)
	out := &service.ChannelMonitor{
		ID:                   row.ID,
		Name:                 row.Name,
		Provider:             string(row.Provider),
		APIMode:              defaultAPIModeRepo(row.APIMode),
		Endpoint:             row.Endpoint,
		APIKey:               row.APIKeyEncrypted, // 仍为密文，service 层负责解密
		PrimaryModel:         row.PrimaryModel,
		ExtraModels:          extras,
		GroupName:            row.GroupName,
		Enabled:              row.Enabled,
		IntervalSeconds:      row.IntervalSeconds,
		JitterSeconds:        row.JitterSeconds,
		LastCheckedAt:        row.LastCheckedAt,
		CreatedBy:            row.CreatedBy,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		ExtraHeaders:         headers,
		BodyOverrideMode:     row.BodyOverrideMode,
		BodyOverride:         row.BodyOverride,
		CheckMode:            defaultCheckModeRepo(row.CheckMode),
		DuplicateOperationID: duplicateOperationID,
	}
	if row.TemplateID != nil {
		id := *row.TemplateID
		out.TemplateID = &id
	}
	if row.AccountID != nil {
		id := *row.AccountID
		out.AccountID = &id
	}
	return out
}

func channelMonitorHeadersForPersistence(m *service.ChannelMonitor) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	headers := make(map[string]string, len(m.ExtraHeaders)+1)
	for key, value := range m.ExtraHeaders {
		if key == service.ChannelMonitorDuplicateOperationIDMetadataKey {
			continue
		}
		headers[key] = value
	}
	if operationID := strings.TrimSpace(m.DuplicateOperationID); operationID != "" {
		headers[service.ChannelMonitorDuplicateOperationIDMetadataKey] = operationID
	}
	return headers
}

// emptyHeadersIfNilRepo 与 service.emptyHeadersIfNil 功能一致，
// repo 独立一份避免 import 循环。
func emptyHeadersIfNilRepo(h map[string]string) map[string]string {
	if h == nil {
		return map[string]string{}
	}
	return h
}

// defaultBodyModeRepo 空串归一为 off（同上不循环）。
func defaultBodyModeRepo(mode string) string {
	if mode == "" {
		return "off"
	}
	return mode
}

func defaultAPIModeRepo(apiMode string) string {
	if apiMode == "" {
		return "chat_completions"
	}
	return apiMode
}

// defaultCheckModeRepo 空串归一为 probe（存量行有列默认值，这里兜底防御）。
func defaultCheckModeRepo(checkMode string) string {
	if checkMode == "" {
		return "probe"
	}
	return checkMode
}

func emptySliceIfNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
