package service

import "time"

const (
	BillingTypeBalance      int8 = 0 // 钱包余额
	BillingTypeSubscription int8 = 1 // 订阅套餐
)

type UsageLog struct {
	ID        int64
	UserID    int64
	APIKeyID  int64
	AccountID int64
	RequestID string
	Model     string

	GroupID        *int64
	SubscriptionID *int64

	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int

	CacheCreation5mTokens int
	CacheCreation1hTokens int

	InputCost         float64
	OutputCost        float64
	CacheCreationCost float64
	CacheReadCost     float64
	TotalCost         float64
	ActualCost        float64
	RateMultiplier    float64

	BillingType  int8
	Stream       bool
	DurationMs   *int
	FirstTokenMs *int
	UserAgent    *string
	IPAddress    *string

	// 图片生成字段
	ImageCount int
	ImageSize  *string

	CreatedAt time.Time

	User         *User
	APIKey       *APIKey
	Account      *Account
	Group        *Group
	Subscription *UserSubscription
REDACTED

func (u *UsageLog) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens
REDACTED
