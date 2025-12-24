package repository

import (
	"context"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/service/ports"

	"gorm.io/gorm"
)

// UserSubscriptionRepository 用户订阅仓库
type UserSubscriptionRepository struct {
	db *gorm.DB
}

// NewUserSubscriptionRepository 创建用户订阅仓库
func NewUserSubscriptionRepository(db *gorm.DB) *UserSubscriptionRepository {
	return &UserSubscriptionRepository{db: db}
}

// WithTx 在事务内复用订阅仓库。
func (r *UserSubscriptionRepository) WithTx(tx *gorm.DB) ports.UserSubscriptionRepository {
	return &UserSubscriptionRepository{db: tx}
}

// Create 创建订阅
func (r *UserSubscriptionRepository) Create(ctx context.Context, sub *model.UserSubscription) error {
	return r.db.WithContext(ctx).Create(sub).Error
}

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
	}
	return &sub, nil
}

// GetByUserIDAndGroupID 根据用户ID和分组ID获取订阅
func (r *UserSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error) {
	var sub model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ? AND group_id = ?", userID, groupID).
		First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

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
	}
	return &sub, nil
}

// Update 更新订阅
func (r *UserSubscriptionRepository) Update(ctx context.Context, sub *model.UserSubscription) error {
	sub.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(sub).Error
}

// Delete 删除订阅
func (r *UserSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.UserSubscription{}, id).Error
}

// ListByUserID 获取用户的所有订阅
func (r *UserSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&subs).Error
	return subs, err
}

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
}

// ListByGroupID 获取分组的所有订阅（分页）
func (r *UserSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	var subs []model.UserSubscription
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UserSubscription{}).Where("group_id = ?", groupID)

	if err := query.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	err := query.
		Preload("User").
		Preload("Group").
		Order("created_at DESC").
		Offset(params.Offset()).
		Limit(params.Limit()).
		Find(&subs).Error
	if err != nil {
		return nil, nil, err
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return subs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

// List 获取所有订阅（分页，支持筛选）
func (r *UserSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status string) ([]model.UserSubscription, *pagination.PaginationResult, error) {
	var subs []model.UserSubscription
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UserSubscription{})

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, nil, err
	}

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
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return subs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

// IncrementUsage 增加使用量
func (r *UserSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_usage_usd":   gorm.Expr("daily_usage_usd + ?", costUSD),
			"weekly_usage_usd":  gorm.Expr("weekly_usage_usd + ?", costUSD),
			"monthly_usage_usd": gorm.Expr("monthly_usage_usd + ?", costUSD),
			"updated_at":        time.Now(),
		}).Error
}

// ResetDailyUsage 重置日使用量
func (r *UserSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_usage_usd":    0,
			"daily_window_start": newWindowStart,
			"updated_at":         time.Now(),
		}).Error
}

// ResetWeeklyUsage 重置周使用量
func (r *UserSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"weekly_usage_usd":    0,
			"weekly_window_start": newWindowStart,
			"updated_at":          time.Now(),
		}).Error
}

// ResetMonthlyUsage 重置月使用量
func (r *UserSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"monthly_usage_usd":    0,
			"monthly_window_start": newWindowStart,
			"updated_at":           time.Now(),
		}).Error
}

// ActivateWindows 激活所有窗口（首次使用时）
func (r *UserSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, activateTime time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_window_start":   activateTime,
			"weekly_window_start":  activateTime,
			"monthly_window_start": activateTime,
			"updated_at":           time.Now(),
		}).Error
}

// UpdateStatus 更新订阅状态
func (r *UserSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// ExtendExpiry 延长订阅过期时间
func (r *UserSubscriptionRepository) ExtendExpiry(ctx context.Context, id int64, newExpiresAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"expires_at": newExpiresAt,
			"updated_at": time.Now(),
		}).Error
}

// UpdateNotes 更新订阅备注
func (r *UserSubscriptionRepository) UpdateNotes(ctx context.Context, id int64, notes string) error {
	return r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"notes":      notes,
			"updated_at": time.Now(),
		}).Error
}

// ListExpired 获取所有已过期但状态仍为active的订阅
func (r *UserSubscriptionRepository) ListExpired(ctx context.Context) ([]model.UserSubscription, error) {
	var subs []model.UserSubscription
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Find(&subs).Error
	return subs, err
}

// BatchUpdateExpiredStatus 批量更新过期订阅状态
func (r *UserSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("status = ? AND expires_at <= ?", model.SubscriptionStatusActive, time.Now()).
		Updates(map[string]any{
			"status":     model.SubscriptionStatusExpired,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}

// ExistsByUserIDAndGroupID 检查用户是否已有该分组的订阅
func (r *UserSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Count(&count).Error
	return count > 0, err
}

// CountByGroupID 获取分组的订阅数量
func (r *UserSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("group_id = ?", groupID).
		Count(&count).Error
	return count, err
}

// CountActiveByGroupID 获取分组的有效订阅数量
func (r *UserSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSubscription{}).
		Where("group_id = ? AND status = ? AND expires_at > ?",
			groupID, model.SubscriptionStatusActive, time.Now()).
		Count(&count).Error
	return count, err
}

// DeleteByGroupID 删除分组相关的所有订阅记录
func (r *UserSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.UserSubscription{})
	return result.RowsAffected, result.Error
}
