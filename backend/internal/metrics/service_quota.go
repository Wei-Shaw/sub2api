// Package metrics 提供轻量的进程内 metrics 计数器集合。
//
// 当前项目尚未引入 prometheus / OpenTelemetry 依赖，也没有 /metrics HTTP 端点，
// 因此这里使用 sync/atomic 实现内存级 counter（按 label 组合分桶）。
// 设计目标：
//
//  1. 调用方零分配快路径：标签组合首次出现时通过 sync.Map LoadOrStore 创建桶，
//     之后 hot path 仅做 atomic.AddUint64。
//  2. 接口与 prometheus.CounterVec 形似（WithLabelValues(...).Inc()），
//     未来切换 prometheus 时只需替换 NewCounterVec 实现，调用方零改动。
//  3. Snapshot 让运维端点 / 单元测试能枚举所有桶的当前值。
package metrics

import (
	"strings"
	"sync"
	"sync/atomic"
)

// CounterVec 是按一组 label 维度切分的单调递增计数器集合。
//
// 零值不可用，请通过 NewCounterVec 构造。
type CounterVec struct {
	name       string
	labelNames []string
	buckets    sync.Map // labelKey(string) -> *uint64
}

// NewCounterVec 构造一个 CounterVec。labelNames 仅用于 Snapshot 时回填可读 key。
func NewCounterVec(name string, labelNames ...string) *CounterVec {
	return &CounterVec{name: name, labelNames: append([]string(nil), labelNames...)}
}

// WithLabelValues 返回与给定 label 值组合关联的 counter 句柄。
// 调用 .Inc() 即对该桶计数 +1。values 长度可与 labelNames 不一致：
// 多余的会被忽略，缺失的按空字符串处理（防止调用点漏写时 panic）。
func (c *CounterVec) WithLabelValues(values ...string) *CounterHandle {
	key := joinLabelValues(values)
	if v, ok := c.buckets.Load(key); ok {
		ptr, _ := v.(*uint64)
		return &CounterHandle{v: ptr}
	}
	var zero uint64
	actual, _ := c.buckets.LoadOrStore(key, &zero)
	ptr, _ := actual.(*uint64)
	return &CounterHandle{v: ptr}
}

// Snapshot 返回所有 label 组合当前的累计值，便于外部端点导出 / 单测断言。
func (c *CounterVec) Snapshot() map[string]uint64 {
	out := make(map[string]uint64)
	c.buckets.Range(func(k, v any) bool {
		keyStr, _ := k.(string)
		ptr, _ := v.(*uint64)
		out[keyStr] = atomic.LoadUint64(ptr)
		return true
	})
	return out
}

// Name 暴露给运维端点导出时使用。
func (c *CounterVec) Name() string { return c.name }

// LabelNames 返回 label 名称的拷贝，避免外部修改影响内部状态。
func (c *CounterVec) LabelNames() []string {
	return append([]string(nil), c.labelNames...)
}

// CounterHandle 指向某个具体 label 组合的 atomic 计数桶。
type CounterHandle struct{ v *uint64 }

// Inc 将关联桶 +1。无锁。
func (h *CounterHandle) Inc() {
	if h == nil || h.v == nil {
		return
	}
	atomic.AddUint64(h.v, 1)
}

// Add 将关联桶 +n（n<0 在 uint64 上没有意义，会被忽略）。
func (h *CounterHandle) Add(n int64) {
	if h == nil || h.v == nil || n <= 0 {
		return
	}
	atomic.AddUint64(h.v, uint64(n))
}

// joinLabelValues 用一个不可能在 label 值中出现的分隔符拼接 label，作为 sync.Map 的 key。
func joinLabelValues(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "\x1f") // ASCII Unit Separator
}

// ServiceQuotaPreCheckResult 是 ServiceQuotaPreCheckTotal 的 result label 取值集合。
const (
	ServiceQuotaPreCheckResultAllowed  = "allowed"
	ServiceQuotaPreCheckResultDenied   = "denied"
	ServiceQuotaPreCheckResultFailOpen = "fail_open"
)

// ServiceQuotaFailOpenReason 是 ServiceQuotaFailOpenTotal 的 reason label 取值集合。
const (
	ServiceQuotaFailOpenReasonRedisError = "redis_error"
	ServiceQuotaFailOpenReasonTimeout    = "timeout"
)

var (
	// ServiceQuotaPreCheckTotal 记录每次 PreCheck 中对单个 (rule, limiter) 组合的判断结果。
	//
	// label：
	//   - rule_id：规则 ID
	//   - limiter_type：rpm / tpm / tpd / daily_usd / concurrency
	//   - result：allowed（放行）/ denied（拒绝）/ fail_open（限流器报错降级）
	ServiceQuotaPreCheckTotal = NewCounterVec(
		"svcquota_precheck_total",
		"rule_id", "limiter_type", "result",
	)

	// ServiceQuotaFailOpenTotal 记录限流器异常导致的 fail-open 次数。
	//
	// label：
	//   - limiter_type：哪类 limiter 出现问题
	//   - reason：redis_error / timeout
	ServiceQuotaFailOpenTotal = NewCounterVec(
		"svcquota_fail_open_total",
		"limiter_type", "reason",
	)
)
