package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/service/ports"

	"gorm.io/gorm"
)

var (
	ErrSubscriptionNotFound      = errors.New("subscription not found")
	ErrSubscriptionExpired       = errors.New("subscription has expired")
	ErrSubscriptionSuspended     = errors.New("subscription is suspended")
	ErrSubscriptionAlreadyExists = errors.New("subscription already exists for this user and group")
	ErrGroupNotSubscriptionType  = errors.New("group is not a subscription type")
	ErrDailyLimitExceeded        = errors.New("daily usage limit exceeded")
	ErrWeeklyLimitExceeded       = errors.New("weekly usage limit exceeded")
	ErrMonthlyLimitExceeded      = errors.New("monthly usage limit exceeded")
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	groupRepo           ports.GroupRepository
	userSubRepo         ports.UserSubscriptionRepository
	billingCacheService *BillingCacheService
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(groupRepo ports.GroupRepository, userSubRepo ports.UserSubscriptionRepository, billingCacheService *BillingCacheService) *SubscriptionService {
	return &SubscriptionService{
		groupRepo:           groupRepo,
		userSubRepo:         userSubRepo,
		billingCacheService: billingCacheService,
	}
}

func (s *SubscriptionService) WithTx(tx *gorm.DB) *SubscriptionService {
	if s == nil {
		return nil
	}
	// 仅替换需要写入的仓库，保持其他依赖不变。
	return &SubscriptionService{
		groupRepo:           s.groupRepo,
		userSubRepo:         s.userSubRepo.WithTx(tx),
		billingCacheService: s.billingCacheService,
	}
}

// AssignSubscriptionInput 分配订阅输入
type AssignSubscriptionInput struct {
	UserID       int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

// AssignSubscription 分配订阅给用户（不允许重复分配）
func (s *SubscriptionService) AssignSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
	}

	// 检查是否已存在订阅
	exists, err := s.userSubRepo.ExistsByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrSubscriptionAlreadyExists
	}

	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, err
	}

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := input.UserID, input.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}

	return sub, nil
}

// AssignOrExtendSubscription 分配或续期订阅（用于兑换码等场景）
// 如果用户已有同分组的订阅：
//   - 未过期：从当前过期时间累加天数
//   - 已过期：从当前时间开始计算新的过期时间，并激活订阅
//
// 如果没有订阅：创建新订阅
func (s *SubscriptionService) AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, bool, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, false, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, false, ErrGroupNotSubscriptionType
	}

	// 查询是否已有订阅
	existingSub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		// 不存在记录是正常情况，其他错误需要返回
		existingSub = nil
	}

	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}

	// 已有订阅，执行续期
	if existingSub != nil {
		now := time.Now()
		var newExpiresAt time.Time

		if existingSub.ExpiresAt.After(now) {
			// 未过期：从当前过期时间累加
			newExpiresAt = existingSub.ExpiresAt.AddDate(0, 0, validityDays)
		} else {
			// 已过期：从当前时间开始计算
			newExpiresAt = now.AddDate(0, 0, validityDays)
		}

		// 更新过期时间
		if err := s.userSubRepo.ExtendExpiry(ctx, existingSub.ID, newExpiresAt); err != nil {
			return nil, false, fmt.Errorf("extend subscription: %w", err)
		}

		// 如果订阅已过期或被暂停，恢复为active状态
		if existingSub.Status != model.SubscriptionStatusActive {
			if err := s.userSubRepo.UpdateStatus(ctx, existingSub.ID, model.SubscriptionStatusActive); err != nil {
				return nil, false, fmt.Errorf("update subscription status: %w", err)
			}
		}

		// 追加备注
		if input.Notes != "" {
			newNotes := existingSub.Notes
			if newNotes != "" {
				newNotes += "\n"
			}
			newNotes += input.Notes
			if err := s.userSubRepo.UpdateNotes(ctx, existingSub.ID, newNotes); err != nil {
				log.Printf("update subscription notes failed: sub_id=%d err=%v", existingSub.ID, err)
			}
		}

		// 失效订阅缓存
		if s.billingCacheService != nil {
			userID, groupID := input.UserID, input.GroupID
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
			}()
		}

		// 返回更新后的订阅
		sub, err := s.userSubRepo.GetByID(ctx, existingSub.ID)
		return sub, true, err // true 表示是续期
	}

	// 没有订阅，创建新订阅
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
	}

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := input.UserID, input.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}

	return sub, false, nil // false 表示是新建
}

// createSubscription 创建新订阅（内部方法）
func (s *SubscriptionService) createSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, error) {
	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}

	now := time.Now()
	sub := &model.UserSubscription{
		UserID:     input.UserID,
		GroupID:    input.GroupID,
		StartsAt:   now,
		ExpiresAt:  now.AddDate(0, 0, validityDays),
		Status:     model.SubscriptionStatusActive,
		AssignedAt: now,
		Notes:      input.Notes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	// 只有当 AssignedBy > 0 时才设置（0 表示系统分配，如兑换码）
	if input.AssignedBy > 0 {
		sub.AssignedBy = &input.AssignedBy
	}

	if err := s.userSubRepo.Create(ctx, sub); err != nil {
		return nil, err
	}

	// 重新获取完整订阅信息（包含关联）
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

// BulkAssignSubscriptionInput 批量分配订阅输入
type BulkAssignSubscriptionInput struct {
	UserIDs      []int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

// BulkAssignResult 批量分配结果
type BulkAssignResult struct {
	SuccessCount  int
	FailedCount   int
	Subscriptions []model.UserSubscription
	Errors        []string
}

// BulkAssignSubscription 批量分配订阅
func (s *SubscriptionService) BulkAssignSubscription(ctx context.Context, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	result := &BulkAssignResult{
		Subscriptions: make([]model.UserSubscription, 0),
		Errors:        make([]string, 0),
	}

	for _, userID := range input.UserIDs {
		sub, err := s.AssignSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      input.GroupID,
			ValidityDays: input.ValidityDays,
			AssignedBy:   input.AssignedBy,
			Notes:        input.Notes,
		})
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
		} else {
			result.SuccessCount++
			result.Subscriptions = append(result.Subscriptions, *sub)
		}
	}

	return result, nil
}

// RevokeSubscription 撤销订阅
func (s *SubscriptionService) RevokeSubscription(ctx context.Context, subscriptionID int64) error {
	// 先获取订阅信息用于失效缓存
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}

	if err := s.userSubRepo.Delete(ctx, subscriptionID); err != nil {
		return err
	}

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := sub.UserID, sub.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}

	return nil
}

// ExtendSubscription 延长订阅
func (s *SubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*model.UserSubscription, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	// 计算新的过期时间
	newExpiresAt := sub.ExpiresAt.AddDate(0, 0, days)
	if err := s.userSubRepo.ExtendExpiry(ctx, subscriptionID, newExpiresAt); err != nil {
		return nil, err
	}

	// 如果订阅已过期，恢复为active状态
	if sub.Status == model.SubscriptionStatusExpired {
		if err := s.userSubRepo.UpdateStatus(ctx, subscriptionID, model.SubscriptionStatusActive); err != nil {
			return nil, err
		}
	}

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := sub.UserID, sub.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// GetByID 根据ID获取订阅
func (s *SubscriptionService) GetByID(ctx context.Context, id int64) (*model.UserSubscription, error) {
	return s.userSubRepo.GetByID(ctx, id)
}

// GetActiveSubscription 获取用户对特定分组的有效订阅
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return sub, nil
}

// ListUserSubscriptions 获取用户的所有订阅
func (s *SubscriptionService) ListUserSubscriptions(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	subs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, nil
}

// ListActiveUserSubscriptions 获取用户的所有有效订阅
func (s *SubscriptionService) ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, nil
}

// ListGroupSubscriptions 获取分组的所有订阅
func (s *SubscriptionService) ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, pag, nil
}

// List 获取所有订阅（分页，支持筛选）
func (s *SubscriptionService) List(ctx context.Context, page, pageSize int, userID, groupID *int64, status string) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.List(ctx, params, userID, groupID, status)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, pag, nil
}

// normalizeExpiredWindows 将已过期窗口的数据清零（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的当前窗口状态，而不是过期窗口的历史数据
func normalizeExpiredWindows(subs []model.UserSubscription) {
	for i := range subs {
		sub := &subs[i]
		// 日窗口过期：清零展示数据
		if sub.NeedsDailyReset() {
			sub.DailyWindowStart = nil
			sub.DailyUsageUSD = 0
		}
		// 周窗口过期：清零展示数据
		if sub.NeedsWeeklyReset() {
			sub.WeeklyWindowStart = nil
			sub.WeeklyUsageUSD = 0
		}
		// 月窗口过期：清零展示数据
		if sub.NeedsMonthlyReset() {
			sub.MonthlyWindowStart = nil
			sub.MonthlyUsageUSD = 0
		}
	}
}

// startOfDay 返回给定时间所在日期的零点（保持原时区）
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// CheckAndActivateWindow 检查并激活窗口（首次使用时）
func (s *SubscriptionService) CheckAndActivateWindow(ctx context.Context, sub *model.UserSubscription) error {
	if sub.IsWindowActivated() {
		return nil
	}

	// 使用当天零点作为窗口起始时间
	windowStart := startOfDay(time.Now())
	return s.userSubRepo.ActivateWindows(ctx, sub.ID, windowStart)
}

// CheckAndResetWindows 检查并重置过期的窗口
func (s *SubscriptionService) CheckAndResetWindows(ctx context.Context, sub *model.UserSubscription) error {
	// 使用当天零点作为新窗口起始时间
	windowStart := startOfDay(time.Now())
	needsInvalidateCache := false

	// 日窗口重置（24小时）
	if sub.NeedsDailyReset() {
		if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, windowStart); err != nil {
			return err
		}
		sub.DailyWindowStart = &windowStart
		sub.DailyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 周窗口重置（7天）
	if sub.NeedsWeeklyReset() {
		if err := s.userSubRepo.ResetWeeklyUsage(ctx, sub.ID, windowStart); err != nil {
			return err
		}
		sub.WeeklyWindowStart = &windowStart
		sub.WeeklyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 月窗口重置（30天）
	if sub.NeedsMonthlyReset() {
		if err := s.userSubRepo.ResetMonthlyUsage(ctx, sub.ID, windowStart); err != nil {
			return err
		}
		sub.MonthlyWindowStart = &windowStart
		sub.MonthlyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 如果有窗口被重置，失效 Redis 缓存以保持一致性
	if needsInvalidateCache && s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateSubscription(ctx, sub.UserID, sub.GroupID)
	}

	return nil
}

// CheckUsageLimits 检查使用限额（返回错误如果超限）
func (s *SubscriptionService) CheckUsageLimits(ctx context.Context, sub *model.UserSubscription, group *model.Group, additionalCost float64) error {
	if !sub.CheckDailyLimit(group, additionalCost) {
		return ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, additionalCost) {
		return ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, additionalCost) {
		return ErrMonthlyLimitExceeded
	}
	return nil
}

// RecordUsage 记录使用量到订阅
func (s *SubscriptionService) RecordUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	return s.userSubRepo.IncrementUsage(ctx, subscriptionID, costUSD)
}

// SubscriptionProgress 订阅进度
type SubscriptionProgress struct {
	ID            int64                `json:"id"`
	GroupName     string               `json:"group_name"`
	ExpiresAt     time.Time            `json:"expires_at"`
	ExpiresInDays int                  `json:"expires_in_days"`
	Daily         *UsageWindowProgress `json:"daily,omitempty"`
	Weekly        *UsageWindowProgress `json:"weekly,omitempty"`
	Monthly       *UsageWindowProgress `json:"monthly,omitempty"`
}

// UsageWindowProgress 使用窗口进度
type UsageWindowProgress struct {
	LimitUSD        float64   `json:"limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	RemainingUSD    float64   `json:"remaining_usd"`
	Percentage      float64   `json:"percentage"`
	WindowStart     time.Time `json:"window_start"`
	ResetsAt        time.Time `json:"resets_at"`
	ResetsInSeconds int64     `json:"resets_in_seconds"`
}

// GetSubscriptionProgress 获取订阅使用进度
func (s *SubscriptionService) GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*SubscriptionProgress, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	group := sub.Group
	if group == nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}

	progress := &SubscriptionProgress{
		ID:            sub.ID,
		GroupName:     group.Name,
		ExpiresAt:     sub.ExpiresAt,
		ExpiresInDays: sub.DaysRemaining(),
	}

	// 日进度
	if group.HasDailyLimit() && sub.DailyWindowStart != nil {
		limit := *group.DailyLimitUSD
		resetsAt := sub.DailyWindowStart.Add(24 * time.Hour)
		progress.Daily = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.DailyUsageUSD,
			RemainingUSD:    limit - sub.DailyUsageUSD,
			Percentage:      (sub.DailyUsageUSD / limit) * 100,
			WindowStart:     *sub.DailyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Daily.RemainingUSD < 0 {
			progress.Daily.RemainingUSD = 0
		}
		if progress.Daily.Percentage > 100 {
			progress.Daily.Percentage = 100
		}
		if progress.Daily.ResetsInSeconds < 0 {
			progress.Daily.ResetsInSeconds = 0
		}
	}

	// 周进度
	if group.HasWeeklyLimit() && sub.WeeklyWindowStart != nil {
		limit := *group.WeeklyLimitUSD
		resetsAt := sub.WeeklyWindowStart.Add(7 * 24 * time.Hour)
		progress.Weekly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.WeeklyUsageUSD,
			RemainingUSD:    limit - sub.WeeklyUsageUSD,
			Percentage:      (sub.WeeklyUsageUSD / limit) * 100,
			WindowStart:     *sub.WeeklyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Weekly.RemainingUSD < 0 {
			progress.Weekly.RemainingUSD = 0
		}
		if progress.Weekly.Percentage > 100 {
			progress.Weekly.Percentage = 100
		}
		if progress.Weekly.ResetsInSeconds < 0 {
			progress.Weekly.ResetsInSeconds = 0
		}
	}

	// 月进度
	if group.HasMonthlyLimit() && sub.MonthlyWindowStart != nil {
		limit := *group.MonthlyLimitUSD
		resetsAt := sub.MonthlyWindowStart.Add(30 * 24 * time.Hour)
		progress.Monthly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.MonthlyUsageUSD,
			RemainingUSD:    limit - sub.MonthlyUsageUSD,
			Percentage:      (sub.MonthlyUsageUSD / limit) * 100,
			WindowStart:     *sub.MonthlyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Monthly.RemainingUSD < 0 {
			progress.Monthly.RemainingUSD = 0
		}
		if progress.Monthly.Percentage > 100 {
			progress.Monthly.Percentage = 100
		}
		if progress.Monthly.ResetsInSeconds < 0 {
			progress.Monthly.ResetsInSeconds = 0
		}
	}

	return progress, nil
}

// GetUserSubscriptionsWithProgress 获取用户所有订阅及进度
func (s *SubscriptionService) GetUserSubscriptionsWithProgress(ctx context.Context, userID int64) ([]SubscriptionProgress, error) {
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	progresses := make([]SubscriptionProgress, 0, len(subs))
	for _, sub := range subs {
		progress, err := s.GetSubscriptionProgress(ctx, sub.ID)
		if err != nil {
			continue
		}
		progresses = append(progresses, *progress)
	}

	return progresses, nil
}

// UpdateExpiredSubscriptions 更新过期订阅状态（定时任务调用）
func (s *SubscriptionService) UpdateExpiredSubscriptions(ctx context.Context) (int64, error) {
	return s.userSubRepo.BatchUpdateExpiredStatus(ctx)
}

// ValidateSubscription 验证订阅是否有效
func (s *SubscriptionService) ValidateSubscription(ctx context.Context, sub *model.UserSubscription) error {
	if sub.Status == model.SubscriptionStatusExpired {
		return ErrSubscriptionExpired
	}
	if sub.Status == model.SubscriptionStatusSuspended {
		return ErrSubscriptionSuspended
	}
	if sub.IsExpired() {
		// 更新状态
		_ = s.userSubRepo.UpdateStatus(ctx, sub.ID, model.SubscriptionStatusExpired)
		return ErrSubscriptionExpired
	}
	return nil
}
