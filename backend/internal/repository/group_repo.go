package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) Create(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *GroupRepository) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).First(&group, id).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *GroupRepository) Update(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
}

func (r *GroupRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Group{}, id).Error
}

func (r *GroupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", nil)
}

// ListWithFilters lists groups with optional filtering by platform, status, and is_exclusive
func (r *GroupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]model.Group, *pagination.PaginationResult, error) {
	var groups []model.Group
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Group{})

	// Apply filters
	if platform != "" {
		db = db.Where("platform = ?", platform)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if isExclusive != nil {
		db = db.Where("is_exclusive = ?", *isExclusive)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, nil, err
	}

	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return groups, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

func (r *GroupRepository) ListActive(ctx context.Context) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).Where("status = ?", model.StatusActive).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
	}
	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
	}
	return groups, nil
}

func (r *GroupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).Where("status = ? AND platform = ?", model.StatusActive, platform).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
	}
	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
	}
	return groups, nil
}

func (r *GroupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Group{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *GroupRepository) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AccountGroup{}).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
}

// DeleteAccountGroupsByGroupID 删除分组与账号的关联关系
func (r *GroupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.AccountGroup{})
	return result.RowsAffected, result.Error
}

// DB 返回底层数据库连接，用于事务处理
func (r *GroupRepository) DB() *gorm.DB {
	return r.db
}
