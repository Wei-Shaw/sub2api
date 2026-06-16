package service

import "time"

// AccountSchedCheck 是 IsSchedulableWith 接受的"调度上下文"。
//
// 设计目标：让 service_quota 把"该账号当前被预测限额阻塞"这类**请求级**额外判定
// 注入到调度循环，但**禁止**在 *Account 上加字段——避免跨请求污染 Account 缓存。
//
// 调用方在选号前用 caller-side map[accountID]*AccountSchedCheck 持有此上下文，
// listSchedulableAccounts 内的循环以 a.IsSchedulableWith(schedChecks[a.ID]) 形式读取，
// nil 安全（map 读未命中得 nil，IsSchedulableWith 视作"无额外约束"）。
type AccountSchedCheck struct {
	// QuotaBlock 非 nil 表示该账号已被 service_quota 阻塞，调度循环必须跳过；
	// nil 表示无 service_quota 阻塞。
	QuotaBlock *ServiceQuotaPredictedBlock
}

// schedulableNotBlockedByQuota 把 QuotaBlock 判定与"无 sc 时直接放过"合并为一个动词。
// 让 IsSchedulableWith 主流程保持线性、无嵌套。
func (sc *AccountSchedCheck) schedulableNotBlockedByQuota() bool {
	if sc == nil {
		return true
	}
	return sc.QuotaBlock == nil
}

// IsSchedulableWith 是 IsSchedulable 的"携带调度上下文"扩展版。
//
// 行为：先做与 IsSchedulable 完全相同的原生判定（active/schedulable/expires/overload/
// rate-limit/temp-unschedulable/quota），全部通过后再叠加 sc 携带的额外约束。
// 任何一步不通过即返回 false——优先级与原生限流保持一致。
//
// 旧调用方（IsSchedulable()）零修改：内部委托 IsSchedulableWith(nil)，
// nil sc 视作"无额外约束"，等价于历史行为。
func (a *Account) IsSchedulableWith(sc *AccountSchedCheck) bool {
	if a == nil {
		return false
	}
	if !a.IsActive() || !a.Schedulable {
		return false
	}
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
	}
	if a.IsAPIKeyOrBedrock() && a.IsQuotaExceeded() {
		return false
	}
	return sc.schedulableNotBlockedByQuota()
}

// SchedulableReason 返回首条让账号当前不可调度的原因 token。
//
// 用于日志 / 调度统计 / 监控页排查。优先级与 IsSchedulableWith 内部判断顺序一致：
// 原生条件优先于 service_quota（与"原生限流命中→其他平台再 429"的语义保持一致）。
//
// 返回值（已知 token，前端 i18n 用）：
//   - inactive / not_schedulable
//   - expired
//   - overloaded
//   - rate_limited
//   - temp_unschedulable
//   - api_key_quota_exceeded
//   - service_quota:<limiter>:<scope_kind>（含 sc.QuotaBlock 信息）
//   - 空字符串表示"当前可调度"
func (a *Account) SchedulableReason(sc *AccountSchedCheck) string {
	if a == nil {
		return "inactive"
	}
	if !a.IsActive() || !a.Schedulable {
		return "not_schedulable"
	}
	if reason := a.nativeUnschedulableReason(); reason != "" {
		return reason
	}
	if a.IsAPIKeyOrBedrock() && a.IsQuotaExceeded() {
		return "api_key_quota_exceeded"
	}
	if sc != nil && sc.QuotaBlock != nil {
		return "service_quota:" + sc.QuotaBlock.LimiterType + ":" + sc.QuotaBlock.ScopeKind
	}
	return ""
}

// nativeUnschedulableReason 抽出"原生时间窗口判定"四连让 SchedulableReason 主体不超过 30 行。
// 顺序与 IsSchedulableWith 完全一致，保持判定语义同步。
func (a *Account) nativeUnschedulableReason() string {
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return "expired"
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return "overloaded"
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return "rate_limited"
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return "temp_unschedulable"
	}
	return ""
}
