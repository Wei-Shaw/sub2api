package model

import (
	"time"
)

type AccountGroup struct {
	AccountID int64     `gorm:"primaryKey" json:"account_id"`
	GroupID   int64     `gorm:"primaryKey" json:"group_id"`
	Priority  int       `gorm:"default:50;not null" json:"priority"` // 分组内优先级
	CreatedAt time.Time `gorm:"not null" json:"created_at"`

	// 关联
	Account *Account `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	Group   *Group   `gorm:"foreignKey:GroupID" json:"group,omitempty"`
REDACTED

func (AccountGroup) TableName() string {
	return "account_groups"
REDACTED
