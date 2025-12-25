package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"gorm.io/gorm"
)

// UserSubscriptionRepository 用户订阅仓库
type userSubscriptionRepository struct {
	db *gorm.DB
REDACTED

// NewUserSubscriptionRepository 创建用户订阅仓库
func NewUserSubscriptionRepository(db *gorm.DB) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{db: dbREDACTED
REDACTED

// Create 创建订阅
func (r *userSubscriptionRepository) Create(ctx context.Context, sub *model.UserSubscription) error {
	err := r.db.WithContext(ctx).Create(sub).Error
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
REDACTED

// GetByID 根据ID获取订阅
func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Group").
		Preload("AssignedByUser").
		First(&sub, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return &sub, nil
REDACTED

// GetByUserIDAndGroupID 根据用户ID和分组ID获取订阅
func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND group_id = ?", userID, groupID).
		First(&sub).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return &sub, nil
REDACTED

// GetActiveByUserIDAndGroupID 获取用户对特定分组的有效订阅
func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND group_id = ? AND status = ? AND expires_at > ?",
			userID, groupID, model.SubscriptionStatusActive, time.Now()).
		First(&sub).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return &sub, nil
REDACTED

// Update 更新订阅
func (r *userSubscriptionRepository) Update(ctx context.Context, sub *model.UserSubscription) error {
	sub.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(sub).Error
REDACTED

// Delete 删除订阅
func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.UserSubscription{REDACTED, id).Error
REDACTED

// ListByUserID 获取用户的所有订阅
func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&subs).Error
	return subs, err
REDACTED

// ListActiveByUserID 获取用户的所有有效订阅
func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND status = ? AND expires_at > ?",
			userID, model.SubscriptionStatusActive, time.Now()).
		Order("created_at DESC").
		Find(&subs).Error
	return subs, err
REDACTED

// ListByGroupID 获取分组的所有订阅（分页）
func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	var subs []model.UserSubscription
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).Where("group_id = ?", groupID)

	if err := query.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	err := query.
		Preload("User").
		Preload("Group").
		Order("created_at DESC").
		Offset(params.Offset()).
		Limit(params.Limit()).
		Find(&subs).Error
	if err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return subs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

// List 获取所有订阅（分页，支持筛选）
func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status string) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	var subs []model.UserSubscription
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED)

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
REDACTED
	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
REDACTED
	if status != "" {
		query = query.Where("status = ?", status)
REDACTED

	if err := query.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	err := query.
		Preload("User").
		Preload("Group").
		Preload("AssignedByUser").
		Order("created_at DESC").
		Offset(params.Offset()).
		Limit(params.Limit()).
		Find(&subs).Error
	if err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return subs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

// IncrementUsage 增加使用量
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_usage_usd":   gorm.Expr("daily_usage_usd + ?", costUSD),
			"weekly_usage_usd":  gorm.Expr("weekly_usage_usd + ?", costUSD),
			"monthly_usage_usd": gorm.Expr("monthly_usage_usd + ?", costUSD),
			"updated_at":        time.Now(),
	REDACTED).Error
REDACTED

// ResetDailyUsage 重置日使用量
func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_usage_usd":    0,
			"daily_window_start": newWindowStart,
			"updated_at":         time.Now(),
	REDACTED).Error
REDACTED

// ResetWeeklyUsage 重置周使用量
func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"weekly_usage_usd":    0,
			"weekly_window_start": newWindowStart,
			"updated_at":          time.Now(),
	REDACTED).Error
REDACTED

// ResetMonthlyUsage 重置月使用量
func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"monthly_usage_usd":    0,
			"monthly_window_start": newWindowStart,
			"updated_at":           time.Now(),
	REDACTED).Error
REDACTED

// ActivateWindows 激活所有窗口（首次使用时）
func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, activateTime time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_window_start":   activateTime,
			"weekly_window_start":  activateTime,
			"monthly_window_start": activateTime,
			"updated_at":           time.Now(),
	REDACTED).Error
REDACTED

// UpdateStatus 更新订阅状态
func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// ExtendExpiry 延长订阅过期时间
func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, id int64, newExpiresAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"expires_at": newExpiresAt,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// UpdateNotes 更新订阅备注
func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, id int64, notes string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]any{
			"notes":      notes,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// ListExpired 获取所有已过期但状态仍为active的订阅
func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Find(&subs).Error
	return subs, err
REDACTED

// BatchUpdateExpiredStatus 批量更新过期订阅状态
func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Updates(map[string]any{
			"status":     model.SubscriptionStatusExpired,
			"updated_at": time.Now(),
	REDACTED)
	return result.RowsAffected, result.Error
REDACTED

// ExistsByUserIDAndGroupID 检查用户是否已有该分组的订阅
func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Count(&count).Error
	return count > 0, err
REDACTED

// CountByGroupID 获取分组的订阅数量
func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("group_id = ?", groupID).
		Count(&count).Error
	return count, err
REDACTED

// CountActiveByGroupID 获取分组的有效订阅数量
func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("group_id = ? AND status = ? AND expires_at > ?",
			groupID, model.SubscriptionStatusActive, time.Now()).
		Count(&count).Error
	return count, err
REDACTED

// DeleteByGroupID 删除分组相关的所有订阅记录
func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.UserSubscription{REDACTED)
	return result.RowsAffected, result.Error
REDACTED
