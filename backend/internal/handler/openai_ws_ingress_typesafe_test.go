package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// openAIWSIngressRecordingAccountRepoStub 与 openAIWSUsageHandlerAccountRepoStub
// 返回同一批账号，但额外记录调度器实际查询过的 platform，用来区分
// 「账号压根不在候选池」与「账号在候选池里但被传输兼容性过滤掉」。
type openAIWSIngressRecordingAccountRepoStub struct {
	service.AccountRepository
	account service.Account

	mu        sync.Mutex
	platforms []string
}

func (s *openAIWSIngressRecordingAccountRepoStub) record(platform string) {
	s.mu.Lock()
	s.platforms = append(s.platforms, platform)
	s.mu.Unlock()
}

func (s *openAIWSIngressRecordingAccountRepoStub) recordedPlatforms() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.platforms...)
}

func (s *openAIWSIngressRecordingAccountRepoStub) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	s.record(platform)
	if s.account.Platform != platform {
		return nil, nil
	}
	return []service.Account{s.account}, nil
}

func (s *openAIWSIngressRecordingAccountRepoStub) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return s.ListSchedulableByPlatform(ctx, platform)
}

func (s *openAIWSIngressRecordingAccountRepoStub) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	if s.account.ID != id {
		return nil, nil
	}
	account := s.account
	return &account, nil
}

// openAIWSIngressTypesafeCaseResult 汇总一次 WS ingress 尝试的观测结果。
type openAIWSIngressTypesafeCaseResult struct {
	closeCode         coderws.StatusCode
	closeReason       string
	upstreamHits      int64
	recordedPlatforms []string
}

// runOpenAIWSIngressCase 走完「GET /v1/responses → ResponsesWebSocket → 首帧 →
// 调度选号 → 上游 dial」的真实路径，返回客户端被关闭时的 code/reason 以及上游
// HTTP 服务器被连接的次数。
func runOpenAIWSIngressCase(t *testing.T, account service.Account) openAIWSIngressTypesafeCaseResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var upstreamHits atomic.Int64
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		<-r.Context().Done()
	}))
	defer upstreamServer.Close()

	account.Credentials = map[string]any{
		"api_key":  "test-upstream-key",
		"base_url": upstreamServer.URL,
	}

	groupID := int64(8801)
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = service.OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &openAIWSIngressRecordingAccountRepoStub{account: account}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{}
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo,
		usageRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		nil,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil, // userPlatformQuotaRepo
	)

	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}

	apiKey := &service.APIKey{
		ID:      8801,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: account.Platform},
		User:    &service.User{ID: 8801, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.GET("/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(
		dialCtx,
		"ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/v1/responses",
		&coderws.DialOptions{CompressionMode: coderws.CompressionContextTakeover},
	)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"jev-judge-v1"}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	_, _, readErr := clientConn.Read(readCtx)
	cancelRead()

	result := openAIWSIngressTypesafeCaseResult{
		upstreamHits:      upstreamHits.Load(),
		recordedPlatforms: accountRepo.recordedPlatforms(),
	}
	var closeErr coderws.CloseError
	if errors.As(readErr, &closeErr) {
		result.closeCode = closeErr.Code
		result.closeReason = closeErr.Reason
	}
	return result
}

// TestOpenAIResponsesWebSocket_TypeSafeGroupSelectsNoAccountAndCallsNoUpstream 是
// 「GET /v1/responses（Responses WebSocket ingress）在 typesafe 分组下到底怎样」
// 这一事实分歧的实证。
//
// 分歧：
//   - 说法一：fail-closed，选不到账号（ResolveOpenAIResponsesWebSocketV2Mode 对
//     !IsOpenAI() 返回 off；isOpenAIAccountTransportCompatible(typesafe 账号, ingress)
//     为 false）。
//   - 说法二：会选中 typesafe 账号，上游 URL 由 GetOpenAIBaseURL() 得到
//     api.typesafe.ai/responses。
//
// 本测试用真实 OpenAIGatewayService + 真实 WebSocket dial 走完
// 入口 → 首帧 → 调度选号 的完整路径，并统计上游 HTTP 服务器被连接的次数：
//   - 若选不中账号：客户端被 1013(StatusTryAgainLater) 关闭、reason "no available account"，
//     上游命中数为 0（本测试断言）。
//   - 若选中了 typesafe 账号：上游服务器必然被 dial（命中数 > 0）。
//
// 对照组 TestOpenAIResponsesWebSocket_OpenAIAccountStillDialsUpstream 用同一套
// 夹具换成 openai 账号，证明该路径在账号合格时确实会连上游 —— 因此 typesafe
// 的 0 次命中是「选不中账号」而非夹具没跑通。
//
// 该断言在入口门（routes 层 rejectTypeSafeConversationalEndpoint）加入前后都成立：
// 门只是提前拒绝，处理器/调度层的真实行为不变。
func TestOpenAIResponsesWebSocket_TypeSafeGroupSelectsNoAccountAndCallsNoUpstream(t *testing.T) {
	result := runOpenAIWSIngressCase(t, service.Account{
		ID:          8801,
		Name:        "typesafe-jev-judge",
		Platform:    service.PlatformTypeSafe,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		// 即便显式写死 ctx_pool，非 openai 平台也拿不到 WS ingress 资格。
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": service.OpenAIWSIngressModeCtxPool,
		},
	})

	require.Equal(t, coderws.StatusTryAgainLater, result.closeCode, "close reason=%q", result.closeReason)
	require.Contains(t, result.closeReason, "no available account")
	require.Equal(t, int64(0), result.upstreamHits, "typesafe 分组不得发生任何上游调用")
	// 关键：调度器确实按 typesafe 平台查询过候选池（账号在池子里），
	// 但仍被传输兼容性过滤掉 —— 证明 fail-closed 发生在选号过滤而非「压根没查」。
	require.Contains(t, result.recordedPlatforms, service.PlatformTypeSafe,
		"recorded platforms=%v", result.recordedPlatforms)
}

// TestOpenAIResponsesWebSocket_OpenAIAccountStillDialsUpstream 是上一条的阳性对照：
// 同一夹具换成符合条件的 openai 账号后，WS ingress 必须真的连到上游。
func TestOpenAIResponsesWebSocket_OpenAIAccountStillDialsUpstream(t *testing.T) {
	result := runOpenAIWSIngressCase(t, service.Account{
		ID:          8802,
		Name:        "openai-ws-eligible",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": service.OpenAIWSIngressModeCtxPool,
		},
	})

	require.Greater(t, result.upstreamHits, int64(0), "符合条件的 openai 账号必须连到上游（阳性对照）")
}
