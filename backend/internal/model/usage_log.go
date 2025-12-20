package model

import (
	"time"
)

// 消费类型常量
const (
	BillingTypeBalance      int8 = 0 // 钱包余额
	BillingTypeSubscription int8 = 1 // 订阅套餐
)

type UsageLog struct {
	ID        int64  `gorm:"primaryKey" json:"id"`
	UserID    int64  `gorm:"index;not null" json:"user_id"`
	ApiKeyID  int64  `gorm:"index;not null" json:"api_key_id"`
	AccountID int64  `gorm:"index;not null" json:"account_id"`
	RequestID string `gorm:"size:64" json:"request_id"`
	Model     string `gorm:"size:100;index;not null" json:"model"`

	// 订阅关联（可选）
	GroupID        *int64 `gorm:"index" json:"group_id"`
	SubscriptionID *int64 `gorm:"index" json:"subscription_id"`

	// Token使用量（4类）
	InputTokens         int `gorm:"default:0;not null" json:"input_tokens"`
	OutputTokens        int `gorm:"default:0;not null" json:"output_tokens"`
	CacheCreationTokens int `gorm:"default:0;not null" json:"cache_creation_tokens"`
	CacheReadTokens     int `gorm:"default:0;not null" json:"cache_read_tokens"`

	// 详细的缓存创建分类
	CacheCreation5mTokens int `gorm:"default:0;not null" json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int `gorm:"default:0;not null" json:"cache_creation_1h_tokens"`

	// 费用（USD）
	InputCost         float64 `gorm:"type:decimal(20,10);default:0;not null" json:"input_cost"`
	OutputCost        float64 `gorm:"type:decimal(20,10);default:0;not null" json:"output_cost"`
	CacheCreationCost float64 `gorm:"type:decimal(20,10);default:0;not null" json:"cache_creation_cost"`
	CacheReadCost     float64 `gorm:"type:decimal(20,10);default:0;not null" json:"cache_read_cost"`
	TotalCost         float64 `gorm:"type:decimal(20,10);default:0;not null" json:"total_cost"`     // 原始总费用
	ActualCost        float64 `gorm:"type:decimal(20,10);default:0;not null" json:"actual_cost"`    // 实际扣除费用
	RateMultiplier    float64 `gorm:"type:decimal(10,4);default:1;not null" json:"rate_multiplier"` // 计费倍率

	// 元数据
	BillingType  int8 `gorm:"type:smallint;default:0;not null" json:"billing_type"` // 0=余额 1=订阅
	Stream       bool `gorm:"default:false;not null" json:"stream"`
	DurationMs   *int `json:"duration_ms"`
	FirstTokenMs *int `json:"first_token_ms"` // 首字时间（流式请求）

	CreatedAt time.Time `gorm:"index;not null" json:"created_at"`

	// 关联
	User         *User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ApiKey       *ApiKey           `gorm:"foreignKey:ApiKeyID" json:"api_key,omitempty"`
	Account      *Account          `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	Group        *Group            `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Subscription *UserSubscription `gorm:"foreignKey:SubscriptionID" json:"subscription,omitempty"`
}

func (UsageLog) TableName() string {
	return "usage_logs"
}

// TotalTokens 总token数
func (u *UsageLog) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens
}
