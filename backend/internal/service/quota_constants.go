package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// ---- Settings keys（用户每日配额限制，feature issue #1750） ----
const (
	// SettingKeyUsageLimitEnabled 总开关：false 时所有配额检查/累加短路返回
	SettingKeyUsageLimitEnabled = "usage_limit_enabled"
	// SettingKeyDefaultUsageLimitEnabled 用户 usage_limit_enabled=nil 时的回退值
	SettingKeyDefaultUsageLimitEnabled = "default_usage_limit_enabled"
	// SettingKeyDefaultDailyUsageLimitUSD 新建用户默认 daily_usage_limit_usd（0 视为不下发）
	SettingKeyDefaultDailyUsageLimitUSD = "default_daily_usage_limit_usd"
)

// ---- Rule period ----
const (
	QuotaPeriodDaily = "daily"
)

// ---- Error scope values（ErrQuotaExceeded metadata.scope 字段） ----
const (
	QuotaScopeTotal = "total"
	QuotaScopeRule  = "rule"
)

// ---- Redis 缓存 TTL ----
const (
	// QuotaUsageTTL 用量计数器 TTL：48h 覆盖时区切换边界 + 保险
	QuotaUsageTTL = 48 * time.Hour
	// QuotaConfigTTL 用户配额配置缓存 TTL
	QuotaConfigTTL = 5 * time.Minute
	// QuotaConfigTTLJitter 配置缓存抖动幅度，防止雪崩
	QuotaConfigTTLJitter = 30 * time.Second
)

// quotaDateKey 按系统配置时区计算"今日"的日期字符串（YYYYMMDD）。
//
// 时区来源：backend/internal/pkg/timezone 包（Init 时由 cfg.Timezone 设置，默认 Asia/Shanghai）。
// 与 user_subscriptions.daily_usage_usd 的重置时区完全一致。
// 禁止硬编码任何时区字符串。
func quotaDateKey(t time.Time) string {
	return timezone.StartOfDay(t).Format("20060102")
}

// nextQuotaResetTime 返回下一次配额重置的墙钟时间（次日零点，配置时区）
func nextQuotaResetTime() time.Time {
	return timezone.StartOfDay(timezone.Now()).Add(24 * time.Hour)
}
