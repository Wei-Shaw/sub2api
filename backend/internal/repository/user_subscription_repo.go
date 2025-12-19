package repository

import (
	"context"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

// UserSubscriptionRepository 用户订阅仓库
type UserSubscriptionRepository struct {
	db *gorm.DB
REDACTED

// NewUserSubscriptionRepository 创建用户订阅仓库
func NewUserSubscriptionRepository(db *gorm.DB) *UserSubscriptionRepository {
	return &UserSubscriptionRepository{db: dbREDACTED
REDACTED

// Create 创建订阅
func (r *UserSubscriptionRepository) Create(ctx context.Context, sub *model.UserSubscription) error {
	return r.db.WithContext(ctx).Create(sub).Error
REDACTED

// GetByID 根据ID获取订阅
func (r *UserSubscriptionRepository) GetByID(ctx context.Context, id int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Group").
		Preload("AssignedByUser").
		First(&sub, id).Error
	if err != nil {
		return nil, err
REDACTED
	return &sub, nil
REDACTED

// GetByUserIDAndGroupID 根据用户ID和分组ID获取订阅
func (r *UserSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND group_id = ?", userID, groupID).
		First(&sub).Error
	if err != nil {
		return nil, err
REDACTED
	return &sub, nil
REDACTED

// GetActiveByUserIDAndGroupID 获取用户对特定分组的有效订阅
func (r *UserSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND group_id = ? AND status = ? AND expires_at > ?",
			userID, groupID, model.SubscriptionStatusActive, time.Now()).
		First(&sub).Error
	if err != nil {
		return nil, err
REDACTED
	return &sub, nil
REDACTED

// Update 更新订阅
func (r *UserSubscriptionRepository) Update(ctx context.Context, sub *model.UserSubscription) error {
	sub.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(sub).Error
REDACTED

// Delete 删除订阅
func (r *UserSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.UserSubscription{REDACTED, id).Error
REDACTED

// ListByUserID 获取用户的所有订阅
func (r *UserSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&subs).Error
	return subs, err
REDACTED

// ListActiveByUserID 获取用户的所有有效订阅
func (r *UserSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
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
func (r *UserSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.UserSubscription, *pagination.PaginationResult, error) {
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
func (r *UserSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status string) ([]model.UserSubscription, *pagination.PaginationResult, error) {
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
func (r *UserSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"daily_usage_usd":   gorm.Expr("daily_usage_usd + ?", costUSD),
			"weekly_usage_usd":  gorm.Expr("weekly_usage_usd + ?", costUSD),
			"monthly_usage_usd": gorm.Expr("monthly_usage_usd + ?", costUSD),
			"updated_at":        time.Now(),
	REDACTED).Error
REDACTED

// ResetDailyUsage 重置日使用量
func (r *UserSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"daily_usage_usd":    0,
			"daily_window_start": newWindowStart,
			"updated_at":         time.Now(),
	REDACTED).Error
REDACTED

// ResetWeeklyUsage 重置周使用量
func (r *UserSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"weekly_usage_usd":    0,
			"weekly_window_start": newWindowStart,
			"updated_at":          time.Now(),
	REDACTED).Error
REDACTED

// ResetMonthlyUsage 重置月使用量
func (r *UserSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"monthly_usage_usd":    0,
			"monthly_window_start": newWindowStart,
			"updated_at":           time.Now(),
	REDACTED).Error
REDACTED

// ActivateWindows 激活所有窗口（首次使用时）
func (r *UserSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, activateTime time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"daily_window_start":   activateTime,
			"weekly_window_start":  activateTime,
			"monthly_window_start": activateTime,
			"updated_at":           time.Now(),
	REDACTED).Error
REDACTED

// UpdateStatus 更新订阅状态
func (r *UserSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"status":     status,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// ExtendExpiry 延长订阅过期时间
func (r *UserSubscriptionRepository) ExtendExpiry(ctx context.Context, id int64, newExpiresAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"expires_at": newExpiresAt,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// UpdateNotes 更新订阅备注
func (r *UserSubscriptionRepository) UpdateNotes(ctx context.Context, id int64, notes string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("id = ?", id).
		Updates(map[string]interface{REDACTED{
			"notes":      notes,
			"updated_at": time.Now(),
	REDACTED).Error
REDACTED

// ListExpired 获取所有已过期但状态仍为active的订阅
func (r *UserSubscriptionRepository) ListExpired(ctx context.Context) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Find(&subs).Error
	return subs, err
REDACTED

// BatchUpdateExpiredStatus 批量更新过期订阅状态
func (r *UserSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Updates(map[string]interface{REDACTED{
			"status":     model.SubscriptionStatusExpired,
			"updated_at": time.Now(),
	REDACTED)
	return result.RowsAffected, result.Error
REDACTED

// ExistsByUserIDAndGroupID 检查用户是否已有该分组的订阅
func (r *UserSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Count(&count).Error
	return count > 0, err
REDACTED

// CountByGroupID 获取分组的订阅数量
func (r *UserSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("group_id = ?", groupID).
		Count(&count).Error
	return count, err
REDACTED

// CountActiveByGroupID 获取分组的有效订阅数量
func (r *UserSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{REDACTED).
		Where("group_id = ? AND status = ? AND expires_at > ?",
			groupID, model.SubscriptionStatusActive, time.Now()).
		Count(&count).Error
	return count, err
REDACTED

// DeleteByGroupID 删除分组相关的所有订阅记录
func (r *UserSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.UserSubscription{REDACTED)
	return result.RowsAffected, result.Error
REDACTED
