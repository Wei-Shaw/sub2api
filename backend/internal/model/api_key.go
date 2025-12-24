package model

import (
	"time"

	"gorm.io/gorm"
)

type ApiKey struct {
	ID        int64          `gorm:"primaryKey" json:"id"`
	UserID    int64          `gorm:"index;not null" json:"user_id"`
	// 兼容旧版明文 key 字段（迁移完成后将保持为空）
	Key       *string        `gorm:"uniqueIndex;size:128" json:"-"`
	// HMAC 哈希后的 key，用于认证与去重
	KeyHash   *string        `gorm:"uniqueIndex;size:64" json:"-"`
	// 仅保存末 4 位用于脱敏展示
	KeyLast4  string         `gorm:"size:4" json:"-"`
	// API 响应返回的脱敏字段
	MaskedKey string         `gorm:"-" json:"masked_key,omitempty"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	GroupID   *int64         `gorm:"index" json:"group_id"`
	Status    string         `gorm:"size:20;default:active;not null" json:"status"` // active/disabled
	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	User  *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Group *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (ApiKey) TableName() string {
	return "api_keys"
}

// IsActive 检查是否激活
func (k *ApiKey) IsActive() bool {
	return k.Status == "active"
}
