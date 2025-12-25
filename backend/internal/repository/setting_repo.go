package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingRepository 系统设置数据访问层
type SettingRepository struct {
	db *gorm.DB
}

// NewSettingRepository 创建系统设置仓库实例
func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// Get 根据Key获取设置值
func (r *SettingRepository) Get(ctx context.Context, key string) (*model.Setting, error) {
	var setting model.Setting
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&setting).Error
	if err != nil {
		return nil, err
	}
	return &setting, nil
}

// GetValue 获取设置值字符串
func (r *SettingRepository) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// Set 设置值（存在则更新，不存在则创建）
func (r *SettingRepository) Set(ctx context.Context, key, value string) error {
	setting := &model.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(setting).Error
}

// GetMultiple 批量获取设置
func (r *SettingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	var settings []model.Setting
	err := r.db.WithContext(ctx).Where("key IN ?", keys).Find(&settings).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// SetMultiple 批量设置值
func (r *SettingRepository) SetMultiple(ctx context.Context, settings map[string]string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range settings {
			setting := &model.Setting{
				Key:       key,
				Value:     value,
				UpdatedAt: time.Now(),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
			}).Create(setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAll 获取所有设置
func (r *SettingRepository) GetAll(ctx context.Context) (map[string]string, error) {
	var settings []model.Setting
	err := r.db.WithContext(ctx).Find(&settings).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// Delete 删除设置
func (r *SettingRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Where("key = ?", key).Delete(&model.Setting{}).Error
}
