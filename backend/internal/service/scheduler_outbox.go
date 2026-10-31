package service

import (
	"context"
	"time"
)

type SchedulerOutboxEvent struct {
	ID        int64
	EventType string
	AccountID *int64
	GroupID   *int64
	Payload   map[string]any
	CreatedAt time.Time
}

// SchedulerOutboxRepository 提供调度 outbox 的读取接口。
type SchedulerOutboxRepository interface {
	ListAfterAndReleaseDedup(ctx context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error)
	// FirstCreatedAtAfter 返回指定水位之后第一条待消费事件的创建时间，不领取事件或修改去重键。
	FirstCreatedAtAfter(ctx context.Context, afterID int64) (time.Time, bool, error)
	MaxID(ctx context.Context) (int64, error)
	DeleteConsumedUpTo(ctx context.Context, watermark int64, limit int) (int64, error)
	TryAcquireCleanupLock(ctx context.Context) (SchedulerOutboxCleanupLease, bool, error)
}

// SchedulerOutboxCleanupLease holds the PostgreSQL advisory lock used by
// scheduler outbox cleanup.
type SchedulerOutboxCleanupLease interface {
	Release()
}

// SchedulerOutboxQueue 是调度 outbox 的租约式领取接口，与
// auth_cache_invalidation_outbox 同构：FOR UPDATE 只在单条语句内持锁，无法覆盖
// "处理期间"，因此领取归属必须落在列上（claimed_at / claimed_by / available_at）。
//
// 读取路径不再按 Redis watermark 过滤：仍然留在表里的行就是未消费行。watermark
// 退化为观测指标，推进失败不得阻断消费。
type SchedulerOutboxQueue interface {
	// Claim 领取队首最多 limit 条可消费事件并写入租约；租约过期的行可被重新领取。
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]SchedulerOutboxEvent, error)
	// DeleteClaimed 仅当归属仍是 workerID 时删除；error 表示归属已丢失（不是错误删除）。
	DeleteClaimed(ctx context.Context, id int64, workerID string) error
	// RetryClaimed 交还单个事件归属并给出下次可见时间（退避由调用方决定）。
	RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error
	// ReleaseClaimed 批量归还本批未处理到的事件，使其立即可见，保持顺序且不等租约过期。
	ReleaseClaimed(ctx context.Context, ids []int64, availableAt time.Time) error
	// PendingStats 返回真实积压与队首事件年龄，不依赖 watermark。
	PendingStats(ctx context.Context) (SchedulerOutboxPendingStats, error)
}

// SchedulerOutboxPendingStats 是队列的真实积压视图。
type SchedulerOutboxPendingStats struct {
	Pending         int64
	OldestCreatedAt *time.Time
}
