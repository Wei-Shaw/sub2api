package domain

import (
	"errors"
	"time"
)

// ErrUserPlatformQuotaNotFound service 层 sentinel：quota 记录不存在。
// adapter 将 repository.ErrUserPlatformQuotaNotFound 包装为此错误，
// handler 只需引用 service 包，无需直接依赖 repository 包。
var ErrUserPlatformQuotaNotFound = errors.New("user platform quota not found")

// ErrUserPlatformQuotaFKViolation service 层 sentinel：批量 snapshot UPSERT 时存在
// user_id 不在 users 表的记录（外键违反）。adapter 负责将 repository 层同名 sentinel 包装为此错误。
var ErrUserPlatformQuotaFKViolation = errors.New("user platform quota snapshot FK violation")

// UserPlatformQuotaSnapshot 是 service 层 flusher 向 DB 写入快照时使用的传输结构。
// 字段语义与 repository.UserPlatformQuotaSnapshot 完全对应，由 adapter 负责转换。
type UserPlatformQuotaSnapshot struct {
	UserID             int64
	Platform           string
	DailyUsageUSD      float64
	WeeklyUsageUSD     float64
	MonthlyUsageUSD    float64
	DailyWindowStart   time.Time
	WeeklyWindowStart  time.Time
	MonthlyWindowStart time.Time
}

// UserPlatformQuotaRecord service 层传输结构体（与 repository 层解耦）。
type UserPlatformQuotaRecord struct {
	UserID          int64
	Platform        string
	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64
	// 窗口起始时间（可选，用于未来 reset 校验）
	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time
}
