package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 渠道监控「配额模式」的组级聚合。
//
// 单账号绑定回答的是「这一个号还剩多少额度」，一个号耗尽并不代表渠道不可用。
// 账号多的分组（多个 kiro 账号轮换）需要聚合口径：把组内全部 active 账号的
// 额度汇总成一份快照，并按「有额度的账号数」推导渠道状态。
//
// 聚合直接复用 per-account Fetch，因此白拿它的 singleflight 与 TTL 快照缓存：
// 组内 N 个账号摊下来约每 monitorQuotaFetchCacheTTL 一轮上游调用。有界并发 +
// 总时间预算防止冷启动时单次检测拖过调度间隔（interval 最小 15s）。

// FetchGroup 聚合分组内全部 active 账号的配额快照。
// 与 Fetch 一致永不返回 error：所有失败都降级为快照，保证检测历史连续。
func (f *ChannelMonitorQuotaFetcher) FetchGroup(ctx context.Context, groupID int64) *domain.MonitorQuotaSnapshot {
	now := time.Now()
	if f == nil || f.groupAccounts == nil {
		return quotaErrorSnapshot("usage", "quota fetcher is not configured", now)
	}

	accounts, err := f.groupAccounts.ListByGroup(ctx, groupID)
	if err != nil {
		// 分组被删除后 group_id 会被 FK 置空，走不到这里；这里多是库层故障。
		// 与「账号未关联」同样归为配置类问题（degraded），不算渠道故障。
		return quotaErrorSnapshot("usage", monitorQuotaErrGroupNotFound, now)
	}
	if len(accounts) == 0 {
		return quotaErrorSnapshot("usage", monitorQuotaErrGroupNoAccounts, now)
	}

	return f.aggregateAccountSnapshots(ctx, accounts, now)
}

// aggregateAccountSnapshots 有界并发抓取组内每个账号的快照并汇总。
func (f *ChannelMonitorQuotaFetcher) aggregateAccountSnapshots(
	ctx context.Context,
	accounts []Account,
	now time.Time,
) *domain.MonitorQuotaSnapshot {
	// 总预算兜住冷启动：单账号抓取自身超时是 monitorQuotaFetchTimeout（45s），
	// N 个账号排队等满会远超调度间隔。超预算的账号计为「未知」，由
	// deriveGroupQuotaStatus 区别于「确实耗尽」，避免首轮误报 failed。
	// Fetch 内部的实际抓取脱离调用方 ctx，所以被预算掐断的账号仍会在后台把
	// 快照写进缓存，下一轮检测直接命中。
	budgetCtx, cancel := context.WithTimeout(ctx, monitorGroupQuotaBudget)
	defer cancel()

	snapshots := make([]*domain.MonitorQuotaSnapshot, len(accounts))
	sem := make(chan struct{}, monitorGroupQuotaConcurrency)
	var wg sync.WaitGroup
	for i := range accounts {
		wg.Add(1)
		go func(idx int, accountID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			snapshots[idx] = f.Fetch(budgetCtx, accountID)
		}(i, accounts[i].ID)
	}
	wg.Wait()

	return buildGroupQuotaSnapshot(snapshots, now)
}

// groupTierAccumulator 汇总同一 (window, label) 下各账号的额度。
//
// 绝对值（Used/Limit）只有部分平台会给：kiro credits、Anthropic 有请求上限的
// 窗口、Grok 有 limit 的窗口都有；Antigravity per-model 只有百分比。因此分两种
// 口径：全部账号都给了绝对值时按 sum(used)/sum(limit) 算真实占用率（精确）；
// 否则退化为百分比均值（近似，但比丢弃该窗口有用）。
type groupTierAccumulator struct {
	window   string
	label    string
	resetAt  string
	sumUsed  float64
	sumLimit float64
	// withAbsolute 有绝对值的贡献者数；等于 contributors 时才按绝对值汇总。
	withAbsolute int
	sumPercent   float64
	contributors int
}

type groupTierKey struct {
	window string
	label  string
}

// buildGroupQuotaSnapshot 把各账号快照汇总成一份组级快照。
func buildGroupQuotaSnapshot(snapshots []*domain.MonitorQuotaSnapshot, now time.Time) *domain.MonitorQuotaSnapshot {
	agg := &domain.MonitorQuotaSnapshot{
		Source:        "usage",
		FetchedAt:     now,
		AccountsTotal: len(snapshots),
	}

	accumulators := make(map[groupTierKey]*groupTierAccumulator, 8)
	order := make([]groupTierKey, 0, 8)
	var firstError string

	for _, s := range snapshots {
		if s == nil {
			continue
		}
		// Source 取第一个拿到数据的账号：同组账号平台一致，来源必然同构。
		if s.Source != "" {
			agg.Source = s.Source
		}
		if agg.PlanLevel == "" {
			agg.PlanLevel = s.PlanLevel
		}
		if !s.Success {
			if firstError == "" {
				firstError = s.Error
			}
			if s.CredentialInvalid {
				// 凭据失效的号确实用不了，计入耗尽而非未知。
				agg.AccountsExhausted++
			}
			continue
		}
		if monitorAccountQuotaExhausted(s) {
			agg.AccountsExhausted++
		} else {
			agg.AccountsHealthy++
		}
		accumulateGroupTiers(accumulators, &order, s.Tiers)
	}

	// 至少有一个账号查到了数据才算抓取成功；全灭时透出首个错误，
	// 让 deriveGroupQuotaStatus 能区分「没查到」与「确实耗尽」。
	agg.Success = agg.AccountsHealthy > 0 || agg.AccountsExhausted > 0
	if !agg.Success {
		agg.Error = firstError
	}
	agg.Tiers = flattenGroupTiers(accumulators, order)
	return agg
}

func accumulateGroupTiers(
	accumulators map[groupTierKey]*groupTierAccumulator,
	order *[]groupTierKey,
	tiers []domain.MonitorQuotaTier,
) {
	for _, tier := range tiers {
		key := groupTierKey{window: tier.Window, label: tier.Label}
		acc, ok := accumulators[key]
		if !ok {
			acc = &groupTierAccumulator{window: tier.Window, label: tier.Label}
			accumulators[key] = acc
			*order = append(*order, key)
		}
		acc.contributors++
		acc.sumPercent += tier.UsedPercent
		if tier.Limit > 0 {
			acc.withAbsolute++
			acc.sumUsed += tier.Used
			acc.sumLimit += tier.Limit
		}
		// 各账号重置时间可能不同（额度周期独立），取最早的一个当代表：
		// 展示「下一次有号回血」比展示最晚的更有意义。
		if tier.ResetAt != "" && (acc.resetAt == "" || tier.ResetAt < acc.resetAt) {
			acc.resetAt = tier.ResetAt
		}
	}
}

func flattenGroupTiers(
	accumulators map[groupTierKey]*groupTierAccumulator,
	order []groupTierKey,
) []domain.MonitorQuotaTier {
	if len(order) == 0 {
		return nil
	}
	tiers := make([]domain.MonitorQuotaTier, 0, len(order))
	for _, key := range order {
		acc := accumulators[key]
		if acc == nil || acc.contributors == 0 {
			continue
		}
		tier := domain.MonitorQuotaTier{
			Window:  acc.window,
			Label:   acc.label,
			ResetAt: acc.resetAt,
		}
		if acc.withAbsolute == acc.contributors && acc.sumLimit > 0 {
			tier.Used = acc.sumUsed
			tier.Limit = acc.sumLimit
			tier.UsedPercent = acc.sumUsed / acc.sumLimit * 100
		} else {
			tier.UsedPercent = acc.sumPercent / float64(acc.contributors)
		}
		tiers = append(tiers, tier)
	}
	if len(tiers) == 0 {
		return nil
	}
	return tiers
}

// monitorAccountQuotaExhausted 判定单账号快照是否已无额度可用。
//
// 抓取失败（!Success）不算耗尽——那是「没查到」，计为未知；唯一例外是凭据失效，
// 那种号确实用不了，由调用方单独计入耗尽。
//
// 同一 window 下的多档是可替换额度池（kiro credits+bonus、gemini 多档）：
// 这一窗全部打满才算这个号在该窗口不可用。任一 window 整体打满 → 整号耗尽
// （claude 5h 打满就发不出去，即使 7d 还有余）。
func monitorAccountQuotaExhausted(snapshot *domain.MonitorQuotaSnapshot) bool {
	if snapshot == nil || !snapshot.Success {
		return false
	}
	if snapshot.CredentialInvalid || snapshot.BalanceLow {
		return true
	}
	if len(snapshot.Tiers) == 0 {
		return false
	}
	byWindow := make(map[string][]domain.MonitorQuotaTier, 4)
	for _, tier := range snapshot.Tiers {
		byWindow[tier.Window] = append(byWindow[tier.Window], tier)
	}
	for _, tiers := range byWindow {
		if allQuotaTiersExhausted(tiers) {
			return true
		}
	}
	return false
}

func quotaTierExhausted(tier domain.MonitorQuotaTier) bool {
	if tier.Limit > 0 && tier.Used >= tier.Limit {
		return true
	}
	return tier.UsedPercent >= monitorQuotaExhaustedUsedPercent
}

func allQuotaTiersExhausted(tiers []domain.MonitorQuotaTier) bool {
	if len(tiers) == 0 {
		return false
	}
	for _, tier := range tiers {
		if !quotaTierExhausted(tier) {
			return false
		}
	}
	return true
}

// deriveGroupQuotaStatus 组级聚合的状态推导。
//
// 组级监控回答的是「这条渠道还能不能打」，不是「池子里有没有空号」。
// 还有健康账号 → 渠道可用（耗尽/未知只在计数摘要里暴露）；
// 全部耗尽 → failed；全部没查到 → error（冷启动超预算不能污染可用率）。
//
// 90% 阈值只在全员健康时看聚合占用率。部分耗尽时 sum(used)/sum(limit)
// 会被空号拉高，拿来再判降级会绕回「还有余额也降级」。
func deriveGroupQuotaStatus(snapshot *domain.MonitorQuotaSnapshot) (status, message string) {
	total := snapshot.AccountsTotal
	healthy := snapshot.AccountsHealthy
	exhausted := snapshot.AccountsExhausted

	switch {
	case healthy == 0 && exhausted > 0:
		return MonitorStatusFailed, fmt.Sprintf("no quota left: %d/%d accounts exhausted", exhausted, total)
	case healthy == 0:
		return MonitorStatusError, firstNonEmpty(
			snapshot.Error,
			fmt.Sprintf("quota unavailable for all %d accounts", total),
		)
	case healthy == total:
		if hint := quotaDegradedHint(snapshot); hint != "" {
			return MonitorStatusDegraded, hint
		}
		return MonitorStatusOperational, ""
	default:
		return MonitorStatusOperational, ""
	}
}
