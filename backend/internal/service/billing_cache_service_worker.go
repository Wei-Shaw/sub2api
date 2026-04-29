package service

// BillingCacheService 异步缓存写入工作池实现。
// 主 struct 在 billing_cache_service.go 持有 channel / WaitGroup 等字段，
// 本文件承载任务类型、生命周期管理、worker 派发与丢弃节流。

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// 缓存写入任务类型
type cacheWriteKind int

const (
	cacheWriteSetBalance cacheWriteKind = iota
	cacheWriteSetSubscription
	cacheWriteUpdateSubscriptionUsage
	cacheWriteDeductBalance
	cacheWriteUpdateRateLimitUsage
)

// 异步缓存写入工作池配置
//
// 性能优化说明：
// 原实现在请求热路径中使用 goroutine 异步更新缓存，存在以下问题：
// 1. 每次请求创建新 goroutine，高并发下产生大量短生命周期 goroutine
// 2. 无法控制并发数量，可能导致 Redis 连接耗尽
// 3. goroutine 创建/销毁带来额外开销
//
// 新实现使用固定大小的工作池：
// 1. 预创建 10 个 worker goroutine，避免频繁创建销毁
// 2. 使用带缓冲的 channel（1000）作为任务队列，平滑写入峰值
// 3. 非阻塞写入，队列满时关键任务同步回退，非关键任务丢弃并告警
// 4. 统一超时控制，避免慢操作阻塞工作池
const (
	cacheWriteWorkerCount     = 10              // 工作协程数量
	cacheWriteBufferSize      = 1000            // 任务队列缓冲大小
	cacheWriteTimeout         = 2 * time.Second // 单个写入操作超时
	cacheWriteDropLogInterval = 5 * time.Second // 丢弃日志节流间隔
	balanceLoadTimeout        = 3 * time.Second
)

// cacheWriteTask 缓存写入任务
type cacheWriteTask struct {
	kind             cacheWriteKind
	userID           int64
	groupID          int64
	apiKeyID         int64
	balance          float64
	amount           float64
	subscriptionData *SubscriptionCacheData
}

// apiKeyRateLimitLoader defines the interface for loading rate limit data from DB.
type apiKeyRateLimitLoader interface {
	GetRateLimitData(ctx context.Context, keyID int64) (*APIKeyRateLimitData, error)
}

// Stop 关闭缓存写入工作池
func (s *BillingCacheService) Stop() {
	s.cacheWriteStopOnce.Do(func() {
		s.stopped.Store(true)

		s.cacheWriteMu.Lock()
		ch := s.cacheWriteChan
		if ch != nil {
			close(ch)
		}
		s.cacheWriteMu.Unlock()

		if ch == nil {
			return
		}
		s.cacheWriteWg.Wait()

		s.cacheWriteMu.Lock()
		if s.cacheWriteChan == ch {
			s.cacheWriteChan = nil
		}
		s.cacheWriteMu.Unlock()
	})
}

func (s *BillingCacheService) startCacheWriteWorkers() {
	ch := make(chan cacheWriteTask, cacheWriteBufferSize)
	s.cacheWriteChan = ch
	for i := 0; i < cacheWriteWorkerCount; i++ {
		s.cacheWriteWg.Add(1)
		go s.cacheWriteWorker(ch)
	}
}

// enqueueCacheWrite 尝试将任务入队，队列满时返回 false（并记录告警）。
func (s *BillingCacheService) enqueueCacheWrite(task cacheWriteTask) (enqueued bool) {
	if s.stopped.Load() {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	s.cacheWriteMu.RLock()
	defer s.cacheWriteMu.RUnlock()

	if s.cacheWriteChan == nil {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	select {
	case s.cacheWriteChan <- task:
		return true
	default:
		// 队列满时不阻塞主流程，交由调用方决定是否同步回退。
		s.logCacheWriteDrop(task, "full")
		return false
	}
}

// cacheWriteWorker 消费缓存写入队列。每个任务类型都被分派给独立的 processXxxTask
// 方法实现，本函数只负责 context 生命周期与类型分派。
func (s *BillingCacheService) cacheWriteWorker(ch <-chan cacheWriteTask) {
	defer s.cacheWriteWg.Done()
	for task := range ch {
		s.runCacheWriteTask(task)
	}
}

// runCacheWriteTask 在独立 context 内执行单个缓存写入任务。
//
// ---- panic 恢复 ----
// 下游 process* 方法依赖外部组件（Redis / QuotaService / DB）；任一
// 链路里出现 nil 解引用或其他意外 panic，都会把当前 worker goroutine 带走，
// 导致工作池容量永久减一、剩余任务堆积直至队列满（见 logCacheWriteDrop "full"）。
// 所以在任务入口设 recover：记录 panic、调用栈、任务类型，让 worker 能继续
// 消费后续任务（符合 CLAUDE.md §8「并发安全」：避免单个任务带走整个 goroutine）。
func (s *BillingCacheService) runCacheWriteTask(task cacheWriteTask) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("cache write task panic",
				"component", billingCacheLogComponent,
				"kind", cacheWriteKindName(task.kind),
				"user_id", task.userID,
				"group_id", task.groupID,
				"api_key_id", task.apiKeyID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	switch task.kind {
	case cacheWriteSetBalance:
		s.processSetBalanceTask(ctx, task)
	case cacheWriteSetSubscription:
		s.processSetSubscriptionTask(ctx, task)
	case cacheWriteUpdateSubscriptionUsage:
		s.processUpdateSubscriptionUsageTask(ctx, task)
	case cacheWriteDeductBalance:
		s.processDeductBalanceTask(ctx, task)
	case cacheWriteUpdateRateLimitUsage:
		s.processUpdateRateLimitUsageTask(ctx, task)
	}
}

// cacheWriteKindName 用于日志中的任务类型标识，便于排查丢弃原因。
func cacheWriteKindName(kind cacheWriteKind) string {
	switch kind {
	case cacheWriteSetBalance:
		return "set_balance"
	case cacheWriteSetSubscription:
		return "set_subscription"
	case cacheWriteUpdateSubscriptionUsage:
		return "update_subscription_usage"
	case cacheWriteDeductBalance:
		return "deduct_balance"
	case cacheWriteUpdateRateLimitUsage:
		return "update_rate_limit_usage"
	default:
		return "unknown"
	}
}

// logCacheWriteDrop 使用节流方式记录丢弃情况，并汇总丢弃数量。
func (s *BillingCacheService) logCacheWriteDrop(task cacheWriteTask, reason string) {
	var (
		countPtr *uint64
		lastPtr  *int64
	)
	switch reason {
	case "full":
		countPtr = &s.cacheWriteDropFullCount
		lastPtr = &s.cacheWriteDropFullLastLog
	case "closed":
		countPtr = &s.cacheWriteDropClosedCount
		lastPtr = &s.cacheWriteDropClosedLastLog
	default:
		return
	}

	atomic.AddUint64(countPtr, 1)
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(lastPtr)
	if now-last < int64(cacheWriteDropLogInterval) {
		return
	}
	if !atomic.CompareAndSwapInt64(lastPtr, last, now) {
		return
	}
	dropped := atomic.SwapUint64(countPtr, 0)
	if dropped == 0 {
		return
	}
	slog.Warn("cache write queue dropping tasks",
		"component", billingCacheLogComponent,
		"reason", reason,
		"dropped", dropped,
		"interval", cacheWriteDropLogInterval,
		"kind", cacheWriteKindName(task.kind),
		"user_id", task.userID,
		"group_id", task.groupID,
	)
}
