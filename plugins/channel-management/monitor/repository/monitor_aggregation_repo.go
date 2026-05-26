// monitor_aggregation_repo.go — channel-monitor 用户视图与批量聚合查询。
// 与 channel_monitor_repo.go 同 receiver `*channelMonitorRepository`、同 package；
// 包含 ListLatestPerModel / ComputeAvailability 单 monitor 查询，以及 admin/user
// list 调用的 *ForMonitors 批量变体（避免 N+1）。
package monitorrepository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/lib/pq"
)

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

// finalizeAvailabilityRow computes availability percentage and unpacks
// average latency. Shared by single-monitor and batch availability queries.
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
// parallel slices for unnest expansion in ListRecentHistoryForMonitors.
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
