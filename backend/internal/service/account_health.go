package service

import "time"

// HealthCounts 表示一组账号的健康分布计数(用于聚合卡片 §7.2)。
// Total 为账号总数;HealthRate 分母为"纳入健康率统计的账号数"(healthy+error+limited,排除 paused/untested,见计划 §4.2)。
// HealthRate 为指针:分母为 0 时为 nil(前端显示"—")。
type HealthCounts struct {
	Total      int      `json:"total"`
	Healthy    int      `json:"healthy"`
	Error      int      `json:"error"`
	Limited    int      `json:"limited"`
	Paused     int      `json:"paused"`
	Untested   int      `json:"untested"`
	HealthRate *float64 `json:"health_rate"`
}

// ComputeHealthRate 计算并填充 HealthRate(计划 §4.2:healthy/(healthy+error+limited))。
// 分母为 0 时置 nil。应在所有计数累加完成后调用一次。
func (c *HealthCounts) ComputeHealthRate() {
	denom := c.Healthy + c.Error + c.Limited
	if denom <= 0 {
		c.HealthRate = nil
		return
	}
	rate := float64(c.Healthy) / float64(denom)
	c.HealthRate = &rate
}

// HealthSummaryBucket 表示按某个维度(平台或分组)聚合的一个健康卡片。
type HealthSummaryBucket struct {
	Key    string       `json:"key"`   // 平台标识或分组 ID 的字符串形式
	Label  string       `json:"label"` // 展示名(分组名;平台则与 key 相同)
	Counts HealthCounts `json:"counts"`
}

// AccountHealthSummary 是健康聚合接口的完整返回(需求 §7.2)。
type AccountHealthSummary struct {
	Overall    HealthCounts          `json:"overall"`
	ByPlatform []HealthSummaryBucket `json:"by_platform"`
	ByGroup    []HealthSummaryBucket `json:"by_group"`
}

// 账号健康分类常量。对应需求 §6.5 映射表与 §9 判定优先级。
// 注意:本文件的 EvaluateHealth 是健康判定的唯一权威实现。
// 聚合接口(admin_service.GetAccountHealthSummary 的 SQL CASE/FILTER)必须与
// 本函数的优先级严格一致,任一侧改动需同步另一侧,否则聚合数与列表逐个状态会不一致(违反需求 §15.5)。
const (
	HealthHealthy  = "healthy"  // 绿:active + schedulable + 不受限 + 最近检测 success 且未过期
	HealthError    = "error"    // 红:status=error,或最近检测 failed 且未过期
	HealthLimited  = "limited"  // 黄:限流/过载/临时不可调度
	HealthPaused   = "paused"   // 灰:schedulable=false 或 status=disabled(不计入健康率分母)
	HealthUntested = "untested" // 灰:无检测结果或结果已过期
)

// HealthResultTTL 检测结果时效性门槛(需求 §9.1,默认 24 小时)。
// 超过该时长未更新的检测结果视为过期,降级为 untested。
// 后续可提为配置项。
const HealthResultTTL = 24 * time.Hour

// EvaluateHealth 根据账号运行态 + 最近一次检测结果,返回健康分类。
//
// 入参:
//   - acc:    账号(运行态字段)
//   - latest: 该账号最近一次检测结果(可为 nil,表示无检测记录)
//   - now:    当前时间(显式传入以便测试)
//
// 优先级严格按需求 §9(冲突时优先展示更影响可用性的状态):
//  1. status=disabled 或 schedulable=false        → paused
//  2. status=error                                 → error
//  3. 限流/过载/临时不可调度(任一未到期)          → limited
//  4. 最近检测 failed 且未过期(<=TTL)             → error
//  5. 最近检测 success 且未过期(<=TTL)            → healthy
//  6. 否则(无结果或已过期)                         → untested
func EvaluateHealth(acc *Account, latest *ScheduledTestResult, now time.Time) string {
	if acc == nil {
		return HealthUntested
	}

	// 1. 暂停:管理员主动暂停调度,或账号被禁用。不纳入健康率分母。
	if acc.Status == StatusDisabled || !acc.Schedulable {
		return HealthPaused
	}

	// 2. 错误状态:账号被标记 error(如 401/403、凭据失效)。
	if acc.Status == StatusError {
		return HealthError
	}

	// 3. 受限:限流(429)/过载(529)/临时不可调度,三者任一未到期。受限优先级高于检测结果。
	if isAccountLimited(acc, now) {
		return HealthLimited
	}

	// 4/5. 检测结果(需满足未过期门槛)。
	if latest != nil && !latest.FinishedAt.IsZero() {
		if now.Sub(latest.FinishedAt) <= HealthResultTTL {
			switch latest.Status {
			case "failed":
				return HealthError
			case "success":
				return HealthHealthy
			}
		}
	}

	// 6. 无检测结果或结果已过期。
	return HealthUntested
}

// isAccountLimited 判断账号是否处于受限运行态(限流/过载/临时不可调度)。
// 与 Account.IsSchedulable() 中的受限分支口径一致,但只关心"受限"这一类。
func isAccountLimited(acc *Account, now time.Time) bool {
	if acc.RateLimitResetAt != nil && now.Before(*acc.RateLimitResetAt) {
		return true
	}
	if acc.OverloadUntil != nil && now.Before(*acc.OverloadUntil) {
		return true
	}
	if acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil) {
		return true
	}
	return false
}
