package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// 错误定义
// 注：ErrInsufficientBalance在redeem_service.go中定义
// 注：ErrDailyLimitExceeded/ErrWeeklyLimitExceeded/ErrMonthlyLimitExceeded在subscription_service.go中定义
var (
	ErrSubscriptionInvalid = errors.New("subscription is invalid or expired")
)

// subscriptionCacheData 订阅缓存数据结构（内部使用）
type subscriptionCacheData struct {
	Status       string
	ExpiresAt    time.Time
	DailyUsage   float64
	WeeklyUsage  float64
	MonthlyUsage float64
	Version      int64
}

// BillingCacheService 计费缓存服务
// 负责余额和订阅数据的缓存管理，提供高性能的计费资格检查
type BillingCacheService struct {
	cache    ports.BillingCache
	userRepo ports.UserRepository
	subRepo  ports.UserSubscriptionRepository
}

// NewBillingCacheService 创建计费缓存服务
func NewBillingCacheService(cache ports.BillingCache, userRepo ports.UserRepository, subRepo ports.UserSubscriptionRepository) *BillingCacheService {
	return &BillingCacheService{
		cache:    cache,
		userRepo: userRepo,
		subRepo:  subRepo,
	}
}

// ============================================
// 余额缓存方法
// ============================================

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

	// 缓存未命中，从数据库读取
	balance, err = s.getUserBalanceFromDB(ctx, userID)
	if err != nil {
		return 0, err
	}

	// 异步建立缓存
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.setBalanceCache(cacheCtx, userID, balance)
	}()

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
		log.Printf("Warning: set balance cache failed for user %d: %v", userID, err)
	}
}

// DeductBalanceCache 扣减余额缓存（异步调用，用于扣费后更新缓存）
func (s *BillingCacheService) DeductBalanceCache(ctx context.Context, userID int64, amount float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.DeductUserBalance(ctx, userID, amount)
}

// InvalidateUserBalance 失效用户余额缓存
func (s *BillingCacheService) InvalidateUserBalance(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateUserBalance(ctx, userID); err != nil {
		log.Printf("Warning: invalidate balance cache failed for user %d: %v", userID, err)
		return err
	}
	return nil
}

// ============================================
// 订阅缓存方法
// ============================================

// GetSubscriptionStatus 获取订阅状态（优先从缓存读取）
func (s *BillingCacheService) GetSubscriptionStatus(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	if s.cache == nil {
		return s.getSubscriptionFromDB(ctx, userID, groupID)
	}

	// 尝试从缓存读取
	cacheData, err := s.cache.GetSubscriptionCache(ctx, userID, groupID)
	if err == nil && cacheData != nil {
		return s.convertFromPortsData(cacheData), nil
	}

	// 缓存未命中，从数据库读取
	data, err := s.getSubscriptionFromDB(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}

	// 异步建立缓存
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.setSubscriptionCache(cacheCtx, userID, groupID, data)
	}()

	return data, nil
}

func (s *BillingCacheService) convertFromPortsData(data *ports.SubscriptionCacheData) *subscriptionCacheData {
	return &subscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

func (s *BillingCacheService) convertToPortsData(data *subscriptionCacheData) *ports.SubscriptionCacheData {
	return &ports.SubscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

// getSubscriptionFromDB 从数据库获取订阅数据
func (s *BillingCacheService) getSubscriptionFromDB(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	sub, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	return &subscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		DailyUsage:   sub.DailyUsageUSD,
		WeeklyUsage:  sub.WeeklyUsageUSD,
		MonthlyUsage: sub.MonthlyUsageUSD,
		Version:      sub.UpdatedAt.Unix(),
	}, nil
}

// setSubscriptionCache 设置订阅缓存
func (s *BillingCacheService) setSubscriptionCache(ctx context.Context, userID, groupID int64, data *subscriptionCacheData) {
	if s.cache == nil || data == nil {
		return
	}
	if err := s.cache.SetSubscriptionCache(ctx, userID, groupID, s.convertToPortsData(data)); err != nil {
		log.Printf("Warning: set subscription cache failed for user %d group %d: %v", userID, groupID, err)
	}
}

// UpdateSubscriptionUsage 更新订阅用量缓存（异步调用，用于扣费后更新缓存）
func (s *BillingCacheService) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, costUSD float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD)
}

// InvalidateSubscription 失效指定订阅缓存
func (s *BillingCacheService) InvalidateSubscription(ctx context.Context, userID, groupID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateSubscriptionCache(ctx, userID, groupID); err != nil {
		log.Printf("Warning: invalidate subscription cache failed for user %d group %d: %v", userID, groupID, err)
		return err
	}
	return nil
}

// ============================================
// 统一检查方法
// ============================================

// CheckBillingEligibility 检查用户是否有资格发起请求
// 余额模式：检查缓存余额 > 0
// 订阅模式：检查缓存用量未超过限额（Group限额从参数传入）
func (s *BillingCacheService) CheckBillingEligibility(ctx context.Context, user *model.User, apiKey *model.ApiKey, group *model.Group, subscription *model.UserSubscription) error {
	// 判断计费模式
	isSubscriptionMode := group != nil && group.IsSubscriptionType() && subscription != nil

	if isSubscriptionMode {
		return s.checkSubscriptionEligibility(ctx, user.ID, group, subscription)
	}

	return s.checkBalanceEligibility(ctx, user.ID)
}

// checkBalanceEligibility 检查余额模式资格
func (s *BillingCacheService) checkBalanceEligibility(ctx context.Context, userID int64) error {
	balance, err := s.GetUserBalance(ctx, userID)
	if err != nil {
		// 缓存/数据库错误，允许通过（降级处理）
		log.Printf("Warning: get user balance failed, allowing request: %v", err)
		return nil
	}

	if balance <= 0 {
		return ErrInsufficientBalance
	}

	return nil
}

// checkSubscriptionEligibility 检查订阅模式资格
func (s *BillingCacheService) checkSubscriptionEligibility(ctx context.Context, userID int64, group *model.Group, subscription *model.UserSubscription) error {
	// 获取订阅缓存数据
	subData, err := s.GetSubscriptionStatus(ctx, userID, group.ID)
	if err != nil {
		// 缓存/数据库错误，降级使用传入的subscription进行检查
		log.Printf("Warning: get subscription cache failed, using fallback: %v", err)
		return s.checkSubscriptionLimitsFallback(subscription, group)
	}

	// 检查订阅状态
	if subData.Status != model.SubscriptionStatusActive {
		return ErrSubscriptionInvalid
	}

	// 检查是否过期
	if time.Now().After(subData.ExpiresAt) {
		return ErrSubscriptionInvalid
	}

	// 检查限额（使用传入的Group限额配置）
	if group.HasDailyLimit() && subData.DailyUsage >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}

	if group.HasWeeklyLimit() && subData.WeeklyUsage >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}

	if group.HasMonthlyLimit() && subData.MonthlyUsage >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}

	return nil
}

// checkSubscriptionLimitsFallback 降级检查订阅限额
func (s *BillingCacheService) checkSubscriptionLimitsFallback(subscription *model.UserSubscription, group *model.Group) error {
	if subscription == nil {
		return ErrSubscriptionInvalid
	}

	if !subscription.IsActive() {
		return ErrSubscriptionInvalid
	}

	if !subscription.CheckDailyLimit(group, 0) {
		return ErrDailyLimitExceeded
	}

	if !subscription.CheckWeeklyLimit(group, 0) {
		return ErrWeeklyLimitExceeded
	}

	if !subscription.CheckMonthlyLimit(group, 0) {
		return ErrMonthlyLimitExceeded
	}

	return nil
}
