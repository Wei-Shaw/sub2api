package service

import (
	"context"
	"log/slog"
)

// FilterAccountsByServiceQuotaSchedulability 把 SnapshotForAccounts 预测出的"账号被
// service_quota 阻塞"结果应用到 listSchedulableAccounts 返回的候选切片上：命中 quotaBlock
// 的账号被剔除，让调度循环像原生限流命中一样自然走"切到下一个账号"路径。
//
// 全部候选都被剔除时返回空切片——caller 必须把空切片视作"无可用账号"，走原生
// ErrNoAvailableAccounts / 503 错误链路（与 RateLimitResetAt 全部命中字节级一致）。
//
// 设计权衡：
//   - 不在 *Account 上加 schedCtx 字段（避免跨请求污染 Account 缓存）
//   - 不依赖 GatewayService 内部状态，纯函数式入参 → 便于单测与跨平台 gateway 复用
//   - quotaSvc 为 nil / plan 为 nil / accounts 为空时 fast-path 返回原 slice
//   - SnapshotForAccounts 内部 fail-open 已保证 Redis 错误不会传染调度，本函数无需额外处理
//
// 错误码链路：调用方拿到空切片后必须返回 ErrNoAvailableAccounts，
// **禁止**新增 service_quota 专用的 503/429——前端按现有"无可用账号"展示。
func FilterAccountsByServiceQuotaSchedulability(
	ctx context.Context,
	quotaSvc ServiceQuotaService,
	plan *ServiceQuotaPreCheckPlan,
	base ServiceQuotaCheckRequest,
	accounts []Account,
) []Account {
	if quotaSvc == nil || plan == nil || len(plan.Rules) == 0 || len(accounts) == 0 {
		return accounts
	}
	ids := make([]int64, 0, len(accounts))
	for i := range accounts {
		ids = append(ids, accounts[i].ID)
	}
	blocks, err := quotaSvc.SnapshotForAccounts(ctx, base, plan, ids)
	if err != nil {
		// SnapshotForAccounts 当前实现 fail-open 内部已吞错；保留这条防御让接口实现切换时不破语义。
		slog.Warn("service_quota: SnapshotForAccounts returned error, scheduling proceeds without prefilter",
			"error", err)
		return accounts
	}
	if len(blocks) == 0 {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if _, blocked := blocks[accounts[i].ID]; blocked {
			continue
		}
		out = append(out, accounts[i])
	}
	return out
}

// AccountSchedChecksFromBlocks 把 SnapshotForAccounts 的结果转成 caller-side
// map[accountID]*AccountSchedCheck，便于在 isAccountSchedulableForSelection 等位置
// 用 a.IsSchedulableWith(schedChecks[a.ID]) 形式判定。
//
// 与 FilterAccountsByServiceQuotaSchedulability 互补：前者直接剔除候选切片（最简单的接入方式）；
// 本函数让 caller 把同一份 block 信息保留到调度循环尾部，例如用于 SchedulableReason 日志。
func AccountSchedChecksFromBlocks(blocks map[int64]*ServiceQuotaPredictedBlock) map[int64]*AccountSchedCheck {
	if len(blocks) == 0 {
		return nil
	}
	out := make(map[int64]*AccountSchedCheck, len(blocks))
	for id, b := range blocks {
		if b == nil {
			continue
		}
		out[id] = &AccountSchedCheck{QuotaBlock: b}
	}
	return out
}
