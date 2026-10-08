package handler

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
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// WS 入口只在握手时选号。下面这组端到端用例走真实的 ResponsesWebSocket：首轮正常
// 完成后改掉账号状态（管理员停调度、按模型限流），第二轮必须在写上游之前以
// TryAgainLater 关闭客户端连接并解除粘性绑定，让客户端重连后重新选号。

type wsTurnEligibilityAccountRepo struct {
	service.AccountRepository
	mu      sync.Mutex
	account service.Account
}

func (r *wsTurnEligibilityAccountRepo) current() service.Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.account
}

// update 只允许整体替换字段（含 map），不就地改 map，避免与读侧拿到的浅拷贝竞争。
func (r *wsTurnEligibilityAccountRepo) update(fn func(account *service.Account)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.account)
}

func (r *wsTurnEligibilityAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	account := r.current()
	if account.Platform != platform {
		return nil, nil
	}
	return []service.Account{account}, nil
}

func (r *wsTurnEligibilityAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *wsTurnEligibilityAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	account := r.current()
	if account.ID != id {
		return nil, nil
	}
	return &account, nil
}

// wsTurnEligibilityStickyCache 记录粘性绑定的写入与删除，其余行为委托给真实的
// Redis 网关缓存（miniredis）。
type wsTurnEligibilityStickyCache struct {
	service.GatewayCache
	mu      sync.Mutex
	bound   map[string]int64
	deleted map[string]int
}

func (c *wsTurnEligibilityStickyCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	c.mu.Lock()
	c.bound[sessionHash] = accountID
	c.mu.Unlock()
	return c.GatewayCache.SetSessionAccountID(ctx, groupID, sessionHash, accountID, ttl)
}

func (c *wsTurnEligibilityStickyCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	c.mu.Lock()
	c.deleted[sessionHash]++
	c.mu.Unlock()
	return c.GatewayCache.DeleteSessionAccountID(ctx, groupID, sessionHash)
}

// snapshot 返回曾绑定到 accountID 的会话哈希，以及这些绑定被删除的次数。
func (c *wsTurnEligibilityStickyCache) snapshot(accountID int64) (hashes []string, deletedCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash, bound := range c.bound {
		if bound == accountID {
			hashes = append(hashes, hash)
			deletedCount += c.deleted[hash]
		}
	}
	return hashes, deletedCount
}

// wsTurnEligibilityUpstream 同时充当 WS 上游（passthrough / ctx_pool）和 HTTP 上游
// （http_bridge），按到达顺序记录每一轮真正发往上游的请求体。
type wsTurnEligibilityUpstream struct {
	mu            sync.Mutex
	requests      [][]byte
	transports    []string
	afterFirstReq func()
}

func (u *wsTurnEligibilityUpstream) record(transport string, payload []byte) int {
	u.mu.Lock()
	u.requests = append(u.requests, append([]byte(nil), payload...))
	u.transports = append(u.transports, transport)
	n := len(u.requests)
	u.mu.Unlock()
	if n == 1 && u.afterFirstReq != nil {
		u.afterFirstReq()
	}
	return n
}

func (u *wsTurnEligibilityUpstream) snapshot() ([][]byte, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([][]byte(nil), u.requests...), append([]string(nil), u.transports...)
}

func (u *wsTurnEligibilityUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	n := u.record("http", body)
	model := gjson.GetBytes(body, "model").String()
	responseID := fmt.Sprintf("resp_ws_turn_eligibility_%d", n)
	sse := strings.Join([]string{
		fmt.Sprintf(`data: {"type":"response.created","response":{"id":%q,"model":%q}}`, responseID, model),
		"",
		fmt.Sprintf(`data: {"type":"response.completed","response":{"id":%q,"model":%q,"usage":{"input_tokens":2,"output_tokens":1}}}`, responseID, model),
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}, nil
}

func (u *wsTurnEligibilityUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *wsTurnEligibilityUpstream) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	for {
		msgType, payload, readErr := conn.Read(r.Context())
		if readErr != nil {
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			return
		}
		n := u.record("ws", payload)
		response := fmt.Sprintf(
			`{"type":"response.completed","response":{"id":"resp_ws_turn_eligibility_%d","model":%q,"usage":{"input_tokens":2,"output_tokens":1}}}`,
			n, gjson.GetBytes(payload, "model").String(),
		)
		writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
		writeErr := conn.Write(writeCtx, coderws.MessageText, []byte(response))
		cancelWrite()
		if writeErr != nil {
			return
		}
	}
}

type wsTurnEligibilityCase struct {
	ingressMode string
	oauth       bool
	credentials map[string]any
	extra       map[string]any
	// channelMapping 非空时给 API Key 所在分组挂一条 OpenAI 渠道模型映射。
	channelMapping map[string]string
	clientModel    string
	// secondClientModel 为空时第二轮沿用 clientModel。
	secondClientModel string
	// wantFirstUpstreamModel 是首轮实际发往上游的模型，也就是按模型限流应当记录的键。
	wantFirstUpstreamModel string
	// betweenTurns 在上游收到首轮请求后执行，模拟两轮之间账号状态被改。
	betweenTurns func(account *service.Account)
	wantClosed   bool
}

type wsTurnEligibilityResult struct {
	upstreamRequests [][]byte
	stickyHashes     []string
	stickyDeleted    int
}

func runWSTurnEligibilityCase(t *testing.T, tc wsTurnEligibilityCase) wsTurnEligibilityResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upstream := &wsTurnEligibilityUpstream{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(upstream.serveWS))
	defer upstreamServer.Close()

	groupID := int64(4601)
	account := service.Account{
		ID:          9961,
		Name:        "openai-ws-turn-eligibility",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": upstreamServer.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    tc.ingressMode,
		},
	}
	if tc.oauth {
		account.Type = service.AccountTypeOAuth
		account.Credentials = map[string]any{"access_token": "at-test"}
		account.Extra = map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    tc.ingressMode,
		}
	}
	for key, value := range tc.credentials {
		account.Credentials[key] = value
	}
	for key, value := range tc.extra {
		account.Extra[key] = value
	}
	accountRepo := &wsTurnEligibilityAccountRepo{account: account}
	if tc.betweenTurns != nil {
		upstream.afterFirstReq = func() { accountRepo.update(tc.betweenTurns) }
	}

	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.MaxLineSize = 1 << 20
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	var channelSvc *service.ChannelService
	if len(tc.channelMapping) > 0 {
		channelSvc = service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
			channels: []service.Channel{{
				ID:           7761,
				Name:         "openai-ws-turn-eligibility-channel",
				Status:       service.StatusActive,
				GroupIDs:     []int64{groupID},
				ModelMapping: map[string]map[string]string{service.PlatformOpenAI: tc.channelMapping},
			}},
			groupPlatforms: map[int64]string{groupID: service.PlatformOpenAI},
		}, nil, nil, nil, nil)
	}

	stickyCache := &wsTurnEligibilityStickyCache{
		GatewayCache: testutil.NewRedisGatewayCache(t),
		bound:        map[string]int64{},
		deleted:      map[string]int{},
	}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 8)}
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo, usageRepo, nil, nil, nil, nil, stickyCache, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheSvc, upstream, &service.DeferredService{},
		nil, nil, nil, channelSvc, nil, nil, nil,
	)
	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := &OpenAIGatewayHandler{
		cfg:                 cfg,
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatNone, time.Second),
	}

	apiKey := &service.APIKey{
		ID:      1861,
		GroupID: &groupID,
		User:    &service.User{ID: 1761, Status: service.StatusActive},
	}
	handlerDone := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		defer close(handlerDone)
		h.ResponsesWebSocket(c)
	})
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeFrame := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWrite()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	// readTurn 读到本轮 response.completed 为止；连接被关闭时返回读错误。
	readTurn := func() ([]byte, error) {
		for {
			readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
			_, event, readErr := clientConn.Read(readCtx)
			cancelRead()
			if readErr != nil {
				return nil, readErr
			}
			if gjson.GetBytes(event, "type").String() == "response.completed" {
				return event, nil
			}
		}
	}

	turnFrame := func(model string) string {
		return fmt.Sprintf(`{"type":"response.create","model":%q,"stream":true,"prompt_cache_key":"ws-turn-eligibility","input":[{"type":"input_text","text":"hello"}]}`, model)
	}
	writeFrame(turnFrame(tc.clientModel))
	_, err = readTurn()
	require.NoError(t, err, "首轮必须正常完成")
	firstRequests, transports := upstream.snapshot()
	require.Len(t, firstRequests, 1)
	wantTransport := "ws"
	if tc.ingressMode == service.OpenAIWSIngressModeHTTPBridge {
		wantTransport = "http"
	}
	require.Equal(t, []string{wantTransport}, transports, "前置条件：首轮走的是预期的入口模式")
	if tc.wantFirstUpstreamModel != "" {
		require.Equal(t, tc.wantFirstUpstreamModel, gjson.GetBytes(firstRequests[0], "model").String(),
			"前置条件：首轮实际发往上游的模型")
	}
	hashes, _ := stickyCache.snapshot(account.ID)
	require.NotEmpty(t, hashes, "前置条件：握手后会话已粘到该账号")

	secondModel := tc.clientModel
	if tc.secondClientModel != "" {
		secondModel = tc.secondClientModel
	}
	writeFrame(turnFrame(secondModel))
	event, readErr := readTurn()
	if tc.wantClosed {
		require.Error(t, readErr, "账号在两轮之间已不可调度，第二轮必须在写上游前关闭连接，实际却收到了上游结果: %s", event)
		var closeErr coderws.CloseError
		require.ErrorAs(t, readErr, &closeErr)
		require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code, "close code 必须是客户端会自动重连的 TryAgainLater")
		require.Contains(t, closeErr.Reason, "no longer schedulable")
	} else {
		require.NoError(t, readErr, "账号仍然可用时第二轮必须正常完成")
		_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	}

	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("websocket handler did not exit")
	}

	stickyHashes, stickyDeleted := stickyCache.snapshot(account.ID)
	upstreamRequests, _ := upstream.snapshot()
	return wsTurnEligibilityResult{
		upstreamRequests: upstreamRequests,
		stickyHashes:     stickyHashes,
		stickyDeleted:    stickyDeleted,
	}
}

func requireWSTurnClosedBeforeUpstream(t *testing.T, got wsTurnEligibilityResult) {
	t.Helper()
	require.Len(t, got.upstreamRequests, 1, "第二轮不得发到上游")
	require.NotZero(t, got.stickyDeleted, "断开前必须解除粘性绑定，重连才会重新选号")
}

func wsTurnModelRateLimitedExtra(base map[string]any, model string) map[string]any {
	extra := make(map[string]any, len(base)+1)
	for key, value := range base {
		extra[key] = value
	}
	extra["model_rate_limits"] = map[string]any{
		model: map[string]any{
			"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
	return extra
}

func TestOpenAIResponsesWebSocket_TurnAccountEligibility_AdminDisabledClosesBeforeNextTurn(t *testing.T) {
	for _, mode := range []string{
		service.OpenAIWSIngressModePassthrough,
		service.OpenAIWSIngressModeCtxPool,
		service.OpenAIWSIngressModeHTTPBridge,
	} {
		t.Run(mode, func(t *testing.T) {
			got := runWSTurnEligibilityCase(t, wsTurnEligibilityCase{
				ingressMode: mode,
				clientModel: "gpt-5.1",
				betweenTurns: func(account *service.Account) {
					account.Schedulable = false
				},
				wantClosed: true,
			})
			requireWSTurnClosedBeforeUpstream(t, got)
		})
	}
}

func TestOpenAIResponsesWebSocket_TurnAccountEligibility_HealthyAccountKeepsConnection(t *testing.T) {
	for _, mode := range []string{
		service.OpenAIWSIngressModePassthrough,
		service.OpenAIWSIngressModeCtxPool,
		service.OpenAIWSIngressModeHTTPBridge,
	} {
		t.Run(mode, func(t *testing.T) {
			got := runWSTurnEligibilityCase(t, wsTurnEligibilityCase{
				ingressMode: mode,
				clientModel: "gpt-5.1",
				betweenTurns: func(account *service.Account) {
					// 限流的是别的模型，不能误伤本连接。
					account.Extra = wsTurnModelRateLimitedExtra(account.Extra, "gpt-5.2")
				},
			})
			require.Len(t, got.upstreamRequests, 2)
			require.NotEmpty(t, got.stickyHashes)
			require.Zero(t, got.stickyDeleted, "健康账号的粘性绑定不能被动")
		})
	}
}

// 按模型限流记在「实际发往上游的模型」上，而客户端发的是另一个名字：资格复核必须按
// 与转发相同的口径推导模型键，否则客户端一直发原名就永远查不到这条限流。
func TestOpenAIResponsesWebSocket_TurnAccountEligibility_ModelRateLimitOnForwardedModel(t *testing.T) {
	cases := []struct {
		name string
		tc   wsTurnEligibilityCase
	}{
		{
			// 账号级 model_mapping：gpt-5.1 → gpt-5.1-mapped。
			name: "ctx_pool/account_model_mapping",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModeCtxPool,
				credentials:            map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1-mapped"}},
				clientModel:            "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.1-mapped",
			},
		},
		{
			name: "http_bridge/account_model_mapping",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModeHTTPBridge,
				credentials:            map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1-mapped"}},
				clientModel:            "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.1-mapped",
			},
		},
		{
			// 渠道模型映射：passthrough 原样转发渠道映射后的模型。
			name: "passthrough/channel_model_mapping",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModePassthrough,
				channelMapping:         map[string]string{"public-alias": "gpt-5.1"},
				clientModel:            "public-alias",
				wantFirstUpstreamModel: "gpt-5.1",
			},
		},
		{
			// Codex 协议账号的上游模型归一：gpt-5.1 → gpt-5.4。
			name: "http_bridge/oauth_upstream_model_normalization",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModeHTTPBridge,
				oauth:                  true,
				clientModel:            "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.4",
			},
		},
		{
			// openai_passthrough 只影响 HTTP Forward，WS 入口照样做模型归一。
			name: "http_bridge/oauth_openai_passthrough_flag",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModeHTTPBridge,
				oauth:                  true,
				extra:                  map[string]any{"openai_passthrough": true},
				clientModel:            "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.4",
			},
		},
		{
			// 通配映射会把最终模型 gpt-5.4 再映回 gpt-5.1，复核不能二次映射。
			name: "http_bridge/oauth_wildcard_model_mapping",
			tc: wsTurnEligibilityCase{
				ingressMode:            service.OpenAIWSIngressModeHTTPBridge,
				oauth:                  true,
				credentials:            map[string]any{"model_mapping": map[string]any{"*": "gpt-5.1"}},
				clientModel:            "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.4",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := c.tc
			limitedModel := tc.wantFirstUpstreamModel
			tc.betweenTurns = func(account *service.Account) {
				account.Extra = wsTurnModelRateLimitedExtra(account.Extra, limitedModel)
			}
			tc.wantClosed = true
			got := runWSTurnEligibilityCase(t, tc)
			requireWSTurnClosedBeforeUpstream(t, got)
		})
	}
}

// 连接内切换模型：资格复核必须按本轮 response.create 声明的模型判定，而不是建连时的
// 首轮模型。首轮用 gpt-5.2，两轮之间 gpt-5.1 被限流，第二轮改发 gpt-5.1 必须断开。
func TestOpenAIResponsesWebSocket_TurnAccountEligibility_UsesCurrentTurnModel(t *testing.T) {
	for _, mode := range []string{
		service.OpenAIWSIngressModePassthrough,
		service.OpenAIWSIngressModeCtxPool,
		service.OpenAIWSIngressModeHTTPBridge,
	} {
		t.Run(mode, func(t *testing.T) {
			got := runWSTurnEligibilityCase(t, wsTurnEligibilityCase{
				ingressMode:            mode,
				clientModel:            "gpt-5.2",
				secondClientModel:      "gpt-5.1",
				wantFirstUpstreamModel: "gpt-5.2",
				betweenTurns: func(account *service.Account) {
					account.Extra = wsTurnModelRateLimitedExtra(account.Extra, "gpt-5.1")
				},
				wantClosed: true,
			})
			requireWSTurnClosedBeforeUpstream(t, got)
		})
	}
}
