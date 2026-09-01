//go:build unit

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	zhipuMCPTestUserKey   = "sk-sub2api-user-key"
	zhipuMCPTestZhipuKeyA = "zp-key-account-a"
	zhipuMCPTestZhipuKeyB = "zp-key-account-b"
)

func zhipuMCPTestAccount(id int64, apiKey string, priority int, capable bool) service.Account {
	return service.Account{
		ID:          id,
		Platform:    service.PlatformZhipu,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    priority,
		Concurrency: 5,
		Credentials: map[string]any{"api_key": apiKey, "account_mode": service.AccountModeCoding},
		Extra:       map[string]any{service.ZhipuMCPCapabilityExtraKey: capable},
	}
}

// zhipuMCPCapturedRequest fake 上游捕获到的请求。
type zhipuMCPCapturedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// zhipuMCPFakeUpstream 模拟智谱远程 MCP Server：捕获请求，按 attempt 决定响应。
type zhipuMCPFakeUpstream struct {
	t *testing.T

	mu       sync.Mutex
	requests []zhipuMCPCapturedRequest
	respond  func(attempt int, req *zhipuMCPCapturedRequest, w http.ResponseWriter)
}

func (f *zhipuMCPFakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	require.NoError(f.t, err)
	captured := &zhipuMCPCapturedRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body}

	f.mu.Lock()
	attempt := len(f.requests)
	f.requests = append(f.requests, *captured)
	respond := f.respond
	f.mu.Unlock()

	respond(attempt, captured, w)
}

func (f *zhipuMCPFakeUpstream) captured() []zhipuMCPCapturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]zhipuMCPCapturedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// zhipuMCPUpstreamStub service.HTTPUpstream 测试桩：把上游请求重写到 fake server。
type zhipuMCPUpstreamStub struct {
	target *url.URL
}

func (s *zhipuMCPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	req.URL.Scheme = s.target.Scheme
	req.URL.Host = s.target.Host
	return http.DefaultClient.Do(req)
}

func (s *zhipuMCPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

type zhipuMCPAccountRepoStub struct {
	service.AccountRepository

	accountsByID map[int64]*service.Account
	schedulable  []service.Account
}

func (s *zhipuMCPAccountRepoStub) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if acc, ok := s.accountsByID[id]; ok {
		return acc, nil
	}
	return nil, errors.New("account not found")
}

func (s *zhipuMCPAccountRepoStub) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, _ string) ([]service.Account, error) {
	return s.schedulable, nil
}

type zhipuMCPGroupRepoStub struct {
	service.GroupRepository

	group *service.Group
}

func (s *zhipuMCPGroupRepoStub) GetByIDLite(_ context.Context, groupID int64) (*service.Group, error) {
	if s.group != nil && s.group.ID == groupID {
		return s.group, nil
	}
	return nil, errors.New("group not found")
}

func (s *zhipuMCPGroupRepoStub) GetByID(ctx context.Context, groupID int64) (*service.Group, error) {
	return s.GetByIDLite(ctx, groupID)
}

// zhipuMCPSessionStoreFake service.ZhipuMCPSessionStore 的内存实现。
// TTL 语义在 service 层测试里用 miniredis 覆盖，这里只验证绑定关系。
type zhipuMCPSessionStoreFake struct {
	mu       sync.Mutex
	bindings map[string]int64
}

func (s *zhipuMCPSessionStoreFake) SetZhipuMCPSession(_ context.Context, sessionID string, accountID int64, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bindings == nil {
		s.bindings = make(map[string]int64)
	}
	s.bindings[sessionID] = accountID
	return nil
}

func (s *zhipuMCPSessionStoreFake) GetZhipuMCPSession(_ context.Context, sessionID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountID, ok := s.bindings[sessionID]
	if !ok {
		return 0, service.ErrZhipuMCPSessionNotFound
	}
	return accountID, nil
}

func (s *zhipuMCPSessionStoreFake) DeleteZhipuMCPSession(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bindings, sessionID)
	return nil
}

func (s *zhipuMCPSessionStoreFake) get(sessionID string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountID, ok := s.bindings[sessionID]
	return accountID, ok
}

// zhipuMCPBillingCacheStub service.BillingCache 最小桩：只实现余额读取，
// 供 CheckBillingEligibility 的余额模式分支使用；其余方法未预期被调用。
type zhipuMCPBillingCacheStub struct {
	service.BillingCache

	balance float64
}

func (s *zhipuMCPBillingCacheStub) GetUserBalance(_ context.Context, _ int64) (float64, error) {
	return s.balance, nil
}

// zhipuMCPUsageLogRepoStub 捕获 RecordUsage 落库的用量行。
type zhipuMCPUsageLogRepoStub struct {
	service.UsageLogRepository

	calls   int
	lastLog *service.UsageLog
}

func (s *zhipuMCPUsageLogRepoStub) CreateBestEffort(_ context.Context, log *service.UsageLog) error {
	s.calls++
	s.lastLog = log
	return nil
}

func (s *zhipuMCPUsageLogRepoStub) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	return true, nil
}

// zhipuMCPUserRepoStub 捕获余额扣减。
type zhipuMCPUserRepoStub struct {
	service.UserRepository

	deductCalls int
	lastAmount  float64
}

func (s *zhipuMCPUserRepoStub) DeductBalance(_ context.Context, _ int64, amount float64) error {
	s.deductCalls++
	s.lastAmount = amount
	return nil
}

// zhipuMCPBillingEnv 计费链路测试桩集合：余额资格 + usage_log 落库 + 余额扣减。
type zhipuMCPBillingEnv struct {
	cache   *zhipuMCPBillingCacheStub
	logs    *zhipuMCPUsageLogRepoStub
	users   *zhipuMCPUserRepoStub
	billing *service.BillingCacheService
}

// newZhipuMCPTestEnv 组装 handler + fake 上游 + gin 路由。
// accounts 同时进入 GetByID 与可调度列表；injectKey=false 用于未认证用例。
// 计费链路默认可用（余额充足），计费用例可通过 billing 调整桩状态。
func newZhipuMCPTestEnv(
	t *testing.T,
	accounts []service.Account,
	group *service.Group,
	store service.ZhipuMCPSessionStore,
	injectKey bool,
	respond func(attempt int, req *zhipuMCPCapturedRequest, w http.ResponseWriter),
) (*gin.Engine, *zhipuMCPFakeUpstream, *zhipuMCPBillingEnv) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upstream := &zhipuMCPFakeUpstream{t: t, respond: respond}
	upstreamServer := httptest.NewServer(upstream)
	t.Cleanup(upstreamServer.Close)
	target, err := url.Parse(upstreamServer.URL)
	require.NoError(t, err)

	accountsByID := make(map[int64]*service.Account, len(accounts))
	for i := range accounts {
		accountsByID[accounts[i].ID] = &accounts[i]
	}

	groupID := group.ID
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false

	cacheStub := &zhipuMCPBillingCacheStub{balance: 10}
	billingCacheSvc := service.NewBillingCacheService(cacheStub, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)

	logRepo := &zhipuMCPUsageLogRepoStub{}
	userRepo := &zhipuMCPUserRepoStub{}

	gwSvc := service.NewGatewayService(
		&zhipuMCPAccountRepoStub{accountsByID: accountsByID, schedulable: accounts},
		&zhipuMCPGroupRepoStub{group: group},
		logRepo, nil, userRepo, nil, nil, nil, // usageLog/usageBilling/user/userSub/userGroupRate/cache
		cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, nil, nil, // schedulerSnapshot/concurrency/billing/rateLimit/billingCache/identity
		&zhipuMCPUpstreamStub{target: target},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // deferred..userPlatformQuotaRepo
	).WithZhipuMCPSessionStore(store)

	h := &GatewayHandler{gatewayService: gwSvc, cfg: cfg, billingCacheService: billingCacheSvc}

	r := gin.New()
	if injectKey {
		r.Use(func(c *gin.Context) {
			c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
				ID:      1,
				User:    &service.User{ID: 1},
				GroupID: &groupID,
				Group:   group,
			})
			c.Next()
		})
	}
	r.POST("/api/zhipu/api/mcp/:slug/mcp", h.ZhipuMCPPassthrough)
	r.DELETE("/api/zhipu/api/mcp/:slug/mcp", h.ZhipuMCPPassthrough)
	r.GET("/api/zhipu/api/mcp/:slug/mcp", h.ZhipuMCPPassthrough)
	return r, upstream, &zhipuMCPBillingEnv{cache: cacheStub, logs: logRepo, users: userRepo, billing: billingCacheSvc}
}

func zhipuMCPPostRequest(target string, body string, sessionID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	return req
}

// requireNoUserKeyLeak 断言上游收到的任何请求头都不含用户 sub2api key。
func requireNoUserKeyLeak(t *testing.T, req *zhipuMCPCapturedRequest) {
	t.Helper()
	for name, values := range req.Header {
		for _, value := range values {
			require.NotContains(t, value, zhipuMCPTestUserKey, "header %s 泄漏用户 key", name)
		}
	}
}

func TestZhipuMCPPassthrough_Unauthorized(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, false, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{}`, ""))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, upstream.captured())
}

func TestZhipuMCPPassthrough_UnknownSlug(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})

	w := httptest.NewRecorder()
	// vision 是 Local MCP（无远程端点可透传），不在白名单内。
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/vision/mcp", `{}`, ""))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Empty(t, upstream.captured())
}

func TestZhipuMCPPassthrough_NonZhipuGroup(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformOpenAI},
		&zhipuMCPSessionStoreFake{}, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{}`, ""))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "MCP passthrough is only supported for zhipu groups")
	require.Empty(t, upstream.captured())
}

func TestZhipuMCPPassthrough_AuthHeaderRewriteAndBodyPassthrough(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, req *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))
			require.Equal(t, "application/json, text/event-stream", req.Header.Get("Accept"))
		})

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", reqBody, ""))

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	caps := upstream.captured()
	require.Len(t, caps, 1)
	// 认证头按选中账号重写，且用户 sub2api key 不出现在任何请求头。
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
	requireNoUserKeyLeak(t, &caps[0])
	// 请求体原样到达、路径镜像智谱端点形态。
	require.Equal(t, reqBody, string(caps[0].Body))
	require.Equal(t, "/api/mcp/web_search_prime/mcp", caps[0].Path)
}

func TestZhipuMCPPassthrough_SessionAffinity(t *testing.T) {
	store := &zhipuMCPSessionStoreFake{}
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true),
			zhipuMCPTestAccount(12, zhipuMCPTestZhipuKeyB, 2, true),
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		store, true,
		func(_ int, req *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			if req.Header.Get("Mcp-Session-Id") == "" {
				// initialize：上游下发 Mcp-Session-Id。
				w.Header().Set("Mcp-Session-Id", "sess-affinity-1")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"zhipu"}}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{}}`))
		})

	// 第一次请求（initialize）调度到优先级最高的账号 A。
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, zhipuMCPPostRequest("/api/zhipu/api/mcp/zread/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, ""))
	require.Equal(t, http.StatusOK, w1.Code)
	require.Equal(t, "sess-affinity-1", w1.Header().Get("Mcp-Session-Id"))

	// 绑定已写入粘表。
	bound, ok := store.get("sess-affinity-1")
	require.True(t, ok)
	require.Equal(t, int64(11), bound)

	// 第二次请求带 Mcp-Session-Id，应粘到同一账号。
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, zhipuMCPPostRequest("/api/zhipu/api/mcp/zread/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call"}`, "sess-affinity-1"))
	require.Equal(t, http.StatusOK, w2.Code)

	caps := upstream.captured()
	require.Len(t, caps, 2)
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[1].Header.Get("Authorization"), "session 亲和应落在同一账号")
	require.Equal(t, "sess-affinity-1", caps[1].Header.Get("Mcp-Session-Id"))
}

func TestZhipuMCPPassthrough_SSEStreamPassthrough(t *testing.T) {
	chunk1 := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\"}\n\n"
	chunk2 := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{}}\n\n"
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(chunk1))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			_, _ = w.Write([]byte(chunk2))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{"jsonrpc":"2.0","id":3,"method":"tools/call"}`, ""))

	require.Equal(t, http.StatusOK, w.Code)
	// 客户端收到等价字节流，Content-Type 透传。
	require.Equal(t, chunk1+chunk2, w.Body.String())
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Len(t, upstream.captured(), 1)
}

func TestZhipuMCPPassthrough_Upstream429FailsOverToNextAccount(t *testing.T) {
	store := &zhipuMCPSessionStoreFake{}
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true),
			zhipuMCPTestAccount(12, zhipuMCPTestZhipuKeyB, 2, true),
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		store, true,
		func(attempt int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			if attempt == 0 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limited"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "sess-after-failover")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, ""))

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{}}`, w.Body.String())

	// 第一个账号 429 后换第二个账号成功；上游签发的 session 绑定到成功账号。
	caps := upstream.captured()
	require.Len(t, caps, 2)
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyB, caps[1].Header.Get("Authorization"))
	bound, ok := store.get("sess-after-failover")
	require.True(t, ok)
	require.Equal(t, int64(12), bound)
}

func TestZhipuMCPPassthrough_Upstream4xxPassthroughWithoutFailover(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true),
			zhipuMCPTestAccount(12, zhipuMCPTestZhipuKeyB, 2, true),
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			// 客户端协议错误（如 session not found）：原样透传，不换号。
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"session not found"}`))
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{}`, "sess-stale"))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "session not found")
	require.Len(t, upstream.captured(), 1)
}

func TestZhipuMCPPassthrough_DeleteTombstonesSessionAndRejectsReuse(t *testing.T) {
	store := &zhipuMCPSessionStoreFake{}
	require.NoError(t, store.SetZhipuMCPSession(context.Background(), "sess-del-1", 11, time.Minute))

	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		store, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			// 实测形态：上游 DELETE 处理内部出错但 HTTP 仍 200（error body 载体）。
			w.WriteHeader(http.StatusOK)
		})

	req := httptest.NewRequest(http.MethodDelete, "/api/zhipu/api/mcp/web_search_prime/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	req.Header.Set("Mcp-Session-Id", "sess-del-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	caps := upstream.captured()
	require.Len(t, caps, 1)
	require.Equal(t, http.MethodDelete, caps[0].Method)
	require.Equal(t, "sess-del-1", caps[0].Header.Get("Mcp-Session-Id"))
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
	// DELETE 透传不带 body 与 Content-Type（避免上游 transport 解析 body 出错）。
	require.Empty(t, caps[0].Body)
	require.Empty(t, caps[0].Header.Get("Content-Type"))

	// fail-closed：即使上游返回 200，粘表也写入 tombstone（值 0），而非删除 key。
	got, ok := store.get("sess-del-1")
	require.True(t, ok)
	require.Equal(t, service.ZhipuMCPSessionDeletedAccountID, got)

	// 同 session id 的后续 POST（任意 MCP 方法）必须 404，不透传上游。
	post := zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":9,"method":"tools/list","params":{}}`, "sess-del-1")
	wp := httptest.NewRecorder()
	r.ServeHTTP(wp, post)
	require.Equal(t, http.StatusNotFound, wp.Code)
	require.Len(t, upstream.captured(), 1, "tombstoned session 不应再透传上游")

	// 幂等：对已终止 session 的再次 DELETE 也 404。
	del2 := httptest.NewRequest(http.MethodDelete, "/api/zhipu/api/mcp/web_search_prime/mcp", nil)
	del2.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	del2.Header.Set("Mcp-Session-Id", "sess-del-1")
	wd := httptest.NewRecorder()
	r.ServeHTTP(wd, del2)
	require.Equal(t, http.StatusNotFound, wd.Code)
	require.Len(t, upstream.captured(), 1)
}

func TestZhipuMCPPassthrough_ReinitializeAfterDeleteSucceeds(t *testing.T) {
	store := &zhipuMCPSessionStoreFake{}
	require.NoError(t, store.SetZhipuMCPSession(context.Background(), "sess-old", 11, time.Minute))

	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		store, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Mcp-Session-Id", "sess-new")
			w.WriteHeader(http.StatusOK)
		})

	// 终止旧 session。
	del := httptest.NewRequest(http.MethodDelete, "/api/zhipu/api/mcp/web_search_prime/mcp", nil)
	del.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	del.Header.Set("Mcp-Session-Id", "sess-old")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, del)
	require.Equal(t, http.StatusOK, w.Code)

	// 重新 initialize：新 session（不带旧 SID 或带新 SID）不受 tombstone 影响。
	init := zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "")
	wi := httptest.NewRecorder()
	r.ServeHTTP(wi, init)
	require.Equal(t, http.StatusOK, wi.Code)

	// 新 SID 正常工作（粘表被新绑定覆盖）。
	list := zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, "sess-new")
	wl := httptest.NewRecorder()
	r.ServeHTTP(wl, list)
	require.Equal(t, http.StatusOK, wl.Code)
	require.Len(t, upstream.captured(), 3)

	got, ok := store.get("sess-new")
	require.True(t, ok)
	require.Equal(t, int64(11), got)
}

func TestZhipuMCPPassthrough_GetMethodNotAllowed(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})

	req := httptest.NewRequest(http.MethodGet, "/api/zhipu/api/mcp/web_search_prime/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	require.Equal(t, "POST, DELETE", w.Header().Get("Allow"))
	require.Empty(t, upstream.captured())
}

func TestZhipuMCPPassthrough_StickyInvalidFallsBackToScheduling(t *testing.T) {
	store := &zhipuMCPSessionStoreFake{}
	// 粘表指向账号 13（已被关闭 MCP 能力开关），应清粘表并回退正常调度到账号 11。
	require.NoError(t, store.SetZhipuMCPSession(context.Background(), "sess-gone", 13, time.Minute))

	invalid := zhipuMCPTestAccount(13, "zp-key-account-c", 0, false)
	invalid.Extra = map[string]any{service.ZhipuMCPCapabilityExtraKey: false}

	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true),
			invalid,
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		store, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "sess-gone"))

	require.Equal(t, http.StatusOK, w.Code)

	caps := upstream.captured()
	require.Len(t, caps, 1)
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
	_, ok := store.get("sess-gone")
	require.False(t, ok, "失效粘表应被清理")
}

func TestZhipuMCPPassthrough_NonCapableAccountSkippedInScheduling(t *testing.T) {
	// 可调度列表里混入未开 MCP 能力的账号（与模型转发共用账号池的正常场景）：
	// handler 的能力兜底应把它跳过并换号。
	// 非 MCP 账号优先级更高（1 < 2），验证调度先选中它后被 handler 兜底换号。
	notCapable := zhipuMCPTestAccount(13, "zp-key-account-c", 1, false)
	notCapable.Extra = nil

	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			notCapable,
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 2, true),
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, ""))

	require.Equal(t, http.StatusOK, w.Code)

	// 非 MCP 账号在发上游请求前就被兜底跳过，只有具备能力的账号真正触达上游。
	caps := upstream.captured()
	require.Len(t, caps, 1)
	require.Equal(t, "Bearer "+zhipuMCPTestZhipuKeyA, caps[0].Header.Get("Authorization"))
}

func TestZhipuMCPPassthrough_RetriesExhaustedReturns502(t *testing.T) {
	r, upstream, _ := newZhipuMCPTestEnv(t,
		[]service.Account{
			zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true),
			zhipuMCPTestAccount(12, zhipuMCPTestZhipuKeyB, 2, true),
		},
		&service.Group{ID: 7, Platform: service.PlatformZhipu},
		&zhipuMCPSessionStoreFake{}, true, func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusTooManyRequests)
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", `{}`, ""))

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "zhipu_mcp_error")
	require.Len(t, upstream.captured(), 2)
}

// zhipuMCPBillingTestGroup 带显式 MCP 单价的 zhipu 分组：
// search_price_per_1k 在 zhipu 组的语义即 "MCP tools/call 每千次价格"。
func zhipuMCPBillingTestGroup() *service.Group {
	price := 2.0
	return &service.Group{
		ID:               7,
		Platform:         service.PlatformZhipu,
		RateMultiplier:   1.1,
		SearchPricePer1k: &price,
	}
}

func TestZhipuMCPPassthrough_ToolCallRecordsUsageOnce(t *testing.T) {
	r, _, billing := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		zhipuMCPBillingTestGroup(),
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{}}}`, ""))

	require.Equal(t, http.StatusOK, w.Code)

	// 单次 tools/call → 恰好一条 usage 行（handler 测试环境无 worker 池，
	// submitMandatoryUsageRecordTask 同步兜底执行，返回时已落账）。
	require.Equal(t, 1, billing.logs.calls)
	log := billing.logs.lastLog
	require.NotNil(t, log)
	// 伪模型名 + RequestID 幂等键前缀。
	require.Equal(t, "zhipu-mcp-web_search_prime", log.Model)
	require.Equal(t, "zhipu-mcp-web_search_prime", log.RequestedModel)
	require.True(t, strings.HasPrefix(log.RequestID, "zhipu_mcp:"), "RequestID 应为 zhipu_mcp: 前缀，实际 %q", log.RequestID)
	require.NotEqual(t, "zhipu_mcp:", log.RequestID, "RequestID 必须带唯一后缀（计费幂等键）")
	// 端点与归属。
	require.NotNil(t, log.InboundEndpoint)
	require.Equal(t, "/api/zhipu/api/mcp/web_search_prime/mcp", *log.InboundEndpoint)
	require.NotNil(t, log.UpstreamEndpoint)
	require.Equal(t, "zhipu-mcp:web_search_prime", *log.UpstreamEndpoint)
	require.NotNil(t, log.GroupID)
	require.Equal(t, int64(7), *log.GroupID)
	require.Equal(t, int64(1), log.UserID)
	require.Equal(t, int64(1), log.APIKeyID)
	require.Equal(t, int64(11), log.AccountID)
	require.False(t, log.Stream)
	require.Nil(t, log.SubscriptionID)
	// 成本走 SearchCount × search_price_per_1k 通道：1 次 × 2.0/1k = 0.002，倍率 1.1。
	require.InDelta(t, 0.002, log.TotalCost, 1e-12)
	require.InDelta(t, 0.0022, log.ActualCost, 1e-12)
	// SearchCount 是叠加到 token 成本上的 surcharge（token 查无价时结构体
	// BillingMode 为空，resolveBillingMode 回落 "token"）——既有通道行为，锁定不改语义。
	require.NotNil(t, log.BillingMode)
	require.Equal(t, "token", *log.BillingMode)
	// 余额模式按 ActualCost 扣减用户余额。
	require.Equal(t, 1, billing.users.deductCalls)
	require.InDelta(t, 0.0022, billing.users.lastAmount, 1e-12)
}

func TestZhipuMCPPassthrough_BatchToolCallBillsPerCall(t *testing.T) {
	r, upstream, billing := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		zhipuMCPBillingTestGroup(),
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"jsonrpc":"2.0","id":1,"result":{}},{"jsonrpc":"2.0","id":2,"result":{}}]`))
		})

	body := `[
		{"jsonrpc":"2.0","id":1,"method":"tools/call"},
		{"jsonrpc":"2.0","id":2,"method":"tools/call"},
		{"jsonrpc":"2.0","id":3,"method":"initialize"},
		{"jsonrpc":"2.0","method":"notifications/message"}
	]`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp", body, ""))

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, upstream.captured(), 1)
	// batch 只落一条 usage 行，SearchCount = 2 → 2 × 2.0/1k = 0.004（倍率 1.1 → 0.0044）。
	require.Equal(t, 1, billing.logs.calls)
	require.InDelta(t, 0.004, billing.logs.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.0044, billing.logs.lastLog.ActualCost, 1e-12)
}

func TestZhipuMCPPassthrough_InitializeAndDeleteDoNotRecordUsage(t *testing.T) {
	r, upstream, billing := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		zhipuMCPBillingTestGroup(),
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

	// initialize：管理性方法免费。
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`, ""))
	require.Equal(t, http.StatusOK, w1.Code)

	// DELETE：session 终止免费。
	req := httptest.NewRequest(http.MethodDelete, "/api/zhipu/api/mcp/web_search_prime/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+zhipuMCPTestUserKey)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)

	require.Len(t, upstream.captured(), 2)
	require.Equal(t, 0, billing.logs.calls, "initialize/DELETE 不产生 usage 记录")
	require.Equal(t, 0, billing.users.deductCalls)
}

func TestZhipuMCPPassthrough_BillingIneligibleRejectsBeforeScheduling(t *testing.T) {
	r, upstream, billing := newZhipuMCPTestEnv(t,
		[]service.Account{zhipuMCPTestAccount(11, zhipuMCPTestZhipuKeyA, 1, true)},
		zhipuMCPBillingTestGroup(),
		&zhipuMCPSessionStoreFake{}, true,
		func(_ int, _ *zhipuMCPCapturedRequest, w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})

	billing.cache.balance = 0

	w := httptest.NewRecorder()
	r.ServeHTTP(w, zhipuMCPPostRequest("/api/zhipu/api/mcp/web_search_prime/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, ""))

	// 余额不足在调度前被拒（billingErrorDetails 默认映射 403），不上游、不落账。
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "billing_error")
	require.Empty(t, upstream.captured())
	require.Equal(t, 0, billing.logs.calls)
	require.Equal(t, 0, billing.users.deductCalls)
}
