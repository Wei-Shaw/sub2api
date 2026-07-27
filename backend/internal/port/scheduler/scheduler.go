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

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache interface {
	// GetSnapshot 读取快照并返回命中与否（ready + active + 数据完整）。
	GetSnapshot(ctx context.Context, bucket domain.SchedulerBucket) ([]*domain.Account, bool, error)
	// CaptureBucketWriteToken captures the current open epoch without changing
	// retirement state. A tombstoned bucket returns ErrSchedulerBucketRetired.
	CaptureBucketWriteToken(ctx context.Context, bucket domain.SchedulerBucket) (domain.SchedulerBucketWriteToken, error)
	// SetSnapshot 写入快照并切换激活版本。token 必须在 DB load/任务排队前取得。
	SetSnapshot(ctx context.Context, bucket domain.SchedulerBucket, token domain.SchedulerBucketWriteToken, accounts []domain.Account) error
	// RetireBucket persistently tombstones a bucket and fences every older writer.
	// Readers that captured the active version before retirement may finish; new
	// readers observe ready/active as absent.
	RetireBucket(ctx context.Context, bucket domain.SchedulerBucket) error
	// ReopenBucket is the only operation allowed to clear a tombstone. It returns
	// the retirement generation established by RetireBucket; repeated calls for
	// the same generation are idempotent. Callers must serialize a fresh authority
	// check through ReopenBucket with RetireBucket under the same group lifecycle
	// lease; ordinary rebuild paths never call ReopenBucket.
	ReopenBucket(ctx context.Context, bucket domain.SchedulerBucket) (domain.SchedulerBucketWriteToken, error)
	// TryAcquireGroupLifecycleLease serializes authoritative retirement/reopen
	// decisions for one non-zero group across instances.
	TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (domain.SchedulerGroupLifecycleLease, bool, error)
	// ReleaseGroupLifecycleLease releases the lease only if its owner token still
	// matches, so an expired holder cannot delete a successor's lease. Missing,
	// expired, mismatched, and already released leases return
	// ErrSchedulerGroupLifecycleLeaseLost.
	ReleaseGroupLifecycleLease(ctx context.Context, lease domain.SchedulerGroupLifecycleLease) error
	// GetAccount 获取单账号快照。
	GetAccount(ctx context.Context, accountID int64) (*domain.Account, error)
	// SetAccount 写入单账号快照（包含不可调度状态）。
	SetAccount(ctx context.Context, account *domain.Account) error
	// DeleteAccount 删除单账号快照。
	DeleteAccount(ctx context.Context, accountID int64) error
	// UpdateLastUsed 批量更新账号的最后使用时间。
	UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	// TryLockBucket 尝试获取分桶重建锁。
	TryLockBucket(ctx context.Context, bucket domain.SchedulerBucket, ttl time.Duration) (bool, error)
	// UnlockBucket 释放分桶重建锁。
	UnlockBucket(ctx context.Context, bucket domain.SchedulerBucket) error
	// ListBuckets 返回已注册的分桶集合。
	ListBuckets(ctx context.Context) ([]domain.SchedulerBucket, error)
	// GetOutboxWatermark 读取 outbox 水位。
	GetOutboxWatermark(ctx context.Context) (int64, error)
	// SetOutboxWatermark 保存 outbox 水位。
	SetOutboxWatermark(ctx context.Context, id int64) error
}
