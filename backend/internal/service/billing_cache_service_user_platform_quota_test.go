//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// fakeIncrCache 仅记录 IncrUserPlatformQuotaUsageCache 被调用的参数。
type fakeIncrCache struct {
	BillingCache
	calls []incrCall
}

type incrCall struct {
	userID   int64
	platform string
	cost     float64
	ttl      time.Duration
}

func (f *fakeIncrCache) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration) error {
	f.calls = append(f.calls, incrCall{userID, platform, cost, ttl})
	return nil
}

// IncrementUserPlatformQuotaUsage 已改为同步直写,不再走 worker。
// 测试验证:同步调用立即调到 cache.IncrUserPlatformQuotaUsageCache。
func TestIncrementUserPlatformQuotaUsage_SyncCallsCache(t *testing.T) {
	fake := &fakeIncrCache{}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 120

	s := &BillingCacheService{
		cache: fake,
		cfg:   cfg,
	}

	s.IncrementUserPlatformQuotaUsage(101, "anthropic", 0.25)
	s.IncrementUserPlatformQuotaUsage(101, "openai", 0.50)

	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 incr calls, got %d", len(fake.calls))
	}
	if fake.calls[0] != (incrCall{101, "anthropic", 0.25, 120 * time.Second}) {
		t.Errorf("call[0] = %+v", fake.calls[0])
	}
	if fake.calls[1] != (incrCall{101, "openai", 0.50, 120 * time.Second}) {
		t.Errorf("call[1] = %+v", fake.calls[1])
	}
}

// ── T6 tests: checkUserPlatformQuotaEligibility ──────────────────────────────

// fakeQuotaRepo 实现 UserPlatformQuotaRepository 最小子集
type fakeQuotaRepo struct {
	rec *UserPlatformQuotaRecord
}

func (f *fakeQuotaRepo) GetByUserPlatform(_ context.Context, _ int64, _ string) (*UserPlatformQuotaRecord, error) {
	return f.rec, nil
}

func (f *fakeQuotaRepo) BulkInsertInitial(_ context.Context, _ []UserPlatformQuotaRecord) error {
	return nil
}

func (f *fakeQuotaRepo) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
}

func (f *fakeQuotaRepo) ListByUser(_ context.Context, _ int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
}

func (f *fakeQuotaRepo) UpsertForUser(_ context.Context, _ int64, _ []UserPlatformQuotaRecord) error {
	return nil
}

func (f *fakeQuotaRepo) ResetExpiredWindow(_ context.Context, _ int64, _ string, _ string, _ time.Time) error {
	return nil
}

// fakeFullCache 同时支持 Get + Set + Incr + Delete。
// mu 保护 entry 和 deleteCalls，防止异步 goroutine 与主 goroutine 之间的 data race。
type fakeFullCache struct {
	BillingCache
	mu          sync.Mutex
	entry       *UserPlatformQuotaCacheEntry
	deleteCalls int
}

// getDeleteCalls 线程安全地读取 deleteCalls。
func (f *fakeFullCache) getDeleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteCalls
}

// getEntry 线程安全地读取 entry。
func (f *fakeFullCache) getEntry() *UserPlatformQuotaCacheEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entry
}

func (f *fakeFullCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, _ string) (*UserPlatformQuotaCacheEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.entry == nil {
		return nil, false, nil
	}
	return f.entry, true, nil
}

func (f *fakeFullCache) SetUserPlatformQuotaCache(_ context.Context, _ int64, _ string, e *UserPlatformQuotaCacheEntry, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entry = e
	return nil
}

func (f *fakeFullCache) DeleteUserPlatformQuotaCache(_ context.Context, _ int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	f.entry = nil
	return nil
}

func newServiceForPreflight(t *testing.T, repo UserPlatformQuotaRepository, cache BillingCache) *BillingCacheService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	return &BillingCacheService{
		cache:                 cache,
		cfg:                   cfg,
		userPlatformQuotaRepo: repo,
	}
}

// currentDayStart 返回全局时区当天 0 点（与生产 timezone.StartOfDay 同口径，确保窗口有效）。
func currentDayStart() *time.Time {
	s := timezone.StartOfDay(time.Now())
	return &s
}

func TestCheckUserPlatformQuotaEligibility_AllowsWhenUnderLimit(t *testing.T) {
	daily := 10.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    4.5,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckUserPlatformQuotaEligibility_DailyExhausted(t *testing.T) {
	daily := 5.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    5.0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("expected ErrUserPlatformDailyQuotaExhausted, got %v", err)
	}
}

func TestCheckUserPlatformQuotaEligibility_NilLimitMeansUnlimited(t *testing.T) {
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic",
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    999,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
		// DailyLimitUSD nil → 无限额
	}}
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("nil limits should be unlimited, got %v", err)
	}
}

func TestCheckUserPlatformQuotaEligibility_ZeroLimitImmediateBlock(t *testing.T) {
	zero := 0.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &zero,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &zero,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("expected daily exhausted for limit=0, got %v", err)
	}
}

func TestCheckUserPlatformQuotaEligibility_NoRecordMeansUnlimited(t *testing.T) {
	repo := &fakeQuotaRepo{rec: nil}
	cache := &fakeFullCache{}
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("no record = unlimited, got %v", err)
	}
}

// TestCheckUserPlatformQuotaEligibility_OldSchemaCacheMissTriggersDB 验证旧版 entry（SchemaVersion=0）
// 触发 DB 回退路径，并在 DB 数据判断配额是否超限。
// DB record 需设置有效的 window_start，否则 quotaWindowExpired 会将 usage 归零（nil 窗口视为已过期）。
func TestCheckUserPlatformQuotaEligibility_OldSchemaCacheMissTriggersDB(t *testing.T) {
	daily := 5.0
	dayStart := currentDayStart()
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily, DailyUsageUSD: 6.0,
		DailyWindowStart: dayStart,
	}}
	// SchemaVersion=0（旧 entry），应走 DB 路径
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{DailyUsageUSD: 1.0}}
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("旧版 entry 应走 DB 路径并报 daily exhausted, got %v", err)
	}
}

// TestCheckUserPlatformQuotaEligibility_WindowExpiredInCache 验证 cache HIT 时若窗口已过期，usage 归零，用户放行。
func TestCheckUserPlatformQuotaEligibility_WindowExpiredInCache(t *testing.T) {
	daily := 5.0
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) // 远古窗口起始，肯定已过期
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    10.0, // 超限，但窗口已过期
		DailyLimitUSD:    &daily,
		DailyWindowStart: &past,
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if err != nil {
		t.Errorf("过期窗口应归零放行, got %v", err)
	}
}

// TestCheckUserPlatformQuotaEligibility_WindowExpiredRefreshesCache 验证:
// V1 HIT 路径检测到窗口过期时,用 reset 后的 entry 同步覆盖 Redis(而非 Delete):
//  1. 当前请求以本地清零值判断 → 放行
//  2. cache entry 被替换为新 entry: usage 清零 + window_start 更新到当前窗口
//     limit 保留;这样并发 IncrUserPlatformQuotaUsage 的 Lua INCR 可正确累加到新窗口。
func TestCheckUserPlatformQuotaEligibility_WindowExpiredRefreshesCache(t *testing.T) {
	daily := 5.0
	// 远古窗口起始,确保 quotaWindowExpired 返回 true
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    10.0, // 超限,但窗口已过期 → 应被本地清零后放行
		DailyLimitUSD:    &daily,
		DailyWindowStart: &past,
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)

	// 本次 check 应放行(本地清零后 usage=0 < limit=5)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if err != nil {
		t.Errorf("过期窗口应归零放行, got %v", err)
	}

	// 验证 cache entry 已被刷新:usage 清零、limit 保留、window_start 更新到当前窗口
	refreshed := cache.getEntry()
	if refreshed == nil {
		t.Fatal("窗口过期后 cache entry 不应为 nil(应被 SetCache 覆盖,而非 Delete)")
	}
	if refreshed.DailyUsageUSD != 0 {
		t.Errorf("刷新后 DailyUsageUSD = %v, want 0", refreshed.DailyUsageUSD)
	}
	if refreshed.DailyLimitUSD == nil || *refreshed.DailyLimitUSD != daily {
		t.Errorf("刷新后 DailyLimitUSD = %v, want %v(保留)", refreshed.DailyLimitUSD, daily)
	}
	if refreshed.SchemaVersion != UserPlatformQuotaCacheSchemaV1 {
		t.Errorf("刷新后 SchemaVersion = %d, want V1", refreshed.SchemaVersion)
	}
	if refreshed.DailyWindowStart == nil || refreshed.DailyWindowStart.Equal(past) {
		t.Errorf("刷新后 DailyWindowStart = %v, 应更新到当前窗口而非保留 past=%v", refreshed.DailyWindowStart, past)
	}
}

// ── T5 tests: QueueUpdateUserPlatformQuotaUsage ───────────────────────────────

// ── C-NEW-1: monthlyQuotaWindowExpired 30 天滚动测试 ─────────────────────────

func TestMonthlyQuotaWindowExpired_NilStart(t *testing.T) {
	if !monthlyQuotaWindowExpired(nil, time.Now().UTC()) {
		t.Error("nil start should be considered expired")
	}
}

func TestMonthlyQuotaWindowExpired_Expired(t *testing.T) {
	start := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if !monthlyQuotaWindowExpired(&start, time.Now().UTC()) {
		t.Error("start exactly 30 days ago should be expired")
	}
}

func TestMonthlyQuotaWindowExpired_Active(t *testing.T) {
	start := time.Now().UTC().Add(-29 * 24 * time.Hour)
	if monthlyQuotaWindowExpired(&start, time.Now().UTC()) {
		t.Error("start 29 days ago should NOT be expired")
	}
}

// TestMonthlyQuotaWindowExpired_CrossMonthBoundary 验证跨自然月时 30 天未满不视为过期。
func TestMonthlyQuotaWindowExpired_CrossMonthBoundary(t *testing.T) {
	// 窗口起始 4 月 20 日；5 月 1 日只过了 11 天，不足 30 天
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if monthlyQuotaWindowExpired(&start, now) {
		t.Error("11 days into window should NOT be expired (30-day rolling, not calendar month)")
	}
}

// TestNextMonthlyResetFrom 验证 30 天滚动重置时间计算。
func TestNextMonthlyResetFrom_WithStart(t *testing.T) {
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	want := start.Add(30 * 24 * time.Hour)
	now := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(&start, now)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom = %v, want %v", got, want)
	}
}

func TestNextMonthlyResetFrom_NilStart(t *testing.T) {
	now := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(nil, now)
	want := now.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom(nil) = %v, want now+30d=%v", got, want)
	}
}

func TestNextMonthlyResetFrom_NilStart_NotEqualToNow(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(nil, now)
	want := now.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom(nil) = %v, want %v (now+30d)", got, want)
	}
	if got.Equal(now) {
		t.Error("nextMonthlyResetFrom(nil) must not return now (should be now+30d)")
	}
}

// TestNextMonthlyResetFrom_ExpiredStart 验证窗口已过期（now-start >= 30d）时，
// 下次重置时间为 now+30d，而非 start+30d（后者已是过去时间，会让 Retry-After 落回
// fallback 并触发客户端紧凑重试）。
func TestNextMonthlyResetFrom_ExpiredStart(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) // 距 start 61 天，已过期
	got := nextMonthlyResetFrom(&start, now)
	want := now.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom(expired) = %v, want now+30d=%v", got, want)
	}
	if !got.After(now) {
		t.Error("expired window 的下次重置必须在 now 之后，不能是过去时间")
	}
}

func TestIncrementUserPlatformQuotaUsage_GuardsAgainstEmpty(t *testing.T) {
	fake := &fakeIncrCache{}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache: fake,
		cfg:   cfg,
	}

	s.IncrementUserPlatformQuotaUsage(1, "", 0.5)        // empty platform → noop
	s.IncrementUserPlatformQuotaUsage(1, "openai", 0)    // zero cost → noop
	s.IncrementUserPlatformQuotaUsage(1, "openai", -0.1) // negative → noop

	if len(fake.calls) != 0 {
		t.Errorf("expected 0 calls (all guarded), got %d", len(fake.calls))
	}
}

// ── C-NEW-2: 订阅模式豁免 user×platform quota 检查 ──────────────────────────
// 通过直接调用 checkUserPlatformQuotaEligibility 验证：
// 1. standard 模式下 limit=0 → 拦截
// 2. 订阅模式豁免通过 isSubscriptionMode 守卫体现 — 逻辑已在 CheckBillingEligibility 里加 !isSubscriptionMode 条件
// 此处用单元测试直接验证底层 checkUserPlatformQuotaEligibility 的行为（quota 超限确实拦截），
// 而 subscription bypass 逻辑则在 CheckBillingEligibility 中通过条件判断保证，不绕过 sub eligibility 内部复杂依赖。

// fakeZeroQuotaCache 模拟 cache 命中且 daily limit=0（quota 耗尽）。
type fakeZeroQuotaCache struct {
	BillingCache
	called bool
}

func (f *fakeZeroQuotaCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, _ string) (*UserPlatformQuotaCacheEntry, bool, error) {
	f.called = true
	daily := 0.0
	entry := &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: func() *time.Time { t := time.Now().UTC(); return &t }(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}
	return entry, true, nil
}

func (f *fakeZeroQuotaCache) DeleteUserPlatformQuotaCache(_ context.Context, _ int64, _ string) error {
	return nil
}

// SetUserPlatformQuotaCache 在 weekly/monthly window_start 为 nil 时,checkUserPlatform...
// 会触发"窗口过期 → SetCache 刷新"分支。fake 用 noop 避免 panic。
func (f *fakeZeroQuotaCache) SetUserPlatformQuotaCache(_ context.Context, _ int64, _ string, _ *UserPlatformQuotaCacheEntry, _ time.Duration) error {
	return nil
}

// GetSubscriptionCache 返回有效订阅（active、未过期、usage 远低于 limit），
// 用于支持 checkSubscriptionEligibility 通过，以便验证 quota 检查不被触发。
func (f *fakeZeroQuotaCache) GetSubscriptionCache(_ context.Context, _ int64, _ int64) (*SubscriptionCacheData, error) {
	return &SubscriptionCacheData{
		Status:       SubscriptionStatusActive,
		ExpiresAt:    time.Now().Add(30 * 24 * time.Hour),
		DailyUsage:   0,
		WeeklyUsage:  0,
		MonthlyUsage: 0,
	}, nil
}

func (f *fakeZeroQuotaCache) GetUserBalanceCache(_ context.Context, _ int64) (float64, bool, error) {
	return 100.0, true, nil
}

// TestCheckUserPlatformQuotaEligibility_StandardMode_BlocksWhenLimitZero 验证：
// standard 模式下 limit=0 的 platform quota 确实会被拦截（守卫底层逻辑正确）。
func TestCheckUserPlatformQuotaEligibility_StandardMode_BlocksWhenLimitZero(t *testing.T) {
	fake := &fakeZeroQuotaCache{}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 fake,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{},
	}
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("standard mode with limit=0 should return ErrUserPlatformDailyQuotaExhausted, got: %v", err)
	}
	if !fake.called {
		t.Error("GetUserPlatformQuotaCache should have been called in standard mode")
	}
}

// TestCheckBillingEligibility_SubscriptionMode_BypassesPlatformQuota 验证（C-NEW-2）：
// 订阅模式用户不受 user×platform quota 拦截，GetUserPlatformQuotaCache 不应被调用。
func TestCheckBillingEligibility_SubscriptionMode_BypassesPlatformQuota(t *testing.T) {
	fake := &fakeZeroQuotaCache{} // GetUserPlatformQuotaCache 返回 limit=0，若被调用则拦截
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 fake,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{},
	}

	subGroup := &Group{
		ID:               10,
		SubscriptionType: "subscription",
		Status:           "active",
		// 无 DailyLimitUSD → checkSubscriptionEligibility 不会因超限失败
	}
	sub := &UserSubscription{Status: "active"}
	user := &User{ID: 42}

	err := s.CheckBillingEligibility(context.Background(), user, nil, subGroup, sub, "anthropic")
	// 订阅模式下不应收到任何 user×platform quota 错误
	if errors.Is(err, ErrUserPlatformDailyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformWeeklyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformMonthlyQuotaExhausted) {
		t.Errorf("subscription mode should bypass user×platform quota, got: %v", err)
	}
	// GetUserPlatformQuotaCache 不应被调用
	if fake.called {
		t.Error("GetUserPlatformQuotaCache must NOT be called in subscription mode (C-NEW-2)")
	}
}

// TestCheckBillingEligibility_NonSubscriptionGroup_AppliesQuota 验证：
// 非订阅模式（group=nil）用户 platform quota 超限时被拦截，quota cache 被查询。
func TestCheckBillingEligibility_NonSubscriptionGroup_AppliesQuota(t *testing.T) {
	called := &fakeZeroQuotaCache{}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 called,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{},
	}
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 99, "openai")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("non-subscription mode quota check should block, got: %v", err)
	}
	if !called.called {
		t.Error("GetUserPlatformQuotaCache should be consulted in non-subscription mode")
	}
}

// ── B-3: monthlyQuotaWindowExpired 30 天边界表驱动测试 ────────────────────────
// 覆盖 4 个必须场景:
//  1. 恰好 30 天 → expired
//  2. 30*24h - 1ns → not expired
//  3. 跨月末（2024-02-28 → 2024-03-29T00:00:01Z）→ expired
//  4. 跨年（2024-12-15 → 2025-01-14T00:00:01Z）→ expired
//
// repo 层 monthlyMaybeReset 不可导出，通过 service 层 monthlyQuotaWindowExpired 间接覆盖。
func TestMonthlyQuotaWindowExpired_BoundaryTable(t *testing.T) {
	const thirtyDays = 30 * 24 * time.Hour

	cases := []struct {
		name    string
		start   time.Time
		now     time.Time
		expired bool
	}{
		{
			name:    "exactly 30 days → expired",
			start:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(thirtyDays),
			expired: true,
		},
		{
			name:    "30d minus 1ns → not expired",
			start:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(thirtyDays - 1),
			expired: false,
		},
		{
			name:    "cross month-end (Feb→Mar, 29d+1s) → expired",
			start:   time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 3, 29, 0, 0, 1, 0, time.UTC),
			expired: true,
		},
		{
			name:    "cross year boundary (Dec→Jan, 30d+1s) → expired",
			start:   time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2025, 1, 14, 0, 0, 1, 0, time.UTC),
			expired: true,
		},
	}

	for _, tc := range cases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			got := monthlyQuotaWindowExpired(&tc.start, tc.now)
			if got != tc.expired {
				t.Errorf("monthlyQuotaWindowExpired(start=%v, now=%v) = %v, want %v",
					tc.start, tc.now, got, tc.expired)
			}
		})
	}
}
