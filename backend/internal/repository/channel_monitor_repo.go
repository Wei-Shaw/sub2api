package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorhistory"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
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
		SetEndpoint(m.Endpoint).
		SetAPIKeyEncrypted(m.APIKey). // 调用方传入的已是密文
		SetPrimaryModel(m.PrimaryModel).
		SetExtraModels(emptySliceIfNil(m.ExtraModels)).
		SetGroupName(m.GroupName).
		SetEnabled(m.Enabled).
		SetIntervalSeconds(m.IntervalSeconds).
		SetCreatedBy(m.CreatedBy)

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorNotFound, nil)
	}
	m.ID = created.ID
	m.CreatedAt = created.CreatedAt
	m.UpdatedAt = created.UpdatedAt
	return nil
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
		SetEndpoint(m.Endpoint).
		SetAPIKeyEncrypted(m.APIKey).
		SetPrimaryModel(m.PrimaryModel).
		SetExtraModels(emptySliceIfNil(m.ExtraModels)).
		SetGroupName(m.GroupName).
		SetEnabled(m.Enabled).
		SetIntervalSeconds(m.IntervalSeconds)

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
		bulk = append(bulk, c)
	}
	if _, err := client.ChannelMonitorHistory.CreateBulk(bulk...).Save(ctx); err != nil {
		return fmt.Errorf("insert history bulk: %w", err)
	}
	return nil
}

func (r *channelMonitorRepository) DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.ChannelMonitorHistory.Delete().
		Where(channelmonitorhistory.CheckedAtLT(before)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete history before: %w", err)
	}
	return int64(n), nil
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
		}
		out = append(out, entry)
	}
	return out, nil
}

// ---------- 用户视图聚合（原生 SQL） ----------

// ListLatestPerModel 用 DISTINCT ON 取每个 (monitor_id, model) 的最近一条记录。
// 借助 (monitor_id, model, checked_at DESC) 索引可走 Index Scan。
func (r *channelMonitorRepository) ListLatestPerModel(ctx context.Context, monitorID int64) ([]*service.ChannelMonitorLatest, error) {
	const q = `
		SELECT DISTINCT ON (model)
		    model, status, latency_ms, checked_at
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
		var latency sql.NullInt64
		if err := rows.Scan(&l.Model, &l.Status, &latency, &l.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest row: %w", err)
		}
		if latency.Valid {
			v := int(latency.Int64)
			l.LatencyMs = &v
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ComputeAvailability 计算指定窗口内每个模型的可用率与平均延迟。
// "可用" = status IN (operational, degraded)。
func (r *channelMonitorRepository) ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*service.ChannelMonitorAvailability, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	const q = `
		SELECT
		    model,
		    COUNT(*)                                                  AS total_checks,
		    COUNT(*) FILTER (WHERE status IN ('operational','degraded')) AS ok_checks,
		    AVG(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)     AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id = $1
		  AND checked_at >= $2
		GROUP BY model
	`
	from := time.Now().AddDate(0, 0, -windowDays)
	rows, err := r.db.QueryContext(ctx, q, monitorID, from)
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
	const q = `
		SELECT DISTINCT ON (monitor_id, model)
		    monitor_id, model, status, latency_ms, checked_at
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
		var latency sql.NullInt64
		if err := rows.Scan(&monitorID, &l.Model, &l.Status, &latency, &l.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest batch row: %w", err)
		}
		if latency.Valid {
			v := int(latency.Int64)
			l.LatencyMs = &v
		}
		out[monitorID] = append(out[monitorID], l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ComputeAvailabilityForMonitors 一次性计算多个监控在某个窗口内的每模型可用率与平均延迟。
func (r *channelMonitorRepository) ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	out := make(map[int64][]*service.ChannelMonitorAvailability, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	const q = `
		SELECT
		    monitor_id,
		    model,
		    COUNT(*)                                                  AS total_checks,
		    COUNT(*) FILTER (WHERE status IN ('operational','degraded')) AS ok_checks,
		    AVG(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)     AS avg_latency_ms
		FROM channel_monitor_histories
		WHERE monitor_id = ANY($1)
		  AND checked_at >= $2
		GROUP BY monitor_id, model
	`
	from := time.Now().AddDate(0, 0, -windowDays)
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids), from)
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

// ---------- helpers ----------

func entToServiceMonitor(row *dbent.ChannelMonitor) *service.ChannelMonitor {
	if row == nil {
		return nil
	}
	extras := row.ExtraModels
	if extras == nil {
		extras = []string{}
	}
	return &service.ChannelMonitor{
		ID:              row.ID,
		Name:            row.Name,
		Provider:        string(row.Provider),
		Endpoint:        row.Endpoint,
		APIKey:          row.APIKeyEncrypted, // 仍为密文，service 层负责解密
		PrimaryModel:    row.PrimaryModel,
		ExtraModels:     extras,
		GroupName:       row.GroupName,
		Enabled:         row.Enabled,
		IntervalSeconds: row.IntervalSeconds,
		LastCheckedAt:   row.LastCheckedAt,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func emptySliceIfNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
