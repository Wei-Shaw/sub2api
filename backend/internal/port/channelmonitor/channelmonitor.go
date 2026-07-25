// Package channelmonitor contains the port interfaces (repository
// abstractions) for the channel_monitor bounded context: the ChannelMonitor
// aggregate and its RequestTemplate sub-domain. DTO/value types live in
// internal/domain; this package only owns the persistence port contracts.
package channelmonitor

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Repository persists the ChannelMonitor aggregate, its check history, the
// user-view aggregations, and the daily-rollup maintenance window.
type Repository interface {
	// CRUD
	Create(ctx context.Context, m *domain.ChannelMonitor) error
	GetByID(ctx context.Context, id int64) (*domain.ChannelMonitor, error)
	Update(ctx context.Context, m *domain.ChannelMonitor) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params domain.ChannelMonitorListParams) ([]*domain.ChannelMonitor, int64, error)
	FindByDuplicateOperationID(ctx context.Context, operationID string) (*domain.ChannelMonitor, error)

	// 调度器辅助
	ListEnabled(ctx context.Context) ([]*domain.ChannelMonitor, error)
	MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error
	InsertHistoryBatch(ctx context.Context, rows []*domain.ChannelMonitorHistoryRow) error
	DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error)

	// 历史记录
	ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*domain.ChannelMonitorHistoryEntry, error)

	// 用户视图聚合
	ListLatestPerModel(ctx context.Context, monitorID int64) ([]*domain.ChannelMonitorLatest, error)
	ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*domain.ChannelMonitorAvailability, error)

	// 批量聚合（admin/user list 用，避免 N+1）
	ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*domain.ChannelMonitorLatest, error)
	ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*domain.ChannelMonitorAvailability, error)
	// ListRecentHistoryForMonitors 批量取多个 monitor 各自主模型（primaryModels[monitorID]）最近 perMonitorLimit 条历史。
	// 返回的 entry 已按 checked_at DESC 排序（最新在前），不含 message 字段。
	ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primaryModels map[int64]string, perMonitorLimit int) (map[int64][]*domain.ChannelMonitorHistoryEntry, error)

	// ---------- 聚合维护（OpsCleanupService 调用） ----------

	// UpsertDailyRollupsFor 把 targetDate 当天的明细按 (monitor_id, model, bucket_date)
	// 聚合到 channel_monitor_daily_rollups。targetDate 会被截断到日期；
	// 用 ON CONFLICT DO UPDATE 实现幂等回填，返回 upsert 影响的行数。
	UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error)
	// DeleteRollupsBefore 软删 bucket_date < beforeDate 的聚合行，返回删除行数。
	DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error)
	// LoadAggregationWatermark 读 watermark（id=1）。
	// 返回 nil 表示从未聚合过；watermark 表本身预期已存在单行（migration 110 写入）。
	LoadAggregationWatermark(ctx context.Context) (*time.Time, error)
	// UpdateAggregationWatermark 写 watermark（UPSERT 到 id=1）。
	UpdateAggregationWatermark(ctx context.Context, date time.Time) error
}

// TemplateRepository persists ChannelMonitorRequestTemplate and drives the
// apply-to-monitors / associated-monitor projections used by the template UI.
type TemplateRepository interface {
	Create(ctx context.Context, t *domain.ChannelMonitorRequestTemplate) error
	GetByID(ctx context.Context, id int64) (*domain.ChannelMonitorRequestTemplate, error)
	Update(ctx context.Context, t *domain.ChannelMonitorRequestTemplate) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params domain.ChannelMonitorRequestTemplateListParams) ([]*domain.ChannelMonitorRequestTemplate, error)
	// ApplyToMonitors 把模板当前的 api_mode / extra_headers / body_override_mode / body_override
	// 批量覆盖到指定 monitorIDs 的监控上（同时还要求这些监控当前 template_id = id，
	// 防止误覆盖未关联的监控）。monitorIDs 必须非空；空列表直接返回 0 不写库。
	// 返回被覆盖的监控数量。
	ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error)
	// CountAssociatedMonitors 统计 template_id = id 的监控数（用于 UI 展示「应用到 N 个配置」）。
	CountAssociatedMonitors(ctx context.Context, id int64) (int64, error)
	// ListAssociatedMonitors 列出所有 template_id = id 的监控简略信息（id/name/provider/api_mode/enabled）
	// 给 apply picker UI 用，避免前端再做一次 list+filter。
	ListAssociatedMonitors(ctx context.Context, id int64) ([]*domain.AssociatedMonitorBrief, error)
}
