package model

import (
	"time"

	"gorm.io/gorm"
)

// 订阅类型常量
const (
	SubscriptionTypeStandard     = "standard"     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = "subscription" // 订阅模式（按限额控制）
)

type Group struct {
	ID             int64   `gorm:"primaryKey" json:"id"`
	Name           string  `gorm:"uniqueIndex;size:100;not null" json:"name"`
	Description    string  `gorm:"type:text" json:"description"`
	Platform       string  `gorm:"size:50;default:anthropic;not null" json:"platform"` // anthropic/openai/gemini
	RateMultiplier float64 `gorm:"type:decimal(10,4);default:1.0;not null" json:"rate_multiplier"`
	IsExclusive    bool    `gorm:"default:false;not null" json:"is_exclusive"`
	Status         string  `gorm:"size:20;default:active;not null" json:"status"` // active/disabled

	// 订阅功能字段
	SubscriptionType string   `gorm:"size:20;default:standard;not null" json:"subscription_type"` // standard/subscription
	DailyLimitUSD    *float64 `gorm:"type:decimal(20,8)" json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `gorm:"type:decimal(20,8)" json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `gorm:"type:decimal(20,8)" json:"monthly_limit_usd"`

	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	AccountGroups []AccountGroup `gorm:"foreignKey:GroupID" json:"account_groups,omitempty"`

	// 虚拟字段 (不存储到数据库)
	AccountCount int64 `gorm:"-" json:"account_count,omitempty"`
}

func (Group) TableName() string {
	return "groups"
}

// IsActive 检查是否激活
func (g *Group) IsActive() bool {
	return g.Status == "active"
}

// IsSubscriptionType 检查是否为订阅类型分组
func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
}

// IsFreeSubscription 检查是否为免费订阅（不扣余额但有限额）
func (g *Group) IsFreeSubscription() bool {
	return g.IsSubscriptionType() && g.RateMultiplier == 0
}

// HasDailyLimit 检查是否有日限额
func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
}

// HasWeeklyLimit 检查是否有周限额
func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
}

// HasMonthlyLimit 检查是否有月限额
func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
}
