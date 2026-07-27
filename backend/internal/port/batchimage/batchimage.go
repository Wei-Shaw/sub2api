// Package batchimage contains the port interfaces for the BatchImage bounded
// context: the repository, queue, job-lock, lock-refresher, and download-rate-
// limiter contracts. All method signatures reference pure-scalar DTOs defined
// in internal/domain, so the repository layer can implement these contracts
// without importing internal/service. The service package keeps type aliases
// to each interface so existing call sites and test stubs continue to satisfy
// them.
package batchimage

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BatchImageRepository is the persistence contract for batch image jobs, items,
// events, and lifecycle transitions.
type BatchImageRepository interface {
	CreateBatchImageJob(ctx context.Context, params domain.CreateBatchImageJobParams) (*domain.BatchImageJob, error)
	GetBatchImageJobByBatchID(ctx context.Context, batchID string) (*domain.BatchImageJob, error)
	GetBatchImageJobByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*domain.BatchImageJob, error)
	GetBatchImageJobByBatchIDForOwner(ctx context.Context, userID, apiKeyID int64, batchID string) (*domain.BatchImageJob, error)
	GetBatchImageJobByID(ctx context.Context, id int64) (*domain.BatchImageJob, error)
	ListBatchImageJobsForOwner(ctx context.Context, userID, apiKeyID int64, filter domain.BatchImageJobFilter) ([]*domain.BatchImageJob, error)
	TransitionBatchImageJobStatus(ctx context.Context, batchID, toStatus string, opts domain.BatchImageTransitionOptions) error
	// TouchBatchImageJobSubmitting 刷新未提交（created/uploading）job 的 updated_at，
	// 作为慢提交期间的心跳，防止被 stale 恢复扫描误杀。
	TouchBatchImageJobSubmitting(ctx context.Context, batchID string) error
	// FailStaleUnsubmittedBatchImageJob 原子地将仍处于 created/uploading 且
	// provider_job_name 为空、updated_at 早于 cutoff 的 job 转为 failed。
	// 返回 false 表示 job 已被并发推进（如已提交成功），调用方不得释放冻结。
	FailStaleUnsubmittedBatchImageJob(ctx context.Context, batchID string, cutoff time.Time, code, message string) (bool, error)
	UpdateBatchImageJobProviderOutputRef(ctx context.Context, batchID, providerOutputRef string) error
	UpdateBatchImageJobProviderSubmit(ctx context.Context, params domain.UpdateBatchImageJobProviderSubmitParams) error
	RecordBatchImageJobSubmitFailure(ctx context.Context, batchID, code, message string, markFailed bool) error
	MarkBatchImageJobSettled(ctx context.Context, params domain.MarkBatchImageJobSettledParams) error
	SetBatchImageJobSettlementFailed(ctx context.Context, batchID, code, message string) (int, error)
	CreateBatchImageItem(ctx context.Context, params domain.CreateBatchImageItemParams) (*domain.BatchImageItem, error)
	BulkCreateBatchImageItems(ctx context.Context, params []domain.CreateBatchImageItemParams) error
	ReplaceBatchImageItemsForJob(ctx context.Context, batchID string, items []domain.CreateBatchImageItemParams, counts domain.BatchImageCounts) error
	ListBatchImageItems(ctx context.Context, batchID string, filter domain.BatchImageItemFilter) ([]*domain.BatchImageItem, error)
	ListBatchImageItemsForOwner(ctx context.Context, userID, apiKeyID int64, batchID string, filter domain.BatchImageItemFilter) ([]*domain.BatchImageItem, error)
	GetBatchImageJobForDownload(ctx context.Context, userID, apiKeyID int64, batchID string) (*domain.BatchImageJob, error)
	GetBatchImageItemForDownload(ctx context.Context, batchID, customID string) (*domain.BatchImageItem, error)
	ListBatchImageItemsForDownload(ctx context.Context, batchID string, status string, limit int) ([]*domain.BatchImageItem, error)
	ListBatchImageJobsDueForInputCleanup(ctx context.Context, cutoff time.Time, limit int) ([]*domain.BatchImageJob, error)
	ListBatchImageJobsDueForOutputCleanup(ctx context.Context, now time.Time, limit int) ([]*domain.BatchImageJob, error)
	ListStaleUnsubmittedBatchImageJobs(ctx context.Context, cutoff time.Time, limit int) ([]*domain.BatchImageJob, error)
	MarkBatchImageInputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error
	MarkBatchImageOutputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error
	MarkBatchImageDownloaded(ctx context.Context, batchID string, downloadedAt time.Time) error
	MarkBatchImageJobUserDeleted(ctx context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error
	SetBatchImageOutputExpiresAt(ctx context.Context, batchID string, expiresAt time.Time) error
	RecordBatchImageCleanupFailure(ctx context.Context, batchID, code, message string) error
	AppendBatchImageEvent(ctx context.Context, batchID, eventType string, payload any) error
}

// BatchImageJobLock represents an acquired job lock that can be released.
type BatchImageJobLock interface {
	Release(ctx context.Context) error
}

// BatchImageQueue is the ready/delayed/active queue contract for batch image
// jobs, including job-lock acquisition integrated with reservation.
type BatchImageQueue interface {
	Enqueue(ctx context.Context, batchID string) error
	Reserve(ctx context.Context, blockTimeout time.Duration) (domain.ReservedBatchImageJob, error)
	RequeueAfter(ctx context.Context, batchID string, delay time.Duration) error
	Ack(ctx context.Context, batchID string) error
	Heartbeat(ctx context.Context, batchID string) error
	MoveDueDelayedToReady(ctx context.Context, limit int) (int, error)
	RecoverStaleActive(ctx context.Context, staleAfter time.Duration, limit int) (int, error)
	TryAcquireJobLock(ctx context.Context, batchID string, ttl time.Duration) (BatchImageJobLock, bool, error)
}

// BatchImageJobLockRefresher 是可选的锁续期能力；由具体锁实现按需提供。
type BatchImageJobLockRefresher interface {
	Refresh(ctx context.Context, ttl time.Duration) error
}

// BatchImageDownloadLimiter 限制并发下载的获取/释放。
type BatchImageDownloadLimiter interface {
	Acquire(ctx context.Context, userID string, kind string) (BatchImageDownloadPermit, error)
}

// BatchImageDownloadPermit 表示一次已获取的下载许可。
type BatchImageDownloadPermit interface {
	Release(ctx context.Context) error
}
