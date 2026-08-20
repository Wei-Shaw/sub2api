//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// --- 采集器依赖 stub ---

// windowKey 开放行的内存索引键。
func windowKey(accountID int64, windowType string) string {
	return fmt.Sprintf("%d|%s", accountID, windowType)
}

// stubWindowUsageRepo 内存版仓储，模拟 SQL 层的合并/守卫语义
// （GREATEST、last_sample_at 条件计数、finalized_at IS NULL 守卫）。
type stubWindowUsageRepo struct {
	AccountWindowUsageRepository

	mu        sync.Mutex
	open      map[string]*AccountWindowUsageRecord
	finalized []*AccountWindowUsageRecord
	nextID    int64

	// 被动源回放桩数据（ingest 测试用）与调用观测
	monitorObs   []*AccountQuotaObservation
	monitorSince time.Time
	monitorErr   error
	codexObs     []*AccountQuotaObservation
	codexSince   time.Time
	codexErr     error

	upsertCalls        int
	replaceCalls       int
	finalizeCalls      int
	lastUpsertRow      *AccountWindowUsageRecord
	lastReplaceStats   *usagestats.WindowTokenStats
	lastReplaceOldRow  int64
	pruneFinalizedAt   time.Time
	pruneStaleOpenFrom time.Time
}

func newStubWindowUsageRepo() *stubWindowUsageRepo {
	return &stubWindowUsageRepo{open: make(map[string]*AccountWindowUsageRecord)}
}

func (r *stubWindowUsageRepo) GetOpenWindow(_ context.Context, accountID int64, windowType string) (*AccountWindowUsageRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.open[windowKey(accountID, windowType)]; ok {
		cp := *row
		return &cp, nil
	}
	return nil, nil
}

// UpsertOpenWindow 复刻 SQL 合并语义：peak 取 GREATEST、last 覆盖、
// sample_count 仅在观测时刻晚于行内已见时刻时累加（同观测重复回放计数一次）、
// window_end 只前移不回退。
func (r *stubWindowUsageRepo) UpsertOpenWindow(_ context.Context, row *AccountWindowUsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertCalls++
	cp := *row
	r.lastUpsertRow = &cp

	key := windowKey(row.AccountID, row.WindowType)
	if existing, ok := r.open[key]; !ok {
		r.nextID++
		cp.ID = r.nextID
		r.open[key] = &cp
	} else {
		if row.PeakUsedPercent > existing.PeakUsedPercent {
			existing.PeakUsedPercent = row.PeakUsedPercent
		}
		existing.LastUsedPercent = row.LastUsedPercent
		// SQL CASE：观测时刻不晚于行内已见时刻 → 不累加（重复回放幂等）
		if row.LastSampleAt != nil &&
			(existing.LastSampleAt == nil || row.LastSampleAt.After(*existing.LastSampleAt)) {
			existing.SampleCount += row.SampleCount
			existing.LastSampleAt = row.LastSampleAt
		}
		if row.WindowEnd.After(existing.WindowEnd) {
			existing.WindowStart = row.WindowStart
			existing.WindowEnd = row.WindowEnd
		}
	}
	return nil
}

func (r *stubWindowUsageRepo) FinalizeWindow(_ context.Context, id int64, stats *usagestats.WindowTokenStats, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeCalls++
	for key, row := range r.open {
		if row.ID != id {
			continue
		}
		delete(r.open, key)
		applyFinalize(row, stats, now)
		r.finalized = append(r.finalized, row)
		return true, nil
	}
	return false, nil // 幂等守卫：已关闭/不存在 no-op
}

func (r *stubWindowUsageRepo) ReplaceOpenWindow(ctx context.Context, oldID int64, stats *usagestats.WindowTokenStats, newRow *AccountWindowUsageRecord, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replaceCalls++
	r.lastReplaceStats = stats
	r.lastReplaceOldRow = oldID

	for key, row := range r.open {
		if row.ID == oldID {
			delete(r.open, key)
			applyFinalize(row, stats, now)
			r.finalized = append(r.finalized, row)
			break
		}
	}
	// 复用 upsert 合并逻辑写入新行（去掉锁重入，手动内联）
	key := windowKey(newRow.AccountID, newRow.WindowType)
	if existing, ok := r.open[key]; !ok {
		cp := *newRow
		r.nextID++
		cp.ID = r.nextID
		r.open[key] = &cp
	} else {
		if newRow.PeakUsedPercent > existing.PeakUsedPercent {
			existing.PeakUsedPercent = newRow.PeakUsedPercent
		}
		existing.LastUsedPercent = newRow.LastUsedPercent
		if newRow.LastSampleAt != nil &&
			(existing.LastSampleAt == nil || newRow.LastSampleAt.After(*existing.LastSampleAt)) {
			existing.SampleCount += newRow.SampleCount
			existing.LastSampleAt = newRow.LastSampleAt
		}
		if newRow.WindowEnd.After(existing.WindowEnd) {
			existing.WindowStart = newRow.WindowStart
			existing.WindowEnd = newRow.WindowEnd
		}
	}
	return nil
}

func applyFinalize(row *AccountWindowUsageRecord, stats *usagestats.WindowTokenStats, now time.Time) {
	if stats == nil {
		stats = &usagestats.WindowTokenStats{}
	}
	row.Requests = &stats.Requests
	row.TokensTotal = &stats.TokensTotal
	row.TokensInput = &stats.TokensInput
	row.TokensOutput = &stats.TokensOutput
	row.TokensCacheCreation = &stats.TokensCacheCreation
	row.TokensCacheRead = &stats.TokensCacheRead
	row.FinalizedAt = &now
}

func (r *stubWindowUsageRepo) ListExpiredOpenWindows(_ context.Context, cutoff time.Time, _ int) ([]*AccountWindowUsageRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := make([]*AccountWindowUsageRecord, 0)
	for _, row := range r.open {
		if row.WindowEnd.Before(cutoff) {
			cp := *row
			rows = append(rows, &cp)
		}
	}
	return rows, nil
}

func (r *stubWindowUsageRepo) ListHistorySince(_ context.Context, accountID int64, since time.Time) ([]*AccountWindowUsageRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	records := make([]*AccountWindowUsageRecord, 0)
	for _, row := range r.open {
		if row.AccountID == accountID && !row.WindowEnd.Before(since) {
			cp := *row
			records = append(records, &cp)
		}
	}
	records = append(records, r.finalized...)
	return records, nil
}

func (r *stubWindowUsageRepo) PruneFinalizedBefore(_ context.Context, cutoff time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneFinalizedAt = cutoff
	return 0, nil
}

func (r *stubWindowUsageRepo) PruneStaleOpenBefore(_ context.Context, cutoff time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneStaleOpenFrom = cutoff
	return 0, nil
}

func (r *stubWindowUsageRepo) ListMonitorQuotaHistorySince(_ context.Context, since time.Time, _ int) ([]*AccountQuotaObservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.monitorSince = since
	if r.monitorErr != nil {
		return nil, r.monitorErr
	}
	return r.monitorObs, nil
}

func (r *stubWindowUsageRepo) ListCodexUsageUpdatesSince(_ context.Context, since time.Time, _ int) ([]*AccountQuotaObservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codexSince = since
	if r.codexErr != nil {
		return nil, r.codexErr
	}
	return r.codexObs, nil
}

// openRow 快照当前开放行（测试断言用）。
func (r *stubWindowUsageRepo) openRow(accountID int64, windowType string) *AccountWindowUsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.open[windowKey(accountID, windowType)]; ok {
		cp := *row
		return &cp
	}
	return nil
}

// stubWindowUsageLogRepo 记录聚合请求并返回可配置统计。
type stubWindowUsageLogRepo struct {
	UsageLogRepository

	mu         sync.Mutex
	rangeCalls int
	lastStart  time.Time
	lastEnd    time.Time
	rangeStats *usagestats.WindowTokenStats
	rangeErr   error
}

func (s *stubWindowUsageLogRepo) GetAccountWindowStatsRange(_ context.Context, _ int64, start, end time.Time) (*usagestats.WindowTokenStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rangeCalls++
	s.lastStart = start
	s.lastEnd = end
	if s.rangeErr != nil {
		return nil, s.rangeErr
	}
	if s.rangeStats != nil {
		return s.rangeStats, nil
	}
	return &usagestats.WindowTokenStats{}, nil
}

// newIngesterForTest 构造仅填充状态机所需字段的采集器（不启动循环）。
func newIngesterForTest(windowRepo AccountWindowUsageRepository, usageLogRepo UsageLogRepository) *AccountWindowUsageIngester {
	return NewAccountWindowUsageIngester(windowRepo, usageLogRepo)
}

// --- ApplySnapshot 状态机 ---

func tier5h(used float64, resetAt time.Time) domain.MonitorQuotaTier {
	return domain.MonitorQuotaTier{Window: "5h", UsedPercent: used, ResetAt: resetAt.UTC().Format(time.RFC3339)}
}

func snapshotWithTiers(tiers ...domain.MonitorQuotaTier) *domain.MonitorQuotaSnapshot {
	// FetchedAt 必须是当前时刻：陈旧快照守卫会丢弃 fetchedAt 早于开放行
	// window_start 的观测（多副本防污染），零值会被整批拒绝
	return &domain.MonitorQuotaSnapshot{Success: true, Tiers: tiers, FetchedAt: time.Now()}
}

func TestIngester_ApplySnapshot_FirstInsertCreatesOpenRow(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	resetAt := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(42.5, resetAt))))

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, 1, row.SampleCount)
	require.InDelta(t, 42.5, row.PeakUsedPercent, 0.001)
	require.InDelta(t, 42.5, row.LastUsedPercent, 0.001)
	require.Equal(t, resetAt, row.WindowEnd)
	require.Equal(t, resetAt.Add(-5*time.Hour), row.WindowStart)
	require.Nil(t, row.FinalizedAt, "fresh row must stay open")
}

func TestIngester_ApplySnapshot_SameWindowMergesMetrics(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	resetAt := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(30, resetAt))))
	// 同一窗口（reset 抖动 1 秒内）的第二次采样
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(55, resetAt.Add(1*time.Second)))))

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, 2, row.SampleCount)
	require.InDelta(t, 55.0, row.PeakUsedPercent, 0.001, "peak should keep the max sample")
	require.InDelta(t, 55.0, row.LastUsedPercent, 0.001)
	require.Equal(t, resetAt, row.WindowEnd, "same-window jitter must not move bounds")
}

func TestIngester_ApplySnapshot_SlidingWindowMovesBounds(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	oldReset := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(30, oldReset))))

	// reset 前移（滚动窗口滑动），旧 end 仍在未来 → 边界整体前移
	newReset := oldReset.Add(30 * time.Minute)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(35, newReset))))

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, newReset, row.WindowEnd)
	require.Equal(t, newReset.Add(-5*time.Hour), row.WindowStart)
	require.Equal(t, 2, row.SampleCount)
}

func TestIngester_ApplySnapshot_ExpiredWindowFinalizesAndReopens(t *testing.T) {
	repo := newStubWindowUsageRepo()
	usageRepo := &stubWindowUsageLogRepo{
		rangeStats: &usagestats.WindowTokenStats{Requests: 12, TokensTotal: 34567},
	}
	g := newIngesterForTest(repo, usageRepo)

	// 预置一个已过期的开放行（window_end 在过去）
	expiredEnd := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	expiredStart := expiredEnd.Add(-5 * time.Hour)
	require.NoError(t, repo.UpsertOpenWindow(context.Background(), &AccountWindowUsageRecord{
		AccountID: 7, WindowType: "5h",
		WindowStart: expiredStart, WindowEnd: expiredEnd,
		PeakUsedPercent: 80, LastUsedPercent: 78, SampleCount: 3,
	}))
	oldRow := repo.openRow(7, "5h")

	// 新窗口观测（reset 在未来）→ 关闭旧行 + 开新行
	newReset := time.Now().Add(5 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(5, newReset))))

	require.Equal(t, 1, repo.replaceCalls)
	require.Equal(t, oldRow.ID, repo.lastReplaceOldRow, "expired row should be replaced")
	require.Equal(t, int64(12), repo.lastReplaceStats.Requests)
	require.Equal(t, int64(34567), repo.lastReplaceStats.TokensTotal)
	require.Equal(t, 1, usageRepo.rangeCalls)
	require.Equal(t, expiredStart, usageRepo.lastStart, "token aggregation must use old window bounds")
	require.Equal(t, expiredEnd, usageRepo.lastEnd)

	// 旧行已关闭且带 token 明细
	require.Len(t, repo.finalized, 1)
	require.NotNil(t, repo.finalized[0].FinalizedAt)
	require.NotNil(t, repo.finalized[0].TokensTotal)
	require.Equal(t, int64(34567), *repo.finalized[0].TokensTotal)

	// 新开放行边界正确
	newRow := repo.openRow(7, "5h")
	require.NotNil(t, newRow)
	require.Equal(t, newReset, newRow.WindowEnd)
	require.Nil(t, newRow.FinalizedAt)
}

func TestIngester_ApplySnapshot_ResetJitterBackwardNeverRegresses(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	forwardReset := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(30, forwardReset))))

	// reset 后移（上游抖动）→ 只更新指标，绝不回退 window_end。
	// 同时断言传给仓储的行边界：仓储的 SQL 合并（GREATEST）本身也能兜住
	// 回退，这里确保采集器就没把回退值传下去（防御纵深可被检测）
	backwardReset := forwardReset.Add(-20 * time.Minute)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(40, backwardReset))))

	require.NotNil(t, repo.lastUpsertRow)
	require.Equal(t, forwardReset, repo.lastUpsertRow.WindowEnd, "ingester must pass the existing bounds on backward jitter")

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, forwardReset, row.WindowEnd, "window_end must never regress")
	require.Equal(t, forwardReset.Add(-5*time.Hour), row.WindowStart)
	require.Equal(t, 2, row.SampleCount)
	require.InDelta(t, 40.0, row.LastUsedPercent, 0.001)
}

func TestIngester_ApplySnapshot_FiltersNonRecordedAndInvalidTiers(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	reset := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	snapshot := snapshotWithTiers(
		domain.MonitorQuotaTier{Window: "5h", UsedPercent: 50, ResetAt: reset},
		domain.MonitorQuotaTier{Window: "daily", UsedPercent: 50, ResetAt: reset},         // 不记录
		domain.MonitorQuotaTier{Window: "30d", UsedPercent: 50, ResetAt: reset},           // 不记录
		domain.MonitorQuotaTier{Window: "7d", UsedPercent: 60},                            // 无 ResetAt
		domain.MonitorQuotaTier{Window: "weekly", UsedPercent: 70, ResetAt: "not-a-time"}, // 非法时间戳
	)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshot))

	require.NotNil(t, repo.openRow(7, "5h"))
	require.Nil(t, repo.openRow(7, "daily"))
	require.Nil(t, repo.openRow(7, "30d"))
	require.Nil(t, repo.openRow(7, "7d"))
	require.Nil(t, repo.openRow(7, "weekly"))

	// 失败快照整体跳过
	failed := &domain.MonitorQuotaSnapshot{Success: false, Tiers: []domain.MonitorQuotaTier{tier5h(10, time.Now().Add(time.Hour))}}
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, failed))
	require.Nil(t, g.ApplySnapshot(context.Background(), 7, nil))
	require.Equal(t, 1, repo.upsertCalls)
}

// 陈旧快照（reset_at 已过）且无开放行 → 不得新开行：旧行已被并发 finalize
// 或数据本身滞后，此时插入会产生 window_end 在过去的开放行，finalize 扫描
// 会再关一次，形成同一窗口的重复历史
func TestIngester_ApplySnapshot_StalePastResetDoesNotOpenRow(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	staleReset := time.Now().Add(-10 * time.Minute)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(50, staleReset))))

	require.Equal(t, 0, repo.upsertCalls)
	require.Nil(t, repo.openRow(7, "5h"))

	// 仅秒级偏差（时钟抖动容差内）仍应正常开行
	skewedReset := time.Now().Add(-1 * time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(50, skewedReset))))
	require.Equal(t, 1, repo.upsertCalls)
	require.NotNil(t, repo.openRow(7, "5h"))
}

// 多副本陈旧快照守卫：其他副本已把窗口推进到新实例（window_start 前移到
// 边界 T），本副本进程内缓存仍持有 T 之前抓的旧窗口观测（reset_at 落在
// 新行 window_end 之前 → 判为 reset 后移）。不守卫的话上一窗口的峰值会经
// GREATEST 永久写进新窗口。fetchedAt 早于新行 window_start → 整 tier 丢弃。
func TestIngester_ApplySnapshot_StaleFetchedAtDoesNotMergeIntoNewWindow(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	// 副本 A：新窗口已开启（边界在前方 3h，起点 = 边界 - 5h ≈ 2h 前）
	newReset := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(20, newReset))))
	callsAfterOpen := repo.upsertCalls

	// 副本 B：陈旧快照（reset_at 属于上一窗口 → 相对新行是「后移」路径），
	// 抓取时间早于新行 window_start → 必须被丢弃，不产生任何写入
	staleSnapshot := &domain.MonitorQuotaSnapshot{
		Success:   true,
		Tiers:     []domain.MonitorQuotaTier{tier5h(98, newReset.Add(-5*time.Hour))},
		FetchedAt: newReset.Add(-5 * time.Hour).Add(-2 * time.Minute), // 上一窗口期间抓取
	}
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, staleSnapshot))
	require.Equal(t, callsAfterOpen, repo.upsertCalls, "stale snapshot must be dropped, not merged")

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.InDelta(t, 20.0, row.PeakUsedPercent, 0.001, "peak must not inherit the previous window's 98%")
	require.Equal(t, 1, row.SampleCount)
}

func TestIngester_ApplySnapshot_MultipleTiersIndependent(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	reset := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	weeklyReset := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(
		tier5h(50, reset),
		domain.MonitorQuotaTier{Window: "weekly", UsedPercent: 20, ResetAt: weeklyReset.Format(time.RFC3339)},
	)))

	fiveHour := repo.openRow(7, "5h")
	weekly := repo.openRow(7, "weekly")
	require.NotNil(t, fiveHour)
	require.NotNil(t, weekly)
	require.Equal(t, reset, fiveHour.WindowEnd)
	require.Equal(t, weeklyReset, weekly.WindowEnd)
	require.Equal(t, weeklyReset.Add(-7*24*time.Hour), weekly.WindowStart)
}

// 同一观测重复回放（多副本 / 重启回填重扫同一批监控历史）：观测时刻不晚于
// 行内已见时刻 → 跳过，sample_count 恰好计数一次。
func TestIngester_ApplySnapshot_ReplaySameObservationCountsOnce(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	fetchedAt := time.Now().UTC().Truncate(time.Second)
	snapshot := &domain.MonitorQuotaSnapshot{
		Success:   true,
		Tiers:     []domain.MonitorQuotaTier{tier5h(30, fetchedAt.Add(4*time.Hour))},
		FetchedAt: fetchedAt,
	}
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshot))

	// 完全相同的观测回放两遍（模拟重启后重扫）
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshot))
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, snapshot))

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, 1, row.SampleCount, "replayed observation must not inflate sample_count")
	require.NotNil(t, row.LastSampleAt)
	require.Equal(t, fetchedAt, *row.LastSampleAt)
	require.Equal(t, 1, repo.upsertCalls, "process-level guard should skip before hitting the repo")

	// 更晚的观测（同窗口）→ 正常计数
	later := &domain.MonitorQuotaSnapshot{
		Success:   true,
		Tiers:     []domain.MonitorQuotaTier{tier5h(31, fetchedAt.Add(4*time.Hour).Add(time.Minute))},
		FetchedAt: fetchedAt.Add(time.Minute),
	}
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, later))
	row = repo.openRow(7, "5h")
	require.Equal(t, 2, row.SampleCount)
}

// 乱序到达（晚到观测的时刻早于行内已见时刻）同样被去重——不能覆盖 last%。
func TestIngester_ApplySnapshot_OutOfOrderOlderObservationSkipped(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	reset := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	base := time.Now().UTC().Truncate(time.Second)
	newer := &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{tier5h(50, reset)}, FetchedAt: base}
	older := &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{tier5h(10, reset)}, FetchedAt: base.Add(-time.Minute)}

	require.NoError(t, g.ApplySnapshot(context.Background(), 7, newer))
	require.NoError(t, g.ApplySnapshot(context.Background(), 7, older))

	row := repo.openRow(7, "5h")
	require.NotNil(t, row)
	require.Equal(t, 1, row.SampleCount)
	require.InDelta(t, 50.0, row.LastUsedPercent, 0.001, "late older observation must not overwrite last%")
}

// --- 被动源回放（水位） ---

func TestIngester_IngestMonitorHistoryAppliesAndAdvancesWatermark(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	reset := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	repo.monitorObs = []*AccountQuotaObservation{
		{AccountID: 7, Snapshot: &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{tier5h(30, reset)}, FetchedAt: base}},
		{AccountID: 8, Snapshot: &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{tier5h(40, reset)}, FetchedAt: base.Add(time.Minute)}},
	}

	g.ingestMonitorHistory(context.Background())

	require.NotNil(t, repo.openRow(7, "5h"))
	require.NotNil(t, repo.openRow(8, "5h"))
	require.True(t, g.monitorWM.Equal(base.Add(time.Minute)), "watermark must advance to the last observation's FetchedAt")

	// 第二轮：读接口收到上一轮推进后的水位
	g.ingestMonitorHistory(context.Background())
	require.True(t, repo.monitorSince.Equal(base.Add(time.Minute)), "second sweep must read from the advanced watermark")

	// 读取失败（DB 抖动）→ 水位不动，下轮重试
	repo.monitorErr = errors.New("db down")
	g.ingestMonitorHistory(context.Background())
	require.True(t, g.monitorWM.Equal(base.Add(time.Minute)), "failed read must not move the watermark")
}

func TestIngester_IngestCodexUsageUpdatesAppliesObservations(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	repo.codexObs = []*AccountQuotaObservation{
		{AccountID: 9, Snapshot: &domain.MonitorQuotaSnapshot{
			Success: true, FetchedAt: base,
			Tiers: []domain.MonitorQuotaTier{
				{Window: "5h", UsedPercent: 12.5, ResetAt: base.Add(3 * time.Hour).Format(time.RFC3339)},
				{Window: "7d", UsedPercent: 44, ResetAt: base.Add(48 * time.Hour).Format(time.RFC3339)},
			},
		}},
	}

	g.ingestCodexUsageUpdates(context.Background())

	fiveHour := repo.openRow(9, "5h")
	sevenDay := repo.openRow(9, "7d")
	require.NotNil(t, fiveHour)
	require.NotNil(t, sevenDay)
	require.InDelta(t, 12.5, fiveHour.PeakUsedPercent, 0.001)
	require.InDelta(t, 44.0, sevenDay.PeakUsedPercent, 0.001)
	require.True(t, g.codexWM.Equal(base))
}

// --- finalize 扫描 ---

func TestIngester_FinalizeExpiredAggregatesLocalUsage(t *testing.T) {
	repo := newStubWindowUsageRepo()
	usageRepo := &stubWindowUsageLogRepo{
		rangeStats: &usagestats.WindowTokenStats{Requests: 5, TokensTotal: 1000, TokensInput: 600, TokensOutput: 400},
	}
	g := newIngesterForTest(repo, usageRepo)

	// 过期超过 grace 的开放行
	end := time.Now().Add(-(ingesterFinalizeGrace + time.Minute))
	require.NoError(t, repo.UpsertOpenWindow(context.Background(), &AccountWindowUsageRecord{
		AccountID: 7, WindowType: "5h",
		WindowStart: end.Add(-5 * time.Hour), WindowEnd: end,
		PeakUsedPercent: 90, LastUsedPercent: 88, SampleCount: 4,
	}))
	// 未过 grace 的行（window_end 刚过）→ 不应被 finalize
	freshEnd := time.Now().Add(-time.Minute)
	require.NoError(t, repo.UpsertOpenWindow(context.Background(), &AccountWindowUsageRecord{
		AccountID: 8, WindowType: "5h",
		WindowStart: freshEnd.Add(-5 * time.Hour), WindowEnd: freshEnd,
		PeakUsedPercent: 10, SampleCount: 1,
	}))

	g.finalizeExpired(context.Background(), time.Now())

	require.Equal(t, 1, repo.finalizeCalls)
	require.Len(t, repo.finalized, 1)
	require.Equal(t, int64(7), repo.finalized[0].AccountID)
	require.NotNil(t, repo.finalized[0].TokensTotal)
	require.Equal(t, int64(1000), *repo.finalized[0].TokensTotal)
	require.Equal(t, end.Add(-5*time.Hour), usageRepo.lastStart)
	require.Equal(t, end, usageRepo.lastEnd)
	require.NotNil(t, repo.openRow(8, "5h"), "row within grace must stay open")
}

func TestIngester_RunDailyMaintenancePrunesRetentionWindows(t *testing.T) {
	repo := newStubWindowUsageRepo()
	g := newIngesterForTest(repo, &stubWindowUsageLogRepo{})

	now := time.Now()
	g.RunDailyMaintenance(context.Background())

	require.WithinDuration(t, now.AddDate(0, 0, -windowHistoryRetentionDays), repo.pruneFinalizedAt, time.Minute)
	require.WithinDuration(t, now.AddDate(0, 0, -windowStaleOpenRetentionDays), repo.pruneStaleOpenFrom, time.Minute)
}

// --- 查询服务 ---

type stubWindowAccountRepo struct {
	stubOpenAIAccountRepo
}

// GetByID 缺省账号返回类型化 ErrAccountNotFound（对齐真实仓储，
// 查询服务据此走宽松分支）。
func (r stubWindowAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	account, err := r.stubOpenAIAccountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrAccountNotFound
	}
	return account, nil
}

func TestWindowHistoryService_GroupsByWindowType(t *testing.T) {
	windowRepo := newStubWindowUsageRepo()
	end := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	finalizedAt := time.Now()
	row := &AccountWindowUsageRecord{
		ID: 1, AccountID: 7, WindowType: "5h",
		WindowStart: end.Add(-5 * time.Hour), WindowEnd: end,
		PeakUsedPercent: 66, LastUsedPercent: 60, SampleCount: 3,
		FinalizedAt: &finalizedAt,
	}
	windowRepo.finalized = []*AccountWindowUsageRecord{row}

	svc := NewAccountWindowUsageHistoryService(windowRepo, stubWindowAccountRepo{stubOpenAIAccountRepo{accounts: []Account{
		{ID: 7, Platform: domain.PlatformAnthropic},
	}}})

	resp, err := svc.GetWindowHistory(context.Background(), 7, 30)
	require.NoError(t, err)
	require.Len(t, resp.Windows["5h"], 1)

	entry := resp.Windows["5h"][0]
	require.True(t, entry.Finalized)
	require.NotNil(t, entry.FinalUsedPercent)
	require.InDelta(t, 60.0, *entry.FinalUsedPercent, 0.001)
	require.InDelta(t, 66.0, entry.PeakUsedPercent, 0.001)
}

func TestWindowHistoryService_MissingAccountYieldsEmptyResponse(t *testing.T) {
	svc := NewAccountWindowUsageHistoryService(newStubWindowUsageRepo(), stubWindowAccountRepo{stubOpenAIAccountRepo{accounts: nil}})

	resp, err := svc.GetWindowHistory(context.Background(), 404, 30)
	require.NoError(t, err)
	require.Empty(t, resp.Windows)
}

func TestWindowHistoryService_FiltersNonRecordedWindowRows(t *testing.T) {
	windowRepo := newStubWindowUsageRepo()
	finalizedAt := time.Now()
	end := time.Now().Add(-time.Hour)
	windowRepo.finalized = []*AccountWindowUsageRecord{
		{ID: 1, AccountID: 7, WindowType: "5h", WindowStart: end.Add(-5 * time.Hour), WindowEnd: end, FinalizedAt: &finalizedAt},
		{ID: 2, AccountID: 7, WindowType: "daily", WindowStart: end.Add(-24 * time.Hour), WindowEnd: end, FinalizedAt: &finalizedAt},
	}

	svc := NewAccountWindowUsageHistoryService(windowRepo, stubWindowAccountRepo{stubOpenAIAccountRepo{accounts: []Account{
		{ID: 7},
	}}})

	resp, err := svc.GetWindowHistory(context.Background(), 7, 30)
	require.NoError(t, err)
	require.Len(t, resp.Windows["5h"], 1)
	require.NotContains(t, resp.Windows, "daily")
}

// stubWindowUsageLogRepo 必须满足 UsageLogRepository（嵌入即可）。
var _ UsageLogRepository = (*stubWindowUsageLogRepo)(nil)

// 保险：错误路径透传
func TestIngester_ApplySnapshot_RepoErrorPropagates(t *testing.T) {
	repo := newStubWindowUsageRepo()
	usageRepo := &stubWindowUsageLogRepo{rangeErr: errors.New("db down")}
	g := newIngesterForTest(repo, usageRepo)

	end := time.Now().Add(-10 * time.Minute)
	require.NoError(t, repo.UpsertOpenWindow(context.Background(), &AccountWindowUsageRecord{
		AccountID: 7, WindowType: "5h", WindowStart: end.Add(-5 * time.Hour), WindowEnd: end, SampleCount: 1,
	}))

	err := g.ApplySnapshot(context.Background(), 7, snapshotWithTiers(tier5h(5, time.Now().Add(5*time.Hour))))
	require.Error(t, err, "expired-window finalize failure must surface")
}
