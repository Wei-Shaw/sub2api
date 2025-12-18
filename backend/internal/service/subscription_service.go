package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/repository"
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
	repos               *repository.Repositories
	billingCacheService *BillingCacheService
REDACTED

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(repos *repository.Repositories) *SubscriptionService {
	return &SubscriptionService{repos: reposREDACTED
REDACTED

// SetBillingCacheService 设置计费缓存服务（用于缓存失效）
func (s *SubscriptionService) SetBillingCacheService(billingCacheService *BillingCacheService) {
	s.billingCacheService = billingCacheService
REDACTED

// AssignSubscriptionInput 分配订阅输入
type AssignSubscriptionInput struct {
	UserID       int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
REDACTED

// AssignSubscription 分配订阅给用户（不允许重复分配）
func (s *SubscriptionService) AssignSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.repos.Group.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
REDACTED
	if !group.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
REDACTED

	// 检查是否已存在订阅
	exists, err := s.repos.UserSubscription.ExistsByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		return nil, err
REDACTED
	if exists {
		return nil, ErrSubscriptionAlreadyExists
REDACTED

	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, err
REDACTED

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := input.UserID, input.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
	REDACTED()
REDACTED

	return sub, nil
REDACTED

// AssignOrExtendSubscription 分配或续期订阅（用于兑换码等场景）
// 如果用户已有同分组的订阅：
//   - 未过期：从当前过期时间累加天数
//   - 已过期：从当前时间开始计算新的过期时间，并激活订阅
// 如果没有订阅：创建新订阅
func (s *SubscriptionService) AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, bool, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.repos.Group.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, false, fmt.Errorf("group not found: %w", err)
REDACTED
	if !group.IsSubscriptionType() {
		return nil, false, ErrGroupNotSubscriptionType
REDACTED

	// 查询是否已有订阅
	existingSub, err := s.repos.UserSubscription.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		// 不存在记录是正常情况，其他错误需要返回
		existingSub = nil
REDACTED

	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
REDACTED

	// 已有订阅，执行续期
	if existingSub != nil {
		now := time.Now()
		var newExpiresAt time.Time

		if existingSub.ExpiresAt.After(now) {
			// 未过期：从当前过期时间累加
			newExpiresAt = existingSub.ExpiresAt.AddDate(0, 0, validityDays)
	REDACTED else {
			// 已过期：从当前时间开始计算
			newExpiresAt = now.AddDate(0, 0, validityDays)
	REDACTED

		// 更新过期时间
		if err := s.repos.UserSubscription.ExtendExpiry(ctx, existingSub.ID, newExpiresAt); err != nil {
			return nil, false, fmt.Errorf("extend subscription: %w", err)
	REDACTED

		// 如果订阅已过期或被暂停，恢复为active状态
		if existingSub.Status != model.SubscriptionStatusActive {
			if err := s.repos.UserSubscription.UpdateStatus(ctx, existingSub.ID, model.SubscriptionStatusActive); err != nil {
				return nil, false, fmt.Errorf("update subscription status: %w", err)
		REDACTED
	REDACTED

		// 追加备注
		if input.Notes != "" {
			newNotes := existingSub.Notes
			if newNotes != "" {
				newNotes += "\n"
		REDACTED
			newNotes += input.Notes
			if err := s.repos.UserSubscription.UpdateNotes(ctx, existingSub.ID, newNotes); err != nil {
				// 备注更新失败不影响主流程
		REDACTED
	REDACTED

		// 失效订阅缓存
		if s.billingCacheService != nil {
			userID, groupID := input.UserID, input.GroupID
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		REDACTED()
	REDACTED

		// 返回更新后的订阅
		sub, err := s.repos.UserSubscription.GetByID(ctx, existingSub.ID)
		return sub, true, err // true 表示是续期
REDACTED

	// 没有订阅，创建新订阅
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
REDACTED

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := input.UserID, input.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
	REDACTED()
REDACTED

	return sub, false, nil // false 表示是新建
REDACTED

// createSubscription 创建新订阅（内部方法）
func (s *SubscriptionService) createSubscription(ctx context.Context, input *AssignSubscriptionInput) (*model.UserSubscription, error) {
	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
REDACTED

	now := time.Now()
	sub := &model.UserSubscription{
		UserID:    input.UserID,
		GroupID:   input.GroupID,
		StartsAt:  now,
		ExpiresAt: now.AddDate(0, 0, validityDays),
		Status:    model.SubscriptionStatusActive,
		AssignedAt: now,
		Notes:     input.Notes,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	// 只有当 AssignedBy > 0 时才设置（0 表示系统分配，如兑换码）
	if input.AssignedBy > 0 {
		sub.AssignedBy = &input.AssignedBy
REDACTED

	if err := s.repos.UserSubscription.Create(ctx, sub); err != nil {
		return nil, err
REDACTED

	// 重新获取完整订阅信息（包含关联）
	return s.repos.UserSubscription.GetByID(ctx, sub.ID)
REDACTED

// BulkAssignSubscriptionInput 批量分配订阅输入
type BulkAssignSubscriptionInput struct {
	UserIDs      []int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
REDACTED

// BulkAssignResult 批量分配结果
type BulkAssignResult struct {
	SuccessCount int
	FailedCount  int
	Subscriptions []model.UserSubscription
	Errors       []string
REDACTED

// BulkAssignSubscription 批量分配订阅
func (s *SubscriptionService) BulkAssignSubscription(ctx context.Context, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	result := &BulkAssignResult{
		Subscriptions: make([]model.UserSubscription, 0),
		Errors:       make([]string, 0),
REDACTED

	for _, userID := range input.UserIDs {
		sub, err := s.AssignSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      input.GroupID,
			ValidityDays: input.ValidityDays,
			AssignedBy:   input.AssignedBy,
			Notes:        input.Notes,
	REDACTED)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
	REDACTED else {
			result.SuccessCount++
			result.Subscriptions = append(result.Subscriptions, *sub)
	REDACTED
REDACTED

	return result, nil
REDACTED

// RevokeSubscription 撤销订阅
func (s *SubscriptionService) RevokeSubscription(ctx context.Context, subscriptionID int64) error {
	// 先获取订阅信息用于失效缓存
	sub, err := s.repos.UserSubscription.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
REDACTED

	if err := s.repos.UserSubscription.Delete(ctx, subscriptionID); err != nil {
		return err
REDACTED

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := sub.UserID, sub.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
	REDACTED()
REDACTED

	return nil
REDACTED

// ExtendSubscription 延长订阅
func (s *SubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*model.UserSubscription, error) {
	sub, err := s.repos.UserSubscription.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
REDACTED

	// 计算新的过期时间
	newExpiresAt := sub.ExpiresAt.AddDate(0, 0, days)
	if err := s.repos.UserSubscription.ExtendExpiry(ctx, subscriptionID, newExpiresAt); err != nil {
		return nil, err
REDACTED

	// 如果订阅已过期，恢复为active状态
	if sub.Status == model.SubscriptionStatusExpired {
		if err := s.repos.UserSubscription.UpdateStatus(ctx, subscriptionID, model.SubscriptionStatusActive); err != nil {
			return nil, err
	REDACTED
REDACTED

	// 失效订阅缓存
	if s.billingCacheService != nil {
		userID, groupID := sub.UserID, sub.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
	REDACTED()
REDACTED

	return s.repos.UserSubscription.GetByID(ctx, subscriptionID)
REDACTED

// GetByID 根据ID获取订阅
func (s *SubscriptionService) GetByID(ctx context.Context, id int64) (*model.UserSubscription, error) {
	return s.repos.UserSubscription.GetByID(ctx, id)
REDACTED

// GetActiveSubscription 获取用户对特定分组的有效订阅
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	sub, err := s.repos.UserSubscription.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
REDACTED
	return sub, nil
REDACTED

// ListUserSubscriptions 获取用户的所有订阅
func (s *SubscriptionService) ListUserSubscriptions(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	return s.repos.UserSubscription.ListByUserID(ctx, userID)
REDACTED

// ListActiveUserSubscriptions 获取用户的所有有效订阅
func (s *SubscriptionService) ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	return s.repos.UserSubscription.ListActiveByUserID(ctx, userID)
REDACTED

// ListGroupSubscriptions 获取分组的所有订阅
func (s *SubscriptionService) ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]model.UserSubscription, *repository.PaginationResult, error) {
	params := repository.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	return s.repos.UserSubscription.ListByGroupID(ctx, groupID, params)
REDACTED

// List 获取所有订阅（分页，支持筛选）
func (s *SubscriptionService) List(ctx context.Context, page, pageSize int, userID, groupID *int64, status string) ([]model.UserSubscription, *repository.PaginationResult, error) {
	params := repository.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	return s.repos.UserSubscription.List(ctx, params, userID, groupID, status)
REDACTED

// CheckAndActivateWindow 检查并激活窗口（首次使用时）
func (s *SubscriptionService) CheckAndActivateWindow(ctx context.Context, sub *model.UserSubscription) error {
	if sub.IsWindowActivated() {
		return nil
REDACTED

	now := time.Now()
	return s.repos.UserSubscription.ActivateWindows(ctx, sub.ID, now)
REDACTED

// CheckAndResetWindows 检查并重置过期的窗口
func (s *SubscriptionService) CheckAndResetWindows(ctx context.Context, sub *model.UserSubscription) error {
	now := time.Now()

	// 日窗口重置（24小时）
	if sub.NeedsDailyReset() {
		if err := s.repos.UserSubscription.ResetDailyUsage(ctx, sub.ID, now); err != nil {
			return err
	REDACTED
		sub.DailyWindowStart = &now
		sub.DailyUsageUSD = 0
REDACTED

	// 周窗口重置（7天）
	if sub.NeedsWeeklyReset() {
		if err := s.repos.UserSubscription.ResetWeeklyUsage(ctx, sub.ID, now); err != nil {
			return err
	REDACTED
		sub.WeeklyWindowStart = &now
		sub.WeeklyUsageUSD = 0
REDACTED

	// 月窗口重置（30天）
	if sub.NeedsMonthlyReset() {
		if err := s.repos.UserSubscription.ResetMonthlyUsage(ctx, sub.ID, now); err != nil {
			return err
	REDACTED
		sub.MonthlyWindowStart = &now
		sub.MonthlyUsageUSD = 0
REDACTED

	return nil
REDACTED

// CheckUsageLimits 检查使用限额（返回错误如果超限）
func (s *SubscriptionService) CheckUsageLimits(ctx context.Context, sub *model.UserSubscription, group *model.Group, additionalCost float64) error {
	if !sub.CheckDailyLimit(group, additionalCost) {
		return ErrDailyLimitExceeded
REDACTED
	if !sub.CheckWeeklyLimit(group, additionalCost) {
		return ErrWeeklyLimitExceeded
REDACTED
	if !sub.CheckMonthlyLimit(group, additionalCost) {
		return ErrMonthlyLimitExceeded
REDACTED
	return nil
REDACTED

// RecordUsage 记录使用量到订阅
func (s *SubscriptionService) RecordUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	return s.repos.UserSubscription.IncrementUsage(ctx, subscriptionID, costUSD)
REDACTED

// SubscriptionProgress 订阅进度
type SubscriptionProgress struct {
	ID            int64              `json:"id"`
	GroupName     string             `json:"group_name"`
	ExpiresAt     time.Time          `json:"expires_at"`
	ExpiresInDays int                `json:"expires_in_days"`
	Daily         *UsageWindowProgress `json:"daily,omitempty"`
	Weekly        *UsageWindowProgress `json:"weekly,omitempty"`
	Monthly       *UsageWindowProgress `json:"monthly,omitempty"`
REDACTED

// UsageWindowProgress 使用窗口进度
type UsageWindowProgress struct {
	LimitUSD       float64   `json:"limit_usd"`
	UsedUSD        float64   `json:"used_usd"`
	RemainingUSD   float64   `json:"remaining_usd"`
	Percentage     float64   `json:"percentage"`
	WindowStart    time.Time `json:"window_start"`
	ResetsAt       time.Time `json:"resets_at"`
	ResetsInSeconds int64    `json:"resets_in_seconds"`
REDACTED

// GetSubscriptionProgress 获取订阅使用进度
func (s *SubscriptionService) GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*SubscriptionProgress, error) {
	sub, err := s.repos.UserSubscription.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
REDACTED

	group := sub.Group
	if group == nil {
		group, err = s.repos.Group.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	progress := &SubscriptionProgress{
		ID:            sub.ID,
		GroupName:     group.Name,
		ExpiresAt:     sub.ExpiresAt,
		ExpiresInDays: sub.DaysRemaining(),
REDACTED

	// 日进度
	if group.HasDailyLimit() && sub.DailyWindowStart != nil {
		limit := *group.DailyLimitUSD
		resetsAt := sub.DailyWindowStart.Add(24 * time.Hour)
		progress.Daily = &UsageWindowProgress{
			LimitUSD:       limit,
			UsedUSD:        sub.DailyUsageUSD,
			RemainingUSD:   limit - sub.DailyUsageUSD,
			Percentage:     (sub.DailyUsageUSD / limit) * 100,
			WindowStart:    *sub.DailyWindowStart,
			ResetsAt:       resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
	REDACTED
		if progress.Daily.RemainingUSD < 0 {
			progress.Daily.RemainingUSD = 0
	REDACTED
		if progress.Daily.Percentage > 100 {
			progress.Daily.Percentage = 100
	REDACTED
		if progress.Daily.ResetsInSeconds < 0 {
			progress.Daily.ResetsInSeconds = 0
	REDACTED
REDACTED

	// 周进度
	if group.HasWeeklyLimit() && sub.WeeklyWindowStart != nil {
		limit := *group.WeeklyLimitUSD
		resetsAt := sub.WeeklyWindowStart.Add(7 * 24 * time.Hour)
		progress.Weekly = &UsageWindowProgress{
			LimitUSD:       limit,
			UsedUSD:        sub.WeeklyUsageUSD,
			RemainingUSD:   limit - sub.WeeklyUsageUSD,
			Percentage:     (sub.WeeklyUsageUSD / limit) * 100,
			WindowStart:    *sub.WeeklyWindowStart,
			ResetsAt:       resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
	REDACTED
		if progress.Weekly.RemainingUSD < 0 {
			progress.Weekly.RemainingUSD = 0
	REDACTED
		if progress.Weekly.Percentage > 100 {
			progress.Weekly.Percentage = 100
	REDACTED
		if progress.Weekly.ResetsInSeconds < 0 {
			progress.Weekly.ResetsInSeconds = 0
	REDACTED
REDACTED

	// 月进度
	if group.HasMonthlyLimit() && sub.MonthlyWindowStart != nil {
		limit := *group.MonthlyLimitUSD
		resetsAt := sub.MonthlyWindowStart.Add(30 * 24 * time.Hour)
		progress.Monthly = &UsageWindowProgress{
			LimitUSD:       limit,
			UsedUSD:        sub.MonthlyUsageUSD,
			RemainingUSD:   limit - sub.MonthlyUsageUSD,
			Percentage:     (sub.MonthlyUsageUSD / limit) * 100,
			WindowStart:    *sub.MonthlyWindowStart,
			ResetsAt:       resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
	REDACTED
		if progress.Monthly.RemainingUSD < 0 {
			progress.Monthly.RemainingUSD = 0
	REDACTED
		if progress.Monthly.Percentage > 100 {
			progress.Monthly.Percentage = 100
	REDACTED
		if progress.Monthly.ResetsInSeconds < 0 {
			progress.Monthly.ResetsInSeconds = 0
	REDACTED
REDACTED

	return progress, nil
REDACTED

// GetUserSubscriptionsWithProgress 获取用户所有订阅及进度
func (s *SubscriptionService) GetUserSubscriptionsWithProgress(ctx context.Context, userID int64) ([]SubscriptionProgress, error) {
	subs, err := s.repos.UserSubscription.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
REDACTED

	progresses := make([]SubscriptionProgress, 0, len(subs))
	for _, sub := range subs {
		progress, err := s.GetSubscriptionProgress(ctx, sub.ID)
		if err != nil {
			continue
	REDACTED
		progresses = append(progresses, *progress)
REDACTED

	return progresses, nil
REDACTED

// UpdateExpiredSubscriptions 更新过期订阅状态（定时任务调用）
func (s *SubscriptionService) UpdateExpiredSubscriptions(ctx context.Context) (int64, error) {
	return s.repos.UserSubscription.BatchUpdateExpiredStatus(ctx)
REDACTED

// ValidateSubscription 验证订阅是否有效
func (s *SubscriptionService) ValidateSubscription(ctx context.Context, sub *model.UserSubscription) error {
	if sub.Status == model.SubscriptionStatusExpired {
		return ErrSubscriptionExpired
REDACTED
	if sub.Status == model.SubscriptionStatusSuspended {
		return ErrSubscriptionSuspended
REDACTED
	if sub.IsExpired() {
		// 更新状态
		_ = s.repos.UserSubscription.UpdateStatus(ctx, sub.ID, model.SubscriptionStatusExpired)
		return ErrSubscriptionExpired
REDACTED
	return nil
REDACTED
