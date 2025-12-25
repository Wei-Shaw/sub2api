package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// WithTx 在事务内复用用户仓库。
func (r *UserRepository) WithTx(tx *gorm.DB) ports.UserRepository {
	return &UserRepository{db: tx}
}

func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.User{}, id).Error
}

func (r *UserRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
}

// ListWithFilters lists users with optional filtering by status, role, and search query
func (r *UserRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, role, search string) ([]model.User, *pagination.PaginationResult, error) {
	var users []model.User
	var total int64

	db := r.db.WithContext(ctx).Model(&model.User{})

	// Apply filters
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if role != "" {
		db = db.Where("role = ?", role)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where(
			"email ILIKE ? OR username ILIKE ? OR wechat ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Query users with pagination (reuse the same db with filters applied)
	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&users).Error; err != nil {
		return nil, nil, err
	}

	// Batch load subscriptions for all users (avoid N+1)
	if len(users) > 0 {
		userIDs := make([]int64, len(users))
		userMap := make(map[int64]*model.User, len(users))
		for i := range users {
			userIDs[i] = users[i].ID
			userMap[users[i].ID] = &users[i]
		}

		// Query active subscriptions with groups in one query
		var subscriptions []model.UserSubscription
		if err := r.db.WithContext(ctx).
			Preload("Group").
			Where("user_id IN ? AND status = ?", userIDs, model.SubscriptionStatusActive).
			Find(&subscriptions).Error; err != nil {
			return nil, nil, err
		}

		// Associate subscriptions with users
		for i := range subscriptions {
			if user, ok := userMap[subscriptions[i].UserID]; ok {
				user.Subscriptions = append(user.Subscriptions, subscriptions[i])
			}
		}
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return users, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

func (r *UserRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Update("balance", gorm.Expr("balance + ?", amount)).Error
}

// DeductBalance 扣减用户余额，仅当余额充足时执行
func (r *UserRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	result := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND balance >= ?", id, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound // 余额不足或用户不存在
	}
	return nil
}

func (r *UserRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Update("concurrency", gorm.Expr("concurrency + ?", amount)).Error
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

// RemoveGroupFromAllowedGroups 从所有用户的 allowed_groups 数组中移除指定的分组ID
// 使用 PostgreSQL 的 array_remove 函数
func (r *UserRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.User{}).
		Where("? = ANY(allowed_groups)", groupID).
		Update("allowed_groups", gorm.Expr("array_remove(allowed_groups, ?)", groupID))
	return result.RowsAffected, result.Error
}

// GetFirstAdmin 获取第一个管理员用户（用于 Admin API Key 认证）
func (r *UserRepository) GetFirstAdmin(ctx context.Context) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("role = ? AND status = ?", model.RoleAdmin, model.StatusActive).
		Order("id ASC").
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
