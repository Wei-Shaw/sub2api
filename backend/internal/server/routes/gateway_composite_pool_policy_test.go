//go:build unit

package routes

// composite 账号池 handler 级守卫测试：经真实 RegisterGatewayRoutes 链路验证
// 1) previous_response_id 状态归属守卫（owner 合法但调度未命中 pin 时明确拒绝、
//    不外呼、selector 预占槽位恰一次释放）；2) reasoning-effort deny 早退同样
//    恰一次释放；3) 非 pool 请求携带已归属 prevID 保持既有转发行为；4) 池策略
//    cap 按 attempt 应用、failover 跨平台不泄漏。出站经 HTTPUpstream 桩捕获，
//    不发起真实网络请求。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

// countingConcurrencyCache 在 RED fixture 的放行语义上为账号槽位加计数，
// 用于断言守卫早退对 selector 已持有槽位的恰一次释放。
type countingConcurrencyCache struct {
	service.ConcurrencyCache

	acquireAccountCalls atomic.Int64
	releaseAccountCalls atomic.Int64
}

func (m *countingConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (m *countingConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error {
	return nil
}

func (m *countingConcurrencyCache) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	m.acquireAccountCalls.Add(1)
	return true, nil
}

func (m *countingConcurrencyCache) ReleaseAccountSlot(_ context.Context, _ int64, _ string) error {
	m.releaseAccountCalls.Add(1)
	return nil
}

func (m *countingConcurrencyCache) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}

func (m *countingConcurrencyCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = 0
	}
	return result, nil
}

func (m *countingConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (m *countingConcurrencyCache) DecrementAccountWaitCount(context.Context, int64) error {
	return nil
}

func (m *countingConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}

func (m *countingConcurrencyCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (m *countingConcurrencyCache) DecrementWaitCount(context.Context, int64) error {
	return nil
}

func (m *countingConcurrencyCache) GetUserConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}

// stickyMemoryGatewayCache 提供进程内粘性会话绑定，驱动 scheduler 的
// session-sticky 预占槽位路径（Acquired=true + ReleaseFunc）。
type stickyMemoryGatewayCache struct {
	mu         sync.Mutex
	bindings   map[string]int64
	bindingKey func(groupID int64, sessionHash string) string
}

func newStickyMemoryGatewayCache() *stickyMemoryGatewayCache {
	return &stickyMemoryGatewayCache{bindings: make(map[string]int64)}
}

func (c *stickyMemoryGatewayCache) key(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%d:%s", groupID, sessionHash)
}

func (c *stickyMemoryGatewayCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.bindings[c.key(groupID, sessionHash)]
	if !ok {
		return 0, service.ErrStickySessionNotFound
	}
	return id, nil
}

func (c *stickyMemoryGatewayCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings[c.key(groupID, sessionHash)] = accountID
	return nil
}

func (c *stickyMemoryGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *stickyMemoryGatewayCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, c.key(groupID, sessionHash))
	return nil
}

func (c *stickyMemoryGatewayCache) SetGrokVideoPendingBilling(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (c *stickyMemoryGatewayCache) GetGrokVideoPendingBilling(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (c *stickyMemoryGatewayCache) ClaimGrokVideoBilled(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *stickyMemoryGatewayCache) ReleaseGrokVideoBilled(context.Context, string) error {
	return nil
}

func (c *stickyMemoryGatewayCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *stickyMemoryGatewayCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", nil
}

type poolPolicyCapturedRequest struct {
	accountID int64
	host      string
	body      []byte
}

// captureUpstream 捕获出站请求（原始 body），并按 host 注入可配置失败以驱动
// failover；openai 平台账号经 Responses 协议转发，返回最小 completed SSE。
type captureUpstream struct {
	service.HTTPUpstream

	mu       sync.Mutex
	captured []poolPolicyCapturedRequest
	failHost map[string]bool
}

func (u *captureUpstream) capture(req *http.Request, accountID int64) []byte {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	u.mu.Lock()
	u.captured = append(u.captured, poolPolicyCapturedRequest{
		accountID: accountID,
		host:      req.URL.Hostname(),
		body:      body,
	})
	u.mu.Unlock()
	return body
}

func (u *captureUpstream) capturedSnapshot() []poolPolicyCapturedRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]poolPolicyCapturedRequest(nil), u.captured...)
}

func (u *captureUpstream) respondFor(req *http.Request) *http.Response {
	if u.failHost[req.URL.Hostname()] {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"Retry-After":  []string{"60"},
			},
			Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
		}
	}
	if strings.Contains(req.URL.Path, "/responses") {
		sse := "event: response.completed\n" +
			`data: {"type":"response.completed","response":{"id":"resp_pool","object":"response","status":"completed","model":"deepseek-chat","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"
		return compositeDispatchResponse("text/event-stream", sse)
	}
	// id 使用 resp_ 前缀：生产 bindHTTPResponseAccount 会把该 id 绑定到选中
	// 账号，第二条请求的 previous_response_id 需通过 resp_* 形态校验。
	return compositeDispatchResponse("application/json", `{"id":"resp_pool_chat","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
}

func (u *captureUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.capture(req, accountID)
	return u.respondFor(req), nil
}

func (u *captureUpstream) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.capture(req, accountID)
	return u.respondFor(req), nil
}

type poolPolicyRouterInput struct {
	group    *service.Group
	accounts []service.Account
	cache    *countingConcurrencyCache
	upstream *captureUpstream
	apiKeyID int64
}

const (
	poolPolicyGroupID = int64(4210)
	poolPolicyUserID  = int64(4201)
)

func newCompositePoolPolicyTestRouter(t *testing.T, in poolPolicyRouterInput) (*gin.Engine, *service.OpenAIGatewayService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if in.cache == nil {
		in.cache = &countingConcurrencyCache{}
	}
	if in.upstream == nil {
		in.upstream = &captureUpstream{}
	}
	apiKeyID := in.apiKeyID
	if apiKeyID == 0 {
		apiKeyID = 4202
	}
	repo := &compositeDispatchAccountRepo{accounts: in.accounts}
	groupRepo := &compositeDispatchGroupRepo{group: in.group}

	compositeResolver := service.NewCompositeRouteResolver(nil)

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxBodySize = 1024 * 1024
	cfg.Gateway.TextMaxBodySize = 1024 * 1024
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	billingService := service.NewBillingService(cfg, nil)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	concurrencyService := service.NewConcurrencyService(in.cache)
	deferred := &service.DeferredService{}

	genericGateway := service.NewGatewayService(
		repo, groupRepo, nil, nil, nil, nil, nil, nil, cfg, nil,
		concurrencyService, billingService, nil, billingCache, nil,
		in.upstream, deferred, nil, nil, nil, nil, nil, nil, nil, nil,
		compositeResolver, nil, nil,
	)
	openAIGateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, newStickyMemoryGatewayCache(), cfg, nil, concurrencyService,
		billingService, nil, billingCache, in.upstream, deferred,
		nil, nil, nil, nil, nil, nil, nil,
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
			groupID := in.group.ID
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				ID:      apiKeyID,
				GroupID: &groupID,
				User:    &service.User{ID: poolPolicyUserID, Status: service.StatusActive},
				Group:   in.group,
			})
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: poolPolicyUserID, Concurrency: 1})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		compositeResolver,
		cfg,
	)
	return router, openAIGateway
}

func newPoolPolicyGroup(mutate func(group *service.Group)) *service.Group {
	group := &service.Group{
		ID:       poolPolicyGroupID,
		Platform: service.PlatformComposite,
		Status:   service.StatusActive,
	}
	if mutate != nil {
		mutate(group)
	}
	return group
}

func newPoolPolicyAccount(id int64, platform string, priority int) service.Account {
	return newPoolPolicyAccountWithMapping(id, platform, priority, map[string]any{
		"deepseek-flash": "deepseek-chat",
	})
}

func newPoolPolicyAccountWithMapping(id int64, platform string, priority int, mapping map[string]any) service.Account {
	return service.Account{
		ID: id, Name: "pool-" + platform,
		Platform: platform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: priority,
		Credentials: map[string]any{
			"api_key":       "test-key-" + platform,
			"base_url":      "https://" + platform + "-pool-fake.test/v1",
			"model_mapping": mapping,
		},
		GroupIDs:      []int64{poolPolicyGroupID},
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: poolPolicyGroupID, Priority: 1}},
	}
}

func postPoolPolicyRequest(t *testing.T, router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// 池请求携带已归属本用户的 previous_response_id、调度未命中状态归属账号
// （StickyPreviousHit=false）时：任何转发之前明确 400 拒绝，不外呼，sticky
// 层预占的账号槽位被恰一次释放。
func TestCompositePoolPreviousResponseIDRejectsWithoutPin(t *testing.T) {
	cache := &countingConcurrencyCache{}
	upstream := &captureUpstream{}
	group := newPoolPolicyGroup(nil)
	router, openAIGateway := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    group,
		accounts: []service.Account{newPoolPolicyAccount(4211, service.PlatformOpenAI, 1), newPoolPolicyAccount(4212, service.PlatformDeepseek, 2)},
		cache:    cache,
		upstream: upstream,
	})

	// 请求 A（无 prevID，带 prompt_cache_key）：建立 sticky 绑定。
	wA := postPoolPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-pool-guard","stream":false}`)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	acquireAfterA := cache.acquireAccountCalls.Load()

	// 预绑定 response 归属（模拟该 prevID 属于本用户的历史响应）。
	require.NoError(t, openAIGateway.BindOpenAIHTTPResponseOwner(context.Background(), poolPolicyGroupID, "resp_owned", poolPolicyUserID, 4202))

	// 请求 B（同会话 + prevID）：未命中 pin → 拒绝且不外呼。
	wB := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"hi","prompt_cache_key":"sess-pool-guard","previous_response_id":"resp_owned","stream":false}`)

	require.Equal(t, http.StatusBadRequest, wB.Code, wB.Body.String())
	require.Contains(t, wB.Body.String(), "previous_response_id")
	require.Equal(t, acquireAfterA, cache.acquireAccountCalls.Load()-1,
		"request B must pre-acquire the account slot once via the sticky layer")
	require.Equal(t, acquireAfterA+1, cache.releaseAccountCalls.Load(),
		"guard must release the pre-acquired slot exactly once")
}

// reasoning-effort deny 在账号选定后触发：403 拒绝、不外呼、sticky 层预占的
// 槽位恰一次释放（不泄漏、不重复释放）。
func TestCompositePoolReasoningPolicyDenyReleasesSelectorSlot(t *testing.T) {
	cache := &countingConcurrencyCache{}
	upstream := &captureUpstream{}
	group := newPoolPolicyGroup(func(group *service.Group) {
		group.ReasoningEffortMappings = []service.ReasoningEffortMapping{{From: "xhigh", To: "deny"}}
	})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    group,
		accounts: []service.Account{newPoolPolicyAccount(4211, service.PlatformOpenAI, 1), newPoolPolicyAccount(4212, service.PlatformDeepseek, 2)},
		cache:    cache,
		upstream: upstream,
	})

	// 请求 A：建立 sticky 绑定（无 deny 冲突的正常 effort）。
	wA := postPoolPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-pool-deny","reasoning_effort":"low","stream":false}`)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	acquireAfterA := cache.acquireAccountCalls.Load()
	releaseAfterA := cache.releaseAccountCalls.Load()

	// 请求 B：同会话 + deny effort → 选号预占后由策略守卫终止。
	wB := postPoolPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"sess-pool-deny","reasoning_effort":"xhigh","stream":false}`)

	require.Equal(t, http.StatusForbidden, wB.Code, wB.Body.String())
	require.Equal(t, upstream.capturedSnapshot()[len(upstream.capturedSnapshot())-1].accountID != 0, true)
	require.Equal(t, acquireAfterA+1, cache.acquireAccountCalls.Load(),
		"request B must pre-acquire the account slot once via the sticky layer")
	require.Equal(t, releaseAfterA+1, cache.releaseAccountCalls.Load(),
		"deny path must release the pre-acquired slot exactly once")
}

// 非 pool 请求（单一 openai 目标）携带已归属 prevID 保持既有转发行为，不受
// 池状态守卫影响。
func TestNonCompositePoolPreviousResponseIDStillForwards(t *testing.T) {
	upstream := &captureUpstream{}
	group := &service.Group{ID: poolPolicyGroupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	router, openAIGateway := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    group,
		accounts: []service.Account{newPoolPolicyAccount(4211, service.PlatformOpenAI, 1)},
		upstream: upstream,
	})
	require.NoError(t, openAIGateway.BindOpenAIHTTPResponseOwner(context.Background(), poolPolicyGroupID, "resp_owned", poolPolicyUserID, 4202))

	w := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"hi","previous_response_id":"resp_owned","stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotEmpty(t, upstream.capturedSnapshot(), "non-pool requests keep the existing prevID forwarding behavior")
}

// 池策略 cap 按 attempt 应用：deepseek 账号首跳失败后 failover 到 openai，
// openai attempt 使用从 canonical body 新鲜 cap 的 medium——策略按 attempt
// 应用、canonical body 不被改写。
func TestCompositePoolReasoningPolicyCapAppliedPerAttempt(t *testing.T) {
	upstream := &captureUpstream{failHost: map[string]bool{
		"deepseek-pool-fake.test": true,
	}}
	group := newPoolPolicyGroup(func(group *service.Group) {
		group.MaxReasoningEffort = "medium"
	})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group: group,
		accounts: []service.Account{
			newPoolPolicyAccount(4211, service.PlatformDeepseek, 1),
			newPoolPolicyAccount(4212, service.PlatformOpenAI, 2),
		},
		upstream: upstream,
	})

	w := postPoolPolicyRequest(t, router, "/v1/chat/completions",
		`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"xhigh","stream":false}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	captured := upstream.capturedSnapshot()
	require.Len(t, captured, 2, "one attempt per account after failover")
	byHost := make(map[string]poolPolicyCapturedRequest, len(captured))
	for _, record := range captured {
		byHost[record.host] = record
	}
	deepseekAttempt, ok := byHost["deepseek-pool-fake.test"]
	require.True(t, ok, "deepseek account must be attempted first")
	require.Equal(t, "xhigh", gjson.GetBytes(deepseekAttempt.body, "reasoning_effort").String(),
		"non-openai attempt must keep the client effort uncapped")
	openaiAttempt, ok := byHost["openai-pool-fake.test"]
	require.True(t, ok, "failover must reach the openai account")
	// openai 平台账号经 CC→Responses 协议转发，出站体为 reasoning.effort。
	require.Equal(t, "medium", gjson.GetBytes(openaiAttempt.body, "reasoning.effort").String(),
		"openai attempt must carry the group-capped effort applied per attempt")
}

// CN 账号真正的 previous_response pin（正向，依赖 2A 的 pool HTTP 资格门修复）：
// 第一条用仅 CN 父账号 claim 的模型建立 resp→CN 绑定；第二条换新 session 种子
// 并让另一候选优先级更高（排除了 same-session sticky 的解释），只有
// previous_response_id 命中原 CN 账号才能选中它并转发。2A 修复前该用例 RED。
func TestCompositePoolCNPreviousResponseIDPinBeatsPriorityAndSeed(t *testing.T) {
	upstream := &captureUpstream{}
	group := newPoolPolicyGroup(nil)
	// kimi 优先级更高（p1）：无 pin 时 load-balance 会选 kimi；
	// deepseek（p2，native Responses）是 resp id 的归属账号。
	cnParent := newPoolPolicyAccountWithMapping(4211, service.PlatformDeepseek, 2, map[string]any{
		"deepseek-flash": "deepseek-chat",
		"pool-model":     "deepseek-chat",
	})
	cnParent.Credentials["api_protocol"] = service.APIProtocolResponses
	other := newPoolPolicyAccountWithMapping(4212, service.PlatformKimi, 1, map[string]any{
		"pool-model": "kimi-chat",
	})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    group,
		accounts: []service.Account{cnParent, other},
		upstream: upstream,
	})

	// 第一条：仅 deepseek claim 的模型 → 绑定 resp id → deepseek。
	wA := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"hi","prompt_cache_key":"sess-pin-a","stream":false}`)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	capturedA := upstream.capturedSnapshot()
	require.Len(t, capturedA, 1)
	require.Equal(t, int64(4211), capturedA[0].accountID, "first request must be served by the CN parent account")

	responseID := gjson.GetBytes(wA.Body.Bytes(), "id").String()
	require.True(t, strings.HasPrefix(responseID, "resp_"), "downstream response must carry a resp_* id, got %q", responseID)

	// 第二条：新 session 种子（sticky 不可命中）+ 双账号池模型 + kimi 优先级更高；
	// 只有 previous_response_id 的真 pin 才能解释选中 deepseek。
	wB := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"pool-model","input":"continue","prompt_cache_key":"sess-pin-b","previous_response_id":"`+responseID+`","stream":false}`)

	require.Equal(t, http.StatusOK, wB.Code, wB.Body.String())
	captured := upstream.capturedSnapshot()
	require.Len(t, captured, 2)
	require.Equal(t, int64(4211), captured[1].accountID,
		"previous_response_id pin must select the owning CN account over the higher-priority candidate")
	require.Equal(t, "deepseek-pool-fake.test", captured[1].host)
}

// CN→CN 未 hit 跨账号转移必须拒绝：resp id 归属 deepseek，deepseek 首跳 429
// failover 后选中另一 CN 候选（kimi）时，状态守卫在任何转发前 400 终止，
// kimi 不收到带旧 prevID 的请求（该用例在 2A 修复前后都必须保持绿）。
func TestCompositePoolPreviousResponseIDRejectsCrossCNSelection(t *testing.T) {
	upstream := &captureUpstream{}
	group := newPoolPolicyGroup(nil)
	cnParent := newPoolPolicyAccountWithMapping(4211, service.PlatformDeepseek, 1, map[string]any{
		"deepseek-flash": "deepseek-chat",
		"pool-model":     "deepseek-chat",
	})
	cnParent.Credentials["api_protocol"] = service.APIProtocolResponses
	other := newPoolPolicyAccountWithMapping(4212, service.PlatformKimi, 2, map[string]any{
		"pool-model": "kimi-chat",
	})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    group,
		accounts: []service.Account{cnParent, other},
		upstream: upstream,
	})

	// 第一条：仅 deepseek claim 的模型 → 绑定 resp id → deepseek。
	wA := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"deepseek-flash","input":"hi","prompt_cache_key":"sess-cross-a","stream":false}`)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	responseID := gjson.GetBytes(wA.Body.Bytes(), "id").String()
	require.True(t, strings.HasPrefix(responseID, "resp_"), "downstream response must carry a resp_* id, got %q", responseID)

	// 第二条：新 session 种子 + 双账号池模型；武装 deepseek 首跳 429，
	// failover 到 kimi 时守卫必须拒绝——kimi 不收到带旧 prevID 的转发。
	upstream.mu.Lock()
	upstream.failHost = map[string]bool{"deepseek-pool-fake.test": true}
	upstream.mu.Unlock()
	capturedBefore := len(upstream.capturedSnapshot())
	wB := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"pool-model","input":"continue","prompt_cache_key":"sess-cross-b","previous_response_id":"`+responseID+`","stream":false}`)

	require.Equal(t, http.StatusBadRequest, wB.Code, wB.Body.String())
	require.Contains(t, wB.Body.String(), "previous_response_id")
	for _, record := range upstream.capturedSnapshot()[capturedBefore:] {
		require.NotEqual(t, "kimi-pool-fake.test", record.host,
			"the non-owning CN account must never receive the old prevID")
	}
}

// claude-* 别名被两个兼容族账号同时 claim 时，/v1/messages 不套 gpt-5.x
// 默认 dispatch 映射（池含 CN），仍按账号级 model_mapping 转发。
func TestCompositePoolMessagesClaudeAliasUsesAccountMapping(t *testing.T) {
	upstream := &captureUpstream{}
	group := newPoolPolicyGroup(nil)
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group: group,
		accounts: []service.Account{
			newPoolPolicyAccountWithMapping(4211, service.PlatformDeepseek, 1, map[string]any{"claude-alias": "deepseek-chat"}),
			newPoolPolicyAccountWithMapping(4212, service.PlatformKimi, 2, map[string]any{"claude-alias": "kimi-claude"}),
		},
		upstream: upstream,
	})

	w := postPoolPolicyRequest(t, router, "/v1/messages",
		`{"model":"claude-alias","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	captured := upstream.capturedSnapshot()
	require.NotEmpty(t, captured, "claude alias claimed by pool accounts must forward")
	require.Equal(t, "deepseek-pool-fake.test", captured[0].host)
	require.Equal(t, "deepseek-chat", gjson.GetBytes(captured[0].body, "model").String(),
		"outbound model must come from the selected account's model_mapping, not the gpt-5.x dispatch default")
}
