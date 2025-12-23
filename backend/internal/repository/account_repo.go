package repository

import (
	"context"
	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"time"

	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Create(account).Error
}

func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Preload("Proxy").Preload("AccountGroups").First(&account, id).Error
	if err != nil {
		return nil, err
	}
	// 填充 GroupIDs 虚拟字段
	account.GroupIDs = make([]int64, 0, len(account.AccountGroups))
	for _, ag := range account.AccountGroups {
		account.GroupIDs = append(account.GroupIDs, ag.GroupID)
	}
	return &account, nil
}

func (r *AccountRepository) Update(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Save(account).Error
}

func (r *AccountRepository) Delete(ctx context.Context, id int64) error {
	// 先删除账号与分组的绑定关系
	if err := r.db.WithContext(ctx).Where("account_id = ?", id).Delete(&model.AccountGroup{}).Error; err != nil {
		return err
	}
	// 再删除账号
	return r.db.WithContext(ctx).Delete(&model.Account{}, id).Error
}

func (r *AccountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "")
}

// ListWithFilters lists accounts with optional filtering by platform, type, status, and search query
func (r *AccountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]model.Account, *pagination.PaginationResult, error) {
	var accounts []model.Account
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Account{})

	// Apply filters
	if platform != "" {
		db = db.Where("platform = ?", platform)
	}
	if accountType != "" {
		db = db.Where("type = ?", accountType)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where("name ILIKE ?", searchPattern)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Preload("Proxy").Preload("AccountGroups").Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, nil, err
	}

	// 填充每个 Account 的 GroupIDs 虚拟字段
	for i := range accounts {
		accounts[i].GroupIDs = make([]int64, 0, len(accounts[i].AccountGroups))
		for _, ag := range accounts[i].AccountGroups {
			accounts[i].GroupIDs = append(accounts[i].GroupIDs, ag.GroupID)
		}
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return accounts, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

func (r *AccountRepository) ListByGroup(ctx context.Context, groupID int64) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ? AND accounts.status = ?", groupID, model.StatusActive).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
}

func (r *AccountRepository) ListActive(ctx context.Context) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusActive).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
}

func (r *AccountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).Update("last_used_at", now).Error
}

func (r *AccountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        model.StatusError,
			"error_message": errorMsg,
		}).Error
}

func (r *AccountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	ag := &model.AccountGroup{
		AccountID: accountID,
		GroupID:   groupID,
		Priority:  priority,
	}
	return r.db.WithContext(ctx).Create(ag).Error
}

func (r *AccountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	return r.db.WithContext(ctx).Where("account_id = ? AND group_id = ?", accountID, groupID).
		Delete(&model.AccountGroup{}).Error
}

func (r *AccountRepository) GetGroups(ctx context.Context, accountID int64) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.group_id = groups.id").
		Where("account_groups.account_id = ?", accountID).
		Find(&groups).Error
	return groups, err
}

func (r *AccountRepository) ListByPlatform(ctx context.Context, platform string) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Where("platform = ? AND status = ?", platform, model.StatusActive).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
}

func (r *AccountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	// 删除现有绑定
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Delete(&model.AccountGroup{}).Error; err != nil {
		return err
	}

	// 添加新绑定
	if len(groupIDs) > 0 {
		accountGroups := make([]model.AccountGroup, 0, len(groupIDs))
		for i, groupID := range groupIDs {
			accountGroups = append(accountGroups, model.AccountGroup{
				AccountID: accountID,
				GroupID:   groupID,
				Priority:  i + 1, // 使用索引作为优先级
			})
		}
		return r.db.WithContext(ctx).Create(&accountGroups).Error
	}

	return nil
}

// ListSchedulable 获取所有可调度的账号
func (r *AccountRepository) ListSchedulable(ctx context.Context) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("status = ? AND schedulable = ?", model.StatusActive, true).
		Where("(overload_until IS NULL OR overload_until <= ?)", now).
		Where("(rate_limit_reset_at IS NULL OR rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
}

// ListSchedulableByGroupID 按组获取可调度的账号
func (r *AccountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ?", groupID).
		Where("accounts.status = ? AND accounts.schedulable = ?", model.StatusActive, true).
		Where("(accounts.overload_until IS NULL OR accounts.overload_until <= ?)", now).
		Where("(accounts.rate_limit_reset_at IS NULL OR accounts.rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
}

// ListSchedulableByPlatform 按平台获取可调度的账号
func (r *AccountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("platform = ?", platform).
		Where("status = ? AND schedulable = ?", model.StatusActive, true).
		Where("(overload_until IS NULL OR overload_until <= ?)", now).
		Where("(rate_limit_reset_at IS NULL OR rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
}

// ListSchedulableByGroupIDAndPlatform 按组和平台获取可调度的账号
func (r *AccountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ?", groupID).
		Where("accounts.platform = ?", platform).
		Where("accounts.status = ? AND accounts.schedulable = ?", model.StatusActive, true).
		Where("(accounts.overload_until IS NULL OR accounts.overload_until <= ?)", now).
		Where("(accounts.rate_limit_reset_at IS NULL OR accounts.rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
}

// SetRateLimited 标记账号为限流状态(429)
func (r *AccountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).
		Updates(map[string]any{
			"rate_limited_at":     now,
			"rate_limit_reset_at": resetAt,
		}).Error
}

// SetOverloaded 标记账号为过载状态(529)
func (r *AccountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).
		Update("overload_until", until).Error
}

// ClearRateLimit 清除账号的限流状态
func (r *AccountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).
		Updates(map[string]any{
			"rate_limited_at":     nil,
			"rate_limit_reset_at": nil,
			"overload_until":      nil,
		}).Error
}

// UpdateSessionWindow 更新账号的5小时时间窗口信息
func (r *AccountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	updates := map[string]any{
		"session_window_status": status,
	}
	if start != nil {
		updates["session_window_start"] = start
	}
	if end != nil {
		updates["session_window_end"] = end
	}
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).Updates(updates).Error
}

// SetSchedulable 设置账号的调度开关
func (r *AccountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).
		Update("schedulable", schedulable).Error
}
