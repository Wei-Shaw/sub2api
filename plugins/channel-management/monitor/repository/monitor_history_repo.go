// monitor_history_repo.go — channel-monitor 明细历史的 CRUD 与按时间裁剪。
// 与 channel_monitor_repo.go 同 receiver `*channelMonitorRepository`、同 package；
// 仅按职责（明细历史）从原大文件拆出，无逻辑变更。
package monitorrepository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// InsertHistoryBatch 批量插入检测历史。空切片直接返回 nil。
// 单条 INSERT 多 VALUES 形式，参数列数 colsPerRow=7。
func (r *channelMonitorRepository) InsertHistoryBatch(ctx context.Context, rows []*monitorservice.ChannelMonitorHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
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

// channelMonitorPruneHistorySQL 是按 batch 软删 history 表的语句。
// id IN (SELECT ...) + LIMIT 形式确保单事务尺寸可控。
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
