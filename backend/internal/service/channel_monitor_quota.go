package service

import "time"

// Coding Plan 配额快照（DeepSeek / GLM / Kimi 套餐监控）。
// 参考 cc-switch 的 coding_plan 服务：每次检测除了可用性状态外，
// 还把套餐配额解析成若干 tier 存入 history.message，供前端渲染进度条。
const (
	// QuotaTierFiveHour 5 小时滚动窗口限额。
	QuotaTierFiveHour = "five_hour"
	// QuotaTierWeekly 周（7 天）窗口限额。
	QuotaTierWeekly = "weekly_limit"
	// QuotaTierBalance 账户余额（DeepSeek 只有余额查询，无窗口限额）。
	QuotaTierBalance = "balance"
)

// MonitorQuotaTier 单个窗口/余额的配额快照。
// Utilization 为已用百分比（0-100）；余额类条目（balance）不使用 Utilization。
type MonitorQuotaTier struct {
	Name        string     `json:"name"` // five_hour / weekly_limit / balance
	Utilization *float64   `json:"utilization,omitempty"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	// 余额类字段（仅 balance tier 使用；字符串原样保留上游精度）。
	Balance        *string `json:"balance,omitempty"`
	Currency       *string `json:"currency,omitempty"`
	GrantedBalance *string `json:"granted_balance,omitempty"`
	ToppedUp       *string `json:"topped_up_balance,omitempty"`
}

// MonitorQuotaSnapshot 一次 coding-plan 检测的配额快照。
type MonitorQuotaSnapshot struct {
	Tiers     []MonitorQuotaTier `json:"tiers"`
	PlanLevel string             `json:"plan_level,omitempty"` // 套餐等级（如 GLM 的 level）
	Available *bool              `json:"available,omitempty"`  // DeepSeek is_available
}

// isCodingPlanProvider 判断 provider 是否为国产 Coding Plan 配额监控。
func isCodingPlanProvider(p string) bool {
	switch p {
	case MonitorProviderDeepseek, MonitorProviderGLM, MonitorProviderKimi:
		return true
	default:
		return false
	}
}
