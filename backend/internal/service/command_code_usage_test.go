//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// commandCodeAlphaUpstream 按路径返回 /alpha 接口响应，并记录请求。
type commandCodeAlphaUpstream struct {
	mu        sync.Mutex
	responses map[string]commandCodeAlphaResponse
	requests  []*http.Request
}

type commandCodeAlphaResponse struct {
	status int
	body   string
}

func (u *commandCodeAlphaUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.mu.Unlock()
	resp, ok := u.responses[req.URL.Path]
	if !ok {
		resp = commandCodeAlphaResponse{status: http.StatusNotFound, body: `{"error":"not found"}`}
	}
	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (u *commandCodeAlphaUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func (u *commandCodeAlphaUpstream) request(path string) *http.Request {
	for _, req := range u.requests {
		if req.URL.Path == path {
			return req
		}
	}
	return nil
}

const (
	commandCodeTestCredits = `{
		"credits": {"belowThreshold": false, "monthlyCredits": "34.9", "purchasedCredits": 0, "freeCredits": 0.1},
		"windowLimits": {
			"limited": true,
			"fiveHour": {"used": 1.5, "cap": 14, "resetAt": 1790000000000},
			"weekly": {"used": 35, "cap": 35, "resetAt": 1790400000000}
		}
	}`
	commandCodeTestSubscription = `{"data": {"planId": "individual-goat", "status": "active",
		"currentPeriodStart": "2026-09-01T00:00:00Z", "currentPeriodEnd": "2026-10-01T00:00:00Z"}}`
	commandCodeTestSummary = `{"totalCost": 40, "totalMonthlyCredits": 35.1}`
)

func commandCodeUsageAccount() *Account {
	account := commandCodeTestAccount(77)
	account.Concurrency = 1
	return account
}

func newCommandCodeAlphaUpstream(whoami string) *commandCodeAlphaUpstream {
	return &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		"/alpha/whoami":                {status: http.StatusOK, body: whoami},
		"/alpha/billing/credits":       {status: http.StatusOK, body: commandCodeTestCredits},
		"/alpha/billing/subscriptions": {status: http.StatusOK, body: commandCodeTestSubscription},
		"/alpha/usage/summary":         {status: http.StatusOK, body: commandCodeTestSummary},
	}}
}

func TestCommandCodeQuotaProbeParsesWindowsAndBalance(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{"user": {"userName": "alice"}, "org": null}`)
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{})

	result, err := svc.QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.Persisted)
	require.Equal(t, PlatformCommandCode, result.Provider)
	require.Equal(t, "GOAT", result.PlanLevel)
	require.Equal(t, []CNQuotaTier{
		{Window: "5h", UsedPercent: 10.71, ResetAt: time.UnixMilli(1790000000000).UTC().Format(time.RFC3339)},
		{Window: "weekly", UsedPercent: 100, ResetAt: time.UnixMilli(1790400000000).UTC().Format(time.RFC3339)},
		{Window: "monthly", UsedPercent: 50.14, ResetAt: "2026-10-01T00:00:00Z"},
	}, result.Tiers)
	require.NotNil(t, result.Balance)
	require.InDelta(t, 35.0, result.Balance.Balance, 1e-9)
	require.Equal(t, "USD", result.Balance.Currency)

	// 个人账户不带 orgId；本周期用量从订阅周期起点统计。
	for _, req := range upstream.requests {
		require.Equal(t, "Bearer user_test_key", req.Header.Get("Authorization"))
		require.Equal(t, "api.commandcode.ai", req.URL.Host)
		require.False(t, req.URL.Query().Has("orgId"), req.URL.String())
	}
	require.Equal(t, "2026-09-01T00:00:00Z", upstream.request("/alpha/usage/summary").URL.Query().Get("since"))

	require.Len(t, repo.extraWrites, 1)
	extra := repo.extraWrites[0]
	require.Equal(t, 10.71, extra["command_code_5h_used_percent"])
	require.Equal(t, 100.0, extra["command_code_weekly_used_percent"])
	require.Equal(t, 50.14, extra["command_code_monthly_used_percent"])
	require.Equal(t, "2026-10-01T00:00:00Z", extra["command_code_monthly_reset_at"])
	require.InDelta(t, 35.0, extra["command_code_balance"], 1e-9)
	require.Equal(t, "USD", extra["command_code_balance_currency"])
	require.Equal(t, 0.0, extra["command_code_purchased_credits"])
	require.Equal(t, false, extra["command_code_balance_low"])
}

func TestCommandCodeQuotaProbePassesOrgID(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{"user": {"userName": "alice"}, "org": {"id": "9a0f5d0e-6d2c-4f36-9c55-0b8b0c0d1e2f", "login": "team"}}`)
	svc := NewCNProviderQuotaService(&cnBalanceProbeRepo{account: account}, nil, upstream, &config.Config{})

	_, err := svc.QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	for _, path := range []string{"/alpha/billing/credits", "/alpha/billing/subscriptions", "/alpha/usage/summary"} {
		require.Equal(t, "9a0f5d0e-6d2c-4f36-9c55-0b8b0c0d1e2f", upstream.request(path).URL.Query().Get("orgId"), path)
	}
}

func TestCommandCodeQuotaProbeUnlimitedPlanClearsRollingWindows(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
	upstream.responses["/alpha/billing/credits"] = commandCodeAlphaResponse{status: http.StatusOK, body: `{
		"credits": {"monthlyCredits": 10, "purchasedCredits": 25, "freeCredits": 0},
		"windowLimits": {"limited": false}
	}`}
	upstream.responses["/alpha/usage/summary"] = commandCodeAlphaResponse{status: http.StatusOK, body: `{"totalCost": 5}`}
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{})

	result, err := svc.QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.Equal(t, []CNQuotaTier{{Window: "monthly", UsedPercent: 33.33, ResetAt: "2026-10-01T00:00:00Z"}}, result.Tiers)
	extra := repo.extraWrites[0]
	for _, key := range []string{"command_code_5h_used_percent", "command_code_5h_reset_at", "command_code_weekly_used_percent", "command_code_weekly_reset_at"} {
		value, ok := extra[key]
		require.True(t, ok, key)
		require.Nil(t, value, key)
	}
	require.Equal(t, 25.0, extra["command_code_purchased_credits"])
	require.InDelta(t, 35.0, extra["command_code_balance"], 1e-9)
}

// 窗口尚未开始（resetAt 为空）时清掉上一个窗口的重置时间，不与新的用量配对。
func TestCommandCodeQuotaProbeClearsResetOfUnstartedWindow(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
	upstream.responses["/alpha/billing/credits"] = commandCodeAlphaResponse{status: http.StatusOK, body: `{
		"credits": {"monthlyCredits": 34.9, "purchasedCredits": 0, "freeCredits": 0},
		"windowLimits": {
			"limited": true,
			"fiveHour": {"used": 0, "cap": 14, "resetAt": null},
			"weekly": {"used": 35, "cap": 35, "resetAt": 1790400000000}
		}
	}`}
	repo := &cnBalanceProbeRepo{account: account}

	_, err := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{}).QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	extra := repo.extraWrites[0]
	require.Equal(t, 0.0, extra["command_code_5h_used_percent"])
	value, written := extra["command_code_5h_reset_at"]
	require.True(t, written)
	require.Nil(t, value)
	require.Equal(t, time.UnixMilli(1790400000000).UTC().Format(time.RFC3339), extra["command_code_weekly_reset_at"])
	require.Equal(t, "2026-10-01T00:00:00Z", extra["command_code_monthly_reset_at"])
}

func TestCommandCodeQuotaProbeAuthFailureDoesNotPersist(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{}`)
	upstream.responses["/alpha/whoami"] = commandCodeAlphaResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`}
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{})

	result, err := svc.QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, http.StatusUnauthorized, result.StatusCode)
	require.Equal(t, "Authentication failed (HTTP 401)", result.Error)
	require.Empty(t, repo.extraWrites)
	require.Len(t, upstream.requests, 1)
}

// 订阅周期或本周期用量取数失败：滚动窗口照常更新，月度快照保留上一次的值（无从判断
// 月度窗口是否还在）；订阅确实没有周期时才清掉月度快照。
// whoami 失败（非鉴权错误）时无从判断是否组织账户：探测整体失败，不按个人账户去查
// credits，也不写快照。
func TestCommandCodeProbeFailsWhenWhoamiFails(t *testing.T) {
	for _, whoami := range []commandCodeAlphaResponse{
		{status: http.StatusBadGateway, body: `bad gateway`},
		{status: http.StatusOK, body: `not json`},
	} {
		account := commandCodeUsageAccount()
		upstream := newCommandCodeAlphaUpstream(`{}`)
		upstream.responses["/alpha/whoami"] = whoami
		repo := &cnBalanceProbeRepo{account: account}

		result, err := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{}).QueryUsage(context.Background(), account.ID)
		require.NoError(t, err, whoami.body)
		require.False(t, result.Success, whoami.body)
		require.NotEmpty(t, result.Error, whoami.body)

		balance, err := NewCNProviderBalanceService(repo, nil, upstream, &config.Config{}).QueryBalance(context.Background(), account.ID)
		require.NoError(t, err, whoami.body)
		require.False(t, balance.Success, whoami.body)

		require.Nil(t, upstream.request("/alpha/billing/credits"), whoami.body)
		require.Empty(t, repo.extraWrites, whoami.body)
	}

	// 请求本身失败（网络错误）同样整体失败。
	account := commandCodeUsageAccount()
	repo := &cnBalanceProbeRepo{account: account}
	_, err := NewCNProviderQuotaService(repo, nil, failingCommandCodeUpstream{}, &config.Config{}).QueryUsage(context.Background(), account.ID)
	require.Error(t, err)
	require.Empty(t, repo.extraWrites)
}

// failingCommandCodeUpstream 让每个请求都在网络层失败。
type failingCommandCodeUpstream struct{}

func (failingCommandCodeUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, errors.New("dial tcp: i/o timeout")
}

func (u failingCommandCodeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestCommandCodeQuotaProbeKeepsWindowsWhenPeriodEndpointsFail(t *testing.T) {
	for _, tc := range []struct {
		name        string
		path        string
		response    commandCodeAlphaResponse
		planLevel   string
		keepMonthly bool
	}{
		{"subscriptions failed", "/alpha/billing/subscriptions", commandCodeAlphaResponse{status: http.StatusInternalServerError, body: `oops`}, "", true},
		{"summary failed", "/alpha/usage/summary", commandCodeAlphaResponse{status: http.StatusBadGateway, body: `oops`}, "GOAT", true},
		{"no subscription period", "/alpha/billing/subscriptions", commandCodeAlphaResponse{status: http.StatusOK, body: `{"data":{}}`}, "", false},
	} {
		account := commandCodeUsageAccount()
		upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
		upstream.responses[tc.path] = tc.response
		repo := &cnBalanceProbeRepo{account: account}

		result, err := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{}).QueryUsage(context.Background(), account.ID)

		require.NoError(t, err, tc.name)
		require.True(t, result.Success, tc.name)
		require.Len(t, result.Tiers, 2, tc.name)
		require.Equal(t, tc.planLevel, result.PlanLevel, tc.name)
		if tc.path == "/alpha/billing/subscriptions" {
			require.Nil(t, upstream.request("/alpha/usage/summary"), tc.name)
		}
		extra := repo.extraWrites[0]
		require.Equal(t, 10.71, extra["command_code_5h_used_percent"], tc.name)
		for _, key := range []string{"command_code_monthly_used_percent", "command_code_monthly_reset_at"} {
			value, written := extra[key]
			require.Equal(t, !tc.keepMonthly, written, tc.name+": "+key)
			require.Nil(t, value, tc.name+": "+key)
		}
	}
}

func TestCommandCodeUsageRequiresOfficialHost(t *testing.T) {
	account := commandCodeUsageAccount()
	require.Equal(t, PlatformCommandCode, account.GetCodingPlanProvider())
	require.NoError(t, validateCodingPlanAccount(account))
	require.NoError(t, validatePayGAccount(account))

	account.Credentials["base_url"] = "https://relay.example.com/v1"
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Empty(t, account.GetCodingPlanProvider())
	require.Error(t, validateCodingPlanAccount(account))
	require.Error(t, validatePayGAccount(account))
}

func TestCommandCodeBalanceProbeFetchesCreditsOnly(t *testing.T) {
	account := commandCodeUsageAccount()
	upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderBalanceService(repo, nil, upstream, &config.Config{})

	result, err := svc.QueryBalance(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.Persisted)
	require.InDelta(t, 35.0, result.Balance, 1e-9)
	require.Len(t, upstream.requests, 2)
	require.Len(t, repo.extraWrites, 1)
	_, hasWindow := repo.extraWrites[0]["command_code_5h_used_percent"]
	require.False(t, hasWindow)
	require.InDelta(t, 35.0, repo.extraWrites[0]["command_code_balance"], 1e-9)
}

func TestCommandCodeWindowsIgnoredWithPurchasedCredits(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	account := commandCodeUsageAccount()
	account.Extra = map[string]any{
		"command_code_5h_used_percent":   100.0,
		"command_code_5h_reset_at":       future,
		"command_code_purchased_credits": 0.0,
	}
	require.NotEmpty(t, cnProviderThresholdCandidates(account, PlatformCommandCode))
	require.NotNil(t, cnProviderQuotaSnapshotReset(account, time.Now()))

	account.Extra["command_code_purchased_credits"] = 12.5
	require.Empty(t, cnProviderThresholdCandidates(account, PlatformCommandCode))
	require.Nil(t, cnProviderQuotaSnapshotReset(account, time.Now()))
}

// commandCodeCheckRepo 按平台列出账号，并记录周期检测对账号的停调。
type commandCodeCheckRepo struct {
	AccountRepository
	accounts []Account
	mu       sync.Mutex
	paused   []int64
}

func (r *commandCodeCheckRepo) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	var accounts []Account
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r *commandCodeCheckRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = append(r.paused, id)
	return nil
}

type commandCodeBalanceProber struct {
	mu      sync.Mutex
	probed  []int64
	balance float64
}

func (p *commandCodeBalanceProber) QueryUsage(_ context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probed = append(p.probed, accountID)
	return &CNProviderQuotaProbeResult{
		Success: true,
		Balance: &CNProviderBalanceResult{Success: true, Balance: p.balance, Currency: "USD", Available: true},
	}, nil
}

func TestCNProviderBalanceCheckRunOnceProbesCommandCode(t *testing.T) {
	official := *commandCodeUsageAccount()
	official.ID, official.Schedulable = 1, true
	manualOff := *commandCodeUsageAccount()
	manualOff.ID, manualOff.Schedulable = 2, false
	relay := *commandCodeTestAccount(3)
	relay.Schedulable = true
	relay.Credentials = map[string]any{"api_key": "k", "api_protocol": APIProtocolChatCompletions, "base_url": "https://relay.example.com/v1"}
	inactive := *commandCodeUsageAccount()
	inactive.ID, inactive.Status = 4, StatusDisabled

	repo := &commandCodeCheckRepo{accounts: []Account{official, manualOff, relay, inactive}}
	prober := &commandCodeBalanceProber{balance: 0.2}
	cfg := &config.Config{}
	cfg.Gateway.CNProviders.BalanceThreshold = 0.5
	svc := &CNProviderBalanceCheckService{accountRepo: repo, quotaService: prober, cfg: cfg}

	svc.runOnce()

	require.ElementsMatch(t, []int64{1, 2}, prober.probed)
	require.Equal(t, []int64{1}, repo.paused)
}

// 积分不足（402）是可恢复状态：临时停调并标记 balance_low，不能永久置 error。
func TestHandleUpstreamError_CommandCode402PausesInsteadOfError(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()

	shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusPaymentRequired,
		http.Header{}, []byte(`{"error":{"message":"Insufficient credits"}}`))

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)
	require.Equal(t, true, repo.lastExtraUpdates["command_code_balance_low"])
}

func TestHandleUpstreamError_CommandCode429InsufficientBalancePauses(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()

	service.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests,
		http.Header{}, []byte(`{"error":{"type":"insufficient_credits","message":"no credits left"}}`))

	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)
	require.Equal(t, 0, repo.rateLimitedCalls)
}

// commandCodeRateLimitRepo 记录 Command Code 响应式处理写入的临时停调与模型级限流。
type commandCodeRateLimitRepo struct {
	rateLimitAccountRepoStub
	tempUntil   time.Time
	modelLimits map[string]time.Time
	reasons     map[string]string
}

func (r *commandCodeRateLimitRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempUntil = until
	return r.rateLimitAccountRepoStub.SetTempUnschedulable(ctx, id, until, reason)
}

func (r *commandCodeRateLimitRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reason ...string) error {
	if r.modelLimits == nil {
		r.modelLimits = map[string]time.Time{}
		r.reasons = map[string]string{}
	}
	r.modelLimits[scope] = resetAt
	if len(reason) > 0 {
		r.reasons[scope] = reason[0]
	}
	return nil
}

// 窗口用满：按文案中的重置时间写带原因的临时停调（充值后可据原因解除），不依赖额度快照。
func TestHandleUpstreamError_CommandCodeWindowLimitCoolsDownToMessageReset(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden, http.StatusPaymentRequired} {
		repo := &commandCodeRateLimitRepo{}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := commandCodeUsageAccount()

		shouldDisable := service.HandleUpstreamError(context.Background(), account, status, http.Header{},
			[]byte(`{"error":{"type":"usage_exceeded","message":"You've reached your 5-hour usage limit. Resets in 2h 41m (3:00 PM)."}}`))

		require.Equal(t, status != http.StatusTooManyRequests, shouldDisable, status)
		require.Equal(t, 0, repo.setErrorCalls, status)
		require.Equal(t, 0, repo.rateLimitedCalls, status)
		require.Equal(t, 1, repo.tempCalls, status)
		require.True(t, strings.HasPrefix(repo.lastTempReason, commandCodeUsageLimitReason+": "), repo.lastTempReason)
		expected := time.Now().Add(2*time.Hour + 41*time.Minute)
		require.WithinDuration(t, expected, repo.tempUntil, time.Minute, status)
	}
}

// 文案没有重置时间时回落额度快照中最早的窗口重置点。
func TestHandleUpstreamError_CommandCodeWindowLimitFallsBackToSnapshot(t *testing.T) {
	repo := &commandCodeRateLimitRepo{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()
	account.Extra = map[string]any{
		"command_code_5h_used_percent":   100.0,
		"command_code_5h_reset_at":       time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339),
		"command_code_purchased_credits": 0.0,
	}

	service.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{},
		[]byte(`{"error":{"message":"You've reached your weekly usage limit."}}`))

	require.Equal(t, 1, repo.tempCalls)
	require.WithinDuration(t, time.Now().Add(90*time.Minute), repo.tempUntil, time.Minute)
}

// 普通频率限制很快恢复：即使快照里有窗口重置点，也不冷却到窗口重置。
func TestHandleUpstreamError_CommandCodeRateLimitUsesDefaultCooldown(t *testing.T) {
	repo := &commandCodeRateLimitRepo{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()
	account.Extra = map[string]any{
		"command_code_5h_used_percent":   97.0,
		"command_code_5h_reset_at":       time.Now().Add(4 * time.Hour).UTC().Format(time.RFC3339),
		"command_code_purchased_credits": 0.0,
	}

	service.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{},
		[]byte(`{"error":{"type":"rate_limit","message":"Rate limit exceeded. Please wait a moment and try again."}}`))

	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	if repo.rateLimitedCalls > 0 {
		require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(30*time.Minute)), "must not cool down to the 5h window reset")
	}
}

const (
	commandCodeSpendLimitBody       = `{"error":{"type":"usage_exceeded","message":"You've reached the $10.00/mo spending limit your organization set for GLM-5. It resets in 3 days."}}`
	commandCodeMemberSpendLimitBody = `{"error":{"type":"usage_exceeded","message":"You've reached the $50.00/mo spending limit your organization set. It resets in 12 days."}}`
)

// 组织为某个模型设的花费上限只限当前模型：写模型级限流到文案中的重置时间，不停整个账号。
func TestHandleUpstreamError_CommandCodeSpendLimitIsModelScoped(t *testing.T) {
	repo := &commandCodeRateLimitRepo{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()
	ctx := withTempUnschedulableModel(context.Background(), []string{"glm-5"})

	shouldDisable := service.HandleUpstreamError(ctx, account, http.StatusForbidden, http.Header{}, []byte(commandCodeSpendLimitBody))

	require.False(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Contains(t, repo.modelLimits, "glm-5")
	require.WithinDuration(t, time.Now().Add(72*time.Hour), repo.modelLimits["glm-5"], time.Minute)
	require.Equal(t, commandCodeSpendLimitReason, repo.reasons["glm-5"])
}

// 成员总额度耗尽：即使请求模型已知也按账号冷却，但最长一小时（管理员可能调高上限）。
func TestHandleUpstreamError_CommandCodeMemberSpendLimitIsAccountScoped(t *testing.T) {
	repo := &commandCodeRateLimitRepo{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()
	ctx := withTempUnschedulableModel(context.Background(), []string{"glm-5"})

	shouldDisable := service.HandleUpstreamError(ctx, account, http.StatusForbidden, http.Header{}, []byte(commandCodeMemberSpendLimitBody))

	require.True(t, shouldDisable)
	require.Empty(t, repo.modelLimits)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, commandCodeSpendLimitReason, repo.lastTempReason)
	require.WithinDuration(t, time.Now().Add(commandCodeAccountSpendLimitRecheck), repo.tempUntil, time.Minute)

	// 文案中的重置时间早于一小时时以文案为准。
	soon := &commandCodeRateLimitRepo{}
	NewRateLimitService(soon, nil, &config.Config{}, nil, nil).HandleUpstreamError(ctx, account, http.StatusTooManyRequests, http.Header{},
		[]byte(`{"error":{"message":"You've reached the $50.00/mo spending limit your organization set. It resets in 20 minutes."}}`))
	require.WithinDuration(t, time.Now().Add(20*time.Minute), soon.tempUntil, time.Minute)
}

// 不知道模型时按账号冷却（最长一小时），不停账号数天，也不进入 403 累计禁用。
func TestHandleUpstreamError_CommandCodeSpendLimitWithoutModelShortCooldown(t *testing.T) {
	repo := &commandCodeRateLimitRepo{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := commandCodeUsageAccount()

	for i := 0; i < 5; i++ {
		service.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, []byte(commandCodeSpendLimitBody))
	}

	require.Equal(t, 0, repo.setErrorCalls)
	require.Empty(t, repo.modelLimits)
	require.Equal(t, 5, repo.tempCalls)
	require.False(t, repo.tempUntil.After(time.Now().Add(commandCodeAccountSpendLimitRecheck)))
}

func TestCommandCodeSpendLimitScope(t *testing.T) {
	require.True(t, commandCodeSpendLimitIsModelScoped([]byte(commandCodeSpendLimitBody)))
	require.True(t, commandCodeSpendLimitIsModelScoped([]byte(`spending limit your organization set for gpt-5.5. It resets in 2 days.`)))
	require.False(t, commandCodeSpendLimitIsModelScoped([]byte(commandCodeMemberSpendLimitBody)))
	require.False(t, commandCodeSpendLimitIsModelScoped([]byte(`You've reached the $50.00/mo spending limit your organization set for you.`)))
	require.False(t, commandCodeSpendLimitIsModelScoped([]byte(`spending limit reached`)))
}

// commandCodeStatefulRepo 把停调写回同一个账号对象，用于验证完整的状态转换。
type commandCodeStatefulRepo struct {
	AccountRepository
	account    *Account
	clearCalls int
}

func (r *commandCodeStatefulRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	clone := *r.account
	return &clone, nil
}

func (r *commandCodeStatefulRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return nil
}

func (r *commandCodeStatefulRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.account.TempUnschedulableUntil = &until
	r.account.TempUnschedulableReason = reason
	return nil
}

func (r *commandCodeStatefulRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	r.clearCalls++
	r.account.TempUnschedulableUntil = nil
	r.account.TempUnschedulableReason = ""
	return nil
}

func commandCodeCreditsBody(purchased float64) string {
	return fmt.Sprintf(`{"credits":{"monthlyCredits":0,"purchasedCredits":%g,"freeCredits":0},
		"windowLimits":{"limited":true,"fiveHour":{"used":14,"cap":14,"resetAt":1790000000000}}}`, purchased)
}

// 窗口用满进入冷却 → 充值 → 下一次额度探测发现充值积分 → 恢复可调度。
func TestCommandCodeWindowCooldownClearedAfterTopUp(t *testing.T) {
	account := commandCodeUsageAccount()
	account.Schedulable = true
	repo := &commandCodeStatefulRepo{account: account}
	rateLimit := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	rateLimit.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{},
		[]byte(`{"error":{"message":"You've reached your 5-hour usage limit. Resets in 3h 10m."}}`))
	require.False(t, account.IsSchedulable())

	// 未充值：探测不解除冷却。
	upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
	upstream.responses["/alpha/billing/credits"] = commandCodeAlphaResponse{status: http.StatusOK, body: commandCodeCreditsBody(0)}
	quota := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{})
	_, err := quota.QueryUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 0, repo.clearCalls)
	require.False(t, account.IsSchedulable())

	// 充值后：额度探测与余额探测都能解除窗口冷却。
	upstream.responses["/alpha/billing/credits"] = commandCodeAlphaResponse{status: http.StatusOK, body: commandCodeCreditsBody(20)}
	_, err = quota.QueryUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, repo.clearCalls)
	require.True(t, account.IsSchedulable())

	rateLimit.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{},
		[]byte(`{"error":{"message":"You've reached your weekly usage limit. Resets in 2 days."}}`))
	require.False(t, account.IsSchedulable())
	_, err = NewCNProviderBalanceService(repo, nil, upstream, &config.Config{}).QueryBalance(context.Background(), account.ID)
	require.NoError(t, err)
	require.True(t, account.IsSchedulable())
}

// 充值积分只解除窗口冷却：积分不足、花费上限等其他原因的停调保持不变。
func TestCommandCodeTopUpKeepsOtherCooldowns(t *testing.T) {
	for _, reason := range []string{cnBalanceLowReason("no credits"), commandCodeSpendLimitReason, "manual rule"} {
		until := time.Now().Add(time.Hour)
		account := commandCodeUsageAccount()
		account.Schedulable = true
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
		repo := &commandCodeStatefulRepo{account: account}
		upstream := newCommandCodeAlphaUpstream(`{"org": null}`)
		upstream.responses["/alpha/billing/credits"] = commandCodeAlphaResponse{status: http.StatusOK, body: commandCodeCreditsBody(20)}

		_, err := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{}).QueryUsage(context.Background(), account.ID)

		require.NoError(t, err)
		require.Equal(t, 0, repo.clearCalls, reason)
		require.False(t, account.IsSchedulable(), reason)
	}
}

func TestClassifyCommandCodeUsageError(t *testing.T) {
	cases := map[string]commandCodeUsageErrorKind{
		`{"error":{"message":"You've reached your 5-hour usage limit. Resets in 2h 41m (3:00 PM)."}}`: commandCodeUsageWindowLimit,
		`{"error":{"message":"You've reached your weekly usage limit."}}`:                             commandCodeUsageWindowLimit,
		commandCodeSpendLimitBody: commandCodeUsageSpendLimit,
		`{"error":{"type":"usage_exceeded","message":"Monthly credits spent."}}`:                 commandCodeUsageCreditsExhausted,
		`{"error":{"message":"Insufficient credits"}}`:                                           commandCodeUsageCreditsExhausted,
		`{"error":{"type":"rate_limit","message":"Rate limit exceeded. Please wait a moment."}}`: commandCodeUsageErrorNone,
		`{"error":{"message":"forbidden"}}`:                                                      commandCodeUsageErrorNone,
	}
	for body, want := range cases {
		require.Equal(t, want, classifyCommandCodeUsageError([]byte(body)), body)
	}
}
