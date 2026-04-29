package service

import (
	"context"
	"errors"
)

// errBillingTicketClosed 表示 caller 在 Close 之后仍调用 Consume——这是协议错误。
// 内部 sentinel，不暴露给 HTTP 响应；上层应直接 panic? 不：返回 error 让 caller 走 fail-safe。
var errBillingTicketClosed = errors.New("billing_ticket: consume after close")

// BillingTicket 是 BillingCacheService 两阶段计费检查（Prepare → Consume）的状态载体。
//
// 设计目标：让 caller 在路由前完成余额 / 订阅 / RPM 等"必到字段"检查，但把对 channel/account
// 维度敏感的 service quota concurrency / rpm 抢占推迟到选定上游账号之后再执行。
// 这样 path 里限定 channel_id / account_id 的限流规则才能真正生效，否则 PreCheck 永远在
// channelID=0 / accountID=0 状态下匹配，等于失效。
//
// 使用协议（caller 必须严格遵守）：
//
//  1. 调 BillingCacheService.PrepareBillingCheck(...) 获取 *BillingTicket，err 非 nil 时直接返回
//     （ticket 此时为 nil，无需 Close）；
//  2. 立即在所有返回路径上 defer ticket.Close()；
//  3. 选定 channel + account 后调 ticket.Consume(ctx, channelID, accountID)，
//     err 非 nil 时表示限流命中，照样 return，已 defer 的 Close 会兜底释放任何已抢的 lease；
//  4. 同一个 ticket 允许多次 Consume——在 fallback 重试切换账号时常见。
//     Consume 内部检测到 lease 已存在会先释放再重新抢，使 lease 始终对应"当前生效的 channel/account"。
//
// feature flag service_quota_precheck_two_phase 关闭（默认）时：
//   - Prepare 阶段就完成旧 PreCheck（一次性 acquire），ticket 持有 lease；
//   - Consume 退化为 noop（不会重新 acquire）；
//   - 行为与历史 CheckBillingEligibility 完全一致。
//
// flag 开启时：
//   - Prepare 阶段只调 PreCheckSelect 选规则，不抢 concurrency；
//   - Consume 阶段才调 PreCheckAcquire 按完整 channel/account 维度抢槽位 / 增 RPM。
//
// 并发：BillingTicket 不要跨 goroutine 共享。每个请求一个 ticket，由 caller 的请求 goroutine 独占。
type BillingTicket struct {
	svc      *BillingCacheService
	quotaReq ServiceQuotaCheckRequest
	plan     *ServiceQuotaPreCheckPlan
	lease    *ServiceQuotaLease

	// twoPhase=true 表示 Prepare 阶段只选 plan，Consume 阶段才 acquire（feature flag 开启）。
	// twoPhase=false 表示 Prepare 已完成 PreCheck，Consume 是 noop。
	twoPhase bool
	// legacyOneOff=true 标记本 ticket 由 CheckBillingEligibility（兼容入口）创建，无视 flag
	// 走单阶段路径；CheckBillingEligibility 调用 ticket.Consume 后会 detachLease，所以
	// Consume 内部允许在 legacyOneOff 时把 lease 转移所有权而不抢新的。
	legacyOneOff bool
	closed       bool
}

// Consume 完成两阶段计费检查的第二阶段：基于 caller 选定的 channel + account 抢 service quota
// concurrency / 增 RPM。
//
// 行为分支：
//   - feature flag off / legacyOneOff：lease 已在 Prepare 时获得，本方法只校验 ticket 状态后立即返回 nil。
//   - feature flag on：基于 plan 调用 ServiceQuotaService.PreCheckAcquire；如本 ticket 已有 lease
//     （上一次 Consume 抢的，常见于 fallback 切换账号），先 Release 再抢新的，使 lease 与当前
//     channel/account 严格对应。
//
// 失败时不会更新 ticket.lease，caller 通过 defer ticket.Close() 兜底；不会 panic。
//
// channelID/accountID==0 表示 caller 没有这个维度的选择（例如某些 fallback 路径仅 group/platform 限定）。
// 此时只命中那些 path.channel_id / path.account_id 为 nil 的规则。
func (t *BillingTicket) Consume(ctx context.Context, channelID, accountID int64) error {
	if t == nil {
		return nil
	}
	if t.closed {
		return errBillingTicketClosed
	}
	if !t.twoPhase {
		// flag off：Prepare 已完成 PreCheck，无需再做。lease 已经持有（若有规则匹配）。
		// 把入参里的 channel/account 也回填到 quotaReq，方便 detachLease 之外的观测。
		t.quotaReq.ChannelID = channelID
		t.quotaReq.AccountID = accountID
		return nil
	}
	if t.svc == nil || t.svc.serviceQuota == nil {
		return nil
	}

	// 上一次 Consume 已经抢过 lease：fallback 切换 channel/account 时先释放旧的，
	// 重新基于新 channel/account 抢，避免 lease 计数泄漏到旧 path。
	if t.lease != nil {
		t.lease.Release()
		t.lease = nil
	}

	t.quotaReq.ChannelID = channelID
	t.quotaReq.AccountID = accountID

	acquired, err := t.svc.serviceQuota.PreCheckAcquire(ctx, t.plan, channelID, accountID)
	if err != nil {
		return err
	}
	t.lease = wrapServiceQuotaLeaseOnce(acquired)
	return nil
}

// Close 释放 ticket 持有的任何 service quota concurrency 槽位。多次调用安全（once 保护）。
//
// caller 必须在所有返回路径上 defer ticket.Close()——包括 Consume 失败、forward 失败、
// panic 兜底等异常路径。lease 内部已 wrapServiceQuotaLeaseOnce，不会双释放。
func (t *BillingTicket) Close() {
	if t == nil || t.closed {
		return
	}
	t.closed = true
	if t.lease != nil {
		// matched rules 仅含 RPM/deferred 时 lease.Release 为 nil（无 concurrency 槽位需释放）。
		if t.lease.Release != nil {
			t.lease.Release()
		}
		t.lease = nil
	}
}

// PreCheckPlan 暴露 ticket 内部缓存的 service_quota PreCheckSelect 计划，给调度阶段
// 用于批量预测候选账号是否被 service_quota 阻塞（详见 ServiceQuotaService.SnapshotForAccounts）。
//
// 返回值语义：
//   - service_quota 关闭 / 无规则匹配 → nil（caller 跳过预过滤）
//   - feature flag off / legacyOneOff（CheckBillingEligibility 路径）→ 也返回 nil：
//     单阶段路径 plan 不被持有，没有可用的候选规则集合
//   - twoPhase=true 且 PreCheckSelect 选出至少一条候选规则 → 非 nil *ServiceQuotaPreCheckPlan
//
// 不暴露内部 svc / lease / quotaReq 字段，保持 BillingTicket 的封装边界——caller 只能
// 通过 Consume / Close 改变 ticket 状态，不能旁路调用 PreCheckAcquire / Release。
//
// nil-safe：t == nil 时返回 nil（与其他 receiver 方法一致）。
func (t *BillingTicket) PreCheckPlan() *ServiceQuotaPreCheckPlan {
	if t == nil {
		return nil
	}
	return t.plan
}

// detachLease 把 lease 所有权从 ticket 转移给 caller，并把 ticket 置为已关闭。
// 仅供 CheckBillingEligibility 兼容路径使用——它需要返回原始 *ServiceQuotaLease 让旧 caller
// 沿用 defer ReleaseQuotaLease(lease) 协议；ticket 在转移后不再持有任何资源。
func (t *BillingTicket) detachLease() *ServiceQuotaLease {
	if t == nil {
		return nil
	}
	lease := t.lease
	t.lease = nil
	t.closed = true
	return lease
}
