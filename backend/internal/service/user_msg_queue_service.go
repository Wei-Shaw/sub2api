package service

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// UserMsgQueueCache 用户消息串行队列 Redis 缓存接口
type UserMsgQueueCache interface {
	// AcquireLock 尝试获取账号级串行锁
	AcquireLock(ctx context.Context, accountID int64, requestID string, lockTtlMs int) (acquired bool, err error)
	// ReleaseLock 释放锁并记录完成时间
	ReleaseLock(ctx context.Context, accountID int64, requestID string) (released bool, err error)
	// GetLastCompletedMs 获取上次完成时间（毫秒时间戳，Redis TIME 源）
	GetLastCompletedMs(ctx context.Context, accountID int64) (int64, error)
	// GetCurrentTimeMs 获取 Redis 服务器当前时间（毫秒），与 ReleaseLock 记录的时间源一致
	GetCurrentTimeMs(ctx context.Context) (int64, error)
	// ForceReleaseLock 强制释放锁（孤儿锁清理）
	ForceReleaseLock(ctx context.Context, accountID int64) error
	// ScanLockKeys 扫描 PTTL == -1 的孤儿锁 key，返回 accountID 列表
	ScanLockKeys(ctx context.Context, maxCount int) ([]int64, error)
}

// QueueLockResult 锁获取结果
type QueueLockResult struct {
	Acquired  bool
	RequestID string
}

// UserMessageQueueService 用户消息串行队列服务
// 对真实用户消息实施账号级串行化 + RPM 自适应延迟
type UserMessageQueueService struct {
	cache    UserMsgQueueCache
	rpmCache RPMCache
	cfg      *config.UserMessageQueueConfig
	stopCh   chan struct{} // graceful shutdown
	stopOnce sync.Once     // 确保 Stop() 并发安全
}

// NewUserMessageQueueService 创建用户消息串行队列服务
func NewUserMessageQueueService(cache UserMsgQueueCache, rpmCache RPMCache, cfg *config.UserMessageQueueConfig) *UserMessageQueueService {
	return &UserMessageQueueService{
		cache:    cache,
		rpmCache: rpmCache,
		cfg:      cfg,
		stopCh:   make(chan struct{}),
	}
}

// IsRealUserMessage 检测是否为真实用户消息（非 tool_result）
// 与 claude-relay-service 的检测逻辑一致：
// 1. messages 非空
// 2. 最后一条消息 role == "user"
// 3. 最后一条消息 content（如果是数组）中不含 type:"tool_result" / "tool_use_result"
func IsRealUserMessage(parsed *ParsedRequest) bool {
	if parsed == nil || len(parsed.Messages) == 0 {
		return false
	}

	lastMsg := parsed.Messages[len(parsed.Messages)-1]
	msgMap, ok := lastMsg.(map[string]any)
	if !ok {
		return false
	}

	role, _ := msgMap["role"].(string)
	if role != "user" {
		return false
	}

	// 检查 content 是否包含 tool_result 类型
	content, ok := msgMap["content"]
	if !ok {
		return true // 没有 content 字段，视为普通用户消息
	}

	contentArr, ok := content.([]any)
	if !ok {
		return true // content 不是数组（可能是 string），视为普通用户消息
	}

	for _, item := range contentArr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if itemType == "tool_result" || itemType == "tool_use_result" {
			return false
		}
	}
	return true
}

// TryAcquire 尝试立即获取串行锁
func (s *UserMessageQueueService) TryAcquire(ctx context.Context, accountID int64) (*QueueLockResult, error) {
	if s.cache == nil {
		return &QueueLockResult{Acquired: true}, nil // fail-open
	}

	requestID := generateUMQRequestID()
	lockTTL := s.cfg.LockTTLMs
	if lockTTL <= 0 {
		lockTTL = 120000
	}

	acquired, err := s.cache.AcquireLock(ctx, accountID, requestID, lockTTL)
	if err != nil {
		logger.LegacyPrintf("service.umq", "AcquireLock failed for account %d: %v", accountID, err)
		return &QueueLockResult{Acquired: true}, nil // fail-open
	}

	return &QueueLockResult{
		Acquired:  acquired,
		RequestID: requestID,
	}, nil
}

// Release 释放串行锁
func (s *UserMessageQueueService) Release(ctx context.Context, accountID int64, requestID string) error {
	if s.cache == nil || requestID == "" {
		return nil
	}
	released, err := s.cache.ReleaseLock(ctx, accountID, requestID)
	if err != nil {
		logger.LegacyPrintf("service.umq", "ReleaseLock failed for account %d: %v", accountID, err)
		return err
	}
	if !released {
		logger.LegacyPrintf("service.umq", "ReleaseLock no-op for account %d (requestID mismatch or expired)", accountID)
	}
	return nil
}

// EnforceDelay 根据 RPM 负载执行自适应延迟
// 使用 Redis TIME 确保与 releaseLockScript 记录的时间源一致
func (s *UserMessageQueueService) EnforceDelay(ctx context.Context, accountID int64, baseRPM int) error {
	if s.cache == nil {
		return nil
	}

	// 先检查历史记录：没有历史则无需延迟，避免不必要的 RPM 查询
	lastMs, err := s.cache.GetLastCompletedMs(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("service.umq", "GetLastCompletedMs failed for account %d: %v", accountID, err)
		return nil // fail-open
	}
	if lastMs == 0 {
		return nil // 没有历史记录，无需延迟
	}

	delay := s.CalculateRPMAwareDelay(ctx, accountID, baseRPM)
	if delay <= 0 {
		return nil
	}

	// 获取 Redis 当前时间（与 lastMs 同源，避免时钟偏差）
	nowMs, err := s.cache.GetCurrentTimeMs(ctx)
	if err != nil {
		logger.LegacyPrintf("service.umq", "GetCurrentTimeMs failed: %v", err)
		return nil // fail-open
	}

	elapsed := time.Duration(nowMs-lastMs) * time.Millisecond
	if elapsed < 0 {
		// 时钟异常（Redis 故障转移等），fail-open
		return nil
	}
	remaining := delay - elapsed
	if remaining <= 0 {
		return nil
	}

	// 执行延迟
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// CalculateRPMAwareDelay 根据当前 RPM 负载计算自适应延迟
// ratio = currentRPM / baseRPM
// ratio < 0.5  → MinDelay
// 0.5 ≤ ratio < 0.8 → 线性插值 MinDelay..MaxDelay
// ratio ≥ 0.8 → MaxDelay
// 返回值包含 ±15% 随机抖动（anti-detection + 避免惊群效应）
func (s *UserMessageQueueService) CalculateRPMAwareDelay(ctx context.Context, accountID int64, baseRPM int) time.Duration {
	minDelay := time.Duration(s.cfg.MinDelayMs) * time.Millisecond
	maxDelay := time.Duration(s.cfg.MaxDelayMs) * time.Millisecond

	if minDelay <= 0 {
		minDelay = 200 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 2000 * time.Millisecond
	}
	// 防止配置错误：minDelay > maxDelay 时交换
	if minDelay > maxDelay {
		minDelay, maxDelay = maxDelay, minDelay
	}

	var baseDelay time.Duration

	if baseRPM <= 0 || s.rpmCache == nil {
		baseDelay = minDelay
	} else {
		currentRPM, err := s.rpmCache.GetRPM(ctx, accountID)
		if err != nil {
			logger.LegacyPrintf("service.umq", "GetRPM failed for account %d: %v", accountID, err)
			baseDelay = minDelay // fail-open
		} else {
			ratio := float64(currentRPM) / float64(baseRPM)
			if ratio < 0.5 {
				baseDelay = minDelay
			} else if ratio >= 0.8 {
				baseDelay = maxDelay
			} else {
				// 线性插值: 0.5 → minDelay, 0.8 → maxDelay
				t := (ratio - 0.5) / 0.3
				interpolated := float64(minDelay) + t*(float64(maxDelay)-float64(minDelay))
				baseDelay = time.Duration(math.Round(interpolated))
			}
		}
	}

	// ±15% 随机抖动
	return applyJitter(baseDelay, 0.15)
}

// StartCleanupWorker 启动孤儿锁清理 worker
// 定期 SCAN umq:*:lock 并清理 PTTL == -1 的异常锁（PTTL 检查在 cache.ScanLockKeys 内完成）
func (s *UserMessageQueueService) StartCleanupWorker(interval time.Duration) {
	if s == nil || s.cache == nil || interval <= 0 {
		return
	}

	runCleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		accountIDs, err := s.cache.ScanLockKeys(ctx, 1000)
		if err != nil {
			logger.LegacyPrintf("service.umq", "Cleanup scan failed: %v", err)
			return
		}

		cleaned := 0
		for _, accountID := range accountIDs {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := s.cache.ForceReleaseLock(cleanCtx, accountID); err != nil {
				logger.LegacyPrintf("service.umq", "Cleanup force release failed for account %d: %v", accountID, err)
			} else {
				cleaned++
			}
			cleanCancel()
		}

		if cleaned > 0 {
			logger.LegacyPrintf("service.umq", "Cleanup completed: released %d orphaned locks", cleaned)
		}
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				runCleanup()
			}
		}
	}()
}

// Stop 停止后台 cleanup worker
func (s *UserMessageQueueService) Stop() {
	if s != nil && s.stopCh != nil {
		s.stopOnce.Do(func() {
			close(s.stopCh)
		})
	}
}

// applyJitter 对延迟值施加 ±jitterPct 的随机抖动
// 使用 math/rand/v2（Go 1.22+ 自动使用 crypto/rand 种子），与 nextBackoff 一致
// 例如 applyJitter(200ms, 0.15) 返回 170ms ~ 230ms
func applyJitter(d time.Duration, jitterPct float64) time.Duration {
	if d <= 0 || jitterPct <= 0 {
		return d
	}
	// [-jitterPct, +jitterPct]
	jitter := (rand.Float64()*2 - 1) * jitterPct
	return time.Duration(float64(d) * (1 + jitter))
}

// generateUMQRequestID 生成唯一请求 ID（与 generateRequestID 一致的 fallback 模式）
func generateUMQRequestID() string {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
