//go:build unit

package routes

// RED 回归测试（feature/composite-cross-platform-scheduling）。
//
// 已知故障：composite 分组中同一公开模型被多个不同平台的 group-bound 账号
// 同时声明（各有账号级 model_mapping），且未配置显式 composite route 时，
// CompositeRouteResolver.Resolve 命中 ownership.Ambiguous 后直接返回
// Matched=false（不再回落 DetectModelPlatform）。中间件因此不写入
// ResolvedTargetPlatform，routes 层的 isOpenAIResponsesCompatibleGatewayPlatform
// 判定失败，把 POST /v1/chat/completions 误分发给通用 Anthropic 网关
// handler（h.Gateway.ChatCompletions）。该 handler 走 CC→Responses→Anthropic
// 桥接，出站请求被拼成 {base_url}/v1/messages?beta=true——当账号 base_url
// 以 /v1 结尾时即为 /v1/v1/messages。
//
// 期望行为（待实现修绿）：该场景应统一进入 OpenAI 兼容网关 handler，由其
// 现有 selector 在跨平台候选池中选号；无论选中哪个账号，出站都必须是
// OpenAI Chat Completions 转发（{base_url}/chat/completions，base 以 /v1
// 结尾时不得重复拼版本段），且 body.model 为选中账号 model_mapping 映射后
// 的上游模型。
//
// 链路真实性：本测试经 RegisterGatewayRoutes 挂载真实中间件链
// （bodyLimit→apiKeyAuth→groupModelAllowlist→compositeTarget→…→handler 分发），
// composite 中间件持有与 GatewayService 真实 resolveCompositeModelOwnership
// 绑定的 CompositeRouteResolver；两条 handler 分支都是真实 handler
// （GatewayHandler + OpenAIGatewayHandler），账号与分组通过
// RegisterGatewayRoutes 生产同款注入方式放置。出站经 HTTPUpstream 桩捕获，
// 不发起任何真实网络请求。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
	compositeDispatchGroupID    = int64(4101)
	compositeDispatchDeepseekID = int64(4111)
	compositeDispatchOpenCodeID = int64(4112)

	compositeDispatchDeepseekBase = "https://deepseek-composite-fake.test/v1"
	compositeDispatchOpenCodeBase = "https://opencode-composite-fake.test/v1"

	// compositeDispatchPublicModel 是两个平台账号同时声明（各自 model_mapping
	// 的 key）的公开模型名，命中内置 detector 的 deepseek- 前缀，因此歧义
	// 只能由账号归属判断暴露。
	compositeDispatchPublicModel = "deepseek-flash"
)

// compositeDispatchUpstream 捕获两条网关服务（通用 + OpenAI 兼容）的全部出站。
type compositeDispatchUpstream struct {
	service.HTTPUpstream

	mu       sync.Mutex
	captured []compositeDispatchCapturedRequest
}

type compositeDispatchCapturedRequest struct {
	accountID int64
	url       string
	path      string
	host      string
	model     string
}

func (u *compositeDispatchUpstream) record(req *http.Request, accountID int64) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	captured := compositeDispatchCapturedRequest{
		accountID: accountID,
		url:       req.URL.String(),
		path:      req.URL.Path,
		host:      req.URL.Hostname(),
		model:     gjson.GetBytes(body, "model").String(),
	}
	u.mu.Lock()
	u.captured = append(u.captured, captured)
	u.mu.Unlock()
}

func (u *compositeDispatchUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.record(req, accountID)
	return u.respond(req), nil
}

func (u *compositeDispatchUpstream) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.record(req, accountID)
	return u.respond(req), nil
}

// respond 按出站 path 返回最小可解析响应：CC 路径回最小 CC JSON；其他路径
// （即错误分发的 Anthropic 桥）回最小 Anthropic SSE，让两条链路都能完整走完，
// 避免把 RED 失败变成 failover 风暴或解析 panic。
func (u *compositeDispatchUpstream) respond(req *http.Request) *http.Response {
	if strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return compositeDispatchResponse("application/json", `{"id":"chatcmpl-red","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}
	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg-red","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	return compositeDispatchResponse("text/event-stream", sse)
}

func compositeDispatchResponse(contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func (u *compositeDispatchUpstream) snapshot() []compositeDispatchCapturedRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]compositeDispatchCapturedRequest(nil), u.captured...)
}

// compositeDispatchAccountRepo 提供 deepseek + opencode_go 两个可调度账号。
type compositeDispatchAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func cloneCompositeDispatchAccounts(accounts []service.Account) []service.Account {
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		copy := account
		credentials := make(map[string]any, len(account.Credentials))
		for key, value := range account.Credentials {
			credentials[key] = value
		}
		copy.Credentials = credentials
		out = append(out, copy)
	}
	return out
}

func (r *compositeDispatchAccountRepo) schedulable() []service.Account {
	out := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.IsSchedulable() {
			out = append(out, account)
		}
	}
	return out
}

func (r *compositeDispatchAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			copy := account
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *compositeDispatchAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	out := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		if account, err := r.GetByID(context.Background(), id); err == nil && account != nil {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *compositeDispatchAccountRepo) ListSchedulable(context.Context) ([]service.Account, error) {
	return cloneCompositeDispatchAccounts(r.schedulable()), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		for _, id := range account.GroupIDs {
			if id == groupID {
				out = append(out, account)
				break
			}
		}
	}
	return cloneCompositeDispatchAccounts(out), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return cloneCompositeDispatchAccounts(out), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		if account.Platform != platform {
			continue
		}
		for _, id := range account.GroupIDs {
			if id == groupID {
				out = append(out, account)
				break
			}
		}
	}
	return cloneCompositeDispatchAccounts(out), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(context.Background(), platform)
}

func (r *compositeDispatchAccountRepo) ListSchedulableByPlatforms(_ context.Context, platforms []string) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		for _, platform := range platforms {
			if account.Platform == platform {
				out = append(out, account)
				break
			}
		}
	}
	return cloneCompositeDispatchAccounts(out), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		matchedPlatform := false
		for _, platform := range platforms {
			if account.Platform == platform {
				matchedPlatform = true
				break
			}
		}
		if !matchedPlatform {
			continue
		}
		for _, id := range account.GroupIDs {
			if id == groupID {
				out = append(out, account)
				break
			}
		}
	}
	return cloneCompositeDispatchAccounts(out), nil
}

func (r *compositeDispatchAccountRepo) ListSchedulableUngroupedByPlatforms(_ context.Context, platforms []string) ([]service.Account, error) {
	return r.ListSchedulableByPlatforms(context.Background(), platforms)
}

// ListModelAvailabilityCandidates 对齐接口语义：仅按持久化配置（active +
// schedulable）返回候选，不做瞬态运行时过滤；groupID 为 nil 时 includeGrouped
// 决定扫全量还是仅未分组账号。WIP 的 resolveCompositeModelOwnership 经此查询
// composite 账号归属。
func (r *compositeDispatchAccountRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, account := range r.schedulable() {
		if len(platforms) > 0 {
			matchedPlatform := false
			for _, platform := range platforms {
				if account.Platform == platform {
					matchedPlatform = true
					break
				}
			}
			if !matchedPlatform {
				continue
			}
		}
		if groupID == nil && !includeGrouped && len(account.GroupIDs) > 0 {
			continue
		}
		if groupID != nil {
			inGroup := false
			for _, id := range account.GroupIDs {
				if id == *groupID {
					inGroup = true
					break
				}
			}
			if !inGroup {
				continue
			}
		}
		out = append(out, account)
	}
	return cloneCompositeDispatchAccounts(out), nil
}

// compositeDispatchGroupRepo 满足通用调度器 resolveGatewayGroup 的分组读取。
type compositeDispatchGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *compositeDispatchGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

func (r *compositeDispatchGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

// compositeDispatchConcurrencyCache 放行用户/账号槽位并安全释放。
type compositeDispatchConcurrencyCache struct {
	service.ConcurrencyCache
}

func (m *compositeDispatchConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (m *compositeDispatchConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error {
	return nil
}

func (m *compositeDispatchConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (m *compositeDispatchConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func (m *compositeDispatchConcurrencyCache) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}

func (m *compositeDispatchConcurrencyCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = 0
	}
	return result, nil
}

func (m *compositeDispatchConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (m *compositeDispatchConcurrencyCache) DecrementAccountWaitCount(context.Context, int64) error {
	return nil
}

func (m *compositeDispatchConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}

func (m *compositeDispatchConcurrencyCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (m *compositeDispatchConcurrencyCache) DecrementWaitCount(context.Context, int64) error {
	return nil
}

func (m *compositeDispatchConcurrencyCache) GetUserConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}

func TestCompositeAmbiguousModelChatCompletionsForwardedViaOpenAIGatewayUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := compositeDispatchGroupID
	group := &service.Group{
		ID:       groupID,
		Platform: service.PlatformComposite,
		Status:   service.StatusActive,
	}
	user := &service.User{ID: 4103, Status: service.StatusActive}
	apiKey := &service.APIKey{
		ID:      4102,
		GroupID: &groupID,
		User:    user,
		Group:   group,
	}

	repo := &compositeDispatchAccountRepo{
		accounts: []service.Account{
			{
				ID: compositeDispatchDeepseekID, Name: "cn-deepseek",
				Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
				Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: 1,
				Credentials: map[string]any{
					"api_key":       "test-deepseek-key",
					"base_url":      compositeDispatchDeepseekBase,
					"model_mapping": map[string]any{compositeDispatchPublicModel: "deepseek-chat"},
				},
				GroupIDs:      []int64{compositeDispatchGroupID},
				AccountGroups: []service.AccountGroup{{AccountID: compositeDispatchDeepseekID, GroupID: compositeDispatchGroupID, Priority: 1}},
			},
			{
				ID: compositeDispatchOpenCodeID, Name: "opencode-go",
				Platform: service.PlatformOpenCodeGo, Type: service.AccountTypeAPIKey,
				Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: 2,
				Credentials: map[string]any{
					"api_key":       "test-opencode-key",
					"base_url":      compositeDispatchOpenCodeBase,
					"model_mapping": map[string]any{compositeDispatchPublicModel: "deepseek-v4"},
				},
				GroupIDs:      []int64{compositeDispatchGroupID},
				AccountGroups: []service.AccountGroup{{AccountID: compositeDispatchOpenCodeID, GroupID: compositeDispatchGroupID, Priority: 1}},
			},
		},
	}
	groupRepo := &compositeDispatchGroupRepo{group: group}
	upstream := &compositeDispatchUpstream{}

	// 无显式 route：空 repo 的 resolver，经 NewGatewayService 绑定真实的
	// resolveCompositeModelOwnership（账号归属判定），与生产装配同构。
	compositeResolver := service.NewCompositeRouteResolver(nil)

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxBodySize = 1024 * 1024
	cfg.Gateway.TextMaxBodySize = 1024 * 1024
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	billingService := service.NewBillingService(cfg, nil)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	concurrencyService := service.NewConcurrencyService(&compositeDispatchConcurrencyCache{})
	deferred := &service.DeferredService{}

	genericGateway := service.NewGatewayService(
		repo, groupRepo, nil, nil, nil, nil, nil, nil, cfg, nil,
		concurrencyService, billingService, nil, billingCache, nil,
		upstream, deferred, nil, nil, nil, nil, nil, nil, nil, nil,
		compositeResolver, nil, nil,
	)
	openAIGateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		billingService, nil, billingCache, upstream, deferred,
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
			c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: user.ID, Concurrency: 1})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		compositeResolver,
		cfg,
	)

	requestBody := `{"model":"` + compositeDispatchPublicModel + `","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	captured := upstream.snapshot()
	require.NotEmpty(t, captured,
		"composite CC request must be forwarded to an upstream, got status=%d body=%s", w.Code, w.Body.String())

	expectedModelByHost := map[string]string{
		"deepseek-composite-fake.test": "deepseek-chat",
		"opencode-composite-fake.test": "deepseek-v4",
	}
	for _, request := range captured {
		require.NotContainsf(t, request.path, "/messages",
			"composite chat completions must never be dispatched through the Anthropic messages bridge (account=%d url=%s model=%s)",
			request.accountID, request.url, request.model)
		// 精确相等而非后缀匹配：两个 fake base 均以 /v1 结尾，后缀匹配放得过
		// /v1/v1/chat/completions 这类重复版本段拼接。
		require.Equalf(t, "/v1/chat/completions", request.path,
			"outbound must hit the OpenAI-compatible chat completions endpoint exactly once-versioned (account=%d url=%s)", request.accountID, request.url)
		expectedModel, knownHost := expectedModelByHost[request.host]
		require.Truef(t, knownHost, "outbound must only hit the fake composite upstreams (url=%s)", request.url)
		require.Equalf(t, expectedModel, request.model,
			"outbound body.model must be the selected account's model_mapping target (account=%d url=%s)", request.accountID, request.url)
	}
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
