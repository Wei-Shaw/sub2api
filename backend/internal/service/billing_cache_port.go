package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status        string
	ExpiresAt     time.Time
	FiveHourUsage float64
	DailyUsage    float64
	WeeklyUsage   float64
	MonthlyUsage  float64
	Version       int64
}
