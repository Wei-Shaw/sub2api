// service_maintenance.go — channel-monitor 的每日维护：
// 把昨天之前未聚合的明细 upsert 到 daily rollup，软删过期的明细与聚合行，
// 并维护 watermark。由 host 的 OpsCleanupService 通过 MaintenanceExtension
// gRPC 调用触发。
//
// 与 service.go 同 receiver `*ChannelMonitorService`、同 package；
// 仅按职责从原 service.go 拆出，无逻辑变更。
package monitorservice

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// maintenanceTaskAggregate / maintenanceTaskPruneHistory /
// maintenanceTaskPruneRollups are stable task names reported in
// MaintenanceResult and forwarded to the host's OpsCleanupService logs.
const (
	maintenanceTaskAggregate    = "aggregate"
	maintenanceTaskPruneHistory = "prune_history"
	maintenanceTaskPruneRollups = "prune_rollups"
)

// MaintenanceResult reports the outcome of a single maintenance sub-task.
// The MaintenanceExtension gRPC server translates these into proto
// MaintenanceTaskResult messages for the host.
type MaintenanceResult struct {
	Task         string
	DeletedCount int64
	Error        string
}

// RunDailyMaintenanceWithResults runs all daily maintenance steps and
// collects per-task structured results. Each step runs independently;
// a failure in one step does not prevent subsequent steps.
//
// Idempotency:
//   - watermark prevents re-aggregation of already-processed dates;
//   - UpsertDailyRollupsFor uses ON CONFLICT DO UPDATE internally.
func (s *ChannelMonitorService) RunDailyMaintenanceWithResults(ctx context.Context) []MaintenanceResult {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	results := make([]MaintenanceResult, 0, 3)

	results = append(results, s.runAggregationTask(ctx, today))
	results = append(results, s.runPruneHistoryTask(ctx))
	results = append(results, s.runPruneRollupsTask(ctx, today))
	return results
}

// RunDailyMaintenance is the backward-compatible entry point. It delegates
// to RunDailyMaintenanceWithResults and discards the structured results.
func (s *ChannelMonitorService) RunDailyMaintenance(ctx context.Context) error {
	s.RunDailyMaintenanceWithResults(ctx)
	return nil
}

// runAggregationTask wraps runDailyAggregation into a MaintenanceResult.
func (s *ChannelMonitorService) runAggregationTask(ctx context.Context, today time.Time) MaintenanceResult {
	affected, err := s.runDailyAggregation(ctx, today)
	r := MaintenanceResult{Task: maintenanceTaskAggregate, DeletedCount: affected}
	if err != nil {
		r.Error = err.Error()
		slog.Warn("channel_monitor: maintenance step failed",
			"step", maintenanceTaskAggregate, "error", err)
	}
	return r
}

// runPruneHistoryTask wraps cleanupOldHistory into a MaintenanceResult.
func (s *ChannelMonitorService) runPruneHistoryTask(ctx context.Context) MaintenanceResult {
	deleted, err := s.cleanupOldHistory(ctx)
	r := MaintenanceResult{Task: maintenanceTaskPruneHistory, DeletedCount: deleted}
	if err != nil {
		r.Error = err.Error()
		slog.Warn("channel_monitor: maintenance step failed",
			"step", maintenanceTaskPruneHistory, "error", err)
	}
	return r
}

// runPruneRollupsTask wraps cleanupOldRollups into a MaintenanceResult.
func (s *ChannelMonitorService) runPruneRollupsTask(ctx context.Context, today time.Time) MaintenanceResult {
	deleted, err := s.cleanupOldRollups(ctx, today)
	r := MaintenanceResult{Task: maintenanceTaskPruneRollups, DeletedCount: deleted}
	if err != nil {
		r.Error = err.Error()
		slog.Warn("channel_monitor: maintenance step failed",
			"step", maintenanceTaskPruneRollups, "error", err)
	}
	return r
}

// cleanupOldHistory 删除 monitorHistoryRetentionDays 天之前的明细历史记录。
// SoftDeleteMixin 自动把 DELETE 改为 UPDATE deleted_at。
func (s *ChannelMonitorService) cleanupOldHistory(ctx context.Context) (int64, error) {
	before := time.Now().UTC().AddDate(0, 0, -monitorHistoryRetentionDays)
	deleted, err := s.repo.DeleteHistoryBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("delete history before %s: %w", before.Format(time.RFC3339), err)
	}
	if deleted > 0 {
		slog.Info("channel_monitor: history cleanup",
			"deleted_rows", deleted, "before", before.Format(time.RFC3339))
	}
	return deleted, nil
}

// runDailyAggregation 从 watermark+1 聚合到昨天（UTC）。
// 首次跑（watermark nil）：从 today-monitorRollupRetentionDays 开始回填。
// 每次最多聚合 monitorMaintenanceMaxDaysPerRun 天，避免长事务。
// 返回受影响的总行数。
func (s *ChannelMonitorService) runDailyAggregation(ctx context.Context, today time.Time) (int64, error) {
	watermark, err := s.repo.LoadAggregationWatermark(ctx)
	if err != nil {
		return 0, fmt.Errorf("load watermark: %w", err)
	}

	start := s.resolveAggregationStart(watermark, today)
	if !start.Before(today) {
		return 0, nil // 没有需要聚合的日期
	}

	var totalAffected int64
	iterations := 0
	for d := start; d.Before(today); d = d.Add(24 * time.Hour) {
		if iterations >= monitorMaintenanceMaxDaysPerRun {
			slog.Info("channel_monitor: maintenance aggregation capped",
				"max_days", monitorMaintenanceMaxDaysPerRun,
				"next_resume", d.Format("2006-01-02"))
			break
		}
		affected, upErr := s.repo.UpsertDailyRollupsFor(ctx, d)
		if upErr != nil {
			return totalAffected, fmt.Errorf("upsert rollups for %s: %w", d.Format("2006-01-02"), upErr)
		}
		if err := s.repo.UpdateAggregationWatermark(ctx, d); err != nil {
			return totalAffected, fmt.Errorf("update watermark to %s: %w", d.Format("2006-01-02"), err)
		}
		slog.Info("channel_monitor: rollups upserted",
			"date", d.Format("2006-01-02"), "affected_rows", affected)
		totalAffected += affected
		iterations++
	}
	return totalAffected, nil
}

// resolveAggregationStart 计算本次聚合起点：
//   - watermark == nil：today - monitorRollupRetentionDays（首次回填最多 30 天）
//   - watermark != nil：*watermark + 1 day
func (s *ChannelMonitorService) resolveAggregationStart(watermark *time.Time, today time.Time) time.Time {
	if watermark == nil {
		return today.AddDate(0, 0, -monitorRollupRetentionDays)
	}
	return watermark.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
}

// cleanupOldRollups 软删 bucket_date < today - monitorRollupRetentionDays 的日聚合行。
func (s *ChannelMonitorService) cleanupOldRollups(ctx context.Context, today time.Time) (int64, error) {
	cutoff := today.AddDate(0, 0, -monitorRollupRetentionDays)
	deleted, err := s.repo.DeleteRollupsBefore(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete rollups before %s: %w", cutoff.Format("2006-01-02"), err)
	}
	if deleted > 0 {
		slog.Info("channel_monitor: rollups cleanup",
			"deleted_rows", deleted, "before", cutoff.Format("2006-01-02"))
	}
	return deleted, nil
}
