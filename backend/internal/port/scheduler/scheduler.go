// Package scheduler contains the port interfaces for the scheduler bounded
// context: the routing-snapshot cache, the event outbox, and the transient
// unschedulable state. Contracts reference only domain types so the repository
// layer can implement them without importing internal/service. The service
// package keeps a type alias to each interface so existing call sites and test
// stubs continue to satisfy the contracts.
package scheduler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// TempUnschedCache 临时不可调度缓存接口。
type TempUnschedCache interface {
	SetTempUnsched(ctx context.Context, accountID int64, state *domain.TempUnschedState) error
	GetTempUnsched(ctx context.Context, accountID int64) (*domain.TempUnschedState, error)
	DeleteTempUnsched(ctx context.Context, accountID int64) error
}

// SchedulerOutboxRepository 提供调度 outbox 的读取接口。
type SchedulerOutboxRepository interface {
	ListAfterAndReleaseDedup(ctx context.Context, afterID int64, limit int) ([]domain.SchedulerOutboxEvent, error)
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
