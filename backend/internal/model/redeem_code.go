package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type RedeemCode struct {
	ID        int64      `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:32;not null" json:"code"`
	Type      string     `gorm:"size:20;default:balance;not null" json:"type"`  // balance/concurrency/subscription
	Value     float64    `gorm:"type:decimal(20,8);not null" json:"value"`      // 面值(USD)或并发数或有效天数
	Status    string     `gorm:"size:20;default:unused;not null" json:"status"` // unused/used
	UsedBy    *int64     `gorm:"index" json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	Notes     string     `gorm:"type:text" json:"notes"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`

	// 订阅类型专用字段
	GroupID      *int64 `gorm:"index" json:"group_id"`           // 订阅分组ID (仅subscription类型使用)
	ValidityDays int    `gorm:"default:30" json:"validity_days"` // 订阅有效天数 (仅subscription类型使用)

	// 关联
	User  *User  `gorm:"foreignKey:UsedBy" json:"user,omitempty"`
	Group *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (RedeemCode) TableName() string {
	return "redeem_codes"
}

// IsUsed 检查是否已使用
func (r *RedeemCode) IsUsed() bool {
	return r.Status == "used"
}

// CanUse 检查是否可以使用
func (r *RedeemCode) CanUse() bool {
	return r.Status == "unused"
}

// GenerateRedeemCode 生成唯一的兑换码
func GenerateRedeemCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
