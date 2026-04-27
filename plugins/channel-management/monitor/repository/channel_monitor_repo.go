// Package monitorrepository implements the channel-monitor data access layer
// on top of the SDK-provided *sql.DB. The host plugin module does not ship
// its own ent client, so all queries are written against PostgreSQL via raw
// SQL (in line with V5-CURATE D5 — "no plugin ent module in V5").
//
// SQL bodies are ported from the upstream commit 09fd83ab implementation in
// backend/internal/repository/channel_monitor_repo.go. The CRUD functions
// (Create / GetByID / Update / Delete / List / ListEnabled / MarkChecked /
// InsertHistoryBatch / DeleteHistoryBefore / ListHistory) re-express the ent
// builder calls as raw INSERT/SELECT/UPDATE statements; the aggregation
// queries (ListLatestPerModel, ComputeAvailability, the *ForMonitors batch
// variants, and the rollup maintenance helpers) are copied verbatim because
// they were already raw SQL upstream — only the package name changes.
package monitorrepository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/lib/pq"
)

// channelMonitorRepository is the channel-monitor data access implementation.
// db is the SDK-provided handle whose driver proxies queries through gRPC
// back to the host's connection pool.
type channelMonitorRepository struct {
	db *sql.DB
}

// NewChannelMonitorRepository wires the repo on top of the SDK DB handle.
func NewChannelMonitorRepository(db *sql.DB) monitorservice.ChannelMonitorRepository {
	return &channelMonitorRepository{db: db}
}

// channelMonitorColumns lists the columns selected when scanning a full
// monitor row. Centralised so SELECT and scanMonitorRow stay in sync.
const channelMonitorColumns = `id, name, provider, endpoint, api_key_encrypted, primary_model,
	extra_models, group_name, enabled, interval_seconds, last_checked_at,
	created_by, created_at, updated_at,
	template_id, extra_headers, body_override_mode, body_override`

// ---------- CRUD ----------

func (r *channelMonitorRepository) Create(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	extras, err := marshalJSONBSlice(m.ExtraModels)
	if err != nil {
		return fmt.Errorf("marshal extra_models: %w", err)
	}
	headers, err := marshalJSONBStringMap(m.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(m.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	// body_override is JSONB NULLABLE. We send a NULL placeholder when the
	// service didn't provide a value to avoid PostgreSQL coercing the empty
	// string to invalid JSON; otherwise the JSON text is cast via $14::jsonb.
	const q = `
		INSERT INTO channel_monitors (
			name, provider, endpoint, api_key_encrypted, primary_model,
			extra_models, group_name, enabled, interval_seconds, created_by,
			template_id, extra_headers, body_override_mode, body_override
		) VALUES (
			$1, $2, $3, $4, $5,
			$6::jsonb, $7, $8, $9, $10,
			$11, $12::jsonb, $13, $14::jsonb
		)
		RETURNING id, created_at, updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	return r.db.QueryRowContext(ctx, q,
		m.Name, m.Provider, m.Endpoint, m.APIKey, m.PrimaryModel,
		string(extras), m.GroupName, m.Enabled, m.IntervalSeconds, m.CreatedBy,
		m.TemplateID, string(headers), defaultBodyMode(m.BodyOverrideMode), bodyArg,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (r *channelMonitorRepository) GetByID(ctx context.Context, id int64) (*monitorservice.ChannelMonitor, error) {
	const q = `SELECT ` + channelMonitorColumns + ` FROM channel_monitors WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	m, err := scanMonitorRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, monitorservice.ErrChannelMonitorNotFound
		}
		return nil, fmt.Errorf("get monitor by id: %w", err)
	}
	return m, nil
}

func (r *channelMonitorRepository) Update(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	extras, err := marshalJSONBSlice(m.ExtraModels)
	if err != nil {
		return fmt.Errorf("marshal extra_models: %w", err)
	}
	headers, err := marshalJSONBStringMap(m.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(m.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	const q = `
		UPDATE channel_monitors SET
			name=$2, provider=$3, endpoint=$4, api_key_encrypted=$5, primary_model=$6,
			extra_models=$7::jsonb, group_name=$8, enabled=$9, interval_seconds=$10,
			template_id=$11, extra_headers=$12::jsonb, body_override_mode=$13,
			body_override=$14::jsonb, updated_at=NOW()
		WHERE id=$1
		RETURNING updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	if err := r.db.QueryRowContext(ctx, q,
		m.ID, m.Name, m.Provider, m.Endpoint, m.APIKey, m.PrimaryModel,
		string(extras), m.GroupName, m.Enabled, m.IntervalSeconds,
		m.TemplateID, string(headers), defaultBodyMode(m.BodyOverrideMode), bodyArg,
	).Scan(&m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitorservice.ErrChannelMonitorNotFound
		}
		return fmt.Errorf("update monitor: %w", err)
	}
	return nil
}

func (r *channelMonitorRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM channel_monitors WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete monitor: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete monitor rows affected: %w", err)
	}
	if n == 0 {
		return monitorservice.ErrChannelMonitorNotFound
	}
	return nil
}

func (r *channelMonitorRepository) List(ctx context.Context, params monitorservice.ChannelMonitorListParams) ([]*monitorservice.ChannelMonitor, int64, error) {
	where, args := buildListWhere(params)

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM channel_monitors `+where, args...,
	).Scan(&total); err != nil {
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
	offset := (page - 1) * pageSize

	q := `SELECT ` + channelMonitorColumns + ` FROM channel_monitors ` + where +
		fmt.Sprintf(` ORDER BY id DESC LIMIT %d OFFSET %d`, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitor, 0)
	for rows.Next() {
		m, err := scanMonitorRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan monitor list row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// buildListWhere builds the WHERE clause + positional args for List filters.
func buildListWhere(params monitorservice.ChannelMonitorListParams) (string, []any) {
	conds := []string{}
	args := []any{}
	if params.Provider != "" {
		args = append(args, params.Provider)
		conds = append(conds, fmt.Sprintf("provider = $%d", len(args)))
	}
	if params.Enabled != nil {
		args = append(args, *params.Enabled)
		conds = append(conds, fmt.Sprintf("enabled = $%d", len(args)))
	}
	if s := strings.TrimSpace(params.Search); s != "" {
		args = append(args, "%"+s+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf(
			"(name ILIKE $%d OR group_name ILIKE $%d OR primary_model ILIKE $%d)",
			idx, idx, idx,
		))
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// ---------- Scheduler helpers ----------

func (r *channelMonitorRepository) ListEnabled(ctx context.Context) ([]*monitorservice.ChannelMonitor, error) {
	q := `SELECT ` + channelMonitorColumns + ` FROM channel_monitors WHERE enabled = TRUE`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitor, 0)
	for rows.Next() {
		m, err := scanMonitorRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled monitor row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *channelMonitorRepository) MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error {
	const q = `UPDATE channel_monitors SET last_checked_at = $2 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, checkedAt)
	if err != nil {
		return fmt.Errorf("mark checked: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark checked rows affected: %w", err)
	}
	if n == 0 {
		return monitorservice.ErrChannelMonitorNotFound
	}
	return nil
}

func (r *channelMonitorRepository) InsertHistoryBatch(ctx context.Context, rows []*monitorservice.ChannelMonitorHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	// Build a single multi-VALUES INSERT.
	const colsPerRow = 7
	args := make([]any, 0, len(rows)*colsPerRow)
	placeholders := make([]string, 0, len(rows))
	for i, row := range rows {
		base := i*colsPerRow + 1
		placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base, base+1, base+2, base+3, base+4, base+5, base+6))
		args = append(args,
			row.MonitorID, row.Model, row.Status,
			nullableInt(row.LatencyMs), nullableInt(row.PingLatencyMs),
			row.Message, row.CheckedAt,
		)
	}
	q := `INSERT INTO channel_monitor_histories
		(monitor_id, model, status, latency_ms, ping_latency_ms, message, checked_at)
		VALUES ` + strings.Join(placeholders, ",")
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert history bulk: %w", err)
	}
	return nil
}

// DeleteHistoryBefore physically deletes histories whose checked_at < before,
// in batches of channelMonitorPruneBatchSize rows.
func (r *channelMonitorRepository) DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error) {
	return deleteChannelMonitorBatched(ctx, r.db, channelMonitorPruneHistorySQL, before)
}

// ListHistory returns the most recent N history entries for a monitor.
func (r *channelMonitorRepository) ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*monitorservice.ChannelMonitorHistoryEntry, error) {
	conds := []string{"monitor_id = $1"}
	args := []any{monitorID}
	if strings.TrimSpace(model) != "" {
		args = append(args, model)
		conds = append(conds, fmt.Sprintf("model = $%d", len(args)))
	}
	q := fmt.Sprintf(`
		SELECT id, model, status, latency_ms, ping_latency_ms, message, checked_at
		FROM channel_monitor_histories
		WHERE %s
		ORDER BY checked_at DESC
		LIMIT %d
	`, strings.Join(conds, " AND "), limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitorHistoryEntry, 0)
	for rows.Next() {
		entry := &monitorservice.ChannelMonitorHistoryEntry{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Model, &entry.Status, &latency, &ping, &entry.Message, &entry.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		assignNullInt(&entry.LatencyMs, latency)
		assignNullInt(&entry.PingLatencyMs, ping)
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ---------- 用户视图聚合（原生 SQL） ----------

// ListLatestPerModel uses DISTINCT ON to get the latest record per
// (monitor_id, model). Identical to the upstream raw SQL.
func (r *channelMonitorRepository) ListLatestPerModel(ctx context.Context, monitorID int64) ([]*monitorservice.ChannelMonitorLatest, error) {
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

	out := make([]*monitorservice.ChannelMonitorLatest, 0)
	for rows.Next() {
		l := &monitorservice.ChannelMonitorLatest{}
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

// ComputeAvailability returns the per-model availability % and average
// latency for the given window.
func (r *channelMonitorRepository) ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*monitorservice.ChannelMonitorAvailability, error) {
	if windowDays <= 0 {
		windowDays = 7
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

	out := make([]*monitorservice.ChannelMonitorAvailability, 0)
	for rows.Next() {
		row := &monitorservice.ChannelMonitorAvailability{WindowDays: windowDays}
		var avgLatency sql.NullFloat64
		if err := rows.Scan(&row.Model, &row.TotalChecks, &row.OperationalChecks, &avgLatency); err != nil {
			return nil, fmt.Errorf("scan availability row: %w", err)
		}
		finalizeAvailabilityRow(row, avgLatency)
		out = append(out, row)
	}
	return out, rows.Err()
}

// ---------- 批量聚合 ----------

func (r *channelMonitorRepository) ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*monitorservice.ChannelMonitorLatest, error) {
	out := make(map[int64][]*monitorservice.ChannelMonitorLatest, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT DISTINCT ON (monitor_id, model)
		    monitor_id, model, status, latency_ms, ping_latency_ms, checked_at
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
		l := &monitorservice.ChannelMonitorLatest{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&monitorID, &l.Model, &l.Status, &latency, &ping, &l.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest batch row: %w", err)
		}
		assignNullInt(&l.LatencyMs, latency)
		assignNullInt(&l.PingLatencyMs, ping)
		out[monitorID] = append(out[monitorID], l)
	}
	return out, rows.Err()
}

func (r *channelMonitorRepository) ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*monitorservice.ChannelMonitorAvailability, error) {
	out := make(map[int64][]*monitorservice.ChannelMonitorAvailability, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	const q = `
		SELECT monitor_id, model,
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
		row := &monitorservice.ChannelMonitorAvailability{WindowDays: windowDays}
		var avgLatency sql.NullFloat64
		if err := rows.Scan(&monitorID, &row.Model, &row.TotalChecks, &row.OperationalChecks, &avgLatency); err != nil {
			return nil, fmt.Errorf("scan availability batch row: %w", err)
		}
		finalizeAvailabilityRow(row, avgLatency)
		out[monitorID] = append(out[monitorID], row)
	}
	return out, rows.Err()
}

func (r *channelMonitorRepository) ListRecentHistoryForMonitors(
	ctx context.Context,
	ids []int64,
	primaryModels map[int64]string,
	perMonitorLimit int,
) (map[int64][]*monitorservice.ChannelMonitorHistoryEntry, error) {
	out := make(map[int64][]*monitorservice.ChannelMonitorHistoryEntry, len(ids))
	pairIDs, pairModels := buildMonitorModelPairs(ids, primaryModels)
	if len(pairIDs) == 0 {
		return out, nil
	}
	perMonitorLimit = clampTimelineLimit(perMonitorLimit)

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
		entry := &monitorservice.ChannelMonitorHistoryEntry{}
		var latency, ping sql.NullInt64
		if err := rows.Scan(&monitorID, &entry.Status, &latency, &ping, &entry.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan recent history row: %w", err)
		}
		assignNullInt(&entry.LatencyMs, latency)
		assignNullInt(&entry.PingLatencyMs, ping)
		out[monitorID] = append(out[monitorID], entry)
	}
	return out, rows.Err()
}

// ---------- Daily rollup maintenance ----------

func (r *channelMonitorRepository) UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error) {
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

func (r *channelMonitorRepository) DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error) {
	return deleteChannelMonitorBatched(ctx, r.db, channelMonitorPruneRollupSQL, beforeDate)
}

func (r *channelMonitorRepository) LoadAggregationWatermark(ctx context.Context) (*time.Time, error) {
	const q = `SELECT last_aggregated_date FROM channel_monitor_aggregation_watermark WHERE id = 1`
	var t sql.NullTime
	if err := r.db.QueryRowContext(ctx, q).Scan(&t); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load aggregation watermark: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

func (r *channelMonitorRepository) UpdateAggregationWatermark(ctx context.Context, date time.Time) error {
	const q = `
		INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
		VALUES (1, $1::date, NOW())
		ON CONFLICT (id) DO UPDATE SET
		    last_aggregated_date = EXCLUDED.last_aggregated_date,
		    updated_at           = NOW()
	`
	if _, err := r.db.ExecContext(ctx, q, date); err != nil {
		return fmt.Errorf("update aggregation watermark: %w", err)
	}
	return nil
}

// ---------- helpers ----------

// scanRow is the minimal interface needed by scanMonitorRow so the helper
// can be shared between *sql.Row (Get) and *sql.Rows (List).
type scanRow interface {
	Scan(dest ...any) error
}

// scanMonitorRow projects a row in channelMonitorColumns order into a service
// model. Centralised so SELECT and Scan call sites stay in sync.
func scanMonitorRow(r scanRow) (*monitorservice.ChannelMonitor, error) {
	m := &monitorservice.ChannelMonitor{}
	var (
		extrasRaw   []byte
		headersRaw  []byte
		bodyRaw     sql.NullString
		lastChecked sql.NullTime
		bodyMode    sql.NullString
		templateID  sql.NullInt64
		groupName   sql.NullString
	)
	if err := r.Scan(
		&m.ID, &m.Name, &m.Provider, &m.Endpoint, &m.APIKey, &m.PrimaryModel,
		&extrasRaw, &groupName, &m.Enabled, &m.IntervalSeconds, &lastChecked,
		&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		&templateID, &headersRaw, &bodyMode, &bodyRaw,
	); err != nil {
		return nil, err
	}
	m.GroupName = groupName.String
	if lastChecked.Valid {
		t := lastChecked.Time
		m.LastCheckedAt = &t
	}
	if templateID.Valid {
		id := templateID.Int64
		m.TemplateID = &id
	}
	if bodyMode.Valid {
		m.BodyOverrideMode = bodyMode.String
	}

	if err := unmarshalJSONBSlice(extrasRaw, &m.ExtraModels); err != nil {
		return nil, fmt.Errorf("decode extra_models: %w", err)
	}
	if m.ExtraModels == nil {
		m.ExtraModels = []string{}
	}
	if err := unmarshalJSONBStringMap(headersRaw, &m.ExtraHeaders); err != nil {
		return nil, fmt.Errorf("decode extra_headers: %w", err)
	}
	if m.ExtraHeaders == nil {
		m.ExtraHeaders = map[string]string{}
	}
	if bodyRaw.Valid && strings.TrimSpace(bodyRaw.String) != "" {
		var body map[string]any
		if err := json.Unmarshal([]byte(bodyRaw.String), &body); err != nil {
			return nil, fmt.Errorf("decode body_override: %w", err)
		}
		m.BodyOverride = body
	}
	return m, nil
}

// nullableInt converts *int into a value driver-friendly representation.
// Nil becomes nil (NULL), else the int is sent as int64.
func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// assignNullInt unpacks sql.NullInt64 into a *int target.
func assignNullInt(dst **int, n sql.NullInt64) {
	if !n.Valid {
		return
	}
	v := int(n.Int64)
	*dst = &v
}

// finalizeAvailabilityRow computes availability percentage and unpacks
// average latency.
func finalizeAvailabilityRow(row *monitorservice.ChannelMonitorAvailability, avgLatency sql.NullFloat64) {
	if row.TotalChecks > 0 {
		row.AvailabilityPct = float64(row.OperationalChecks) * 100.0 / float64(row.TotalChecks)
	}
	if avgLatency.Valid {
		v := int(avgLatency.Float64)
		row.AvgLatencyMs = &v
	}
}

// buildMonitorModelPairs filters ids against primaryModels, returning
// parallel slices for unnest expansion.
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

// timelineLimit* mirrors the upstream constants. Callers cap perMonitorLimit
// to this range to keep ROW_NUMBER windows bounded.
const (
	timelineLimitMin = 1
	timelineLimitMax = 200
)

func clampTimelineLimit(n int) int {
	if n < timelineLimitMin {
		return timelineLimitMin
	}
	if n > timelineLimitMax {
		return timelineLimitMax
	}
	return n
}

// channelMonitorPruneBatchSize is the upstream batch size for prune helpers.
const channelMonitorPruneBatchSize = 5000

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

// ---------- JSONB helpers ----------

func marshalJSONBSlice(in []string) ([]byte, error) {
	if in == nil {
		in = []string{}
	}
	return json.Marshal(in)
}

func marshalJSONBStringMap(in map[string]string) ([]byte, error) {
	if in == nil {
		in = map[string]string{}
	}
	return json.Marshal(in)
}

func marshalJSONBOptional(in map[string]any) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(in)
}

func unmarshalJSONBSlice(raw []byte, dst *[]string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func unmarshalJSONBStringMap(raw []byte, dst *map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func defaultBodyMode(mode string) string {
	if mode == "" {
		return monitorservice.MonitorBodyOverrideModeOff
	}
	return mode
}
