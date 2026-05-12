package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"
)

// ── Outbox event types ──────────────────────────────────────────────────────

const (
	// EventTypeAuthCacheInvalidate 失效 auth_snapshot 缓存。
	EventTypeAuthCacheInvalidate = "auth_cache.invalidate"
	// EventTypeRateCacheInvalidate 失效 user_group_rate 缓存。
	EventTypeRateCacheInvalidate = "rate_cache.invalidate"
	// EventTypeMixedCacheInvalidate 同时失效 auth_snapshot 和 user_group_rate 缓存。
	EventTypeMixedCacheInvalidate = "mixed_cache.invalidate"

	// Reason 常量——触发缓存失效的业务原因。
	ReasonPoolMemberRemoved = "pool_member_removed"
	ReasonPoolGrantRemoved  = "pool_grant_removed"
	ReasonPoolGrantReplaced = "pool_grant_replaced"
	ReasonPoolDisabled      = "pool_disabled"
	ReasonPoolDeleted       = "pool_deleted"
	ReasonGroupDeleted      = "group_deleted"
	ReasonRPMTightened      = "rpm_tightened"
	ReasonRateChanged       = "rate_changed"
	ReasonPermissionRevoked = "permission_revoked"

	// CacheType 常量——需要失效的缓存层。
	CacheTypeAuthSnapshot  = "auth_snapshot"
	CacheTypeUserGroupRate = "user_group_rate"

	// Outbox row status 常量。
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusSucceeded  = "succeeded"
	OutboxStatusFailed     = "failed"
	OutboxStatusDead       = "dead"

	// EventPayloadSchemaVersion 当前 payload schema 版本。
	EventPayloadSchemaVersion = 1
)

// CacheInvalidationEvent outbox 事件，对应 cache_invalidation_outbox 表的一行。
type CacheInvalidationEvent struct {
	ID             int64
	EventType      string // auth_cache.invalidate | rate_cache.invalidate | mixed_cache.invalidate
	AggregateType  string // user_pool | user_pool_member | user_pool_group_grant | group 等
	AggregateID    *int64
	Reason         string   // pool_member_removed | pool_grant_removed | ...
	CacheTypes     []string // 至少含 auth_snapshot；rate 变化时同时含 user_group_rate
	Payload        EventPayload
	Status         string
	Attempts       int
	MaxAttempts    int
	NextAttemptAt  time.Time
	LockedAt       *time.Time
	LockedBy       string
	ProcessedAt    *time.Time
	LastError      string
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EventPayload outbox 事件 payload，序列化为 JSONB 存储。
// auth_cache_keys 存储的是已经 hash 过的 auth cache key，禁止存储明文 API Key。
type EventPayload struct {
	SchemaVersion    int         `json:"schema_version"`
	Operation        string      `json:"operation"`
	AffectedUserIDs  []int64     `json:"affected_user_ids,omitempty"`
	AffectedGroupIDs []int64     `json:"affected_group_ids,omitempty"`
	AuthCacheKeys    []string    `json:"auth_cache_keys,omitempty"` // 已 hash 的 auth cache key
	RatePairs        []RatePair  `json:"rate_pairs,omitempty"`
	DiffSummary      DiffSummary `json:"diff_summary"`
}

// RatePair 标识一个需要失效 rate cache 的 (user_id, group_id) 组合。
type RatePair struct {
	UserID  int64 `json:"user_id"`
	GroupID int64 `json:"group_id"`
}

// DiffSummary 描述本次变更的语义，用于下游判断失效策略的严重程度。
type DiffSummary struct {
	PermissionRevoked bool `json:"permission_revoked"`
	RPMTightened      bool `json:"rpm_tightened"`
	RateChanged       bool `json:"rate_changed"`
}

// ── Repository interface ─────────────────────────────────────────────────────

// CacheInvalidationOutboxRepository 提供 cache_invalidation_outbox 表的读写接口。
// Enqueue 必须在调用方已有的事务上下文中执行，失败会导致外层业务事务回滚。
type CacheInvalidationOutboxRepository interface {
	// Enqueue 将事件写入 outbox 表，使用当前上下文中的事务。
	// 若 ctx 携带活跃事务，则在该事务中执行；否则在非事务模式下写入。
	Enqueue(ctx context.Context, event CacheInvalidationEvent) error

	// ClaimReady 以 FOR UPDATE SKIP LOCKED 抢占就绪行（status IN ('pending','failed') AND next_attempt_at <= NOW()），
	// 置为 processing 并写入 locked_by/locked_at，返回已锁定的事件列表。
	ClaimReady(ctx context.Context, workerID string, limit int, lockTimeout time.Duration) ([]CacheInvalidationEvent, error)

	// MarkSucceeded 将指定行标记为 succeeded 并记录 processed_at。
	MarkSucceeded(ctx context.Context, id int64) error

	// MarkFailed 将指定行标记为 failed，记录错误和下次重试时间。
	MarkFailed(ctx context.Context, id int64, err error, nextAttemptAt time.Time) error

	// MarkDead 将指定行标记为 dead（超过最大重试次数），记录最终错误。
	MarkDead(ctx context.Context, id int64, err error) error

	// RequeueStaleProcessing 将 locked_at 早于 olderThan 的 processing 行退回 pending，
	// 返回受影响行数。用于 worker 启动或周期性心跳检测死锁行。
	RequeueStaleProcessing(ctx context.Context, olderThan time.Time) (int64, error)
}

// ── CacheInvalidationOutboxWorker ────────────────────────────────────────────

// CacheInvalidationOutboxWorker polls the outbox table and executes cache invalidations
// with exponential back-off retry and dead-letter escalation.
type CacheInvalidationOutboxWorker struct {
	repo            CacheInvalidationOutboxRepository
	authInvalidator StrictAPIKeyAuthCacheInvalidator
	rateInvalidator UserGroupRateCacheInvalidator
	pollInterval    time.Duration
	batchSize       int
	lockTimeout     time.Duration
	maxAttempts     int
	deadLetterAlert int
	workerID        string

	stopCh chan struct{}
	stopWg sync.WaitGroup
}

// NewCacheInvalidationOutboxWorker creates a worker with the provided config.
func NewCacheInvalidationOutboxWorker(
	repo CacheInvalidationOutboxRepository,
	authInvalidator StrictAPIKeyAuthCacheInvalidator,
	rateInvalidator UserGroupRateCacheInvalidator,
	pollInterval time.Duration,
	batchSize int,
	lockTimeout time.Duration,
	maxAttempts int,
	deadLetterAlert int,
	workerID string,
) *CacheInvalidationOutboxWorker {
	if batchSize <= 0 {
		batchSize = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 12
	}
	if workerID == "" {
		workerID = "outbox-worker"
	}
	return &CacheInvalidationOutboxWorker{
		repo:            repo,
		authInvalidator: authInvalidator,
		rateInvalidator: rateInvalidator,
		pollInterval:    pollInterval,
		batchSize:       batchSize,
		lockTimeout:     lockTimeout,
		maxAttempts:     maxAttempts,
		deadLetterAlert: deadLetterAlert,
		workerID:        workerID,
		stopCh:          make(chan struct{}),
	}
}

// Start begins the background polling loop.  Must be called once after construction.
func (w *CacheInvalidationOutboxWorker) Start() {
	w.stopWg.Add(1)
	go func() {
		defer w.stopWg.Done()
		w.run()
	}()
}

// Stop signals the worker to stop and waits for the current poll cycle to finish.
func (w *CacheInvalidationOutboxWorker) Stop() {
	close(w.stopCh)
	w.stopWg.Wait()
}

func (w *CacheInvalidationOutboxWorker) run() {
	// Requeue stale processing rows on startup.
	w.requeueStale(context.Background())

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.requeueStale(context.Background())
			w.processBatch(context.Background())
		}
	}
}

func (w *CacheInvalidationOutboxWorker) requeueStale(ctx context.Context) {
	olderThan := time.Now().Add(-w.lockTimeout)
	n, err := w.repo.RequeueStaleProcessing(ctx, olderThan)
	if err != nil {
		slog.Warn("outbox_worker: requeue_stale failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("outbox_worker: requeued stale processing rows", "count", n)
	}
}

func (w *CacheInvalidationOutboxWorker) processBatch(ctx context.Context) {
	events, err := w.repo.ClaimReady(ctx, w.workerID, w.batchSize, w.lockTimeout)
	if err != nil {
		slog.Warn("outbox_worker: claim_ready failed", "error", err)
		return
	}
	for _, ev := range events {
		w.processEvent(ctx, ev)
	}
}

func (w *CacheInvalidationOutboxWorker) processEvent(ctx context.Context, ev CacheInvalidationEvent) {
	var processErr error

	for _, cacheType := range ev.CacheTypes {
		switch cacheType {
		case CacheTypeAuthSnapshot:
			if len(ev.Payload.AuthCacheKeys) > 0 && w.authInvalidator != nil {
				// auth_cache_keys 已是 hash 后的 cache key，不能再次 hash。
				if err := w.authInvalidator.InvalidateAuthCacheByCacheKeysStrict(ctx, ev.Payload.AuthCacheKeys); err != nil {
					processErr = fmt.Errorf("invalidate auth cache: %w", err)
				}
			} else if len(ev.Payload.AffectedUserIDs) > 0 && w.authInvalidator != nil {
				if err := w.authInvalidator.InvalidateAuthCacheByUserIDsStrict(ctx, ev.Payload.AffectedUserIDs); err != nil {
					processErr = fmt.Errorf("invalidate auth cache by user ids: %w", err)
				}
			}
		case CacheTypeUserGroupRate:
			if len(ev.Payload.RatePairs) > 0 && w.rateInvalidator != nil {
				if err := w.rateInvalidator.InvalidateUserGroupRateCache(ctx, ev.Payload.RatePairs); err != nil {
					if processErr == nil {
						processErr = fmt.Errorf("invalidate rate cache: %w", err)
					}
				}
			}
		}
	}

	if processErr == nil {
		if err := w.repo.MarkSucceeded(ctx, ev.ID); err != nil {
			slog.Warn("outbox_worker: mark_succeeded failed", "id", ev.ID, "error", err)
		}
		return
	}

	// Failure: apply exponential back-off or dead-letter.
	newAttempts := ev.Attempts + 1
	maxAttempts := ev.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = w.maxAttempts
	}
	if newAttempts >= maxAttempts {
		slog.Error("outbox_worker: event exceeded max_attempts, moving to dead",
			"id", ev.ID, "attempts", newAttempts, "max", maxAttempts, "error", processErr)
		if err := w.repo.MarkDead(ctx, ev.ID, processErr); err != nil {
			slog.Warn("outbox_worker: mark_dead failed", "id", ev.ID, "error", err)
		}
		return
	}

	// Exponential back-off: 2^attempt seconds, capped at 1 hour.
	backoff := time.Duration(math.Pow(2, float64(newAttempts))) * time.Second
	if backoff > time.Hour {
		backoff = time.Hour
	}
	nextAttemptAt := time.Now().Add(backoff)
	slog.Warn("outbox_worker: event failed, scheduling retry",
		"id", ev.ID, "attempt", newAttempts, "next_at", nextAttemptAt, "error", processErr)
	if err := w.repo.MarkFailed(ctx, ev.ID, processErr, nextAttemptAt); err != nil {
		slog.Warn("outbox_worker: mark_failed failed", "id", ev.ID, "error", err)
	}
}
