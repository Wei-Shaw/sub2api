package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AccountWindowUsageIngester 账号滚动窗口用量采集器（纯被动）。
//
// 不做任何主动探测：只按水位回放两条既有观测流，并维护开放行状态机：
//   - 被动源①：渠道监控明细历史（channel_monitor_histories 持久化的按账号
//     配额快照，quota/quota_probe 模式）——覆盖配置了监控的 Anthropic /
//     国产 coding plan 账号
//   - 被动源②：OpenAI/Codex 真实流量响应头归一化落库的 accounts.extra
//     快照（网关侧按账号 30s 节流写入）——覆盖全部有流量的 openai 账号
//   - finalize：window_end 过后 finalizeGrace 仍未被新观测推进的开放行，
//     用 usage_logs 在 [window_start, window_end) 内聚合回填 token 明细并关闭
//
// 调度形态：单 goroutine 循环（ingesterTickInterval），每轮读取两个被动源
// 的新增观测（限量）、逐条 ApplySnapshot、再做 finalize 扫描。全部操作是
// DB 读写，不触碰上游；服务重启后从 DB 行与水位回放窗口恢复。
//
// 水位为进程内状态，初始值为 now - ingesterBackfillWindow：重启（或多副本）
// 会重扫回填窗口内的观测并重新应用——upsert 按 last_sample_at 去重，同一
// 观测重复回放恰好计数一次，幂等无害。渠道监控明细保留 30 天，回填窗口
// 取 7 天：首次部署即可重建近一周的窗口历史，同时避免重启时全量重放。
//
// 多副本部署下每个副本都会运行本采集器：观测回放的原子 upsert 与 finalize
// 的 finalized_at IS NULL 守卫使重复写入幂等，最坏情况是重复的 DB 读。
type AccountWindowUsageIngester struct {
	windowRepo   AccountWindowUsageRepository
	usageLogRepo UsageLogRepository

	parentCtx    context.Context
	parentCancel context.CancelFunc

	wg      sync.WaitGroup
	started bool
	stopped bool
	mu      sync.Mutex

	// monitorWM/codexWM 两个被动源各自已回放的观测时刻水位（进程内）
	monitorWM time.Time
	codexWM   time.Time
	wmMu      sync.Mutex
}

// 采集器节奏与守卫常量。
const (
	// ingesterTickInterval 循环粒度：驱动被动源水位读取与 finalize 扫描
	ingesterTickInterval = 15 * time.Second
	// ingesterFinalizeGrace window_end 过后的收敛等待（容忍迟到 usage_logs 写入）
	ingesterFinalizeGrace = 5 * time.Minute
	// ingesterSweepLimit 单轮被动源读取/finalize 扫描的行数上限
	ingesterSweepLimit = 500
	// ingesterBackfillWindow 水位初始回看窗口：重启/首启重放的观测范围
	ingesterBackfillWindow = 7 * 24 * time.Hour
	// ingesterResetEpsilon 两次观测的 reset_at 视为同一窗口的容差
	// （供应商时间戳存在秒级抖动）
	ingesterResetEpsilon = 2 * time.Second
	// windowHistoryRetentionDays 已关闭窗口历史的保留天数
	windowHistoryRetentionDays = 90
	// windowStaleOpenRetentionDays 僵尸开放行的保留天数（账号软删/数据源消失兜底）
	windowStaleOpenRetentionDays = 14
)

// NewAccountWindowUsageIngester 构造采集器。
func NewAccountWindowUsageIngester(
	windowRepo AccountWindowUsageRepository,
	usageLogRepo UsageLogRepository,
) *AccountWindowUsageIngester {
	ctx, cancel := context.WithCancel(context.Background())
	backfillFrom := time.Now().Add(-ingesterBackfillWindow)
	return &AccountWindowUsageIngester{
		windowRepo:   windowRepo,
		usageLogRepo: usageLogRepo,
		parentCtx:    ctx,
		parentCancel: cancel,
		monitorWM:    backfillFrom,
		codexWM:      backfillFrom,
	}
}

// Start 启动采集器循环。调用方需保证只调一次（wire provider 内调用）。
func (g *AccountWindowUsageIngester) Start() {
	if g == nil || g.windowRepo == nil || g.usageLogRepo == nil {
		return
	}
	g.mu.Lock()
	if g.started || g.stopped {
		g.mu.Unlock()
		return
	}
	g.started = true
	g.mu.Unlock()

	g.wg.Add(1)
	go g.runLoop()
	slog.Info("account_window_usage: ingester started",
		"backfill_window", ingesterBackfillWindow.String())
}

// Stop 优雅停止：取消循环并等待在飞任务结束。
func (g *AccountWindowUsageIngester) Stop() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	g.stopped = true
	g.parentCancel()
	g.mu.Unlock()

	g.wg.Wait()
}

// RunDailyMaintenance 每日维护：保留期清理（OpsCleanupService cron 驱动，
// 复用 leader lock）。
func (g *AccountWindowUsageIngester) RunDailyMaintenance(ctx context.Context) {
	if g == nil || g.windowRepo == nil {
		return
	}

	finalizedCutoff := time.Now().AddDate(0, 0, -windowHistoryRetentionDays)
	if deleted, err := g.windowRepo.PruneFinalizedBefore(ctx, finalizedCutoff); err != nil {
		slog.Warn("account_window_usage: prune finalized failed", "error", err)
	} else if deleted > 0 {
		slog.Info("account_window_usage: pruned finalized rows", "deleted", deleted)
	}

	staleCutoff := time.Now().AddDate(0, 0, -windowStaleOpenRetentionDays)
	if deleted, err := g.windowRepo.PruneStaleOpenBefore(ctx, staleCutoff); err != nil {
		slog.Warn("account_window_usage: prune stale open rows failed", "error", err)
	} else if deleted > 0 {
		slog.Info("account_window_usage: pruned stale open rows", "deleted", deleted)
	}
}

// runLoop 主循环：每 tick 回放被动源新增观测，再做 finalize 扫描。
func (g *AccountWindowUsageIngester) runLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(ingesterTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-g.parentCtx.Done():
			return
		case <-ticker.C:
			g.runOnce(g.parentCtx)
		}
	}
}

// runOnce 单轮调度。errors 只记日志：单轮失败不影响下一轮。
func (g *AccountWindowUsageIngester) runOnce(ctx context.Context) {
	// ticker goroutine 的 panic 兜底，否则一次 panic 会静默杀死该进程余生的采集循环
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("account_window_usage: scheduler tick panic", "panic", rec)
		}
	}()

	// 单轮整体封顶一个 tick：被动源积压（重启回填）时按限量跨轮排空，
	// 不阻塞下一轮的 finalize 调度
	fctx, cancel := context.WithTimeout(ctx, ingesterTickInterval)
	defer cancel()

	now := time.Now()
	g.ingestMonitorHistory(fctx)
	g.ingestCodexUsageUpdates(fctx)
	g.finalizeExpired(fctx, now)
}

// ingestMonitorHistory 回放被动源①：渠道监控明细历史的新增快照。
func (g *AccountWindowUsageIngester) ingestMonitorHistory(ctx context.Context) {
	observations, err := g.windowRepo.ListMonitorQuotaHistorySince(ctx, g.currentWM(&g.monitorWM), ingesterSweepLimit)
	if err != nil {
		slog.Warn("account_window_usage: list monitor quota history failed", "error", err)
		return
	}
	if len(observations) == 0 {
		return
	}

	applied := 0
	for _, obs := range observations {
		if err := g.ApplyObservation(ctx, obs); err != nil {
			slog.Warn("account_window_usage: apply monitor observation failed",
				"account_id", obs.AccountID, "error", err)
			continue
		}
		applied++
	}
	g.advanceWM(&g.monitorWM, observations[len(observations)-1].Snapshot.FetchedAt)
	if n := len(observations); n == ingesterSweepLimit {
		slog.Info("account_window_usage: monitor history backlog draining", "rows", n, "applied", applied)
	}
}

// ingestCodexUsageUpdates 回放被动源②：openai 账号 extra 快照的新增更新。
func (g *AccountWindowUsageIngester) ingestCodexUsageUpdates(ctx context.Context) {
	observations, err := g.windowRepo.ListCodexUsageUpdatesSince(ctx, g.currentWM(&g.codexWM), ingesterSweepLimit)
	if err != nil {
		slog.Warn("account_window_usage: list codex usage updates failed", "error", err)
		return
	}
	if len(observations) == 0 {
		return
	}

	for _, obs := range observations {
		if err := g.ApplyObservation(ctx, obs); err != nil {
			slog.Warn("account_window_usage: apply codex observation failed",
				"account_id", obs.AccountID, "error", err)
		}
	}
	g.advanceWM(&g.codexWM, observations[len(observations)-1].Snapshot.FetchedAt)
}

// ApplyObservation 把一次按账号的配额观测交给状态机（独立导出便于单元测试）。
func (g *AccountWindowUsageIngester) ApplyObservation(ctx context.Context, obs *AccountQuotaObservation) error {
	if obs == nil {
		return nil
	}
	return g.ApplySnapshot(ctx, obs.AccountID, obs.Snapshot)
}

// ApplySnapshot 把一次配额快照的各窗口 tier 合并进开放行（状态机核心）。
//
// 单个 tier 的迁移：
//
//	无开放行                → 插入（start = reset - duration）
//	|reset - windowEnd| ≤ ε → 同窗口：peak=max(peak, used%)、last=used%、计数+1
//	reset 前移 && 旧 end>now → 滚动窗口滑动：更新指标 + 重算 start/end
//	旧 windowEnd ≤ now      → 旧窗口关闭（回填 token）+ 插入新窗口
//	reset 后移（上游抖动）   → 只更新指标，绝不回退 window_end
//
// 同一观测重复回放（多副本/重启回填）由 last_sample_at 去重：观测时刻不晚于
// 行内已见时刻时直接跳过。
func (g *AccountWindowUsageIngester) ApplySnapshot(ctx context.Context, accountID int64, snapshot *domain.MonitorQuotaSnapshot) error {
	if snapshot == nil || !snapshot.Success {
		return nil
	}
	now := time.Now()

	// 单个 tier 失败不阻断其余窗口：记录首个错误，处理完所有 tier 后返回
	var firstErr error
	for _, tier := range snapshot.Tiers {
		if err := g.applyTier(ctx, accountID, tier, snapshot.FetchedAt, now); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// applyTier 处理单个窗口 tier 的状态迁移。
func (g *AccountWindowUsageIngester) applyTier(ctx context.Context, accountID int64, tier domain.MonitorQuotaTier, fetchedAt, now time.Time) error {
	if !recordedWindow(tier.Window) || tier.ResetAt == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, tier.ResetAt)
	if err != nil {
		return nil // 未知格式的时间戳跳过，不阻断其他 tier
	}
	windowType := tier.Window
	duration := windowTypeDuration[windowType]

	open, err := g.windowRepo.GetOpenWindow(ctx, accountID, windowType)
	if err != nil {
		return err
	}

	// 重复观测去重：观测时刻不晚于行内已见时刻（同刻或乱序到达）→ 跳过。
	// 这与 upsert 里的 CASE 条件双保险：进程内先挡掉绝大多数重复，
	// SQL 层兜底并发竞态（两个副本同时回放同一观测时恰好计数一次）。
	if open != nil && open.LastSampleAt != nil && !fetchedAt.After(*open.LastSampleAt) {
		return nil
	}

	// 陈旧快照守卫：其他副本（或本副本的回填重放）可能仍持有上一窗口实例
	// 的观测。这类快照若落入「reset 后移」分支，上一窗口的峰值会经 GREATEST
	// 永久写进新窗口（peak 单调无自愈路径）；若落入「旧窗口过期」分支则会按
	// 旧边界重开重复行。抓取时间早于当前开放行的窗口起点 → 属于上一窗口
	// 实例，丢弃（容时钟秒级偏差）。
	if open != nil && fetchedAt.Before(open.WindowStart.Add(-ingesterResetEpsilon)) {
		return nil
	}

	// 构造本次采样的指标增量（sample_count 语义：插入时为 1，合并时累加 1）
	buildRow := func(start, end time.Time) *AccountWindowUsageRecord {
		sampledAt := fetchedAt
		return &AccountWindowUsageRecord{
			AccountID:       accountID,
			WindowType:      windowType,
			WindowStart:     start,
			WindowEnd:       end,
			PeakUsedPercent: tier.UsedPercent,
			LastUsedPercent: tier.UsedPercent,
			SampleCount:     1,
			LastSampleAt:    &sampledAt,
		}
	}

	// 分支 1：无开放行 → 直接插入。reset_at 已过的快照是陈旧数据（或旧行
	// 已被并发 finalize）：此时新开的行 window_end 在过去，finalize 扫描会
	// 再关一次，产生同一窗口的重复历史行——仅容忍秒级时钟偏差
	if open == nil {
		if !resetAt.After(now.Add(-ingesterResetEpsilon)) {
			return nil
		}
		return g.windowRepo.UpsertOpenWindow(ctx, buildRow(resetAt.Add(-duration), resetAt))
	}

	sameWindow := resetAt.Sub(open.WindowEnd) <= ingesterResetEpsilon &&
		resetAt.Sub(open.WindowEnd) >= -ingesterResetEpsilon
	windowExpired := !open.WindowEnd.After(now)

	switch {
	// 分支 2：同一窗口（reset 抖动在容差内）→ 合并指标
	case sameWindow:
		return g.windowRepo.UpsertOpenWindow(ctx, buildRow(open.WindowStart, open.WindowEnd))

	// 分支 3：旧窗口已过期 → 关闭旧行（回填 token）+ 写入新窗口行
	case windowExpired:
		stats, err := g.usageLogRepo.GetAccountWindowStatsRange(ctx, accountID, open.WindowStart, open.WindowEnd)
		if err != nil {
			return err
		}
		return g.windowRepo.ReplaceOpenWindow(ctx, open.ID, stats, buildRow(resetAt.Add(-duration), resetAt), now)

	// 分支 4：reset 前移且旧 end 仍在未来 → 滚动窗口滑动，整体前移
	case resetAt.After(open.WindowEnd):
		return g.windowRepo.UpsertOpenWindow(ctx, buildRow(resetAt.Add(-duration), resetAt))

	// 分支 5：reset 后移（上游抖动）→ 只更新指标，保留原窗口边界
	default:
		return g.windowRepo.UpsertOpenWindow(ctx, buildRow(open.WindowStart, open.WindowEnd))
	}
}

// finalizeExpired 关闭已过期（window_end + grace 已过）的开放行并回填 token 明细。
func (g *AccountWindowUsageIngester) finalizeExpired(ctx context.Context, now time.Time) {
	cutoff := now.Add(-ingesterFinalizeGrace)
	rows, err := g.windowRepo.ListExpiredOpenWindows(ctx, cutoff, ingesterSweepLimit)
	if err != nil {
		slog.Warn("account_window_usage: list expired open windows failed", "error", err)
		return
	}
	for _, rec := range rows {
		stats, err := g.usageLogRepo.GetAccountWindowStatsRange(ctx, rec.AccountID, rec.WindowStart, rec.WindowEnd)
		if err != nil {
			slog.Warn("account_window_usage: aggregate window usage failed",
				"account_id", rec.AccountID, "window_type", rec.WindowType, "error", err)
			continue
		}
		if _, err := g.windowRepo.FinalizeWindow(ctx, rec.ID, stats, now); err != nil {
			slog.Warn("account_window_usage: finalize window failed",
				"account_id", rec.AccountID, "window_type", rec.WindowType, "error", err)
		}
	}
}

// currentWM 读取指定水位（调用方持锁语义由 wmMu 保证）。
func (g *AccountWindowUsageIngester) currentWM(wm *time.Time) time.Time {
	g.wmMu.Lock()
	defer g.wmMu.Unlock()
	return *wm
}

// advanceWM 前移水位（只进不退：读取失败/空结果不动）。
func (g *AccountWindowUsageIngester) advanceWM(wm *time.Time, to time.Time) {
	if to.IsZero() {
		return
	}
	g.wmMu.Lock()
	defer g.wmMu.Unlock()
	if to.After(*wm) {
		*wm = to
	}
}
