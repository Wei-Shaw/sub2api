package service

// BillingCacheService 余额缓存相关方法。
// 主 struct 与入队逻辑在 billing_cache_service.go / billing_cache_service_worker.go，
// 本文件承载余额读取、回源、异步扣减与失效。

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// GetUserBalance 获取用户余额（优先从缓存读取）
func (s *BillingCacheService) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	if s.cache == nil {
		// Redis不可用，直接查询数据库
		return s.getUserBalanceFromDB(ctx, userID)
	}

	// 尝试从缓存读取
	balance, err := s.cache.GetUserBalance(ctx, userID)
	if err == nil {
		return balance, nil
	}

	// 缓存未命中：singleflight 合并同一 userID 的并发回源请求。
	value, err, _ := s.balanceLoadSF.Do(strconv.FormatInt(userID, 10), func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), balanceLoadTimeout)
		defer cancel()

		balance, err := s.getUserBalanceFromDB(loadCtx, userID)
		if err != nil {
			return nil, err
		}

		// 异步建立缓存
		_ = s.enqueueCacheWrite(cacheWriteTask{
			kind:    cacheWriteSetBalance,
			userID:  userID,
			balance: balance,
		})
		return balance, nil
	})
	if err != nil {
		return 0, err
	}
	balance, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected balance type: %T", value)
	}
	return balance, nil
}

// getUserBalanceFromDB 从数据库获取用户余额
func (s *BillingCacheService) getUserBalanceFromDB(ctx context.Context, userID int64) (float64, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get user balance: %w", err)
	}
	return user.Balance, nil
}

// setBalanceCache 设置余额缓存
func (s *BillingCacheService) setBalanceCache(ctx context.Context, userID int64, balance float64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.SetUserBalance(ctx, userID, balance); err != nil {
		slog.Warn("set balance cache failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"error", err,
		)
	}
}

// DeductBalanceCache 扣减余额缓存（同步调用）
func (s *BillingCacheService) DeductBalanceCache(ctx context.Context, userID int64, amount float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.DeductUserBalance(ctx, userID, amount)
}

// QueueDeductBalance 异步扣减余额缓存
func (s *BillingCacheService) QueueDeductBalance(userID int64, amount float64) {
	if s.cache == nil {
		return
	}
	// 队列满时同步回退，避免关键扣减被静默丢弃。
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: userID,
		amount: amount,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.DeductBalanceCache(ctx, userID, amount); err != nil {
		slog.Warn("deduct balance cache fallback failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"error", err,
		)
	}
}

// InvalidateUserBalance 失效用户余额缓存
func (s *BillingCacheService) InvalidateUserBalance(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateUserBalance(ctx, userID); err != nil {
		slog.Warn("invalidate balance cache failed",
			"component", billingCacheLogComponent,
			"user_id", userID,
			"error", err,
		)
		return err
	}
	return nil
}

// processSetBalanceTask 处理 cacheWriteSetBalance：异步把 DB 读回的余额写入缓存。
func (s *BillingCacheService) processSetBalanceTask(ctx context.Context, task cacheWriteTask) {
	s.setBalanceCache(ctx, task.userID, task.balance)
}

// processDeductBalanceTask 处理 cacheWriteDeductBalance：异步扣减缓存中的余额。
func (s *BillingCacheService) processDeductBalanceTask(ctx context.Context, task cacheWriteTask) {
	if s.cache == nil {
		return
	}
	if err := s.cache.DeductUserBalance(ctx, task.userID, task.amount); err != nil {
		slog.Warn("deduct balance cache failed",
			"component", billingCacheLogComponent,
			"user_id", task.userID,
			"error", err,
		)
	}
}
