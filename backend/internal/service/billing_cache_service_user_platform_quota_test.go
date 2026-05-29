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
REDACTED

type incrCall struct {
	userID   int64
	platform string
	cost     float64
	ttl      time.Duration
REDACTED

func (f *fakeIncrCache) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration) error {
	f.calls = append(f.calls, incrCall{userID, platform, cost, ttlREDACTED)
	return nil
REDACTED

// IncrementUserPlatformQuotaUsage 已改为同步直写,不再走 worker。
// 测试验证:同步调用立即调到 cache.IncrUserPlatformQuotaUsageCache。
func TestIncrementUserPlatformQuotaUsage_SyncCallsCache(t *testing.T) {
	fake := &fakeIncrCache{REDACTED
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 120

	s := &BillingCacheService{
		cache: fake,
		cfg:   cfg,
REDACTED

	s.IncrementUserPlatformQuotaUsage(101, "anthropic", 0.25)
	s.IncrementUserPlatformQuotaUsage(101, "openai", 0.50)

	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 incr calls, got %d", len(fake.calls))
REDACTED
	if fake.calls[0] != (incrCall{101, "anthropic", 0.25, 120 * time.SecondREDACTED) {
		t.Errorf("call[0] = %+v", fake.calls[0])
REDACTED
	if fake.calls[1] != (incrCall{101, "openai", 0.50, 120 * time.SecondREDACTED) {
		t.Errorf("call[1] = %+v", fake.calls[1])
REDACTED
REDACTED

// ── T6 tests: checkUserPlatformQuotaEligibility ──────────────────────────────

// fakeQuotaRepo 实现 UserPlatformQuotaRepository 最小子集
type fakeQuotaRepo struct {
	rec *UserPlatformQuotaRecord
REDACTED

func (f *fakeQuotaRepo) GetByUserPlatform(_ context.Context, _ int64, _ string) (*UserPlatformQuotaRecord, error) {
	return f.rec, nil
REDACTED

func (f *fakeQuotaRepo) BulkInsertInitial(_ context.Context, _ []UserPlatformQuotaRecord) error {
	return nil
REDACTED

func (f *fakeQuotaRepo) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
REDACTED

func (f *fakeQuotaRepo) ListByUser(_ context.Context, _ int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
REDACTED

func (f *fakeQuotaRepo) UpsertForUser(_ context.Context, _ int64, _ []UserPlatformQuotaRecord) error {
	return nil
REDACTED

func (f *fakeQuotaRepo) ResetExpiredWindow(_ context.Context, _ int64, _ string, _ string, _ time.Time) error {
	return nil
REDACTED

// fakeFullCache 同时支持 Get + Set + Incr + Delete。
// mu 保护 entry 和 deleteCalls，防止异步 goroutine 与主 goroutine 之间的 data race。
type fakeFullCache struct {
	BillingCache
	mu          sync.Mutex
	entry       *UserPlatformQuotaCacheEntry
	deleteCalls int
	setCalls    int           // SetUserPlatformQuotaCache 调用次数
	lastSetTTL  time.Duration // 最近一次 Set 的 ttl
	getErr      error         // 非 nil 时 Get 先返回 (nil,false,getErr)
	setErr      error         // 非 nil 时 Set 返回该 err(setCalls 仍+1)
REDACTED

// getDeleteCalls 线程安全地读取 deleteCalls。
func (f *fakeFullCache) getDeleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteCalls
REDACTED

// getEntry 线程安全地读取 entry。
func (f *fakeFullCache) getEntry() *UserPlatformQuotaCacheEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entry
REDACTED

// getSetCalls 线程安全地读取 setCalls。
func (f *fakeFullCache) getSetCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setCalls
REDACTED

// getLastSetTTL 线程安全地读取 lastSetTTL。
func (f *fakeFullCache) getLastSetTTL() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastSetTTL
REDACTED

func (f *fakeFullCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, _ string) (*UserPlatformQuotaCacheEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, false, f.getErr
REDACTED
	if f.entry == nil {
		return nil, false, nil
REDACTED
	return f.entry, true, nil
REDACTED

func (f *fakeFullCache) SetUserPlatformQuotaCache(_ context.Context, _ int64, _ string, e *UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
REDACTED
	f.entry = e
	f.lastSetTTL = ttl
	return nil
REDACTED

func (f *fakeFullCache) DeleteUserPlatformQuotaCache(_ context.Context, _ int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	f.entry = nil
	return nil
REDACTED

func newServiceForPreflight(t *testing.T, repo UserPlatformQuotaRepository, cache BillingCache) *BillingCacheService {
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	return &BillingCacheService{
		cache:                 cache,
		cfg:                   cfg,
		userPlatformQuotaRepo: repo,
REDACTED
REDACTED

// currentDayStart 返回全局时区当天 0 点（与生产 timezone.StartOfDay 同口径，确保窗口有效）。
func currentDayStart() *time.Time {
	s := timezone.StartOfDay(time.Now())
	return &s
REDACTED

func TestCheckUserPlatformQuotaEligibility_AllowsWhenUnderLimit(t *testing.T) {
	daily := 10.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    4.5,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("expected nil, got %v", err)
REDACTED
REDACTED

func TestCheckUserPlatformQuotaEligibility_DailyExhausted(t *testing.T) {
	daily := 5.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    5.0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("expected ErrUserPlatformDailyQuotaExhausted, got %v", err)
REDACTED
REDACTED

func TestCheckUserPlatformQuotaEligibility_NilLimitMeansUnlimited(t *testing.T) {
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic",
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    999,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
		// DailyLimitUSD nil → 无限额
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("nil limits should be unlimited, got %v", err)
REDACTED
REDACTED

func TestCheckUserPlatformQuotaEligibility_ZeroLimitImmediateBlock(t *testing.T) {
	zero := 0.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &zero,
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &zero,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("expected daily exhausted for limit=0, got %v", err)
REDACTED
REDACTED

func TestCheckUserPlatformQuotaEligibility_NoRecordMeansUnlimited(t *testing.T) {
	repo := &fakeQuotaRepo{rec: nilREDACTED
	cache := &fakeFullCache{REDACTED
	s := newServiceForPreflight(t, repo, cache)
	if err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Errorf("no record = unlimited, got %v", err)
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_OldSchemaCacheMissTriggersDB 验证旧版 entry（SchemaVersion=0）
// 触发 DB 回退路径，并在 DB 数据判断配额是否超限。
// DB record 需设置有效的 window_start，否则 quotaWindowExpired 会将 usage 归零（nil 窗口视为已过期）。
func TestCheckUserPlatformQuotaEligibility_OldSchemaCacheMissTriggersDB(t *testing.T) {
	daily := 5.0
	dayStart := currentDayStart()
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily, DailyUsageUSD: 6.0,
		DailyWindowStart: dayStart,
REDACTEDREDACTED
	// SchemaVersion=0（旧 entry），应走 DB 路径
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{DailyUsageUSD: 1.0REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("旧版 entry 应走 DB 路径并报 daily exhausted, got %v", err)
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_WindowExpiredInCache 验证 cache HIT 时若窗口已过期，usage 归零，用户放行。
func TestCheckUserPlatformQuotaEligibility_WindowExpiredInCache(t *testing.T) {
	daily := 5.0
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) // 远古窗口起始，肯定已过期
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    10.0, // 超限，但窗口已过期
		DailyLimitUSD:    &daily,
		DailyWindowStart: &past,
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if err != nil {
		t.Errorf("过期窗口应归零放行, got %v", err)
REDACTED
REDACTED

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
REDACTEDREDACTED
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    10.0, // 超限,但窗口已过期 → 应被本地清零后放行
		DailyLimitUSD:    &daily,
		DailyWindowStart: &past,
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTEDREDACTED
	s := newServiceForPreflight(t, repo, cache)

	// 本次 check 应放行(本地清零后 usage=0 < limit=5)
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if err != nil {
		t.Errorf("过期窗口应归零放行, got %v", err)
REDACTED

	// 验证 cache entry 已被刷新:usage 清零、limit 保留、window_start 更新到当前窗口
	refreshed := cache.getEntry()
	if refreshed == nil {
		t.Fatal("窗口过期后 cache entry 不应为 nil(应被 SetCache 覆盖,而非 Delete)")
REDACTED
	if refreshed.DailyUsageUSD != 0 {
		t.Errorf("刷新后 DailyUsageUSD = %v, want 0", refreshed.DailyUsageUSD)
REDACTED
	if refreshed.DailyLimitUSD == nil || *refreshed.DailyLimitUSD != daily {
		t.Errorf("刷新后 DailyLimitUSD = %v, want %v(保留)", refreshed.DailyLimitUSD, daily)
REDACTED
	if refreshed.SchemaVersion != UserPlatformQuotaCacheSchemaV1 {
		t.Errorf("刷新后 SchemaVersion = %d, want V1", refreshed.SchemaVersion)
REDACTED
	if refreshed.DailyWindowStart == nil || refreshed.DailyWindowStart.Equal(past) {
		t.Errorf("刷新后 DailyWindowStart = %v, 应更新到当前窗口而非保留 past=%v", refreshed.DailyWindowStart, past)
REDACTED
REDACTED

// ── T5 tests: QueueUpdateUserPlatformQuotaUsage ───────────────────────────────

// ── C-NEW-1: monthlyQuotaWindowExpired 30 天滚动测试 ─────────────────────────

func TestMonthlyQuotaWindowExpired_NilStart(t *testing.T) {
	if !monthlyQuotaWindowExpired(nil, time.Now().UTC()) {
		t.Error("nil start should be considered expired")
REDACTED
REDACTED

func TestMonthlyQuotaWindowExpired_Expired(t *testing.T) {
	start := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if !monthlyQuotaWindowExpired(&start, time.Now().UTC()) {
		t.Error("start exactly 30 days ago should be expired")
REDACTED
REDACTED

func TestMonthlyQuotaWindowExpired_Active(t *testing.T) {
	start := time.Now().UTC().Add(-29 * 24 * time.Hour)
	if monthlyQuotaWindowExpired(&start, time.Now().UTC()) {
		t.Error("start 29 days ago should NOT be expired")
REDACTED
REDACTED

// TestMonthlyQuotaWindowExpired_CrossMonthBoundary 验证跨自然月时 30 天未满不视为过期。
func TestMonthlyQuotaWindowExpired_CrossMonthBoundary(t *testing.T) {
	// 窗口起始 4 月 20 日；5 月 1 日只过了 11 天，不足 30 天
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if monthlyQuotaWindowExpired(&start, now) {
		t.Error("11 days into window should NOT be expired (30-day rolling, not calendar month)")
REDACTED
REDACTED

// TestNextMonthlyResetFrom 验证 30 天滚动重置时间计算。
func TestNextMonthlyResetFrom_WithStart(t *testing.T) {
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	want := start.Add(30 * 24 * time.Hour)
	now := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(&start, now)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom = %v, want %v", got, want)
REDACTED
REDACTED

func TestNextMonthlyResetFrom_NilStart(t *testing.T) {
	now := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(nil, now)
	want := now.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom(nil) = %v, want now+30d=%v", got, want)
REDACTED
REDACTED

func TestNextMonthlyResetFrom_NilStart_NotEqualToNow(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	got := nextMonthlyResetFrom(nil, now)
	want := now.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("nextMonthlyResetFrom(nil) = %v, want %v (now+30d)", got, want)
REDACTED
	if got.Equal(now) {
		t.Error("nextMonthlyResetFrom(nil) must not return now (should be now+30d)")
REDACTED
REDACTED

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
REDACTED
	if !got.After(now) {
		t.Error("expired window 的下次重置必须在 now 之后，不能是过去时间")
REDACTED
REDACTED

func TestIncrementUserPlatformQuotaUsage_GuardsAgainstEmpty(t *testing.T) {
	fake := &fakeIncrCache{REDACTED
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache: fake,
		cfg:   cfg,
REDACTED

	s.IncrementUserPlatformQuotaUsage(1, "", 0.5)        // empty platform → noop
	s.IncrementUserPlatformQuotaUsage(1, "openai", 0)    // zero cost → noop
	s.IncrementUserPlatformQuotaUsage(1, "openai", -0.1) // negative → noop

	if len(fake.calls) != 0 {
		t.Errorf("expected 0 calls (all guarded), got %d", len(fake.calls))
REDACTED
REDACTED

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
REDACTED

func (f *fakeZeroQuotaCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, _ string) (*UserPlatformQuotaCacheEntry, bool, error) {
	f.called = true
	daily := 0.0
	entry := &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: func() *time.Time { t := time.Now().UTC(); return &t REDACTED(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
REDACTED
	return entry, true, nil
REDACTED

func (f *fakeZeroQuotaCache) DeleteUserPlatformQuotaCache(_ context.Context, _ int64, _ string) error {
	return nil
REDACTED

// SetUserPlatformQuotaCache 在 weekly/monthly window_start 为 nil 时,checkUserPlatform...
// 会触发"窗口过期 → SetCache 刷新"分支。fake 用 noop 避免 panic。
func (f *fakeZeroQuotaCache) SetUserPlatformQuotaCache(_ context.Context, _ int64, _ string, _ *UserPlatformQuotaCacheEntry, _ time.Duration) error {
	return nil
REDACTED

// GetSubscriptionCache 返回有效订阅（active、未过期、usage 远低于 limit），
// 用于支持 checkSubscriptionEligibility 通过，以便验证 quota 检查不被触发。
func (f *fakeZeroQuotaCache) GetSubscriptionCache(_ context.Context, _ int64, _ int64) (*SubscriptionCacheData, error) {
	return &SubscriptionCacheData{
		Status:       SubscriptionStatusActive,
		ExpiresAt:    time.Now().Add(30 * 24 * time.Hour),
		DailyUsage:   0,
		WeeklyUsage:  0,
		MonthlyUsage: 0,
REDACTED, nil
REDACTED

func (f *fakeZeroQuotaCache) GetUserBalanceCache(_ context.Context, _ int64) (float64, bool, error) {
	return 100.0, true, nil
REDACTED

// TestCheckUserPlatformQuotaEligibility_StandardMode_BlocksWhenLimitZero 验证：
// standard 模式下 limit=0 的 platform quota 确实会被拦截（守卫底层逻辑正确）。
func TestCheckUserPlatformQuotaEligibility_StandardMode_BlocksWhenLimitZero(t *testing.T) {
	fake := &fakeZeroQuotaCache{REDACTED
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 fake,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{REDACTED,
REDACTED
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("standard mode with limit=0 should return ErrUserPlatformDailyQuotaExhausted, got: %v", err)
REDACTED
	if !fake.called {
		t.Error("GetUserPlatformQuotaCache should have been called in standard mode")
REDACTED
REDACTED

// TestCheckBillingEligibility_SubscriptionMode_BypassesPlatformQuota 验证（C-NEW-2）：
// 订阅模式用户不受 user×platform quota 拦截，GetUserPlatformQuotaCache 不应被调用。
func TestCheckBillingEligibility_SubscriptionMode_BypassesPlatformQuota(t *testing.T) {
	fake := &fakeZeroQuotaCache{REDACTED // GetUserPlatformQuotaCache 返回 limit=0，若被调用则拦截
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 fake,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{REDACTED,
REDACTED

	subGroup := &Group{
		ID:               10,
		SubscriptionType: "subscription",
		Status:           "active",
		// 无 DailyLimitUSD → checkSubscriptionEligibility 不会因超限失败
REDACTED
	sub := &UserSubscription{Status: "active"REDACTED
	user := &User{ID: 42REDACTED

	err := s.CheckBillingEligibility(context.Background(), user, nil, subGroup, sub, "anthropic")
	// 订阅模式下不应收到任何 user×platform quota 错误
	if errors.Is(err, ErrUserPlatformDailyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformWeeklyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformMonthlyQuotaExhausted) {
		t.Errorf("subscription mode should bypass user×platform quota, got: %v", err)
REDACTED
	// GetUserPlatformQuotaCache 不应被调用
	if fake.called {
		t.Error("GetUserPlatformQuotaCache must NOT be called in subscription mode (C-NEW-2)")
REDACTED
REDACTED

// TestCheckBillingEligibility_NonSubscriptionGroup_AppliesQuota 验证：
// 非订阅模式（group=nil）用户 platform quota 超限时被拦截，quota cache 被查询。
func TestCheckBillingEligibility_NonSubscriptionGroup_AppliesQuota(t *testing.T) {
	called := &fakeZeroQuotaCache{REDACTED
	cfg := &config.Config{REDACTED
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	s := &BillingCacheService{
		cache:                 called,
		cfg:                   cfg,
		userPlatformQuotaRepo: &fakeQuotaRepo{REDACTED,
REDACTED
	err := s.checkUserPlatformQuotaEligibility(context.Background(), 99, "openai")
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Errorf("non-subscription mode quota check should block, got: %v", err)
REDACTED
	if !called.called {
		t.Error("GetUserPlatformQuotaCache should be consulted in non-subscription mode")
REDACTED
REDACTED

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
REDACTED{
		{
			name:    "exactly 30 days → expired",
			start:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(thirtyDays),
			expired: true,
	REDACTED,
		{
			name:    "30d minus 1ns → not expired",
			start:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(thirtyDays - 1),
			expired: false,
	REDACTED,
		{
			name:    "cross month-end (Feb→Mar, 29d+1s) → expired",
			start:   time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2024, 3, 29, 0, 0, 1, 0, time.UTC),
			expired: true,
	REDACTED,
		{
			name:    "cross year boundary (Dec→Jan, 30d+1s) → expired",
			start:   time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC),
			now:     time.Date(2025, 1, 14, 0, 0, 1, 0, time.UTC),
			expired: true,
	REDACTED,
REDACTED

	for _, tc := range cases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			got := monthlyQuotaWindowExpired(&tc.start, tc.now)
			if got != tc.expired {
				t.Errorf("monthlyQuotaWindowExpired(start=%v, now=%v) = %v, want %v",
					tc.start, tc.now, got, tc.expired)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_NoRow_WritesSentinel 验证:
// cache MISS + DB 无行时,回填 sentinel entry(三 limit 全 nil,三 window_start 全 non-nil),
// TTL = UserPlatformQuotaSentinelTTLSeconds,函数返回 nil(fail-open)。
func TestCheckUserPlatformQuotaEligibility_NoRow_WritesSentinel(t *testing.T) {
	repo := &fakeQuotaRepo{rec: nilREDACTED // DB 无行
	cache := &fakeFullCache{REDACTED        // entry=nil → Get 返回 MISS
	svc := newServiceForPreflight(t, repo, cache)
	svc.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds = 3600

	if err := svc.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Fatalf("expected nil (fail-open), got %v", err)
REDACTED
	if cache.getSetCalls() != 1 {
		t.Fatalf("expected 1 SetUserPlatformQuotaCache call for sentinel, got %d", cache.getSetCalls())
REDACTED
	sentinel := cache.getEntry()
	if sentinel == nil {
		t.Fatal("expected sentinel entry backfilled")
REDACTED
	if sentinel.DailyLimitUSD != nil || sentinel.WeeklyLimitUSD != nil || sentinel.MonthlyLimitUSD != nil {
		t.Errorf("sentinel must have all-nil limits")
REDACTED
	if sentinel.DailyWindowStart == nil || sentinel.WeeklyWindowStart == nil || sentinel.MonthlyWindowStart == nil {
		t.Errorf("sentinel must have non-nil window_start to avoid refresh churn")
REDACTED
	if sentinel.SchemaVersion != UserPlatformQuotaCacheSchemaV1 {
		t.Errorf("sentinel schema = %d, want V1", sentinel.SchemaVersion)
REDACTED
	if cache.getLastSetTTL() != 3600*time.Second {
		t.Errorf("sentinel ttl = %v, want 3600s", cache.getLastSetTTL())
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_RedisGetError_NoSentinelBackfill 验证:
// Redis GET 故障(cacheErr!=nil)+ DB 无行时,不应回填 sentinel(与 "Redis 故障时不回填" 一致),且 fail-open。
func TestCheckUserPlatformQuotaEligibility_RedisGetError_NoSentinelBackfill(t *testing.T) {
	repo := &fakeQuotaRepo{rec: nilREDACTED
	cache := &fakeFullCache{getErr: errors.New("redis get down")REDACTED
	svc := newServiceForPreflight(t, repo, cache)
	svc.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds = 3600

	if err := svc.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Fatalf("redis 故障应 fail-open, got %v", err)
REDACTED
	if cache.getSetCalls() != 0 {
		t.Errorf("redis-get-error 时不应回填 sentinel, got %d set calls", cache.getSetCalls())
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_NoRow_SentinelSetFailsFailOpen 验证:
// sentinel SET 失败时 fail-open(返回 nil)且计 metric。
func TestCheckUserPlatformQuotaEligibility_NoRow_SentinelSetFailsFailOpen(t *testing.T) {
	before := userPlatformQuotaSentinelSetCacheErrorTotal.Load()
	repo := &fakeQuotaRepo{rec: nilREDACTED
	cache := &fakeFullCache{setErr: errors.New("redis set timeout")REDACTED
	svc := newServiceForPreflight(t, repo, cache)
	svc.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds = 3600

	if err := svc.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Fatalf("sentinel set 失败应 fail-open, got %v", err)
REDACTED
	if cache.getSetCalls() != 1 {
		t.Errorf("应尝试 set sentinel 恰好一次, got %d", cache.getSetCalls())
REDACTED
	if got := userPlatformQuotaSentinelSetCacheErrorTotal.Load() - before; got != 1 {
		t.Errorf("set 失败应使 metric +1, got delta %d", got)
REDACTED
REDACTED

// TestCheckUserPlatformQuotaEligibility_SentinelCrossDay_NoRefresh 验证:
// 命中 sentinel(三 limit 全 nil)且跨窗口(daily/weekly 过期)时,不应触发 refresh SetCache
// (否则会把短 sentinel TTL 误升级为 quota cache 默认 86400s)。
func TestCheckUserPlatformQuotaEligibility_SentinelCrossDay_NoRefresh(t *testing.T) {
	yesterday := timezone.StartOfDay(time.Now().AddDate(0, 0, -1))
	lastWeek := timezone.StartOfWeek(time.Now().AddDate(0, 0, -7))
	monthAgoOK := time.Now().AddDate(0, 0, -5) // <30d, monthly 不过期
	sentinel := &UserPlatformQuotaCacheEntry{
		SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
		DailyWindowStart:   &yesterday, // 跨日 → daily windowExpired = true
		WeeklyWindowStart:  &lastWeek,  // 跨周 → weekly windowExpired = true
		MonthlyWindowStart: &monthAgoOK,
		// limits 全 nil → sentinel
REDACTED
	cache := &fakeFullCache{entry: sentinelREDACTED // entry 非 nil → Get HIT
	svc := newServiceForPreflight(t, &fakeQuotaRepo{REDACTED, cache)

	if err := svc.checkUserPlatformQuotaEligibility(context.Background(), 1, "anthropic"); err != nil {
		t.Fatalf("sentinel = no limit, expected nil, got %v", err)
REDACTED
	if cache.getSetCalls() != 0 {
		t.Errorf("sentinel cross-window must NOT trigger refresh SetCache, got %d calls", cache.getSetCalls())
REDACTED
REDACTED
