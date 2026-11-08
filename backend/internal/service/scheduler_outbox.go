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
