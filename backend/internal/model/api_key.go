package model

import (
	"time"

	"gorm.io/gorm"
)

type ApiKey struct {
	ID        int64          `gorm:"primaryKey" json:"id"`
	UserID    int64          `gorm:"index;not null" json:"user_id"`
	Key       string         `gorm:"uniqueIndex;size:128;not null" json:"key"` // sk-xxx
	Name      string         `gorm:"size:100;not null" json:"name"`
	GroupID   *int64         `gorm:"index" json:"group_id"`
	Status    string         `gorm:"size:20;default:active;not null" json:"status"` // active/disabled
	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	User  *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Group *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
REDACTED

func (ApiKey) TableName() string {
	return "api_keys"
REDACTED

// IsActive 检查是否激活
func (k *ApiKey) IsActive() bool {
	return k.Status == "active"
REDACTED
