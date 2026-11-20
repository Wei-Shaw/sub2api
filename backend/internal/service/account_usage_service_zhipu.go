package service

import (
	"context"
	"fmt"
	"time"
)

// GLM Coding Plan 账号的 usage 查询：接入统一 usage 管线（5h + weekly 两窗口）。
//
// 数据链路与 OpenAI codex 对齐：zhipu_* 快照由 CNProviderQuotaService 探测并
// 持久化（探测端点为智谱官方 monitor 接口，个人版/团队版均支持）；usage 查询
// 先读快照，按需（快照缺失或过期）经 TTL 节流后触发一次上游探测，最后叠加
// 本地窗口统计（usage_logs 按窗口起点聚合的请求数 / tokens / 费用）。
//
// 探测失败不阻塞快照展示：与 OpenAI 一致，降级返回已有快照；只有完全没有
// 数据时才把错误透出（常见于自定义中转域名或凭据失效的账号）。

const (
	// zhipuCodingProbeCacheTTL 限制同一账号对智谱 monitor 端点的最小探测间隔，
	// 防止账号列表挂载/批量拉取放大成探测风暴（对齐 OpenAI codex 探针节流）。
	zhipuCodingProbeCacheTTL = 10 * time.Minute
	// zhipuCodingSnapshotStaleAfter 判定 zhipu_* 快照过期的时长，超过即允许
	// 在下一个 TTL 窗口内重新探测（与 CN 用量格的快照过期阈值同量级）。
	zhipuCodingSnapshotStaleAfter = 15 * time.Minute
)

// getZhipuUsage GLM Coding Plan 账号的 usage：5h + weekly（映射 SevenDay）两窗口。
func (s *AccountUsageService) getZhipuUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	now := time.Now()
	usage := &UsageInfo{UpdatedAt: &now}

	if account == nil {
		return usage, nil
	}

	applyZhipuExtraToUsage(usage, account.Extra, now)

	if force || zhipuSnapshotNeedsRefresh(account, usage, now) {
		if s.shouldProbeZhipuSnapshot(account.ID, now, force) {
			s.probeZhipuSnapshot(ctx, account, usage, now)
		}
	}

	if s.usageLogRepo == nil {
		return usage, nil
	}

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, codexWindowStatsStart(usage.FiveHour, 5*time.Hour, now)); err == nil {
		if usage.FiveHour == nil {
			usage.FiveHour = &UsageProgress{Utilization: 0}
		}
		usage.FiveHour.WindowStats = windowStatsFromAccountStats(stats)
	}

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, codexWindowStatsStart(usage.SevenDay, 7*24*time.Hour, now)); err == nil {
		if usage.SevenDay == nil {
			usage.SevenDay = &UsageProgress{Utilization: 0}
		}
		usage.SevenDay.WindowStats = windowStatsFromAccountStats(stats)
	}

	return usage, nil
}

// probeZhipuSnapshot 触发一次 CN 配额探测并合并结果到内存副本。
// CNProviderQuotaService 已把快照持久化到 zhipu_* Extra 键，这里按同一格式
// 合并进账号的内存 Extra，使返回值与落库状态一致。
func (s *AccountUsageService) probeZhipuSnapshot(ctx context.Context, account *Account, usage *UsageInfo, now time.Time) {
	if s.cnProviderQuotaService == nil {
		return
	}

	result, err := s.cnProviderQuotaService.QueryUsageForAccount(ctx, account)
	switch {
	case err != nil:
		if usage.FiveHour == nil && usage.SevenDay == nil {
			usage.Error = err.Error()
		}
		return
	case result == nil || !result.Success:
		if usage.FiveHour == nil && usage.SevenDay == nil {
			if result != nil && result.Error != "" {
				usage.Error = result.Error
			} else {
				usage.Error = "zhipu quota probe failed"
			}
		}
		return
	}

	if updates := cnQuotaExtraUpdates(PlatformZhipu, result.Tiers, time.Unix(result.FetchedAt, 0)); len(updates) > 0 {
		mergeAccountExtra(account, updates)
		applyZhipuExtraToUsage(usage, account.Extra, now)
	}
}

// shouldProbeZhipuSnapshot CN 配额探测的 TTL 节流；force（手动查询）跳过节流，
// 挂载/批量等自动路径受 TTL 保护（与 OpenAI codex 探针节流语义一致）。
func (s *AccountUsageService) shouldProbeZhipuSnapshot(accountID int64, now time.Time, force ...bool) bool {
	if s == nil || s.cache == nil || accountID <= 0 {
		return true
	}
	forceProbe := len(force) > 0 && force[0]
	if !forceProbe {
		if cached, ok := s.cache.cnQuotaProbeCache.Load(accountID); ok {
			if ts, ok := cached.(time.Time); ok && now.Sub(ts) < zhipuCodingProbeCacheTTL {
				return false
			}
		}
	}
	s.cache.cnQuotaProbeCache.Store(accountID, now)
	return true
}

// zhipuSnapshotNeedsRefresh 判定是否需要重新探测：窗口缺失（从未探测过）或
// 快照时间戳超过 zhipuCodingSnapshotStaleAfter。
func zhipuSnapshotNeedsRefresh(account *Account, usage *UsageInfo, now time.Time) bool {
	if account == nil {
		return false
	}
	if usage == nil || usage.FiveHour == nil || usage.SevenDay == nil {
		return true
	}
	if account.Extra == nil {
		return true
	}
	raw, ok := account.Extra[cnExtraKey(PlatformZhipu, cnExtraSuffixUsageUpdated)]
	if !ok {
		return true
	}
	ts, err := parseTime(fmt.Sprint(raw))
	if err != nil {
		return true
	}
	return now.Sub(ts) >= zhipuCodingSnapshotStaleAfter
}

// applyZhipuExtraToUsage 从 zhipu_* 快照构建 5h / weekly（映射 SevenDay）窗口。
func applyZhipuExtraToUsage(usage *UsageInfo, extra map[string]any, now time.Time) {
	if usage == nil {
		return
	}
	usage.FiveHour = buildZhipuUsageProgressFromExtra(extra, "5h", now)
	usage.SevenDay = buildZhipuUsageProgressFromExtra(extra, "weekly", now)
}

// buildZhipuUsageProgressFromExtra 从 zhipu_* Extra 快照构建指定窗口的用量进度。
// 窗口已过期（reset_at 早于 now）时使用率归零，与 codex 快照的语义一致。
func buildZhipuUsageProgressFromExtra(extra map[string]any, window string, now time.Time) *UsageProgress {
	if len(extra) == 0 {
		return nil
	}

	var usedPercentKey, resetAtKey string
	switch window {
	case "5h":
		usedPercentKey = cnExtraKey(PlatformZhipu, cnExtraSuffix5hUsed)
		resetAtKey = cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset)
	case "weekly":
		usedPercentKey = cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyUsed)
		resetAtKey = cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset)
	default:
		return nil
	}

	usedRaw, ok := extra[usedPercentKey]
	if !ok {
		return nil
	}

	progress := &UsageProgress{Utilization: parseExtraFloat64(usedRaw)}
	if resetAtRaw, ok := extra[resetAtKey]; ok {
		if resetAt, err := parseTime(fmt.Sprint(resetAtRaw)); err == nil {
			progress.ResetsAt = &resetAt
			progress.RemainingSeconds = int(time.Until(resetAt).Seconds())
			if progress.RemainingSeconds < 0 {
				progress.RemainingSeconds = 0
			}
		}
	}

	if progress.ResetsAt != nil && !now.Before(*progress.ResetsAt) {
		progress.Utilization = 0
	}

	return progress
}
