package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type ApiKeyRepository struct {
	db *gorm.DB
}

func NewApiKeyRepository(db *gorm.DB) *ApiKeyRepository {
	return &ApiKeyRepository{db: db}
}

func (r *ApiKeyRepository) Create(ctx context.Context, key *model.ApiKey) error {
	return r.db.WithContext(ctx).Create(key).Error
}

func (r *ApiKeyRepository) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	var key model.ApiKey
	err := r.db.WithContext(ctx).Preload("User").Preload("Group").First(&key, id).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *ApiKeyRepository) GetByKey(ctx context.Context, key string) (*model.ApiKey, error) {
	var apiKey model.ApiKey
	err := r.db.WithContext(ctx).Preload("User").Preload("Group").Where("key = ?", key).First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (r *ApiKeyRepository) GetByHash(ctx context.Context, keyHash string) (*model.ApiKey, error) {
	var apiKey model.ApiKey
	err := r.db.WithContext(ctx).Preload("User").Preload("Group").Where("key_hash = ?", keyHash).First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (r *ApiKeyRepository) Update(ctx context.Context, key *model.ApiKey) error {
	return r.db.WithContext(ctx).Model(key).Select("name", "group_id", "status", "updated_at").Updates(key).Error
}

// UpdateKeyMaterial 用于将旧版明文 key 迁移为哈希存储。
func (r *ApiKeyRepository) UpdateKeyMaterial(ctx context.Context, id int64, keyHash *string, keyLast4 string, legacyKey *string) error {
	return r.db.WithContext(ctx).Model(&model.ApiKey{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"key_hash":  keyHash,
			"key_last4": keyLast4,
			"key":       legacyKey,
			"updated_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *ApiKeyRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.ApiKey{}, id).Error
}

func (r *ApiKeyRepository) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]model.ApiKey, *pagination.PaginationResult, error) {
	var keys []model.ApiKey
	var total int64

	db := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("user_id = ?", userID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Preload("Group").Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&keys).Error; err != nil {
		return nil, nil, err
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return keys, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

func (r *ApiKeyRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *ApiKeyRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("key = ?", key).Count(&count).Error
	return count > 0, err
}

func (r *ApiKeyRepository) ExistsByHash(ctx context.Context, keyHash string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("key_hash = ?", keyHash).Count(&count).Error
	return count > 0, err
}

func (r *ApiKeyRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.ApiKey, *pagination.PaginationResult, error) {
	var keys []model.ApiKey
	var total int64

	db := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("group_id = ?", groupID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Preload("User").Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&keys).Error; err != nil {
		return nil, nil, err
	}

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}

	return keys, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

// SearchApiKeys searches API keys by user ID and/or keyword (name)
func (r *ApiKeyRepository) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]model.ApiKey, error) {
	var keys []model.ApiKey

	db := r.db.WithContext(ctx).Model(&model.ApiKey{})

	if userID > 0 {
		db = db.Where("user_id = ?", userID)
	}

	if keyword != "" {
		searchPattern := "%" + keyword + "%"
		db = db.Where("name ILIKE ?", searchPattern)
	}

	if err := db.Limit(limit).Order("id DESC").Find(&keys).Error; err != nil {
		return nil, err
	}

	return keys, nil
}

// ClearGroupIDByGroupID 将指定分组的所有 API Key 的 group_id 设为 nil
func (r *ApiKeyRepository) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.ApiKey{}).
		Where("group_id = ?", groupID).
		Update("group_id", nil)
	return result.RowsAffected, result.Error
}

// CountByGroupID 获取分组的 API Key 数量
func (r *ApiKeyRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
}
