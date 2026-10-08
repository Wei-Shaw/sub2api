package service

import (
	"time"
)

const SubscriptionLimitCacheSchemaVersion = 1

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status             string
	ExpiresAt          time.Time
	DailyUsage         float64
	WeeklyUsage        float64
	MonthlyUsage       float64
	DailyLimitUSD      *float64
	WeeklyLimitUSD     *float64
	MonthlyLimitUSD    *float64
	LimitSchemaVersion int
	Version            int64
}
