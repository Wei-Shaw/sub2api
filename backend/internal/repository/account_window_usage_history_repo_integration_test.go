//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// newWindowRepoForTest 构造被测仓储。
func newWindowRepoForTest(t *testing.T) (*accountWindowUsageRepository, *dbent.Client) {
	t.Helper()
	client := testEntClient(t)
	return &accountWindowUsageRepository{client: client, db: integrationDB}, client
}

func mustSeedUsageLog(t *testing.T, client *dbent.Client, accountID int64, at time.Time, in, out, cacheCreate, cacheRead int) {
	t.Helper()
	user := mustCreateUser(t, client, &service.User{Email: "win-" + uuid.NewString() + "@example.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-win-" + uuid.NewString(), Name: "k"})
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	_, err := repo.Create(context.Background(), &service.UsageLog{
		UserID:              user.ID,
		APIKeyID:            apiKey.ID,
		AccountID:           accountID,
		RequestID:           uuid.NewString(),
		Model:               "claude-3",
		InputTokens:         in,
		OutputTokens:        out,
		CacheCreationTokens: cacheCreate,
		CacheReadTokens:     cacheRead,
		TotalCost:           0.1,
		ActualCost:          0.1,
		CreatedAt:           at,
	})
	require.NoError(t, err, "seed usage log")
}

// upsertWithSample 构造带观测时刻的采样行（last_sample_at 单调前移时
// sample_count 才累加——对齐 SQL CASE 语义）。
func upsertWithSample(accountID int64, windowType string, start, end time.Time, peak float64, sampledAt time.Time) *service.AccountWindowUsageRecord {
	return &service.AccountWindowUsageRecord{
		AccountID: accountID, WindowType: windowType, WindowStart: start, WindowEnd: end,
		PeakUsedPercent: peak, LastUsedPercent: peak, SampleCount: 1,
		LastSampleAt: &sampledAt,
	}
}

func TestAccountWindowUsage_UpsertMergeSemantics(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "win-" + uuid.NewString()})

	start := time.Now().Add(-5 * time.Hour).UTC().Truncate(time.Second)
	end := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)

	// 首次插入
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", start, end, 30, start.Add(time.Minute))))
	row, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, 1, row.SampleCount)
	require.InDelta(t, 30.0, row.PeakUsedPercent, 0.001)
	require.NotNil(t, row.LastSampleAt)
	require.Equal(t, start.Add(time.Minute), *row.LastSampleAt)

	// 合并：peak 取 GREATEST、更晚观测累加 sample
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", start, end, 25, start.Add(2*time.Minute))))
	row, err = repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.Equal(t, 2, row.SampleCount)
	require.InDelta(t, 30.0, row.PeakUsedPercent, 0.001, "peak must keep the max")
	require.InDelta(t, 25.0, row.LastUsedPercent, 0.001)

	// window_end 只前移不回退（reset 抖动安全）
	newEnd := end.Add(10 * time.Minute)
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", end, newEnd, 50, start.Add(3*time.Minute))))
	row, err = repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.Equal(t, newEnd, row.WindowEnd)
	require.Equal(t, end, row.WindowStart, "start must move with a forward end")

	// 回退观测（观测时刻更晚）：边界不动，指标照常合并
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", start, end, 60, start.Add(4*time.Minute))))
	row, err = repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.Equal(t, newEnd, row.WindowEnd, "window_end must never regress")
	require.InDelta(t, 60.0, row.PeakUsedPercent, 0.001)
	require.Equal(t, 4, row.SampleCount)
}

// 同一观测并发回放（多副本 / 重启回填重扫同一批历史）：last_sample_at 相同
// → SQL CASE 恰好计数一次。
func TestAccountWindowUsage_ConcurrentReplayCountsOnce(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "win-" + uuid.NewString()})

	start := time.Now().Add(-5 * time.Hour).UTC()
	end := time.Now().Add(time.Hour).UTC()
	sampledAt := time.Now().UTC().Truncate(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", start, end, 30, sampledAt))
		}()
	}
	wg.Wait()

	row, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, 1, row.SampleCount, "replayed observation must count exactly once even under concurrency")
	require.InDelta(t, 30.0, row.PeakUsedPercent, 0.001)
}

func TestAccountWindowUsage_FinalizeAndReplace(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "win-" + uuid.NewString()})

	start := time.Now().Add(-5 * time.Hour).UTC().Truncate(time.Second)
	end := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	sampledAt := start.Add(time.Minute)
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", start, end, 80, sampledAt)))
	open, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.NotNil(t, open)

	stats := &usagestats.WindowTokenStats{Requests: 9, TokensTotal: 12345}
	ok, err := repo.FinalizeWindow(ctx, open.ID, stats, time.Now())
	require.NoError(t, err)
	require.True(t, ok, "first finalize should win")

	// 幂等守卫：重复关闭 no-op
	ok, err = repo.FinalizeWindow(ctx, open.ID, stats, time.Now())
	require.NoError(t, err)
	require.False(t, ok)

	// 关闭后不再是开放行；token 明细已回填
	after, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.Nil(t, after)

	// Replace：旧行（已关闭）+ 新开放行
	newStart := time.Now().UTC().Truncate(time.Second)
	newEnd := newStart.Add(5 * time.Hour)
	require.NoError(t, repo.ReplaceOpenWindow(ctx, open.ID, stats, upsertWithSample(account.ID, "5h", newStart, newEnd, 5, newStart), time.Now()))
	fresh, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.NotNil(t, fresh)
	require.Equal(t, newEnd, fresh.WindowEnd)
	require.Nil(t, fresh.FinalizedAt)
}

func TestAccountWindowUsage_HistoryAndPrune(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "win-" + uuid.NewString()})

	now := time.Now().UTC()
	// 旧窗口（已关闭，window_end/finalized_at 都在 40 天前——生产中 finalize
	// 紧跟窗口结束，保留期语义按 finalized_at 计算）
	oldEnd := now.AddDate(0, 0, -40)
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", oldEnd.Add(-5*time.Hour), oldEnd, 10, oldEnd.Add(-time.Hour))))
	openOld, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	_, err = repo.FinalizeWindow(ctx, openOld.ID, &usagestats.WindowTokenStats{}, oldEnd.Add(time.Minute))
	require.NoError(t, err)

	// 当前开放行
	require.NoError(t, repo.UpsertOpenWindow(ctx, upsertWithSample(account.ID, "5h", now.Add(-5*time.Hour), now.Add(time.Hour), 20, now.Add(-time.Minute))))

	// since=30d：只有当前行；since=60d：两行
	recent, err := repo.ListHistorySince(ctx, account.ID, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	require.Len(t, recent, 1)
	wider, err := repo.ListHistorySince(ctx, account.ID, now.AddDate(0, 0, -60))
	require.NoError(t, err)
	require.Len(t, wider, 2)

	// 保留清理按 finalized_at 计算，且是全局删除：断言用「本账号视角」
	// 验证（ListHistorySince），deleted 只验证方向，避免历史遗留行干扰。
	var deleted int64
	_, err = repo.PruneFinalizedBefore(ctx, now.AddDate(0, 0, -90))
	require.NoError(t, err)
	after90d, err := repo.ListHistorySince(ctx, account.ID, now.AddDate(0, 0, -60))
	require.NoError(t, err)
	require.Len(t, after90d, 2, "90d retention must keep the 40-day-old window")

	deleted, err = repo.PruneFinalizedBefore(ctx, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, int64(1), "30d cutoff must drop the 40-day-old window")
	after30d, err := repo.ListHistorySince(ctx, account.ID, now.AddDate(0, 0, -60))
	require.NoError(t, err)
	require.Len(t, after30d, 1, "only the open row should remain")

	// 开放行不受 finalized 清理影响，但受僵尸清理影响
	staleRows, err := repo.ListExpiredOpenWindows(ctx, now.Add(2*time.Hour), 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(staleRows), 1)
	deleted, err = repo.PruneStaleOpenBefore(ctx, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, int64(1))
	gone, err := repo.GetOpenWindow(ctx, account.ID, "5h")
	require.NoError(t, err)
	require.Nil(t, gone, "stale open row must be pruned")
}

// --- 被动源读取 ---

// mustCreateQuotaMonitor 建号 + 建配额模式监控（绑定账号），返回两者。
func mustCreateQuotaMonitor(t *testing.T, client *dbent.Client, name string) (*service.Account, *service.ChannelMonitor) {
	t.Helper()
	account := mustCreateAccount(t, client, &service.Account{
		Name: name + "-acc", Platform: domain.PlatformKimi, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-kimi", "account_mode": service.AccountModeCoding},
	})
	monitorRepo := NewChannelMonitorRepository(client, integrationDB)
	monitor := &service.ChannelMonitor{
		Name:             name + "-mon",
		Provider:         service.MonitorProviderKimi,
		APIMode:          service.MonitorAPIModeChatCompletions,
		APIKey:           "encrypted",
		PrimaryModel:     "quota",
		Enabled:          true,
		IntervalSeconds:  60,
		CheckMode:        service.MonitorCheckModeQuota,
		AccountID:        &account.ID,
		BodyOverrideMode: service.MonitorBodyOverrideModeOff,
	}
	require.NoError(t, monitorRepo.Create(context.Background(), monitor))
	return account, monitor
}

func TestAccountWindowUsage_ListMonitorQuotaHistorySince(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)
	account, monitor := mustCreateQuotaMonitor(t, client, "win-mq-"+uuid.NewString()[:8])

	base := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	mustInsertHistory := func(checkedAt time.Time, snapshot *domain.MonitorQuotaSnapshot) {
		t.Helper()
		require.NoError(t, NewChannelMonitorRepository(client, integrationDB).InsertHistoryBatch(ctx, []*service.ChannelMonitorHistoryRow{
			{MonitorID: monitor.ID, Model: "quota", Status: service.MonitorStatusOperational, Message: "ok", CheckedAt: checkedAt, Quota: snapshot},
		}))
	}

	// 水位之前的行（应被过滤）
	mustInsertHistory(base.Add(-time.Minute), &domain.MonitorQuotaSnapshot{
		Success: true, FetchedAt: base.Add(-time.Minute),
		Tiers: []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 5, ResetAt: base.Add(time.Hour).Format(time.RFC3339)}},
	})
	// 水位之后的行：FetchedAt 缺失（旧数据）→ 回退 checked_at
	mustInsertHistory(base.Add(time.Minute), &domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers:   []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 15, ResetAt: base.Add(time.Hour).Format(time.RFC3339)}},
	})
	// 探活行（quota NULL）→ 跳过
	require.NoError(t, NewChannelMonitorRepository(client, integrationDB).InsertHistoryBatch(ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: monitor.ID, Model: "quota", Status: service.MonitorStatusOperational, Message: "ok", CheckedAt: base.Add(2 * time.Minute)},
	}))
	// 结构损坏的 quota JSON（tiers 类型错误）→ 单行跳过不阻断整批
	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO channel_monitor_histories (monitor_id, model, status, message, checked_at, quota)
		 VALUES ($1, 'quota', 'operational', 'ok', $2, '{"success":true,"tiers":5}'::jsonb)`,
		monitor.ID, base.Add(3*time.Minute))
	require.NoError(t, err)
	// 正常行
	latest := base.Add(4 * time.Minute)
	mustInsertHistory(latest, &domain.MonitorQuotaSnapshot{
		Success: true, FetchedAt: latest,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "5h", UsedPercent: 25, ResetAt: base.Add(time.Hour).Format(time.RFC3339)},
			{Window: "7d", UsedPercent: 60, ResetAt: base.Add(48 * time.Hour).Format(time.RFC3339)},
		},
	})

	observations, err := repo.ListMonitorQuotaHistorySince(ctx, base, 100)
	require.NoError(t, err)

	// 水位前行 + NULL 行 + 损坏行都被排除
	require.Len(t, observations, 2, "only post-watermark healthy quota rows should be read back")
	// 按 checked_at 升序：先是 FetchedAt 缺失的行
	first := observations[0]
	require.Equal(t, account.ID, first.AccountID, "observations must join back to the monitored account")
	require.Equal(t, base.Add(time.Minute), first.Snapshot.FetchedAt, "missing fetched_at must fall back to checked_at")
	// 最新行：FetchedAt 原样保留，两个 tier 都在
	last := observations[1]
	require.Equal(t, latest, last.Snapshot.FetchedAt)
	require.Len(t, last.Snapshot.Tiers, 2)

	// 软删账号后不再产出观测
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at = NOW() WHERE id = $1`, account.ID)
	require.NoError(t, err)
	after, err := repo.ListMonitorQuotaHistorySince(ctx, base, 100)
	require.NoError(t, err)
	require.Empty(t, after, "soft-deleted accounts must be excluded")

	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at = NULL WHERE id = $1`, account.ID)
	require.NoError(t, err)
}

func TestAccountWindowUsage_ListCodexUsageUpdatesSince(t *testing.T) {
	ctx := context.Background()
	repo, client := newWindowRepoForTest(t)

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	mustCreateCodexAccount := func(name string, updatedAt time.Time, extra map[string]any) *service.Account {
		t.Helper()
		return mustCreateAccount(t, client, &service.Account{
			Name: name, Platform: domain.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-" + uuid.NewString()},
			Extra:       extra,
		})
	}
	codexExtra := func(updatedAt time.Time, withReset bool) map[string]any {
		extra := map[string]any{
			"codex_usage_updated_at": updatedAt.Format(time.RFC3339Nano),
			"codex_5h_used_percent":  42.5,
		}
		if withReset {
			extra["codex_5h_reset_at"] = updatedAt.Add(3 * time.Hour).Format(time.RFC3339Nano)
		}
		return extra
	}

	// 水位之前更新的账号（应被过滤）
	mustCreateCodexAccount("win-codex-old", base.Add(-time.Minute), codexExtra(base.Add(-time.Minute), true))
	// 水位之后：完整快照
	fresh := mustCreateCodexAccount("win-codex-fresh", base.Add(time.Minute), codexExtra(base.Add(time.Minute), true))
	// 缺 reset_at 的 tier 被丢弃 → 整条观测跳过
	mustCreateCodexAccount("win-codex-noreset", base.Add(2*time.Minute), codexExtra(base.Add(2*time.Minute), false))
	// 非 openai 平台即使带同名字段也不读
	mustCreateAccount(t, client, &service.Account{
		Name: "win-codex-anthropic", Platform: domain.PlatformAnthropic, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{}, Extra: codexExtra(base.Add(3*time.Minute), true),
	})

	observations, err := repo.ListCodexUsageUpdatesSince(ctx, base, 100)
	require.NoError(t, err)

	require.Len(t, observations, 1, "only fresh openai accounts with usable tiers should be read back")
	obs := observations[0]
	require.Equal(t, fresh.ID, obs.AccountID)
	require.Equal(t, base.Add(time.Minute), obs.Snapshot.FetchedAt)
	require.True(t, obs.Snapshot.Success)
	require.Len(t, obs.Snapshot.Tiers, 1)
	require.Equal(t, "5h", obs.Snapshot.Tiers[0].Window)
	require.InDelta(t, 42.5, obs.Snapshot.Tiers[0].UsedPercent, 0.001)

	// 软删后不再产出观测
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at = NOW() WHERE id = $1`, fresh.ID)
	require.NoError(t, err)
	after, err := repo.ListCodexUsageUpdatesSince(ctx, base, 100)
	require.NoError(t, err)
	require.Empty(t, after)
}

func TestGetAccountWindowStatsRange_AggregatesOnlyWithinBounds(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "win-" + uuid.NewString()})
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	// 用固定的过去日期（同 TestGetUserStats 先例）：近 now 的播种会污染同套件
	// dashboard 断言（今日活跃用户/近期小时桶的绝对计数）
	windowStart := time.Date(2025, 2, 20, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(5 * time.Hour)

	// 窗口内 3 条
	mustSeedUsageLog(t, client, account.ID, windowStart.Add(10*time.Minute), 100, 50, 10, 5)
	mustSeedUsageLog(t, client, account.ID, windowStart.Add(1*time.Hour), 200, 100, 20, 10)
	mustSeedUsageLog(t, client, account.ID, windowEnd.Add(-time.Second), 1, 1, 1, 1)
	// 边界精确钉死半开语义：恰在 start（计入，>=）与恰在 end（不计入，<）
	mustSeedUsageLog(t, client, account.ID, windowStart, 2, 1, 0, 0)
	mustSeedUsageLog(t, client, account.ID, windowEnd, 4, 2, 0, 0)
	// 窗口外 2 条（起点前 + 终点后）
	mustSeedUsageLog(t, client, account.ID, windowStart.Add(-time.Minute), 999, 999, 999, 999)
	mustSeedUsageLog(t, client, account.ID, windowEnd.Add(time.Minute), 999, 999, 999, 999)

	stats, err := repo.GetAccountWindowStatsRange(ctx, account.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, int64(4), stats.Requests)
	// 165 + 330 + 4 + 3 = 502；恰在 end 的行不计入（半开区间），窗口外两条亦然
	require.Equal(t, int64(502), stats.TokensTotal)
	require.Equal(t, int64(303), stats.TokensInput)        // 100+200+1+2
	require.Equal(t, int64(152), stats.TokensOutput)       // 50+100+1+1
	require.Equal(t, int64(31), stats.TokensCacheCreation) // 10+20+1+0
	require.Equal(t, int64(16), stats.TokensCacheRead)     // 5+10+1+0
}
