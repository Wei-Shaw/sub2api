//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

// --- fetcher 依赖 stub ---

type stubMonitorUsageSource struct {
	usage *UsageInfo
	err   error
	// block 非 nil 时 GetUsage 阻塞在该 channel 上，用于并发/singleflight 测试。
	block chan struct{REDACTED

	mu      sync.Mutex
	calls   int
	lastCtx context.Context
REDACTED

func (s *stubMonitorUsageSource) GetUsage(ctx context.Context, accountID int64, force ...bool) (*UsageInfo, error) {
	s.mu.Lock()
	s.calls++
	s.lastCtx = ctx
	s.mu.Unlock()
	if s.block != nil {
		<-s.block
REDACTED
	return s.usage, s.err
REDACTED

func (s *stubMonitorUsageSource) getCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
REDACTED

type stubMonitorCNQuotaSource struct {
	result *CNProviderQuotaProbeResult
	err    error
	calls  int
REDACTED

func (s *stubMonitorCNQuotaSource) QueryUsage(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	s.calls++
	return s.result, s.err
REDACTED

type stubMonitorCNBalanceSource struct {
	result *CNProviderBalanceResult
	err    error
	calls  int
REDACTED

func (s *stubMonitorCNBalanceSource) QueryBalance(ctx context.Context, accountID int64) (*CNProviderBalanceResult, error) {
	s.calls++
	return s.result, s.err
REDACTED

type stubMonitorAccountSource struct {
	accounts map[int64]*Account
	err      error
	calls    int
REDACTED

func (s *stubMonitorAccountSource) GetByID(ctx context.Context, id int64) (*Account, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
REDACTED
	return s.accounts[id], nil
REDACTED

func newQuotaFetcherTestSetup(t *testing.T) (*ChannelMonitorQuotaFetcher, *stubMonitorUsageSource, *stubMonitorCNQuotaSource, *stubMonitorCNBalanceSource, *stubMonitorAccountSource) {
REDACTED
	usage := &stubMonitorUsageSource{REDACTED
	cnQuota := &stubMonitorCNQuotaSource{REDACTED
	cnBalance := &stubMonitorCNBalanceSource{REDACTED
	accounts := &stubMonitorAccountSource{accounts: make(map[int64]*Account)REDACTED
	fetcher := &ChannelMonitorQuotaFetcher{
		usage:     usage,
		cnQuota:   cnQuota,
		cnBalance: cnBalance,
		accounts:  accounts,
		cache:     make(map[int64]monitorQuotaCacheEntry),
REDACTED
	return fetcher, usage, cnQuota, cnBalance, accounts
REDACTED

// --- 分派 ---

func TestQuotaFetcher_OverseasAccountUsesUsageService(t *testing.T) {
	fetcher, usage, _, cnQuota, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[7] = &Account{ID: 7, Platform: domain.PlatformAnthropicREDACTED
	resets := time.Now().Add(2 * time.Hour).UTC()
	usage.usage = &UsageInfo{
		FiveHour:         &UsageProgress{Utilization: 42.5, UsedRequests: 17, LimitRequests: 40, ResetsAt: &resetsREDACTED,
		SevenDay:         &UsageProgress{Utilization: 10REDACTED,
		SubscriptionTier: "PRO",
REDACTED

	snapshot := fetcher.Fetch(context.Background(), 7)

	require.True(t, snapshot.Success)
	require.Equal(t, "usage", snapshot.Source)
	require.Equal(t, "PRO", snapshot.PlanLevel)
	require.False(t, snapshot.CredentialInvalid)
	require.Empty(t, snapshot.Error)
	require.Len(t, snapshot.Tiers, 2)

	fiveHour := snapshot.Tiers[0]
	require.Equal(t, "5h", fiveHour.Window)
	require.Empty(t, fiveHour.Label)
	require.InDelta(t, 42.5, fiveHour.UsedPercent, 0.001)
	require.Equal(t, float64(17), fiveHour.Used)
	require.Equal(t, float64(40), fiveHour.Limit)
	require.NotEmpty(t, fiveHour.ResetAt)

	require.Equal(t, "7d", snapshot.Tiers[1].Window)
	require.Equal(t, 1, usage.getCalls())
	require.Equal(t, 0, cnQuota.calls)
REDACTED

func TestQuotaFetcher_CodingPlanAccountUsesCNQuota(t *testing.T) {
	fetcher, _, cnQuota, cnBalance, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[9] = &Account{
		ID:          9,
		Platform:    domain.PlatformKimi,
REDACTED"account_mode": AccountModeCodingREDACTED,
REDACTED
	cnQuota.result = &CNProviderQuotaProbeResult{
		Success:         true,
		CredentialValid: true,
		PlanLevel:       "",
		Tiers: []CNQuotaTier{
			{Window: "5h", UsedPercent: 33.3, ResetAt: "2026-08-18T06:00:00Z"REDACTED,
			{Window: "weekly", UsedPercent: 12REDACTED,
	REDACTED,
REDACTED

	snapshot := fetcher.Fetch(context.Background(), 9)

	require.True(t, snapshot.Success)
	require.Equal(t, "cn_quota", snapshot.Source)
	require.Len(t, snapshot.Tiers, 2)
	require.Equal(t, "5h", snapshot.Tiers[0].Window)
	require.InDelta(t, 33.3, snapshot.Tiers[0].UsedPercent, 0.001)
	require.Equal(t, "weekly", snapshot.Tiers[1].Window)
	require.Equal(t, 1, cnQuota.calls)
	require.Equal(t, 0, cnBalance.calls)
REDACTED

func TestQuotaFetcher_PayGAccountUsesCNBalance(t *testing.T) {
	fetcher, _, _, cnBalance, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[11] = &Account{
		ID:          11,
		Platform:    domain.PlatformDeepseek,
REDACTED"account_mode": AccountModePayGREDACTED,
REDACTED
	cnBalance.result = &CNProviderBalanceResult{
		Success:  true,
		Balance:  12.34,
		Currency: "CNY",
		Balances: []CNProviderBalanceEntry{
			{Currency: "CNY", Balance: 12.34REDACTED,
			{Currency: "USD", Balance: 1.5REDACTED,
	REDACTED,
REDACTED

	snapshot := fetcher.Fetch(context.Background(), 11)

	require.True(t, snapshot.Success)
	require.Equal(t, "cn_balance", snapshot.Source)
	require.NotNil(t, snapshot.Balance)
	require.InDelta(t, 12.34, *snapshot.Balance, 0.001)
	require.Equal(t, "CNY", snapshot.Currency)
	require.Len(t, snapshot.Balances, 2)
	require.Equal(t, "USD", snapshot.Balances[1].Currency)
	require.Empty(t, snapshot.Error)
REDACTED

// --- 失败路径（Fetch 永不返回 error） ---

func TestQuotaFetcher_AccountMissingYieldsLinkedAccountSnapshot(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.err = errors.New("not found")

	snapshot := fetcher.Fetch(context.Background(), 404)

	require.False(t, snapshot.Success)
	require.Equal(t, "linked account not found", snapshot.Error)
	require.Equal(t, 0, usage.getCalls()) // 未走到数据源
REDACTED

func TestQuotaFetcher_UsageAuthErrorMarksCredentialInvalid(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[3] = &Account{ID: 3, Platform: domain.PlatformOpenAIREDACTED
	usage.err = errors.New("API returned 401: unauthorized")

	snapshot := fetcher.Fetch(context.Background(), 3)

	require.False(t, snapshot.Success)
	require.True(t, snapshot.CredentialInvalid)
	require.Contains(t, snapshot.Error, "401")
REDACTED

// 值通道失败：antigravity/grok 等平台 err==nil 但错误降级在 UsageInfo 字段里，
// 必须识别为失败快照，否则会被误判为 operational。
func TestQuotaFetcher_UsageValueChannelFailureYieldsFailureSnapshot(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)

	// 凭据失效（401 语义）→ failed。
	accounts.accounts[3] = &Account{ID: 3, Platform: domain.PlatformAnthropicREDACTED
	usage.usage = &UsageInfo{Error: "usage API error: HTTP 401", ErrorCode: errorCodeUnauthenticated, NeedsReauth: trueREDACTED
	snapshot := fetcher.Fetch(context.Background(), 3)
	require.False(t, snapshot.Success)
	require.True(t, snapshot.CredentialInvalid)
	require.Contains(t, snapshot.Error, "401")
	require.Equal(t, MonitorStatusFailed, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)

	// 限流等非凭据失败 → error（而非 operational）。
	accounts.accounts[13] = &Account{ID: 13, Platform: domain.PlatformAnthropicREDACTED
	usage.usage = &UsageInfo{Error: "usage API error: HTTP 429", ErrorCode: errorCodeRateLimitedREDACTED
	snapshot = fetcher.Fetch(context.Background(), 13)
	require.False(t, snapshot.Success)
	require.False(t, snapshot.CredentialInvalid)
	require.Contains(t, snapshot.Error, "429")
	require.Equal(t, MonitorStatusError, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)

	// grok 已知未知态（尚未观测到计费/限流头）不算失败。
	accounts.accounts[14] = &Account{ID: 14, Platform: domain.PlatformGrokREDACTED
	usage.usage = &UsageInfo{ErrorCode: "quota_unknown", Error: "Grok quota is unknown until billing is probed"REDACTED
	snapshot = fetcher.Fetch(context.Background(), 14)
	require.True(t, snapshot.Success)
	require.Empty(t, snapshot.Error)
	require.Empty(t, snapshot.Tiers)
	require.Equal(t, MonitorStatusOperational, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
REDACTED

func TestUsageFailureInfo_ClassificationMatrix(t *testing.T) {
	cases := []struct {
		name              string
		usage             *UsageInfo
		failed            bool
		credentialInvalid bool
		msg               string
REDACTED{
		{name: "nil usage", usage: nilREDACTED,
		{name: "healthy empty", usage: &UsageInfo{REDACTEDREDACTED,
		{name: "error text only", usage: &UsageInfo{Error: "boom"REDACTED, failed: true, msg: "boom"REDACTED,
		{name: "needs reauth", usage: &UsageInfo{NeedsReauth: trueREDACTED, failed: true, credentialInvalid: true, msg: "usage fetch failed"REDACTED,
		{name: "banned", usage: &UsageInfo{IsBanned: trueREDACTED, failed: true, credentialInvalid: true, msg: "usage fetch failed"REDACTED,
		{name: "forbidden with reason", usage: &UsageInfo{IsForbidden: true, ForbiddenReason: "usage limited"REDACTED, failed: true, credentialInvalid: true, msg: "usage limited"REDACTED,
		{name: "error code unauthenticated", usage: &UsageInfo{ErrorCode: errorCodeUnauthenticatedREDACTED, failed: true, credentialInvalid: true, msg: errorCodeUnauthenticatedREDACTED,
		{name: "error code forbidden", usage: &UsageInfo{ErrorCode: errorCodeForbiddenREDACTED, failed: true, credentialInvalid: true, msg: errorCodeForbiddenREDACTED,
		{name: "error code rate limited", usage: &UsageInfo{ErrorCode: errorCodeRateLimitedREDACTED, failed: true, msg: errorCodeRateLimitedREDACTED,
		{name: "error code network error", usage: &UsageInfo{ErrorCode: errorCodeNetworkErrorREDACTED, failed: true, msg: errorCodeNetworkErrorREDACTED,
		{name: "grok quota unknown exempted", usage: &UsageInfo{ErrorCode: "quota_unknown", Error: "Grok quota is unknown until billing is probed"REDACTEDREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failed, credentialInvalid, msg := usageFailureInfo(tc.usage)
			require.Equal(t, tc.failed, failed)
			require.Equal(t, tc.credentialInvalid, credentialInvalid)
			if tc.msg != "" {
				require.Equal(t, tc.msg, msg)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestQuotaFetcher_CNQuotaCredentialInvalidFlagPropagates(t *testing.T) {
	fetcher, _, cnQuota, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[5] = &Account{
		ID:          5,
		Platform:    domain.PlatformZhipu,
REDACTED"account_mode": AccountModeCodingREDACTED,
REDACTED
	cnQuota.result = &CNProviderQuotaProbeResult{Success: false, CredentialValid: false, Error: "api key expired"REDACTED

	snapshot := fetcher.Fetch(context.Background(), 5)

	require.False(t, snapshot.Success)
	require.True(t, snapshot.CredentialInvalid)
	require.Equal(t, "api key expired", snapshot.Error)
REDACTED

func TestQuotaFetcher_CNBalanceHTTP403MarksCredentialInvalid(t *testing.T) {
	fetcher, _, _, cnBalance, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[6] = &Account{ID: 6, Platform: domain.PlatformKimiREDACTED
	cnBalance.result = &CNProviderBalanceResult{Success: false, StatusCode: 403, Error: "forbidden"REDACTED

	snapshot := fetcher.Fetch(context.Background(), 6)

	require.False(t, snapshot.Success)
	require.True(t, snapshot.CredentialInvalid)
REDACTED

func TestQuotaFetcher_NilDependenciesProduceErrorSnapshots(t *testing.T) {
	// fetcher 本体为 nil：直接降级为错误快照，不 panic。
	var nilFetcher *ChannelMonitorQuotaFetcher
	snapshot := nilFetcher.Fetch(context.Background(), 1)
	require.False(t, snapshot.Success)
	require.Equal(t, "quota fetcher is not configured", snapshot.Error)

	// 数据源缺失：账号能加载，但对应服务未注入。
	fetcher, _, _, _, accounts := newQuotaFetcherTestSetup(t)
	fetcher.usage = nil
	accounts.accounts[2] = &Account{ID: 2, Platform: domain.PlatformOpenAIREDACTED
	snapshot = fetcher.Fetch(context.Background(), 2)
	require.False(t, snapshot.Success)
	require.Contains(t, snapshot.Error, "not configured")
REDACTED

// --- TTL 缓存 ---

func TestQuotaFetcher_CachesSuccessSnapshotPerAccount(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[8] = &Account{ID: 8, Platform: domain.PlatformOpenAIREDACTED
	usage.usage = &UsageInfo{FiveHour: &UsageProgress{Utilization: 10REDACTEDREDACTED

	for i := 0; i < 3; i++ {
		snapshot := fetcher.Fetch(context.Background(), 8)
		require.True(t, snapshot.Success)
REDACTED
	require.Equal(t, 1, usage.getCalls(), "success snapshots should be served from cache")

	// 缓存过期后重新拉取。
	fetcher.mu.Lock()
	entry := fetcher.cache[8]
	entry.expiry = time.Now().Add(-time.Second)
	fetcher.cache[8] = entry
	fetcher.mu.Unlock()

	_ = fetcher.Fetch(context.Background(), 8)
	require.Equal(t, 2, usage.getCalls())
REDACTED

func TestQuotaFetcher_CachesFailureSnapshotWithShortTTL(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[4] = &Account{ID: 4, Platform: domain.PlatformOpenAIREDACTED
	usage.err = errors.New("boom")

	for i := 0; i < 2; i++ {
		snapshot := fetcher.Fetch(context.Background(), 4)
		require.False(t, snapshot.Success)
REDACTED
	require.Equal(t, 1, usage.getCalls(), "failure snapshots should be served from the short negative cache")

	// 失败快照的 TTL 是负缓存时长（而非成功 TTL）。
	fetcher.mu.Lock()
	entry := fetcher.cache[4]
	require.WithinDuration(t, entry.snapshot.FetchedAt.Add(monitorQuotaErrorCacheTTL), entry.expiry, time.Second)
	entry.expiry = time.Now().Add(-time.Second)
	fetcher.cache[4] = entry
	fetcher.mu.Unlock()

	_ = fetcher.Fetch(context.Background(), 4)
	require.Equal(t, 2, usage.getCalls(), "expired negative cache should refetch")
REDACTED

func TestQuotaFetcher_ConcurrentFetchesShareSingleFlight(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[12] = &Account{ID: 12, Platform: domain.PlatformOpenAIREDACTED
	usage.usage = &UsageInfo{FiveHour: &UsageProgress{Utilization: 10REDACTEDREDACTED
	usage.block = make(chan struct{REDACTED)

	var wg sync.WaitGroup
	snapshots := make([]*domain.MonitorQuotaSnapshot, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			snapshots[idx] = fetcher.Fetch(context.Background(), 12)
	REDACTED(i)
REDACTED

	// 上游被 block 卡住时，5 个并发 Fetch 应只产生 1 次真实查询。
	require.Eventually(t, func() bool { return usage.getCalls() == 1 REDACTED,
		5*time.Second, 10*time.Millisecond)
	close(usage.block)
	wg.Wait()

	for _, snapshot := range snapshots {
		require.NotNil(t, snapshot)
		require.True(t, snapshot.Success)
REDACTED
	require.Equal(t, 1, usage.getCalls())

	// 成功快照已缓存：再取一次仍不打上游。
	_ = fetcher.Fetch(context.Background(), 12)
	require.Equal(t, 1, usage.getCalls())
REDACTED

// --- UsageInfo → tiers 归一 ---

func TestUsageQuotaTiers_MapsAllWindowKinds(t *testing.T) {
	limit := int64(1000)
	remaining := int64(400)
	resetUnix := int64(1777283883)
	usage := &UsageInfo{
		FiveHour:          &UsageProgress{Utilization: 50REDACTED,
		SevenDay:          &UsageProgress{Utilization: 60REDACTED,
		SevenDaySonnet:    &UsageProgress{Utilization: 70REDACTED,
		SevenDayFable:     &UsageProgress{Utilization: 80REDACTED,
		ThirtyDay:         &UsageProgress{Utilization: 20REDACTED,
		GeminiSharedDaily: &UsageProgress{Utilization: 11REDACTED,
		GeminiProDaily:    &UsageProgress{Utilization: 22REDACTED,
		GeminiFlashDaily:  &UsageProgress{Utilization: 33REDACTED,
		GrokRequestQuota:  &xai.QuotaWindow{Limit: &limit, Remaining: &remaining, ResetUnix: &resetUnixREDACTED,
		GrokTokenQuota:    &xai.QuotaWindow{Limit: &limit, Remaining: &remaining, ResetAt: "2026-08-19T00:00:00Z"REDACTED,
		AntigravityQuota: map[string]*AntigravityModelQuota{
			"gemini-3-pro":   {Utilization: 45REDACTED,
			"gemini-3-flash": {Utilization: 55REDACTED,
	REDACTED,
REDACTED

	tiers := usageQuotaTiers(usage)

	// 5h/7d/7d-sonnet/7d-fable/30d + gemini×3 + grok×2 + antigravity×2
	require.Len(t, tiers, 12)

	byKey := make(map[string]domain.MonitorQuotaTier, len(tiers))
	for _, tier := range tiers {
		key := tier.Window
		if tier.Label != "" {
			key = tier.Window + "/" + tier.Label
	REDACTED
		byKey[key] = tier
REDACTED

	require.Contains(t, byKey, "5h")
	require.Contains(t, byKey, "7d")
	require.Contains(t, byKey, "7d-sonnet")
	require.Contains(t, byKey, "7d-fable")
	require.Contains(t, byKey, "30d")
	require.Contains(t, byKey, "daily/shared")
	require.Contains(t, byKey, "daily/pro")
	require.Contains(t, byKey, "daily/flash")
	require.Contains(t, byKey, "daily/requests")
	require.Contains(t, byKey, "daily/tokens")
	require.Contains(t, byKey, "total/gemini-3-pro")
	require.Contains(t, byKey, "total/gemini-3-flash")

	// grok requests 窗口：used = limit - remaining，百分比 60%。
	requests := byKey["daily/requests"]
	require.Equal(t, float64(600), requests.Used)
	require.Equal(t, float64(1000), requests.Limit)
	require.InDelta(t, 60.0, requests.UsedPercent, 0.001)
	require.NotEmpty(t, requests.ResetAt, "ResetUnix should fall back to RFC3339")

	tokens := byKey["daily/tokens"]
	require.Equal(t, "2026-08-19T00:00:00Z", tokens.ResetAt)
REDACTED

func TestUsageQuotaTiers_NilAndEmptyInputs(t *testing.T) {
	require.Nil(t, usageQuotaTiers(nil))
	require.Nil(t, usageQuotaTiers(&UsageInfo{REDACTED))

	// Grok 窗口 limit<=0 时跳过，避免除零。
	var zero int64
	tiers := usageQuotaTiers(&UsageInfo{
		GrokRequestQuota: &xai.QuotaWindow{Limit: &zero, Remaining: &zeroREDACTED,
REDACTED)
	require.Nil(t, tiers)
REDACTED

// --- 状态推导 ---

func TestDeriveQuotaCheckResult_StatusMatrix(t *testing.T) {
	now := time.Now()

	healthy := &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 40REDACTEDREDACTEDREDACTED
	res := deriveQuotaCheckResult(healthy, "quota", now)
	require.Equal(t, MonitorStatusOperational, res.Status)
	require.Equal(t, "quota", res.Model)
	require.Empty(t, res.Message)

	highUsage := &domain.MonitorQuotaSnapshot{Success: true, Tiers: []domain.MonitorQuotaTier{
		{Window: "5h", UsedPercent: 30REDACTED,
		{Window: "daily", Label: "pro", UsedPercent: 95REDACTED,
REDACTEDREDACTED
	res = deriveQuotaCheckResult(highUsage, "quota", now)
	require.Equal(t, MonitorStatusDegraded, res.Status)
	require.Contains(t, res.Message, "pro/daily")
	require.Contains(t, res.Message, "95.0%")

	balance := -0.5
	depleted := &domain.MonitorQuotaSnapshot{Success: true, Balance: &balance, Currency: "CNY"REDACTED
	res = deriveQuotaCheckResult(depleted, "quota", now)
	require.Equal(t, MonitorStatusDegraded, res.Status)
	require.Contains(t, res.Message, "balance depleted")

	invalid := &domain.MonitorQuotaSnapshot{Success: false, CredentialInvalid: true, Error: "401 unauthorized"REDACTED
	res = deriveQuotaCheckResult(invalid, "quota", now)
	require.Equal(t, MonitorStatusFailed, res.Status)

	unlinked := &domain.MonitorQuotaSnapshot{Success: false, Error: "linked account not found"REDACTED
	res = deriveQuotaCheckResult(unlinked, "quota", now)
	require.Equal(t, MonitorStatusDegraded, res.Status)

	other := &domain.MonitorQuotaSnapshot{Success: false, Error: "connection refused"REDACTED
	res = deriveQuotaCheckResult(other, "quota", now)
	require.Equal(t, MonitorStatusError, res.Status)
	require.Equal(t, "connection refused", res.Message)

	res = deriveQuotaCheckResult(nil, "quota", now)
	require.Equal(t, MonitorStatusError, res.Status)
REDACTED
