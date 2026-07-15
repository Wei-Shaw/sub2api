package service

import (
	"context"
	"time"
)

func globalTempUnschedulableEnabled(ctx context.Context, settingService *SettingService) bool {
	return settingService == nil || settingService.IsGlobalTempUnschedulableEnabled(ctx)
}

// TempUnschedState 临时不可调度状态
type TempUnschedState struct {
	UntilUnix       int64  `json:"until_unix"`        // 解除时间（Unix 时间戳）
	TriggeredAtUnix int64  `json:"triggered_at_unix"` // 触发时间（Unix 时间戳）
	StatusCode      int    `json:"status_code"`       // 触发的错误码
	MatchedKeyword  string `json:"matched_keyword"`   // 匹配的关键词
	RuleIndex       int    `json:"rule_index"`        // 触发的规则索引
	ErrorMessage    string `json:"error_message"`     // 错误消息
}

// TempUnschedCache 临时不可调度缓存接口
type TempUnschedCache interface {
	SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error
	GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error)
	DeleteTempUnsched(ctx context.Context, accountID int64) error
}

// TempUnschedulableBulkCleaner clears persisted temporary scheduling pauses.
type TempUnschedulableBulkCleaner interface {
	ClearAllTempUnschedulable(ctx context.Context) ([]int64, error)
}

// TempUnschedCacheBulkCleaner clears all temporary scheduling pause cache entries.
type TempUnschedCacheBulkCleaner interface {
	DeleteAllTempUnsched(ctx context.Context) error
}

// GlobalTempUnschedulableCleaner removes active state when the global switch is disabled.
type GlobalTempUnschedulableCleaner struct {
	repo           TempUnschedulableBulkCleaner
	cache          TempUnschedCache
	runtimeBlocker AccountRuntimeBlocker
}

func NewGlobalTempUnschedulableCleaner(
	repo TempUnschedulableBulkCleaner,
	cache TempUnschedCache,
	runtimeBlocker AccountRuntimeBlocker,
) *GlobalTempUnschedulableCleaner {
	return &GlobalTempUnschedulableCleaner{
		repo:           repo,
		cache:          cache,
		runtimeBlocker: runtimeBlocker,
	}
}

func (c *GlobalTempUnschedulableCleaner) Clear(ctx context.Context) (int, error) {
	var accountIDs []int64
	if c != nil && c.repo != nil {
		var err error
		accountIDs, err = c.repo.ClearAllTempUnschedulable(ctx)
		if err != nil {
			return 0, err
		}
	}

	var cacheErr error
	if c != nil && c.cache != nil {
		if bulkCleaner, ok := c.cache.(TempUnschedCacheBulkCleaner); ok {
			cacheErr = bulkCleaner.DeleteAllTempUnsched(ctx)
		}
	}

	if c != nil && c.runtimeBlocker != nil {
		for _, accountID := range accountIDs {
			c.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
		}
	}
	return len(accountIDs), cacheErr
}

// TimeoutCounterCache 超时计数器缓存接口
type TimeoutCounterCache interface {
	// IncrementTimeoutCount 增加账户的超时计数，返回当前计数值
	// windowMinutes 是计数窗口时间（分钟），超过此时间计数器会自动重置
	IncrementTimeoutCount(ctx context.Context, accountID int64, windowMinutes int) (int64, error)
	// GetTimeoutCount 获取账户当前的超时计数
	GetTimeoutCount(ctx context.Context, accountID int64) (int64, error)
	// ResetTimeoutCount 重置账户的超时计数
	ResetTimeoutCount(ctx context.Context, accountID int64) error
	// GetTimeoutCountTTL 获取计数器剩余过期时间
	GetTimeoutCountTTL(ctx context.Context, accountID int64) (time.Duration, error)
}
