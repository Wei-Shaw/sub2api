//go:build unit

package routes

// composite 账号池 per-attempt 平台策略（审查修复增量4）的 handler 级回归：
// 经真实 RegisterGatewayRoutes + fake upstream 验证
//  1. 渠道限制拒绝选中平台时不发付费外呼，普通请求 mask 后换其他平台；
//  2. 平台配额耗尽时不发往该平台，成功请求只向实际选中平台计量配额；
//  3. RPM 每请求只按准入计一次（policy deny 重选不重复计数）；
//  4. 公开模型 → 选中平台渠道映射 → 账号 model_mapping 的顺序按 attempt 独立；
//  5. pinned（previous_response_id 命中状态归属）请求遭配额拒绝时返回原 policy
//     错误（429+Retry-After）、不外呼、不换账号、不删除绑定。
// 出站经 HTTPUpstream 桩捕获，不发起真实网络请求。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	attemptPolicyGroupID = int64(4330)
	attemptPolicyUserID  = int64(4331)
	attemptPolicyAPIKey  = int64(4332)
)

// ── fakes ────────────────────────────────────────────────────────────────────

type attemptPolicyCapturedRequest struct {
	accountID int64
	host      string
	path      string
	body      []byte
}

// attemptPolicyUpstream 捕获出站请求并按 host 注入可配置失败。
type attemptPolicyUpstream struct {
	service.HTTPUpstream

	mu       sync.Mutex
	captured []attemptPolicyCapturedRequest
	failHost map[string]bool
}

func (u *attemptPolicyUpstream) capture(req *http.Request, accountID int64) []byte {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	u.mu.Lock()
	u.captured = append(u.captured, attemptPolicyCapturedRequest{accountID: accountID, host: req.URL.Hostname(), path: req.URL.Path, body: body})
	u.mu.Unlock()
	return body
}

func (u *attemptPolicyUpstream) snapshot() []attemptPolicyCapturedRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]attemptPolicyCapturedRequest(nil), u.captured...)
}

func (u *attemptPolicyUpstream) hosts() []string {
	snapshot := u.snapshot()
	hosts := make([]string, 0, len(snapshot))
	for _, item := range snapshot {
		hosts = append(hosts, item.host)
	}
	return hosts
}

func (u *attemptPolicyUpstream) respondFor(req *http.Request, body []byte) *http.Response {
	if u.failHost[req.URL.Hostname()] {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"60"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
		}
	}
	if strings.Contains(req.URL.Path, "/responses") {
		// 原生 Responses 上游：stream=true 回最小 completed SSE；stream=false 回
		// Responses JSON 对象（含 id，供响应→账号绑定与 usage 解析）。
		if gjson.GetBytes(body, "stream").Bool() {
			sse := "event: response.completed\n" +
				`data: {"type":"response.completed","response":{"id":"resp_pin","object":"response","status":"completed","model":"deepseek-chat","output":[],"usage":{"input_tokens":200000,"output_tokens":100000,"total_tokens":300000}}}` + "\n\n"
			return compositeDispatchResponse("text/event-stream", sse)
		}
		return compositeDispatchResponse("application/json",
			`{"id":"resp_pin","object":"response","status":"completed","model":"deepseek-chat","output":[],"usage":{"input_tokens":200000,"output_tokens":100000,"total_tokens":300000}}`)
	}
	return compositeDispatchResponse("application/json", `{"id":"resp_pin","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":200000,"completion_tokens":100000,"total_tokens":300000}}`)
}

func (u *attemptPolicyUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body := u.capture(req, accountID)
	return u.respondFor(req, body), nil
}

func (u *attemptPolicyUpstream) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	body := u.capture(req, accountID)
	return u.respondFor(req, body), nil
}

// attemptPolicyQuotaCache 平台可感知的 user×platform 配额缓存：entries 缺席视为
// MISS（DB fail-open 放行），incrPlatforms 记录每次配额累加的平台维度。
type attemptPolicyQuotaCache struct {
	service.BillingCache

	mu           sync.Mutex
	entries      map[string]*service.UserPlatformQuotaCacheEntry
	incrPlatform []string
}

func (f *attemptPolicyQuotaCache) GetUserBalance(context.Context, int64) (float64, error) {
	return 100.0, nil
}

func (f *attemptPolicyQuotaCache) InvalidateUserBalance(context.Context, int64) error { return nil }

func (f *attemptPolicyQuotaCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, platform string) (*service.UserPlatformQuotaCacheEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[platform]
	return entry, ok, nil
}

func (f *attemptPolicyQuotaCache) SetUserPlatformQuotaCache(context.Context, int64, string, *service.UserPlatformQuotaCacheEntry, time.Duration) error {
	return nil
}

func (f *attemptPolicyQuotaCache) IncrUserPlatformQuotaUsageCache(_ context.Context, _ int64, platform string, _ float64, _ time.Duration, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.incrPlatform = append(f.incrPlatform, platform)
	return nil
}

func (f *attemptPolicyQuotaCache) setEntry(platform string, entry *service.UserPlatformQuotaCacheEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry == nil {
		delete(f.entries, platform)
		return
	}
	f.entries[platform] = entry
}

func (f *attemptPolicyQuotaCache) incrSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.incrPlatform...)
}

func exhaustedAttemptPolicyEntry(now time.Time) *service.UserPlatformQuotaCacheEntry {
	zero := 0.0
	return &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &zero,
		DailyWindowStart: &now,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
	}
}

func allowedAttemptPolicyEntry(now time.Time) *service.UserPlatformQuotaCacheEntry {
	limit := 5.0
	return &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &limit,
		DailyWindowStart: &now,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
	}
}

// attemptPolicyRPMCache 记录 RPM 递增次数：policy deny 重选不得重复计数。
type attemptPolicyRPMCache struct {
	service.UserRPMCache

	mu    sync.Mutex
	group int
	user  int
}

func (f *attemptPolicyRPMCache) IncrementUserGroupRPM(context.Context, int64, int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.group++
	return f.group, nil
}

func (f *attemptPolicyRPMCache) IncrementUserRPM(context.Context, int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user++
	return f.user, nil
}

func (f *attemptPolicyRPMCache) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.group + f.user
}

type attemptPolicyUserRepo struct {
	service.UserRepository

	mu          sync.Mutex
	deductCalls int
}

func (f *attemptPolicyUserRepo) DeductBalance(context.Context, int64, float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deductCalls++
	return nil
}

// attemptPolicyStickyCache 带计数器的进程内 sticky 会话缓存：验证 policy deny
// masked 重选不会清掉原 sticky 绑定（DeleteSessionAccountID 零调用）。
type attemptPolicyStickyCache struct {
	service.GatewayCache

	mu        sync.Mutex
	bindings  map[string]int64
	setCnt    int
	deleteCnt int
}

func newAttemptPolicyStickyCache() *attemptPolicyStickyCache {
	return &attemptPolicyStickyCache{bindings: make(map[string]int64)}
}

func (c *attemptPolicyStickyCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.bindings[fmt.Sprintf("%d:%s", groupID, sessionHash)]
	if !ok {
		return 0, service.ErrStickySessionNotFound
	}
	return id, nil
}

func (c *attemptPolicyStickyCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCnt++
	c.bindings[fmt.Sprintf("%d:%s", groupID, sessionHash)] = accountID
	return nil
}

func (c *attemptPolicyStickyCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *attemptPolicyStickyCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteCnt++
	delete(c.bindings, fmt.Sprintf("%d:%s", groupID, sessionHash))
	return nil
}

func (c *attemptPolicyStickyCache) SetGrokVideoPendingBilling(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (c *attemptPolicyStickyCache) GetGrokVideoPendingBilling(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (c *attemptPolicyStickyCache) ClaimGrokVideoBilled(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *attemptPolicyStickyCache) ReleaseGrokVideoBilled(context.Context, string) error {
	return nil
}

func (c *attemptPolicyStickyCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *attemptPolicyStickyCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", nil
}

func (c *attemptPolicyStickyCache) snapshot() (map[string]int64, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int64, len(c.bindings))
	for key, value := range c.bindings {
		out[key] = value
	}
	return out, c.setCnt, c.deleteCnt
}

type attemptPolicyQuotaRepo struct {
	service.UserPlatformQuotaRepository
}

func (f *attemptPolicyQuotaRepo) GetByUserPlatform(context.Context, int64, string) (*service.UserPlatformQuotaRecord, error) {
	return nil, nil
}

type attemptPolicyChannelRepo struct {
	service.ChannelRepository
	channels  []service.Channel
	platforms map[int64]string
}

func (f *attemptPolicyChannelRepo) ListAll(context.Context) ([]service.Channel, error) {
	return f.channels, nil
}

func (f *attemptPolicyChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return f.platforms, nil
}

// ── fixtures ─────────────────────────────────────────────────────────────────

func newAttemptPolicyAccount(id int64, platform string, priority int, modelMapping map[string]any) service.Account {
	credentials := map[string]any{
		"api_key":  "test-key-" + platform,
		"base_url": "https://" + platform + "-pool-fake.test/v1",
	}
	if modelMapping != nil {
		credentials["model_mapping"] = modelMapping
	}
	return service.Account{
		ID: id, Name: "pool-" + platform,
		Platform: platform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: priority,
		Credentials: credentials,
		GroupIDs:    []int64{attemptPolicyGroupID},
		AccountGroups: []service.AccountGroup{
			{AccountID: id, GroupID: attemptPolicyGroupID, Priority: 1},
		},
	}
}

type attemptPolicyRouterInput struct {
	accounts    []service.Account
	quotaCache  *attemptPolicyQuotaCache
	rpmCache    *attemptPolicyRPMCache
	upstream    *attemptPolicyUpstream
	channel     *service.Channel
	userRepo    *attemptPolicyUserRepo
	stickyCache *attemptPolicyStickyCache
	groupMutate func(group *service.Group)
}

func newCompositePoolAttemptPolicyRouter(t *testing.T, in attemptPolicyRouterInput) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if in.quotaCache == nil {
		in.quotaCache = &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{}}
	}
	if in.upstream == nil {
		in.upstream = &attemptPolicyUpstream{}
	}
	if in.stickyCache == nil {
		in.stickyCache = newAttemptPolicyStickyCache()
	}
	if in.userRepo == nil {
		in.userRepo = &attemptPolicyUserRepo{}
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.MaxBodySize = 1024 * 1024
	cfg.Gateway.TextMaxBodySize = 1024 * 1024
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	repo := &compositeDispatchAccountRepo{accounts: in.accounts}
	group := &service.Group{
		ID:             attemptPolicyGroupID,
		Platform:       service.PlatformComposite,
		Status:         service.StatusActive,
		RateMultiplier: 1,
	}
	if in.groupMutate != nil {
		in.groupMutate(group)
	}
	groupRepo := &compositeDispatchGroupRepo{group: group}

	var channelSvc *service.ChannelService
	if in.channel != nil {
		channelSvc = service.NewChannelService(&attemptPolicyChannelRepo{
			channels:  []service.Channel{*in.channel},
			platforms: map[int64]string{attemptPolicyGroupID: service.PlatformComposite},
		}, nil, nil, nil, nil)
	}

	billingService := service.NewBillingService(cfg, nil)
	pricingResolver := service.NewModelPricingResolver(nil, billingService)
	billingCache := service.NewBillingCacheService(
		in.quotaCache, nil, nil, nil, in.rpmCache, nil, cfg, &attemptPolicyQuotaRepo{})
	concurrencyService := service.NewConcurrencyService(&compositeDispatchConcurrencyCache{})
	deferred := &service.DeferredService{}

	compositeResolver := service.NewCompositeRouteResolver(nil)

	genericGateway := service.NewGatewayService(
		repo, groupRepo, nil, nil, nil, nil, nil, nil, cfg, nil,
		concurrencyService, billingService, nil, billingCache, nil,
		in.upstream, deferred, nil, nil, nil, nil, nil, nil, channelSvc, nil,
		compositeResolver, nil, nil,
	)
	openAIGateway := service.NewOpenAIGatewayService(
		repo, nil, nil, in.userRepo, nil, nil, in.stickyCache, cfg, nil, concurrencyService,
		billingService, nil, billingCache, in.upstream, deferred,
		nil, nil, pricingResolver, channelSvc, nil, nil, &attemptPolicyQuotaRepo{},
	)

	handlers := &handler.Handlers{
		Gateway: handler.NewGatewayHandler(
			genericGateway, nil, nil, nil, nil,
			concurrencyService, billingCache, nil, nil, nil, nil, nil, nil,
			cfg, nil,
		),
		OpenAIGateway: handler.NewOpenAIGatewayHandler(
			openAIGateway, concurrencyService, billingCache, &service.APIKeyService{},
			nil, nil, nil, nil, cfg,
		),
		AsyncImage: handler.NewAsyncImageHandler(nil, nil),
	}

	router := gin.New()
	RegisterGatewayRoutes(
		router,
		handlers,
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := group.ID
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				ID:      attemptPolicyAPIKey,
				GroupID: &groupID,
				User:    &service.User{ID: attemptPolicyUserID, Status: service.StatusActive},
				Group:   group,
			})
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: attemptPolicyUserID, Concurrency: 1})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		compositeResolver,
		cfg,
	)
	return router
}

func postAttemptPolicyRequest(t *testing.T, router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── 1) 渠道限制拒绝选中平台 → 不付费外呼该平台，mask 后换其他平台 ────────────
func TestCompositePoolAttemptPolicyChannelRestrictionSwitchesPlatform(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:   allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{}
	// 渠道只把 deepseek-flash 列入 openai 平台允许定价；deepseek 平台无任何
	// 定价行 → RestrictModels 下 deepseek 平台对该模型受限。
	channel := &service.Channel{
		ID:                 1,
		Status:             service.StatusActive,
		GroupIDs:           []int64{attemptPolicyGroupID},
		RestrictModels:     true,
		BillingModelSource: service.BillingModelSourceRequested,
		ModelPricing: []service.ChannelModelPricing{
			{Platform: service.PlatformOpenAI, Models: []string{"deepseek-flash"}},
		},
	}
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4334, service.PlatformOpenAI, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache: quotaCache,
		upstream:   upstream,
		channel:    channel,
	})

	w := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	hosts := upstream.hosts()
	require.NotEmpty(t, hosts)
	for _, host := range hosts {
		require.NotContains(t, host, "deepseek-pool-fake.test",
			"channel-restricted platform must never receive the paid request")
		require.Contains(t, host, "openai-pool-fake.test",
			"request must be served by an allowed platform")
	}
}

// ── 2) 平台配额耗尽 → 不发往该平台；成功请求只对选中平台计量配额 ────────────
func TestCompositePoolAttemptPolicyQuotaExhaustedSkipsPlatform(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformOpenAI:   exhaustedAttemptPolicyEntry(now),
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{}
	userRepo := &attemptPolicyUserRepo{}
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4333, service.PlatformOpenAI, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4334, service.PlatformDeepseek, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache: quotaCache,
		upstream:   upstream,
		userRepo:   userRepo,
	})

	w := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, host := range upstream.hosts() {
		require.NotContains(t, host, "openai-pool-fake.test",
			"quota-exhausted platform must not receive the request")
	}
	require.NotEmpty(t, upstream.hosts(), "request must still be served by the healthy platform")

	incremented := quotaCache.incrSnapshot()
	require.NotEmpty(t, incremented, "successful usage must increment the user×platform quota")
	for _, platform := range incremented {
		require.Equal(t, service.PlatformDeepseek, platform,
			"quota must be deducted on the actually selected platform, never composite/openai default")
	}
}

// ── 3) RPM 只按准入计一次（policy deny 重选不重复计数） ───────────────────────
func TestCompositePoolAttemptPolicyDenyDoesNotDoubleCountRPM(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:   allowedAttemptPolicyEntry(now),
	}}
	rpmCache := &attemptPolicyRPMCache{}
	upstream := &attemptPolicyUpstream{}
	channel := &service.Channel{
		ID:                 1,
		Status:             service.StatusActive,
		GroupIDs:           []int64{attemptPolicyGroupID},
		RestrictModels:     true,
		BillingModelSource: service.BillingModelSourceRequested,
		ModelPricing: []service.ChannelModelPricing{
			{Platform: service.PlatformOpenAI, Models: []string{"deepseek-flash"}},
		},
	}
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4334, service.PlatformOpenAI, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache:  quotaCache,
		rpmCache:    rpmCache,
		upstream:    upstream,
		channel:     channel,
		groupMutate: func(group *service.Group) { group.RPMLimit = 10 },
	})

	w := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, upstream.hosts(), 1, "failover via policy mask must end in exactly one paid attempt")
	require.Equal(t, 1, rpmCache.total(),
		"RPM must be counted once at admission; policy deny re-selection must not re-run CheckBillingEligibility")
}

// ── 4) 公开模型 → 选中平台渠道映射 → 账号 mapping，按 attempt 独立 ───────────
func TestCompositePoolAttemptPolicyMappingOrderPerAttempt(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:   allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{failHost: map[string]bool{"deepseek-pool-fake.test": true}}
	// 渠道映射只配置在 openai 平台行：deepseek-flash → deepseek-pro。
	channel := &service.Channel{
		ID:                 1,
		Status:             service.StatusActive,
		GroupIDs:           []int64{attemptPolicyGroupID},
		BillingModelSource: service.BillingModelSourceRequested,
		ModelMapping: map[string]map[string]string{
			service.PlatformOpenAI: {"deepseek-flash": "deepseek-pro"},
		},
	}
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			// deepseek 账号首跳上游失败 → failover；其账号映射 deepseek-flash→deepseek-chat。
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			// openai 账号在渠道映射结果上再套账号映射 deepseek-pro→deepseek-final；
			// identity key 保留 deepseek-flash 以通过池 claims 能力门。公开模型若
			// 被错误地先做账号映射会得到 "account-direct"（渠道行无该映射，保持
			// 原样），与正确顺序的 "deepseek-final" 可判别。
			newAttemptPolicyAccount(4334, service.PlatformOpenAI, 2, map[string]any{"deepseek-flash": "account-direct", "deepseek-pro": "deepseek-final"}),
		},
		quotaCache: quotaCache,
		upstream:   upstream,
		channel:    channel,
	})

	w := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	captured := upstream.snapshot()
	require.Len(t, captured, 2, "one attempt per platform after failover")

	byHost := make(map[string]attemptPolicyCapturedRequest, len(captured))
	for _, item := range captured {
		byHost[item.host] = item
	}
	deepseekAttempt, ok := byHost["deepseek-pool-fake.test"]
	require.True(t, ok, "deepseek account must be attempted first")
	require.Equal(t, "deepseek-chat", gjson.GetBytes(deepseekAttempt.body, "model").String(),
		"deepseek attempt must apply only its own account mapping (no cross-platform channel mapping)")

	openAIAttempt, ok := byHost["openai-pool-fake.test"]
	require.True(t, ok, "failover must reach the openai account")
	require.Equal(t, "deepseek-final", gjson.GetBytes(openAIAttempt.body, "model").String(),
		"openai attempt must apply channel mapping (deepseek-flash→deepseek-pro) then account mapping (→deepseek-final)")
}

// ── 5) pinned 请求遭平台配额拒绝：原 policy 错误、不外呼、不改绑定 ───────────
func TestCompositePoolAttemptPolicyPinnedQuotaDeniedStaysOnOwner(t *testing.T) {
	now := time.Now().UTC()
	// owner 选 openai 平台账号（priority 1）：openai 账号走原生 Responses 转发，
	// 成功后 bindHTTPResponseAccount 落 response→account 与 owner 绑定。
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:   allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{}
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4334, service.PlatformOpenAI, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache: quotaCache,
		upstream:   upstream,
	})

	// 请求 A：/v1/responses 成功 → 上游响应 id=resp_pin 绑定到 openai 账号。
	wA := postAttemptPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"hi","prompt_cache_key":"sess-attempt-pin","stream":false}`)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	capturedAfterA := len(upstream.snapshot())
	require.NotEmpty(t, upstream.hosts())

	// 平台配额耗尽（openai 为状态归属平台）。
	quotaCache.setEntry(service.PlatformOpenAI, exhaustedAttemptPolicyEntry(now))

	// 请求 B：同会话 + prevID 命中状态归属 → 配额拒绝必须以原 policy 错误终止：
	// 429+Retry-After（非 400 掩盖），不外呼、不换账号、绑定不删。
	wB := postAttemptPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"continue","prompt_cache_key":"sess-attempt-pin","previous_response_id":"resp_pin","stream":false}`)
	require.Equal(t, http.StatusTooManyRequests, wB.Code, wB.Body.String())
	require.Contains(t, wB.Body.String(), "rate_limit_exceeded")
	require.NotEmpty(t, wB.Header().Get("Retry-After"))
	require.Len(t, upstream.snapshot(), capturedAfterA, "pinned quota denial must never reach the upstream")

	// 请求 C：配额恢复后同 prevID 仍服务 → 绑定未被删除且未跨账号。
	quotaCache.setEntry(service.PlatformOpenAI, allowedAttemptPolicyEntry(now))
	wC := postAttemptPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"continue","prompt_cache_key":"sess-attempt-pin","previous_response_id":"resp_pin","stream":false}`)
	require.Equal(t, http.StatusOK, wC.Code, wC.Body.String())
	require.Len(t, upstream.snapshot(), capturedAfterA+1)
	lastHost := upstream.hosts()[len(upstream.hosts())-1]
	require.Contains(t, lastHost, "openai-pool-fake.test",
		"binding must stay on the owner platform after a pinned policy denial")
}

// ── 6) policy deny masked 重选不清原 sticky 绑定 ─────────────────────────────
// deny 时同步 failedAccountIDs 排除该账号：下一轮 masked ctx 下 sticky 层对
// excluded 账号提前返回（不 clear），DeleteSessionAccountID 零调用；其它平台
// 成功后 sticky 按用户目标正常改绑（Set 覆盖，而非 Delete）。
func TestCompositePoolAttemptPolicyDenyKeepsStickyBinding(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek: allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:   allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{}
	stickyCache := newAttemptPolicyStickyCache()
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4334, service.PlatformOpenAI, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache:  quotaCache,
		upstream:    upstream,
		stickyCache: stickyCache,
	})

	// 请求 1：session 建立粘性绑定（deepseek priority 1 命中）。
	w1 := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-ap-sticky","stream":false}`)
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())
	bindings1, setCnt1, deleteCnt1 := stickyCache.snapshot()
	t.Logf("DEBUG sticky bindings after R1: %v set=%d delete=%d", bindings1, setCnt1, deleteCnt1)
	require.GreaterOrEqual(t, setCnt1, 1, "session must establish a sticky binding")
	require.Equal(t, 0, deleteCnt1)
	require.NotEmpty(t, bindings1, "sticky binding must exist after the first request")
	for _, accountID := range bindings1 {
		require.Equal(t, int64(4333), accountID, "sticky binding must point at the first-served account")
	}

	// deepseek 平台配额耗尽。
	quotaCache.setEntry(service.PlatformDeepseek, exhaustedAttemptPolicyEntry(now))
	hostsBefore := len(upstream.hosts())

	// 请求 2：同 session → sticky 命中 deepseek → quota 预检拒绝 → mask 重选
	// openai 成功。全程不允许清除原 sticky 绑定。
	w2 := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-ap-sticky","stream":false}`)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	for _, host := range upstream.hosts()[hostsBefore:] {
		require.Contains(t, host, "openai-pool-fake.test",
			"denied platform must not receive the request; failover must reach the healthy platform")
	}

	bindings2, setCnt2, deleteCnt2 := stickyCache.snapshot()
	require.Equal(t, 0, deleteCnt2,
		"policy-deny mask re-selection must never delete the original sticky binding")
	require.Equal(t, 1, len(bindings2), "the same session key must be re-pointed, not fanned out")
	for _, accountID := range bindings2 {
		require.Equal(t, int64(4334), accountID,
			"sticky binding must be re-pointed to the successfully served account via Set (not Delete+re-add churn)")
	}
	require.Greater(t, setCnt2, setCnt1)
}

// ── 7) 兄弟账号同平台：sticky X、deny 落平台后 Y 不外呼、Z 接管、delKeys=0 ────
// 复审 A 场景：池含 X/Y(deepseek)+Z(opencode)，sticky 绑 X；deepseek 平台被
// policy deny 后 masked 重选里 sticky X 与兄弟 Y 都不得被平台失配清绑定，
// 整个 deepseek 平台零外呼，Z 接管后 sticky 经正常 Set 改绑 Z。
func TestCompositePoolAttemptPolicyDenyWithSiblingAccountsKeepsBinding(t *testing.T) {
	now := time.Now().UTC()
	quotaCache := &attemptPolicyQuotaCache{entries: map[string]*service.UserPlatformQuotaCacheEntry{
		service.PlatformDeepseek:   allowedAttemptPolicyEntry(now),
		service.PlatformOpenAI:     allowedAttemptPolicyEntry(now),
		service.PlatformOpenCodeGo: allowedAttemptPolicyEntry(now),
	}}
	upstream := &attemptPolicyUpstream{}
	stickyCache := newAttemptPolicyStickyCache()
	router := newCompositePoolAttemptPolicyRouter(t, attemptPolicyRouterInput{
		accounts: []service.Account{
			newAttemptPolicyAccount(4333, service.PlatformDeepseek, 1, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4335, service.PlatformDeepseek, 2, map[string]any{"deepseek-flash": "deepseek-chat"}),
			newAttemptPolicyAccount(4336, service.PlatformOpenCodeGo, 3, map[string]any{"deepseek-flash": "deepseek-chat"}),
		},
		quotaCache:  quotaCache,
		upstream:    upstream,
		stickyCache: stickyCache,
	})

	// 请求 1：session 绑定 X（deepseek priority 1）。
	w1 := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-ap-sibling","stream":false}`)
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())
	bindings1, _, deleteCnt1 := stickyCache.snapshot()
	require.Equal(t, 0, deleteCnt1)
	require.Len(t, bindings1, 1)
	for _, accountID := range bindings1 {
		require.Equal(t, int64(4333), accountID, "session must bind the first-served deepseek account X")
	}

	// deepseek 平台配额耗尽（X/Y 所在平台整体被 deny）。
	quotaCache.setEntry(service.PlatformDeepseek, exhaustedAttemptPolicyEntry(now))
	hostsBefore := len(upstream.hosts())

	// 请求 2：同 session → sticky X 命中 → deepseek quota 拒绝 → masked 重选：
	// X/Y 同平台均不外呼、绑定不清，Z（opencode）接管成功。
	w2 := postAttemptPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-ap-sibling","stream":false}`)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	hostsR2 := upstream.hosts()[hostsBefore:]
	require.NotEmpty(t, hostsR2)
	for _, host := range hostsR2 {
		require.NotContains(t, host, "deepseek-pool-fake.test",
			"sibling deepseek accounts must never receive the request after the platform denial")
		require.Contains(t, host, "opencode_go-pool-fake.test",
			"request must be served by the remaining healthy platform")
	}

	// 零 delete；成功后 sticky 经 Set 正常改绑 Z。
	_, _, deleteCnt2 := stickyCache.snapshot()
	require.Equal(t, 0, deleteCnt2, "no sticky binding may be deleted by the temporary platform denial")
	bindings2, _, _ := stickyCache.snapshot()
	require.Len(t, bindings2, 1)
	for _, accountID := range bindings2 {
		require.Equal(t, int64(4336), accountID,
			"after a successful take-over the sticky binding is updated normally via Set")
	}
}
