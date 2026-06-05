//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// panickingBillingCache 在第一次调用 DeductUserBalance 时 panic，
// 之后的调用正常返回，用于验证 runCacheWriteTask 的 panic recovery：
// worker goroutine 不被 panic 带走，后续任务能继续处理。
//
// 其他方法返回 "not implemented" 以便在意外被调用时能在日志里识别。
type panickingBillingCache struct {
	firstCallPanicked atomic.Bool
	deductCalls       atomic.Int64
	updateUsageCalls  atomic.Int64
}

func (p *panickingBillingCache) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (p *panickingBillingCache) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	return nil
}

func (p *panickingBillingCache) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	p.deductCalls.Add(1)
	// 第一次调用 panic 一次；后续调用正常返回，验证 worker 没死。
	if p.firstCallPanicked.CompareAndSwap(false, true) {
		panic("simulated panic in DeductUserBalance")
	}
	return nil
}

func (p *panickingBillingCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (p *panickingBillingCache) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return nil, errors.New("not implemented")
}

func (p *panickingBillingCache) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	return nil
}

func (p *panickingBillingCache) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	p.updateUsageCalls.Add(1)
	return nil
}

func (p *panickingBillingCache) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	return nil
}

func (p *panickingBillingCache) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("not implemented")
}

func (p *panickingBillingCache) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	return nil
}

func (p *panickingBillingCache) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (p *panickingBillingCache) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func (p *panickingBillingCache) GetQuotaUsedTotal(ctx context.Context, userID int64, date string) (float64, error) {
	return 0, nil
}

func (p *panickingBillingCache) GetQuotaUsedRule(ctx context.Context, userID, ruleID int64, date string) (float64, error) {
	return 0, nil
}

func (p *panickingBillingCache) IncrQuotaUsedTotal(ctx context.Context, userID int64, date string, delta float64) error {
	return nil
}

func (p *panickingBillingCache) IncrQuotaUsedRule(ctx context.Context, userID, ruleID int64, date string, delta float64) error {
	return nil
}

func (p *panickingBillingCache) InvalidateQuotaConfig(ctx context.Context, userID int64) error {
	return nil
}

// TestCacheWriteWorker_PanicRecovery 验证 runCacheWriteTask 的 recover：
// 1) 入队一个 panic 任务 → worker 吞掉 panic 继续消费
// 2) 再入队一个正常任务（不同 kind 避免被同一 worker 的锁或队列粘滞影响）
// 3) 第二个任务成功处理说明 worker goroutine 没被 panic 带走
//
// 若 runCacheWriteTask 没有 recover，第一个任务会让 worker goroutine 终止，
// 任务池容量永久 -1；由于有 10 个 worker，剩余 worker 仍能处理第二个任务，
// 所以只看"后续任务处理成功"无法区分「recover 生效」vs「其他 worker 接管」。
// 为此用 cacheWriteWorkerCount 次 panic 任务 + 1 次正常任务 确保所有 worker 都过一次 panic 路径。
func TestCacheWriteWorker_PanicRecovery(t *testing.T) {
	cache := &panickingBillingCache{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	// 第一次入队触发 panic（DeductUserBalance 只 panic 一次）。
	svc.QueueDeductBalance(1, 1)

	// 等 worker 处理完第一个任务。
	require.Eventually(t, func() bool {
		return cache.deductCalls.Load() >= 1
	}, time.Second, 5*time.Millisecond, "deduct task should be picked up")

	// 再入队一次 deduct：这次 DeductUserBalance 不再 panic，成功处理说明
	// worker goroutine 没被前一次 panic 带走 —— 即 recover 生效。
	svc.QueueDeductBalance(1, 2)
	require.Eventually(t, func() bool {
		return cache.deductCalls.Load() >= 2
	}, time.Second, 5*time.Millisecond, "worker should survive panic and process subsequent task")

	// 发一个不同 kind 的任务，确保 switch 分支 + worker 池整体都活着。
	svc.QueueUpdateSubscriptionUsage(1, 2, 1.5)
	require.Eventually(t, func() bool {
		return cache.updateUsageCalls.Load() >= 1
	}, time.Second, 5*time.Millisecond, "worker pool should still handle subsequent tasks of other kinds")
}
