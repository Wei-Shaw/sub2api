package service

// BillingCacheService 订阅缓存相关方法。
// 主 struct 与入队逻辑在 billing_cache_service.go / billing_cache_service_worker.go，
// 本文件承载订阅读取、回源、用量更新与失效。

import (
	"context"
	"fmt"
	"log/slog"
)

// GetSubscriptionStatus 获取订阅状态（优先从缓存读取）
func (s *BillingCacheService) GetSubscriptionStatus(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	if s.cache == nil {
		return s.getSubscriptionFromDB(ctx, userID, groupID)
	}

	// 尝试从缓存读取
	cacheData, err := s.cache.GetSubscriptionCache(ctx, userID, groupID)
	if err == nil && cacheData != nil {
		return cacheData, nil
	}

	// 缓存未命中，从数据库读取
	data, err := s.getSubscriptionFromDB(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}

	// 异步建立缓存
	_ = s.enqueueCacheWrite(cacheWriteTask{
		kind:             cacheWriteSetSubscription,
		userID:           userID,
		groupID:          groupID,
		subscriptionData: data,
	})

	return data, nil
}

// getSubscriptionFromDB 从数据库获取订阅数据
func (s *BillingCacheService) getSubscriptionFromDB(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	sub, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	return &SubscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		DailyUsage:   sub.DailyUsageUSD,
		WeeklyUsage:  sub.WeeklyUsageUSD,
		MonthlyUsage: sub.MonthlyUsageUSD,
		Version:      sub.UpdatedAt.Unix(),
	}, nil
}

// setSubscriptionCache 设置订阅缓存
func (s *BillingCacheService) setSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) {
	if s.cache == nil || data == nil {
		return
	}
	if err := s.cache.SetSubscriptionCache(ctx, userID, groupID, data); err != nil {
		slog.Warn("set subscription cache failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"group_id", groupID,
			"error", err,
		)
	}
}

// UpdateSubscriptionUsage 更新订阅用量缓存（同步调用）
func (s *BillingCacheService) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, costUSD float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD)
}

// QueueUpdateSubscriptionUsage 异步更新订阅用量缓存
func (s *BillingCacheService) QueueUpdateSubscriptionUsage(userID, groupID int64, costUSD float64) {
	if s.cache == nil {
		return
	}
	// 队列满时同步回退，确保订阅用量及时更新。
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:    cacheWriteUpdateSubscriptionUsage,
		userID:  userID,
		groupID: groupID,
		amount:  costUSD,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD); err != nil {
		slog.Warn("update subscription cache fallback failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"group_id", groupID,
			"error", err,
		)
	}
}

// InvalidateSubscription 失效指定订阅缓存
func (s *BillingCacheService) InvalidateSubscription(ctx context.Context, userID, groupID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateSubscriptionCache(ctx, userID, groupID); err != nil {
		slog.Warn("invalidate subscription cache failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"group_id", groupID,
			"error", err,
		)
		return err
	}
	return nil
}

// processSetSubscriptionTask 处理 cacheWriteSetSubscription：异步写入订阅缓存。
func (s *BillingCacheService) processSetSubscriptionTask(ctx context.Context, task cacheWriteTask) {
	s.setSubscriptionCache(ctx, task.userID, task.groupID, task.subscriptionData)
}

// processUpdateSubscriptionUsageTask 处理 cacheWriteUpdateSubscriptionUsage：
// 异步累加订阅用量到缓存。
func (s *BillingCacheService) processUpdateSubscriptionUsageTask(ctx context.Context, task cacheWriteTask) {
	if s.cache == nil {
		return
	}
	if err := s.cache.UpdateSubscriptionUsage(ctx, task.userID, task.groupID, task.amount); err != nil {
		slog.Warn("update subscription cache failed",
			"component", billingCacheLogComponent,
			"user_id", task.userID,
			"group_id", task.groupID,
			"error", err,
		)
	}
}
