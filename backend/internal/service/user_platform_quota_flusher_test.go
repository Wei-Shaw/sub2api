package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ---------------------------------------------------------------------------
// Mock: quotaDirtyCache
// ---------------------------------------------------------------------------

type mockQuotaDirtyCache struct {
	// popSequence: 第 0 次 Pop 返回 popSequence[0]，之后返回 nil（空集）
	popSequence [][]UserPlatformQuotaKey
	popCallIdx  int

	// getEntries: BatchGetUserPlatformQuotaCache 返回的 entries（与 keys 对齐）
	getEntries []*UserPlatformQuotaCacheEntry
	getErr     error

	// readdCalled: 记录 Readd 收到的 keys（累积所有次调用）
	readdCalled [][]UserPlatformQuotaKey
	readdErr    error
}

func (m *mockQuotaDirtyCache) PopDirtyUserPlatformQuotaKeys(_ context.Context, _ int) ([]UserPlatformQuotaKey, error) {
	if m.popCallIdx < len(m.popSequence) {
		keys := m.popSequence[m.popCallIdx]
		m.popCallIdx++
		return keys, nil
	}
	// 超出序列 → 空集（模拟脏集已清空）
	return nil, nil
}

func (m *mockQuotaDirtyCache) ReaddDirtyUserPlatformQuotaKeys(_ context.Context, keys []UserPlatformQuotaKey) error {
	m.readdCalled = append(m.readdCalled, keys)
	return m.readdErr
}

func (m *mockQuotaDirtyCache) BatchGetUserPlatformQuotaCache(_ context.Context, _ []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getEntries, nil
}

// ---------------------------------------------------------------------------
// Mock: quotaSnapshotWriter
// ---------------------------------------------------------------------------

type mockQuotaSnapshotWriter struct {
	receivedSnaps []UserPlatformQuotaSnapshot
	returnErr     error
}

func (m *mockQuotaSnapshotWriter) BatchSnapshotUsage(_ context.Context, snaps []UserPlatformQuotaSnapshot, _ time.Time) error {
	m.receivedSnaps = append(m.receivedSnaps, snaps...)
	return m.returnErr
}

// ---------------------------------------------------------------------------
// Helper: 构造窗口起始时间（非 nil）
// ---------------------------------------------------------------------------

func flusherPtrTime(t time.Time) *time.Time { return &t }

func makeEntry(daily, weekly, monthly float64) *UserPlatformQuotaCacheEntry {
	now := time.Now().UTC()
	return &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:      daily,
		WeeklyUsageUSD:     weekly,
		MonthlyUsageUSD:    monthly,
		DailyWindowStart:   flusherPtrTime(now),
		WeeklyWindowStart:  flusherPtrTime(now),
		MonthlyWindowStart: flusherPtrTime(now),
	}
}

// ---------------------------------------------------------------------------
// newTestFlusher: 直接构造 struct（跳过构造函数，B7 才注入）
// ---------------------------------------------------------------------------

func newTestFlusher(cache quotaDirtyCache, writer quotaSnapshotWriter) *UserPlatformQuotaUsageFlusher {
	return &UserPlatformQuotaUsageFlusher{
		cache:        cache,
		quotaRepo:    writer,
		timingWheel:  nil, // 单测不启动 TimingWheel
		interval:     5 * time.Second,
		batchSize:    100,
		flushTimeout: 5 * time.Second,
		metrics:      &FlusherMetrics{},
	}
}

// ---------------------------------------------------------------------------
// 场景 1: PopSnapshotUpsert — 2 key + 2 个含 window 的 entry → writer 收 2 行
// ---------------------------------------------------------------------------

func TestFlusher_PopSnapshotUpsert(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 1, Platform: "anthropic"},
		{UserID: 2, Platform: "openai"},
	}
	cache := &mockQuotaDirtyCache{
		popSequence: [][]UserPlatformQuotaKey{keys}, // 第 1 次返回 keys，之后空
		getEntries: []*UserPlatformQuotaCacheEntry{
			makeEntry(1.0, 2.0, 3.0),
			makeEntry(4.0, 5.0, 6.0),
		},
	}
	writer := &mockQuotaSnapshotWriter{}
	f := newTestFlusher(cache, writer)

	f.flush()

	if len(writer.receivedSnaps) != 2 {
		t.Fatalf("expected 2 snaps, got %d", len(writer.receivedSnaps))
	}
	if f.metrics.FlushBatchSizeTotal.Load() != 2 {
		t.Errorf("FlushBatchSizeTotal = %d, want 2", f.metrics.FlushBatchSizeTotal.Load())
	}
	if f.metrics.FlushSuccessTotal.Load() != 1 {
		t.Errorf("FlushSuccessTotal = %d, want 1", f.metrics.FlushSuccessTotal.Load())
	}
	if f.metrics.FlushErrorTotal.Load() != 0 {
		t.Errorf("FlushErrorTotal = %d, want 0", f.metrics.FlushErrorTotal.Load())
	}
}

// ---------------------------------------------------------------------------
// 场景 2: MissKeySkipped — 2 key，BatchGet 返回 [entry, nil] → 只刷 1 行，nil 跳过，不 Readd
// ---------------------------------------------------------------------------

func TestFlusher_MissKeySkipped(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 1, Platform: "anthropic"},
		{UserID: 2, Platform: "openai"},
	}
	cache := &mockQuotaDirtyCache{
		popSequence: [][]UserPlatformQuotaKey{keys},
		getEntries: []*UserPlatformQuotaCacheEntry{
			makeEntry(1.0, 2.0, 3.0),
			nil, // MISS
		},
	}
	writer := &mockQuotaSnapshotWriter{}
	f := newTestFlusher(cache, writer)

	f.flush()

	if len(writer.receivedSnaps) != 1 {
		t.Fatalf("expected 1 snap, got %d", len(writer.receivedSnaps))
	}
	if writer.receivedSnaps[0].UserID != 1 {
		t.Errorf("expected snap for UserID=1, got %d", writer.receivedSnaps[0].UserID)
	}
	if len(cache.readdCalled) != 0 {
		t.Errorf("Readd should NOT be called on MISS, got %d calls", len(cache.readdCalled))
	}
	if f.metrics.FlushSuccessTotal.Load() != 1 {
		t.Errorf("FlushSuccessTotal = %d, want 1", f.metrics.FlushSuccessTotal.Load())
	}
}

// ---------------------------------------------------------------------------
// 场景 3: UpsertFailReadds — writer 返普通 error → keys 被 Readd，FlushErrorTotal=1，DirtyReaddTotal=len
// ---------------------------------------------------------------------------

func TestFlusher_UpsertFailReadds(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 1, Platform: "anthropic"},
		{UserID: 2, Platform: "openai"},
	}
	cache := &mockQuotaDirtyCache{
		popSequence: [][]UserPlatformQuotaKey{keys},
		getEntries: []*UserPlatformQuotaCacheEntry{
			makeEntry(1.0, 2.0, 3.0),
			makeEntry(4.0, 5.0, 6.0),
		},
	}
	writeErr := errors.New("db connection timeout")
	writer := &mockQuotaSnapshotWriter{returnErr: writeErr}
	f := newTestFlusher(cache, writer)

	f.flush()

	if f.metrics.FlushErrorTotal.Load() != 1 {
		t.Errorf("FlushErrorTotal = %d, want 1", f.metrics.FlushErrorTotal.Load())
	}
	if len(cache.readdCalled) == 0 {
		t.Fatal("Readd should be called after write error")
	}
	totalReadd := 0
	for _, rk := range cache.readdCalled {
		totalReadd += len(rk)
	}
	if totalReadd != len(keys) {
		t.Errorf("DirtyReaddTotal (from Readd calls) = %d, want %d", totalReadd, len(keys))
	}
	if f.metrics.DirtyReaddTotal.Load() != int64(len(keys)) {
		t.Errorf("DirtyReaddTotal metric = %d, want %d", f.metrics.DirtyReaddTotal.Load(), len(keys))
	}
	if f.metrics.FlushSuccessTotal.Load() != 0 {
		t.Errorf("FlushSuccessTotal = %d, want 0", f.metrics.FlushSuccessTotal.Load())
	}
}

// ---------------------------------------------------------------------------
// 场景 4: FKViolationDropsNoReadd — writer 返 ErrUserPlatformQuotaFKViolation → 不 Readd，FlushFKViolationTotal=1
// ---------------------------------------------------------------------------

func TestFlusher_FKViolationDropsNoReadd(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 999, Platform: "anthropic"},
	}
	cache := &mockQuotaDirtyCache{
		popSequence: [][]UserPlatformQuotaKey{keys},
		getEntries: []*UserPlatformQuotaCacheEntry{
			makeEntry(1.0, 2.0, 3.0),
		},
	}
	writer := &mockQuotaSnapshotWriter{returnErr: ErrUserPlatformQuotaFKViolation}
	f := newTestFlusher(cache, writer)

	f.flush()

	if f.metrics.FlushFKViolationTotal.Load() != 1 {
		t.Errorf("FlushFKViolationTotal = %d, want 1", f.metrics.FlushFKViolationTotal.Load())
	}
	if f.metrics.FlushErrorTotal.Load() != 1 {
		t.Errorf("FlushErrorTotal = %d, want 1", f.metrics.FlushErrorTotal.Load())
	}
	if len(cache.readdCalled) != 0 {
		t.Errorf("Readd should NOT be called for FK violation (drop), got %d calls", len(cache.readdCalled))
	}
	if f.metrics.DirtyReaddTotal.Load() != 0 {
		t.Errorf("DirtyReaddTotal = %d, want 0 (FK violation drops)", f.metrics.DirtyReaddTotal.Load())
	}
}

// ---------------------------------------------------------------------------
// 场景 5: NilSafe — var f *UserPlatformQuotaUsageFlusher; f.flush(); f.Stop() 不 panic
// ---------------------------------------------------------------------------

func TestFlusher_NilSafe(t *testing.T) {
	var f *UserPlatformQuotaUsageFlusher
	// 下面两行不应 panic
	f.flush()
	f.Stop()
}

// ---------------------------------------------------------------------------
// 场景 6: StopPreventsFlush — stopped=true 后 tick() 不调 flush（writer 没收到 snaps）
// ---------------------------------------------------------------------------

func TestFlusher_StopPreventsFlush(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 1, Platform: "anthropic"},
	}
	cache := &mockQuotaDirtyCache{
		popSequence: [][]UserPlatformQuotaKey{keys},
		getEntries: []*UserPlatformQuotaCacheEntry{
			makeEntry(1.0, 2.0, 3.0),
		},
	}
	writer := &mockQuotaSnapshotWriter{}
	f := newTestFlusher(cache, writer)

	// 标记为已停止
	f.stopped.Store(true)

	// tick 应该直接返回，不触发 flush
	f.tick()

	if len(writer.receivedSnaps) != 0 {
		t.Errorf("expected 0 snaps after stop, got %d", len(writer.receivedSnaps))
	}
	if cache.popCallIdx != 0 {
		t.Errorf("Pop should not be called after stop, popCallIdx = %d", cache.popCallIdx)
	}
}

// ---------------------------------------------------------------------------
// 场景 B13-1: ZeroPercentCompany — 0% 公司脏集恒空，flusher 空跑无 DB 写
//
// 模拟几乎没有用户配置 quota limit 的公司：脏集始终为空（popSequence 为空切片），
// Pop 每次返回空集。flush() 应早退，不写 DB、不计成功、不 Readd。
// ---------------------------------------------------------------------------

func TestScenario_ZeroPercentCompany(t *testing.T) {
	cache := &mockQuotaDirtyCache{
		// popSequence 为空 → Pop 超出序列 → 始终返回 nil（空集）
		popSequence: [][]UserPlatformQuotaKey{},
	}
	writer := &mockQuotaSnapshotWriter{}
	f := newTestFlusher(cache, writer)

	f.flush()

	if len(writer.receivedSnaps) != 0 {
		t.Errorf("0%% company: expected 0 snaps, got %d", len(writer.receivedSnaps))
	}
	if f.metrics.FlushBatchSizeTotal.Load() != 0 {
		t.Errorf("0%% company: FlushBatchSizeTotal = %d, want 0", f.metrics.FlushBatchSizeTotal.Load())
	}
	if f.metrics.FlushSuccessTotal.Load() != 0 {
		t.Errorf("0%% company: FlushSuccessTotal = %d, want 0 (empty-set early return)", f.metrics.FlushSuccessTotal.Load())
	}
	if f.metrics.FlushErrorTotal.Load() != 0 {
		t.Errorf("0%% company: FlushErrorTotal = %d, want 0", f.metrics.FlushErrorTotal.Load())
	}
	if len(cache.readdCalled) != 0 {
		t.Errorf("0%% company: Readd should never be called, got %d calls", len(cache.readdCalled))
	}
}

// ---------------------------------------------------------------------------
// P1: IntervalFallback — flush_interval_ms ≤0 时回退 2s；正常值保留
// ---------------------------------------------------------------------------

func TestNewUserPlatformQuotaUsageFlusher_IntervalFallback(t *testing.T) {
	cases := []struct {
		name   string
		inMs   int
		wantDu time.Duration
	}{
		{"零值回退 2s", 0, 2 * time.Second},
		{"负数回退 2s", -100, 2 * time.Second},
		{"正常 2000ms 保留", 2000, 2 * time.Second},
		{"正常 500ms 保留", 500, 500 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Database.UserPlatformQuotaFlushIntervalMs = tc.inMs
			f := NewUserPlatformQuotaUsageFlusher(cfg, nil, nil, nil)
			if f.interval != tc.wantDu {
				t.Fatalf("interval = %v, want %v", f.interval, tc.wantDu)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// P1: EnabledField — flusher_enabled 配置正确写入 f.enabled
// ---------------------------------------------------------------------------

func TestNewUserPlatformQuotaUsageFlusher_EnabledField(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := &config.Config{}
		cfg.Database.UserPlatformQuotaFlusherEnabled = enabled
		cfg.Database.UserPlatformQuotaFlushIntervalMs = 500
		f := NewUserPlatformQuotaUsageFlusher(cfg, nil, nil, nil)
		if f.enabled != enabled {
			t.Errorf("enabled = %v, want %v", f.enabled, enabled)
		}
	}
}

// ---------------------------------------------------------------------------
// P2: ReaddFailCounts — BatchGet 失败 + Readd 失败 → DirtyLostTotal 增、DirtyReaddTotal 不变
//                       BatchGet 失败 + Readd 成功 → DirtyReaddTotal 增、DirtyLostTotal 不变
// ---------------------------------------------------------------------------

func TestFlusher_ReaddFailCounts(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 10, Platform: "anthropic"},
		{UserID: 11, Platform: "openai"},
	}

	t.Run("Readd 失败计 DirtyLostTotal", func(t *testing.T) {
		cache := &mockQuotaDirtyCache{
			popSequence: [][]UserPlatformQuotaKey{keys},
			getErr:      errors.New("redis timeout"),            // 触发 BatchGet 失败路径
			readdErr:    errors.New("redis connection refused"), // Readd 也失败
		}
		f := newTestFlusher(cache, &mockQuotaSnapshotWriter{})

		f.flush()

		if f.metrics.DirtyLostTotal.Load() != int64(len(keys)) {
			t.Errorf("DirtyLostTotal = %d, want %d", f.metrics.DirtyLostTotal.Load(), len(keys))
		}
		if f.metrics.DirtyReaddTotal.Load() != 0 {
			t.Errorf("DirtyReaddTotal = %d, want 0 (Readd 失败不应计入)", f.metrics.DirtyReaddTotal.Load())
		}
	})

	t.Run("Readd 成功计 DirtyReaddTotal", func(t *testing.T) {
		cache := &mockQuotaDirtyCache{
			popSequence: [][]UserPlatformQuotaKey{keys},
			getErr:      errors.New("redis timeout"), // 触发 BatchGet 失败路径
			readdErr:    nil,                         // Readd 成功
		}
		f := newTestFlusher(cache, &mockQuotaSnapshotWriter{})

		f.flush()

		if f.metrics.DirtyReaddTotal.Load() != int64(len(keys)) {
			t.Errorf("DirtyReaddTotal = %d, want %d", f.metrics.DirtyReaddTotal.Load(), len(keys))
		}
		if f.metrics.DirtyLostTotal.Load() != 0 {
			t.Errorf("DirtyLostTotal = %d, want 0 (Readd 成功不应计 lost)", f.metrics.DirtyLostTotal.Load())
		}
	})
}

// ---------------------------------------------------------------------------
// ClampsBatchSize — NewUserPlatformQuotaUsageFlusher 构造时按
// [defaultFlushBatchSize, maxFlushBatchSize] 区间 clamp batchSize
// ---------------------------------------------------------------------------

func TestNewUserPlatformQuotaUsageFlusher_ClampsBatchSize(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"超上限被 clamp", 7000, maxFlushBatchSize},
		{"恰好上限保留", maxFlushBatchSize, maxFlushBatchSize},
		{"零回退默认", 0, defaultFlushBatchSize},
		{"负数回退默认", -5, defaultFlushBatchSize},
		{"正常值保留", 500, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Database.UserPlatformQuotaFlushBatchSize = tc.in
			f := NewUserPlatformQuotaUsageFlusher(cfg, nil, nil, nil)
			if f.batchSize != tc.want {
				t.Fatalf("batchSize = %d, want %d", f.batchSize, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 场景 B13-2: NinetyPercentCompany — 90% 公司大量用户配 limit，一批 5 key 批量刷库
//
// 模拟大量用户配置了 quota limit 的公司：脏集第一次 Pop 返回 5 个不同用户的 key，
// 之后返回空集（避免 flush 循环）。flush() 应构造 5 条 snapshot 写入 DB，
// 断言绝对值语义（snap 的 DailyUsageUSD 等于 entry 的值）、metrics 正确、不 Readd。
// ---------------------------------------------------------------------------

func TestScenario_NinetyPercentCompany(t *testing.T) {
	keys := []UserPlatformQuotaKey{
		{UserID: 101, Platform: "anthropic"},
		{UserID: 102, Platform: "anthropic"},
		{UserID: 103, Platform: "openai"},
		{UserID: 104, Platform: "openai"},
		{UserID: 105, Platform: "anthropic"},
	}
	entries := []*UserPlatformQuotaCacheEntry{
		makeEntry(1.1, 2.2, 3.3),
		makeEntry(4.4, 5.5, 6.6),
		makeEntry(7.7, 8.8, 9.9),
		makeEntry(0.5, 1.0, 1.5),
		makeEntry(10.0, 20.0, 30.0),
	}
	cache := &mockQuotaDirtyCache{
		// 第 1 次 Pop 返回 5 keys，之后返回空集（防止 flush 无限循环）
		popSequence: [][]UserPlatformQuotaKey{keys},
		getEntries:  entries,
	}
	writer := &mockQuotaSnapshotWriter{}
	f := newTestFlusher(cache, writer)

	f.flush()

	// 应收到 5 条 snapshot
	if len(writer.receivedSnaps) != 5 {
		t.Fatalf("90%% company: expected 5 snaps, got %d", len(writer.receivedSnaps))
	}

	// 验证绝对值语义：第 1 条 snap 的各窗口 usage 应等于 entries[0] 的值
	snap0 := writer.receivedSnaps[0]
	entry0 := entries[0]
	if snap0.DailyUsageUSD != entry0.DailyUsageUSD {
		t.Errorf("snap[0].DailyUsageUSD = %v, want %v", snap0.DailyUsageUSD, entry0.DailyUsageUSD)
	}
	if snap0.WeeklyUsageUSD != entry0.WeeklyUsageUSD {
		t.Errorf("snap[0].WeeklyUsageUSD = %v, want %v", snap0.WeeklyUsageUSD, entry0.WeeklyUsageUSD)
	}
	if snap0.MonthlyUsageUSD != entry0.MonthlyUsageUSD {
		t.Errorf("snap[0].MonthlyUsageUSD = %v, want %v", snap0.MonthlyUsageUSD, entry0.MonthlyUsageUSD)
	}

	// FlushBatchSizeTotal 应为 5（本批 keys 数量）
	if f.metrics.FlushBatchSizeTotal.Load() != 5 {
		t.Errorf("90%% company: FlushBatchSizeTotal = %d, want 5", f.metrics.FlushBatchSizeTotal.Load())
	}
	// FlushSuccessTotal 应为 1（1 个批次写成功）
	if f.metrics.FlushSuccessTotal.Load() != 1 {
		t.Errorf("90%% company: FlushSuccessTotal = %d, want 1", f.metrics.FlushSuccessTotal.Load())
	}
	// 无错误、无 Readd
	if f.metrics.FlushErrorTotal.Load() != 0 {
		t.Errorf("90%% company: FlushErrorTotal = %d, want 0", f.metrics.FlushErrorTotal.Load())
	}
	if f.metrics.DirtyReaddTotal.Load() != 0 {
		t.Errorf("90%% company: DirtyReaddTotal = %d, want 0", f.metrics.DirtyReaddTotal.Load())
	}
	if len(cache.readdCalled) != 0 {
		t.Errorf("90%% company: Readd should not be called, got %d calls", len(cache.readdCalled))
	}
}
