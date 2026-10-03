//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// clineTestAccount 只带 api_key：端点与测试模型全部来自 provider profile。
func clineTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "cline",
		Platform:    PlatformCline,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "cline_test_key", "api_protocol": APIProtocolAdaptive},
	}
}

// withClinePassSnapshot 模拟用量探测写入的 ClinePass 窗口快照。
func withClinePassSnapshot(account *Account, extra map[string]any) *Account {
	account.Extra = map[string]any{"cline_5h_used_percent": 10.0, "cline_weekly_used_percent": 20.0, "cline_monthly_used_percent": 30.0}
	for key, value := range extra {
		account.Extra[key] = value
	}
	return account
}

func TestClineProfileSingleModeChatCompletionsOnly(t *testing.T) {
	profile := LookupProviderProfile(PlatformCline)
	require.NotNil(t, profile)
	require.Len(t, profile.Modes, 1)

	account := clineTestAccount(1)
	require.True(t, account.IsMultiProtocolAPIKey())
	require.True(t, account.RoutesProtocolByInbound())
	require.False(t, account.routesByModel())
	require.Equal(t, APIProtocolAdaptive, account.GetAPIProtocol())
	require.Equal(t, DefaultClineBaseURL, account.GetOpenAIBaseURL())
	require.False(t, account.SupportsNativeCNResponses())
	require.False(t, account.providerSupportsProtocol(APIProtocolAnthropic))
	for _, inbound := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, inbound, "anthropic/claude-sonnet-4-6", nil), inbound)
	}

	require.False(t, account.clinePassSubscribed())
	require.Equal(t, DefaultClineTestModel, account.providerDefaultTestModel())
	subscribed := withClinePassSnapshot(clineTestAccount(2), nil)
	require.True(t, subscribed.clinePassSubscribed())
	require.Equal(t, DefaultClinePassTestModel, subscribed.providerDefaultTestModel())
	// 取消订阅后探测把窗口写成 nil。
	cancelled := withClinePassSnapshot(clineTestAccount(3), map[string]any{
		"cline_5h_used_percent": nil, "cline_weekly_used_percent": nil, "cline_monthly_used_percent": nil,
	})
	require.False(t, cancelled.clinePassSubscribed())
}

// 经真实入口：三种入站都转换 / 直通到 Chat Completions，模型名带厂商前缀原样发往上游。
func TestClineGatewayForwardsToChatCompletions(t *testing.T) {
	for _, ingress := range routingMatrixIngresses() {
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := routingMatrixCase{ingress: ingress, model: "anthropic/claude-sonnet-4-6"}.body()
		_ = ingress.forward(svc, adaptiveProtocolTestContext(ingress.path, body), clineTestAccount(3), body)

		require.NotEmpty(t, upstream.requests, ingress.name)
		require.Equal(t, "https://api.cline.bot/api/v1/chat/completions", upstream.requests[len(upstream.requests)-1].URL.String(), ingress.name)
		require.Equal(t, "Bearer cline_test_key", upstream.requests[len(upstream.requests)-1].Header.Get("Authorization"), ingress.name)
		require.True(t, gjson.GetBytes(upstream.lastBody, "messages").Exists(), ingress.name)
		require.Equal(t, "anthropic/claude-sonnet-4-6", gjson.GetBytes(upstream.lastBody, "model").String(), ingress.name)
	}
}

// 连接测试未指定模型时：订阅了 ClinePass 用订阅模型（不消耗积分），否则用按量模型；
// ClinePass 钱包冷却中也用按量模型；自适应与固定 Chat Completions 一致。
func TestAccountTestService_ClineDefaultTestModel(t *testing.T) {
	cases := []struct {
		subscribed  bool
		passCooling bool
		apiProtocol string
		want        string
	}{
		{false, false, "", DefaultClineTestModel},
		{true, false, "", DefaultClinePassTestModel},
		{false, false, APIProtocolChatCompletions, DefaultClineTestModel},
		{true, false, APIProtocolChatCompletions, DefaultClinePassTestModel},
		{true, true, "", DefaultClineTestModel},
	}
	for i, tc := range cases {
		account := clineTestAccount(int64(600 + i))
		if tc.subscribed {
			withClinePassSnapshot(account, nil)
		}
		if tc.passCooling {
			now := time.Now()
			setAccountModelRateLimitSnapshot(account, clinePassRateLimitKey, now.Add(time.Hour), clinePassLimitReason, now)
		}
		if tc.apiProtocol != "" {
			account.Credentials["api_protocol"] = tc.apiProtocol
		}
		svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
		c, recorder := newTestContext()

		err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

		require.NoError(t, err, tc)
		require.Len(t, upstream.requests, 1, tc)
		require.Equal(t, "https://api.cline.bot/api/v1/chat/completions", upstream.requests[0].URL.String(), tc)
		require.Equal(t, tc.want, gjson.GetBytes(upstream.lastBody, "model").String(), tc)
		require.Contains(t, recorder.Body.String(), `"type":"test_complete"`, tc)
	}
}

// 响应体样例取自 Cline 客户端的判定文案与官方错误文档。
const (
	clineInsufficientCreditsBody = `{"error":{"code":"insufficient_credits","message":"Insufficient credits","details":{"current_balance":0}}}`
	clineSpendLimitBody          = `{"error":{"code":"SPEND_LIMIT_EXCEEDED","message":"Organization spend limit exceeded"}}`
	clineInferenceCapBody        = `{"error":{"code":"INFERENCE_CAP_ERROR","message":"Inference cap reached"}}`
	clinePass5hBody              = `{"error":{"code":429,"message":"You have reached your 5-hour ClinePass limit. Please try again later."}}`
	clinePassWeeklyBody          = `{"error":{"code":429,"message":"You have reached your weekly ClinePass limit. Please try again later."}}`
	clinePassMonthlyBody         = `{"error":{"code":429,"message":"You have reached your monthly ClinePass limit. Please try again later."}}`
	clineNotSubscribedBody       = `{"error":{"code":403,"message":"The user is not subscribed to required model plan"}}`
	clineOrgPassBody             = `{"error":{"code":403,"message":"Organization accounts cannot use individual model inference subscriptions"}}`
	clineFreeModelLimitBody      = `{"error":{"code":429,"message":"Free limit reached on model cline-free/kimi-k3. Try again in 3h 20m"}}`
	clineRateLimitBody           = `{"error":{"code":429,"message":"Rate limit exceeded"}}`
)

func TestClassifyClineError(t *testing.T) {
	cases := map[string]clineErrorKind{
		clineInsufficientCreditsBody: clineCreditsExhausted,
		clineSpendLimitBody:          clineSpendLimit,
		clineInferenceCapBody:        clineInferenceCap,
		clinePass5hBody:              clinePassLimit,
		clinePassWeeklyBody:          clinePassLimit,
		clineNotSubscribedBody:       clinePassUnavailable,
		clineOrgPassBody:             clinePassUnavailable,
		clineFreeModelLimitBody:      clineFreeModelLimit,
		clineRateLimitBody:           clineErrorNone,
		`{"error":{"message":"You have reached your limit. Please try again later."}}`: clineErrorNone,
	}
	for body, want := range cases {
		require.Equal(t, want, classifyClineError([]byte(body)), body)
	}

	require.Equal(t, "5h", clinePassWindow([]byte(clinePass5hBody)))
	require.Equal(t, "weekly", clinePassWindow([]byte(clinePassWeeklyBody)))
	require.Equal(t, "monthly", clinePassWindow([]byte(clinePassMonthlyBody)))
	require.Equal(t, 3*time.Hour+20*time.Minute, clineFreeModelResetAfter([]byte(clineFreeModelLimitBody)))
	require.Zero(t, clineFreeModelResetAfter([]byte(`Free limit reached on model x. Try again in 3 weeks`)))
}

// ClinePass 超限冷却到快照中该窗口的重置时间；没有可用快照时按窗口估计。
func TestClinePassLimitCooldown(t *testing.T) {
	now := time.Now()
	require.Equal(t, clineLimitRecheck, clinePassLimitCooldown(clineTestAccount(1), "5h", now))
	require.Equal(t, clinePassWeeklyRecheck, clinePassLimitCooldown(clineTestAccount(1), "weekly", now))
	require.Equal(t, clinePassMonthlyRecheck, clinePassLimitCooldown(clineTestAccount(1), "monthly", now))

	account := withClinePassSnapshot(clineTestAccount(2), map[string]any{
		"cline_weekly_reset_at": now.Add(50 * time.Hour).UTC().Format(time.RFC3339),
		"cline_5h_reset_at":     now.Add(-time.Minute).UTC().Format(time.RFC3339),
	})
	require.InDelta(t, (50 * time.Hour).Seconds(), clinePassLimitCooldown(account, "weekly", now).Seconds(), 1)
	require.Equal(t, clineLimitRecheck, clinePassLimitCooldown(account, "5h", now), "past reset falls back to the estimate")
}

// cline-pass/* 与其余付费模型各自落在钱包冷却键上，免费模型不属于任何钱包；调度按
// 请求模型检查所属钱包。
func TestClineWalletRateLimitKeys(t *testing.T) {
	require.Equal(t, clinePassRateLimitKey, clineWalletRateLimitKey("cline-pass/glm-5.3"))
	require.Equal(t, clinePassRateLimitKey, clineWalletRateLimitKey(" Cline-Pass/GLM-5.3 "))
	require.Equal(t, clineCreditsRateLimitKey, clineWalletRateLimitKey("deepseek/deepseek-v4-flash"))
	require.Empty(t, clineWalletRateLimitKey("cline-free/kimi-k3"))
	require.Empty(t, clineWalletRateLimitKey(""))

	account := clineTestAccount(1)
	setAccountModelRateLimitSnapshot(account, clinePassRateLimitKey, time.Now().Add(time.Hour), clinePassLimitReason, time.Now())
	ctx := context.Background()
	require.True(t, account.isModelRateLimitedWithContext(ctx, "cline-pass/glm-5.3"))
	require.False(t, account.isModelRateLimitedWithContext(ctx, "deepseek/deepseek-v4-flash"))
	require.False(t, account.isModelRateLimitedWithContext(ctx, "cline-free/kimi-k3"))

	setAccountModelRateLimitSnapshot(account, clineCreditsRateLimitKey, time.Now().Add(time.Hour), clineCreditsReason, time.Now())
	require.True(t, account.isModelRateLimitedWithContext(ctx, "deepseek/deepseek-v4-flash"))
	require.False(t, account.isModelRateLimitedWithContext(ctx, "cline-free/kimi-k3"))
	require.Equal(t, clineCreditsReason, account.modelRateLimitReason(clineCreditsRateLimitKey))

	// 其他平台不受 Cline 钱包键影响。
	other := &Account{Platform: PlatformDeepseek, Extra: account.Extra}
	require.False(t, other.isModelRateLimitedWithContext(ctx, "cline-pass/glm-5.3"))
}

// 已识别的错误只冷却请求模型所属的钱包（或免费模型本身），不停整个账号、不置 error。
func TestHandleUpstreamError_ClineCoolsDownWallet(t *testing.T) {
	paid := "deepseek/deepseek-v4-flash"
	pass := "cline-pass/glm-5.3"
	now := time.Now()
	snapshot := withClinePassSnapshot(clineTestAccount(1), map[string]any{
		"cline_5h_reset_at": now.Add(2 * time.Hour).UTC().Format(time.RFC3339),
	})
	cases := []struct {
		name    string
		account *Account
		model   string
		status  int
		body    string
		scope   string
		wait    time.Duration
		reason  string
	}{
		{"pass weekly", clineTestAccount(10), pass, http.StatusTooManyRequests, clinePassWeeklyBody, clinePassRateLimitKey, clinePassWeeklyRecheck, clinePassLimitReason},
		{"pass 5h from snapshot", snapshot, pass, http.StatusTooManyRequests, clinePass5hBody, clinePassRateLimitKey, 2 * time.Hour, clinePassLimitReason},
		{"not subscribed", clineTestAccount(11), pass, http.StatusForbidden, clineNotSubscribedBody, clinePassRateLimitKey, clineLimitRecheck, clinePassUnavailableReason},
		{"credits", clineTestAccount(12), paid, http.StatusPaymentRequired, clineInsufficientCreditsBody, clineCreditsRateLimitKey, cnBalanceCheckCooldown(nil), clineCreditsReason},
		{"unrecognized 402", clineTestAccount(13), paid, http.StatusPaymentRequired, `{"error":{"message":"Payment required"}}`, clineCreditsRateLimitKey, cnBalanceCheckCooldown(nil), clineCreditsReason},
		{"spend limit", clineTestAccount(14), paid, http.StatusTooManyRequests, clineSpendLimitBody, clineCreditsRateLimitKey, clineLimitRecheck, clineSpendLimitReason},
		{"inference cap on paid model", clineTestAccount(15), paid, http.StatusTooManyRequests, clineInferenceCapBody, clineCreditsRateLimitKey, clineLimitRecheck, clineInferenceCapReason},
		{"inference cap on pass model", clineTestAccount(16), pass, http.StatusTooManyRequests, clineInferenceCapBody, clinePassRateLimitKey, clineLimitRecheck, clineInferenceCapReason},
		{"unknown model uses the error's wallet", clineTestAccount(17), "", http.StatusForbidden, clineOrgPassBody, clinePassRateLimitKey, clineLimitRecheck, clinePassUnavailableReason},
		{"free model", clineTestAccount(18), "cline-free/kimi-k3", http.StatusTooManyRequests, clineFreeModelLimitBody, "cline-free/kimi-k3", 3*time.Hour + 20*time.Minute, clineFreeModelLimitReason},
	}
	for _, tc := range cases {
		ctx := context.Background()
		if tc.model != "" {
			ctx = withTempUnschedulableModel(ctx, []string{tc.model})
		}
		repo := &commandCodeRateLimitRepo{}

		shouldDisable := NewRateLimitService(repo, nil, &config.Config{}, nil, nil).HandleUpstreamError(ctx, tc.account,
			tc.status, http.Header{}, []byte(tc.body))

		require.False(t, shouldDisable, tc.name)
		require.Equal(t, 0, repo.tempCalls, tc.name)
		require.Equal(t, 0, repo.setErrorCalls, tc.name)
		require.Equal(t, 0, repo.rateLimitedCalls, tc.name)
		require.Len(t, repo.modelLimits, 1, tc.name)
		require.WithinDuration(t, time.Now().Add(tc.wait), repo.modelLimits[tc.scope], time.Minute, tc.name)
		require.True(t, strings.HasPrefix(repo.reasons[tc.scope], tc.reason), tc.name+": "+repo.reasons[tc.scope])
		require.True(t, tc.account.isRateLimitActiveForKey(tc.scope), tc.name)
	}

	// 普通频率限制交回默认 429 逻辑。
	plain := &commandCodeRateLimitRepo{}
	NewRateLimitService(plain, nil, &config.Config{}, nil, nil).HandleUpstreamError(withTempUnschedulableModel(context.Background(), []string{paid}),
		clineTestAccount(30), http.StatusTooManyRequests, http.Header{}, []byte(clineRateLimitBody))
	require.Empty(t, plain.modelLimits)
	require.Equal(t, 0, plain.tempCalls)
}

// clineModelLimitFailRepo 让钱包冷却写入失败。
type clineModelLimitFailRepo struct {
	commandCodeRateLimitRepo
}

func (r *clineModelLimitFailRepo) SetModelRateLimit(context.Context, int64, string, time.Time, ...string) error {
	return errors.New("db unavailable")
}

// 积分冷却写入失败时 402 也不落入默认分支永久置 error：探测只覆盖激活账号，置 error 后
// 充值也无法自动恢复。
func TestHandleUpstreamError_Cline402KeepsAccountWhenCooldownWriteFails(t *testing.T) {
	ctx := withTempUnschedulableModel(context.Background(), []string{"deepseek/deepseek-v4-flash"})
	for _, body := range []string{clineInsufficientCreditsBody, `{"error":{"message":"Payment required"}}`} {
		repo := &clineModelLimitFailRepo{}

		shouldDisable := NewRateLimitService(repo, nil, &config.Config{}, nil, nil).HandleUpstreamError(ctx, clineTestAccount(19),
			http.StatusPaymentRequired, http.Header{}, []byte(body))

		require.False(t, shouldDisable, body)
		require.Equal(t, 0, repo.setErrorCalls, body)
		require.Equal(t, 0, repo.tempCalls, body)
	}
}

// clineAccountRepo 为余额 / 用量探测与周期检测提供 Cline 账号，并记录写入。
type clineAccountRepo struct {
	AccountRepository
	accounts    []Account
	mu          sync.Mutex
	extraWrites map[int64][]map[string]any
	modelLimits map[int64]map[string]time.Time
	reasons     map[int64]map[string]string
	paused      []int64
}

func (r *clineAccountRepo) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	if platform == PlatformCline {
		return r.accounts, nil
	}
	return nil, nil
}

func (r *clineAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (r *clineAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.extraWrites == nil {
		r.extraWrites = map[int64][]map[string]any{}
	}
	r.extraWrites[id] = append(r.extraWrites[id], updates)
	return nil
}

func (r *clineAccountRepo) SetModelRateLimit(_ context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.modelLimits == nil {
		r.modelLimits = map[int64]map[string]time.Time{}
		r.reasons = map[int64]map[string]string{}
	}
	if r.modelLimits[id] == nil {
		r.modelLimits[id] = map[string]time.Time{}
		r.reasons[id] = map[string]string{}
	}
	r.modelLimits[id][scope] = resetAt
	if len(reason) > 0 {
		r.reasons[id][scope] = reason[0]
	}
	return nil
}

func (r *clineAccountRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = append(r.paused, id)
	return nil
}

const (
	clineUsageLimitsPath = "/api/v1/users/me/plan/usage-limits"
	clineUsageLimitsBody = `{"data":{"limits":[{"type":"five_hour","percentUsed":12.5,"resetsAt":"2099-01-01T05:00:00.123456Z"},{"type":"weekly","percentUsed":40},{"type":"monthly","percentUsed":0}]},"success":true}`
)

func newClineAccountUpstream(usageLimits commandCodeAlphaResponse, balance string) *commandCodeAlphaUpstream {
	return &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		clineUsageLimitsPath:        usageLimits,
		"/api/v1/users/me":          {status: http.StatusOK, body: `{"success":true,"data":{"id":"u-1","email":"a@example.com"}}`},
		"/api/v1/users/u-1/balance": {status: http.StatusOK, body: balance},
	}}
}

// balance 是百万分之一积分（1 积分 = 1 美元），按 Cline 的展示精度保留 4 位小数。
func TestClineBalanceProbeConvertsMicrocredits(t *testing.T) {
	for _, tc := range []struct {
		balance string
		want    float64
	}{
		{`{"balance":12340000,"userId":"u-1"}`, 12.34},
		{`{"success":true,"data":{"balance":"500000","userId":"u-1"}}`, 0.5},
		{`{"balance":1234567,"userId":"u-1"}`, 1.2346},
	} {
		account := clineTestAccount(40)
		upstream := newClineAccountUpstream(commandCodeAlphaResponse{}, tc.balance)
		repo := &cnBalanceProbeRepo{account: account}
		svc := NewCNProviderBalanceService(repo, nil, upstream, &config.Config{})

		result, err := svc.QueryBalance(context.Background(), account.ID)

		require.NoError(t, err)
		require.True(t, result.Success)
		require.True(t, result.Persisted)
		require.InDelta(t, tc.want, result.Balance, 1e-9)
		require.Equal(t, "USD", result.Currency)
		require.Len(t, upstream.requests, 2)
		require.Equal(t, "Bearer cline_test_key", upstream.requests[0].Header.Get("Authorization"))
		require.InDelta(t, tc.want, repo.extraWrites[0]["cline_balance"], 1e-9)
	}
}

func TestClineBalanceProbeFailures(t *testing.T) {
	account := clineTestAccount(41)
	upstream := &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		"/api/v1/users/me": {status: http.StatusUnauthorized, body: `{"error":"Unauthorized"}`},
	}}
	repo := &cnBalanceProbeRepo{account: account}
	result, err := NewCNProviderBalanceService(repo, nil, upstream, &config.Config{}).QueryBalance(context.Background(), account.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, http.StatusUnauthorized, result.StatusCode)
	require.Contains(t, result.Error, "HTTP 401")
	require.Empty(t, repo.extraWrites)

	envelope := &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		"/api/v1/users/me": {status: http.StatusOK, body: `{"success":false,"error":"token expired"}`},
	}}
	result, err = NewCNProviderBalanceService(&cnBalanceProbeRepo{account: account}, nil, envelope, &config.Config{}).QueryBalance(context.Background(), account.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "token expired")
}

// 余额与用量只查官方主机上的 API Key 账号：自定义中转的 Key 不发往官方账号接口。
func TestClineAccountAPIRequiresOfficialHost(t *testing.T) {
	require.NoError(t, validatePayGAccount(clineTestAccount(50)))
	require.NoError(t, validateCodingPlanAccount(clineTestAccount(50)))
	require.Equal(t, PlatformCline, clineTestAccount(50).GetCodingPlanProvider())

	relay := clineTestAccount(52)
	relay.Credentials["base_url"] = "https://relay.example.com/v1"
	relay.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Error(t, validatePayGAccount(relay))
	require.Error(t, validateCodingPlanAccount(relay))
	require.Empty(t, relay.GetCodingPlanProvider())
}

func clineUsageTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.CNProviders.BalanceThreshold = 0.5
	return cfg
}

// 一次探测刷新三档窗口与积分余额；两个钱包都可用时不写冷却。
func TestClineUsageProbeWritesWindowsAndBalance(t *testing.T) {
	account := clineTestAccount(60)
	repo := &clineAccountRepo{accounts: []Account{*account}}
	upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: clineUsageLimitsBody}, `{"success":true,"data":{"balance":12340000}}`)

	result, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.CredentialValid)
	require.True(t, result.Persisted)
	require.Equal(t, "ClinePass", result.PlanLevel)
	require.Equal(t, []CNQuotaTier{
		{Window: "5h", UsedPercent: 12.5, ResetAt: "2099-01-01T05:00:00Z"},
		{Window: "weekly", UsedPercent: 40},
		{Window: "monthly", UsedPercent: 0},
	}, result.Tiers)
	require.NotNil(t, result.Balance)
	require.InDelta(t, 12.34, result.Balance.Balance, 1e-9)
	require.Equal(t, "Bearer cline_test_key", upstream.requests[0].Header.Get("Authorization"))

	written := repo.extraWrites[60][0]
	require.Equal(t, 12.5, written["cline_5h_used_percent"])
	require.Equal(t, "2099-01-01T05:00:00Z", written["cline_5h_reset_at"])
	require.Equal(t, 40.0, written["cline_weekly_used_percent"])
	require.Contains(t, written, "cline_weekly_reset_at")
	require.Nil(t, written["cline_weekly_reset_at"], "a window that has not started has no reset time")
	require.InDelta(t, 12.34, written["cline_balance"], 1e-9)
	require.Empty(t, repo.modelLimits)
}

// 窗口用满冷却 ClinePass 到重置时间；积分低于阈值冷却积分钱包；都不停整个账号。
func TestClineUsageProbeCoolsDownExhaustedWallets(t *testing.T) {
	resetAt := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	body := `{"data":{"limits":[{"type":"five_hour","percentUsed":30},{"type":"weekly","percentUsed":100,"resetsAt":"` +
		resetAt.Format(time.RFC3339) + `"}]},"success":true}`
	account := clineTestAccount(61)
	repo := &clineAccountRepo{accounts: []Account{*account}}
	upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: body}, `{"success":true,"data":{"balance":200000}}`)

	_, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, repo.modelLimits[61][clinePassRateLimitKey].Equal(resetAt))
	require.Equal(t, clinePassLimitReason+": weekly window used up", repo.reasons[61][clinePassRateLimitKey])
	require.WithinDuration(t, time.Now().Add(cnBalanceCheckCooldown(nil)), repo.modelLimits[61][clineCreditsRateLimitKey], time.Minute)
	require.True(t, strings.HasPrefix(repo.reasons[61][clineCreditsRateLimitKey], clineCreditsLowReason), repo.reasons[61][clineCreditsRateLimitKey])
	require.Empty(t, repo.paused)
}

// 没有订阅（404 或空列表）：清掉旧窗口快照，ClinePass 模型不调度到该账号。
func TestClineUsageProbeWithoutSubscription(t *testing.T) {
	for _, usage := range []commandCodeAlphaResponse{
		{status: http.StatusNotFound, body: `{"success":false,"error":"no plan history found for user"}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"limits":[]}}`},
	} {
		account := withClinePassSnapshot(clineTestAccount(62), nil)
		repo := &clineAccountRepo{accounts: []Account{*account}}
		upstream := newClineAccountUpstream(usage, `{"success":true,"data":{"balance":5000000}}`)

		result, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

		require.NoError(t, err)
		require.True(t, result.Success)
		require.Empty(t, result.Tiers)
		require.Empty(t, result.PlanLevel)
		written := repo.extraWrites[62][0]
		for _, key := range []string{"cline_5h_used_percent", "cline_weekly_used_percent", "cline_monthly_used_percent"} {
			require.Contains(t, written, key)
			require.Nil(t, written[key])
		}
		require.True(t, strings.HasPrefix(repo.reasons[62][clinePassRateLimitKey], clinePassUnavailableReason))
		require.NotContains(t, repo.modelLimits[62], clineCreditsRateLimitKey)
	}
}

// 数据显示钱包恢复可用时解除冷却；组织花费上限无从探测，不提前解除。
func TestClineUsageProbeClearsRecoveredWallets(t *testing.T) {
	for _, tc := range []struct {
		creditsReason string
		cleared       bool
	}{
		{clineCreditsReason + ": Insufficient credits", true},
		{clineCreditsLowReason + ": balance 0.1 USD below threshold 0.50", true},
		{clineSpendLimitReason + ": Organization spend limit exceeded", false},
	} {
		account := clineTestAccount(63)
		now := time.Now()
		setAccountModelRateLimitSnapshot(account, clinePassRateLimitKey, now.Add(6*time.Hour), clinePassLimitReason, now)
		setAccountModelRateLimitSnapshot(account, clineCreditsRateLimitKey, now.Add(time.Hour), tc.creditsReason, now)
		repo := &clineAccountRepo{accounts: []Account{*account}}
		upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: clineUsageLimitsBody}, `{"success":true,"data":{"balance":5000000}}`)

		_, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

		require.NoError(t, err)
		require.False(t, repo.modelLimits[63][clinePassRateLimitKey].After(time.Now()), tc.creditsReason)
		_, touched := repo.modelLimits[63][clineCreditsRateLimitKey]
		require.Equal(t, tc.cleared, touched, tc.creditsReason)
		if tc.cleared {
			require.False(t, repo.modelLimits[63][clineCreditsRateLimitKey].After(time.Now()), tc.creditsReason)
		}
	}
}

// ClinePass 同样只解除探测能证明已恢复的冷却（窗口用满、未订阅）：组织账户、所需订阅
// 档位与推理额度上限反映不到用量接口里，提前解除会让每个检测周期都重放一次失败请求。
func TestClineUsageProbeClearsOnlyProvablePassCooldowns(t *testing.T) {
	ctx := withTempUnschedulableModel(context.Background(), []string{"cline-pass/glm-5.3"})
	balance := `{"success":true,"data":{"balance":5000000}}`
	for _, tc := range []struct {
		status  int
		body    string
		cleared bool
	}{
		{http.StatusTooManyRequests, clinePassWeeklyBody, true},
		{http.StatusForbidden, clineNotSubscribedBody, false},
		{http.StatusForbidden, clineOrgPassBody, false},
		{http.StatusTooManyRequests, clineInferenceCapBody, false},
	} {
		account := clineTestAccount(65)
		NewRateLimitService(&commandCodeRateLimitRepo{}, nil, &config.Config{}, nil, nil).HandleUpstreamError(ctx, account,
			tc.status, http.Header{}, []byte(tc.body))
		require.True(t, account.isRateLimitActiveForKey(clinePassRateLimitKey), tc.body)
		repo := &clineAccountRepo{accounts: []Account{*account}}
		upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: clineUsageLimitsBody}, balance)

		_, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

		require.NoError(t, err)
		_, touched := repo.modelLimits[65][clinePassRateLimitKey]
		require.Equal(t, tc.cleared, touched, tc.body)
	}

	// 探测发现未订阅设的冷却，订阅后的下一次探测解除。
	repo := &clineAccountRepo{accounts: []Account{*clineTestAccount(66)}}
	unsubscribed := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusNotFound,
		body: `{"success":false,"error":"no plan history found for user"}`}, balance)
	_, err := NewCNProviderQuotaService(repo, nil, unsubscribed, clineUsageTestConfig()).QueryUsage(context.Background(), 66)
	require.NoError(t, err)
	require.True(t, repo.modelLimits[66][clinePassRateLimitKey].After(time.Now()))

	subscribed := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: clineUsageLimitsBody}, balance)
	_, err = NewCNProviderQuotaService(repo, nil, subscribed, clineUsageTestConfig()).QueryUsage(context.Background(), 66)
	require.NoError(t, err)
	require.False(t, repo.modelLimits[66][clinePassRateLimitKey].After(time.Now()))
}

// 用量接口鉴权失败：探测失败但不写快照、不动冷却。
func TestClineUsageProbeAuthFailure(t *testing.T) {
	account := clineTestAccount(64)
	repo := &clineAccountRepo{accounts: []Account{*account}}
	upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusUnauthorized, body: `{"error":"Unauthorized"}`}, `{}`)

	result, err := NewCNProviderQuotaService(repo, nil, upstream, clineUsageTestConfig()).QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.False(t, result.Success)
	require.False(t, result.CredentialValid)
	require.Equal(t, http.StatusUnauthorized, result.StatusCode)
	require.Empty(t, repo.extraWrites)
	require.Empty(t, repo.modelLimits)
}

// 周期检测探测官方主机上的全部激活账号（含手动停调的，钱包冷却与调度开关无关），
// 只冷却钱包，不停整个账号。
func TestCNProviderBalanceCheckRunOnceProbesClineUsage(t *testing.T) {
	official := *clineTestAccount(1)
	manualOff := *clineTestAccount(2)
	manualOff.Schedulable = false
	relay := *clineTestAccount(3)
	relay.Credentials["base_url"] = "https://relay.example.com/v1"
	relay.Credentials["api_protocol"] = APIProtocolChatCompletions

	repo := &clineAccountRepo{accounts: []Account{official, manualOff, relay}}
	upstream := newClineAccountUpstream(commandCodeAlphaResponse{status: http.StatusOK, body: clineUsageLimitsBody}, `{"success":true,"data":{"balance":200000}}`)
	cfg := clineUsageTestConfig()
	svc := &CNProviderBalanceCheckService{
		accountRepo:    repo,
		balanceService: NewCNProviderBalanceService(repo, nil, upstream, cfg),
		quotaService:   NewCNProviderQuotaService(repo, nil, upstream, cfg),
		cfg:            cfg,
	}

	svc.runOnce()

	require.Len(t, upstream.requests, 6, "usage-limits + me + balance for each official account")
	require.Len(t, repo.extraWrites, 2)
	require.Contains(t, repo.modelLimits[1], clineCreditsRateLimitKey)
	require.Contains(t, repo.modelLimits[2], clineCreditsRateLimitKey)
	require.Empty(t, repo.paused)
}

// 同一轮检测里每个账号走自己平台登记的检测函数：积分同样低于阈值，Command Code 停整个
// 账号，Cline 不停（钱包冷却在探测中处理）。
func TestCNProviderBalanceCheckRunOnceDispatchesAccountProbesByPlatform(t *testing.T) {
	commandCode := *commandCodeUsageAccount()
	commandCode.ID, commandCode.Schedulable = 1, true
	cline := *clineTestAccount(2)
	repo := &commandCodeCheckRepo{accounts: []Account{commandCode, cline}}
	prober := &commandCodeBalanceProber{balance: 0.2}
	svc := &CNProviderBalanceCheckService{accountRepo: repo, quotaService: prober, cfg: clineUsageTestConfig()}

	svc.runOnce()

	require.ElementsMatch(t, []int64{1, 2}, prober.probed)
	require.Equal(t, []int64{1}, repo.paused)
}

// 官方主机不做上游计费探测：中转站的计费接口在官方 API 上不存在，只会把 Key 发往官方主机。
func TestClineOfficialHostSkipsUpstreamBillingProbe(t *testing.T) {
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI(DefaultClineBaseURL))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://relay.example.com/v1"))
}
