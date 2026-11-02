package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type openAIWSAccountWaitSession struct {
	conn     *coderws.Conn
	cache    service.ConcurrencyCache
	requests chan []byte
	finished chan struct{}
	cancel   chan context.CancelCauseFunc
	handler  *OpenAIGatewayHandler
	server   *httptest.Server
	upstream *httptest.Server
	groupID  int64
	// gate 非空时上游在收到请求后阻塞到它关闭，用来让一个请求持续占住槽位。
	gate chan struct{}
	// failUpstream 为真时上游对所有请求回 502，用来制造错误轮与 failover。
	failUpstream atomic.Bool
	// rateLimitPOSTs 大于 0 时上游对接下来这么多个 POST 回 429，用来触发 OAuth 同账号重试。
	rateLimitPOSTs atomic.Int32
}

type openAIWSSessionOptions struct {
	mode          string
	timeout       time.Duration
	holdSeconds   int
	burstLimit    int
	waitingLimit  int
	bindings      service.GatewayCache
	concurrency   service.ConcurrencyCache
	extraAccounts []service.Account
	dialHeader    http.Header
	skipDial      bool
	// loadBatch 打开负载批量选号（生产默认开启）；关闭时非高级调度走无溢出语义的快路径。
	loadBatch bool
	// oauth 把主账号 801 换成 OAuth 账号，上游请求改写到测试服务器。
	oauth bool
	// platform 非空时主账号与分组换成该平台，上游请求改写到测试服务器。
	platform string
}

func TestOpenAIWSAccountWait_ExitReleasesResources(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge, service.OpenAIWSIngressModePassthrough} {
		for _, later := range []bool{false, true} {
			for _, reason := range []string{"timeout", "queue_full", "disconnect", "early_frame_disconnect"} {
				t.Run(fmt.Sprintf("%s/later=%v/%s", mode, later, reason), func(t *testing.T) {
					f := newOpenAIWSAccountWaitSession(t, mode, 250*time.Millisecond)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					if later {
						completeOpenAIWSAccountWaitTurn(t, ctx, f)
					}
					// 续聊轮（later=true）固定走续聊类，按 wait:account:cont 计数；首轮仍走旧键。
					incrementWaitCount := f.cache.IncrementAccountWaitCount
					getWaitingCount := f.cache.GetAccountWaitingCount
					if later {
						incrementWaitCount = f.cache.IncrementAccountContinuationWaitCount
						getWaitingCount = f.cache.GetAccountContinuationWaitingCount
					}
					ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other")
					require.NoError(t, err)
					require.True(t, ok)
					if reason == "queue_full" {
						for range 2 {
							ok, err = incrementWaitCount(ctx, 801, 2)
							require.NoError(t, err)
							require.True(t, ok)
						}
					}
					require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"blocked"}`)))
					if strings.Contains(reason, "disconnect") {
						require.Eventually(t, func() bool { n, _ := getWaitingCount(ctx, 801); return n == 1 }, time.Second, time.Millisecond)
						if reason == "early_frame_disconnect" {
							require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"early"}`)))
						}
						_ = f.conn.CloseNow()
					} else {
						_, _, err = f.conn.Read(ctx)
						require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
					}
					select {
					case <-f.finished:
					case <-ctx.Done():
						t.Fatal("handler did not clean up")
					}
					waiting, err := getWaitingCount(ctx, 801)
					require.NoError(t, err)
					if reason == "queue_full" {
						require.Equal(t, 2, waiting)
					} else {
						require.Zero(t, waiting)
					}
					accounts, err := f.cache.GetAccountConcurrency(ctx, 801)
					require.NoError(t, err)
					require.Equal(t, 1, accounts, "must preserve the other request's slot")
					users, err := f.cache.GetUserConcurrency(ctx, 1702)
					require.NoError(t, err)
					require.Zero(t, users)
					select {
					case <-f.requests:
						t.Fatal("canceled/expired request reached upstream")
					default:
					}
				})
			}
		}
	}
}

func TestOpenAIWSAccountWaitBudget_Boundaries(t *testing.T) {
	b := &openAIWSAccountWaitBudget{turn: 1}
	require.True(t, b.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseInitial))
	first := b.waitDeadline(time.Second, time.Time{})
	require.Equal(t, first, b.waitDeadline(time.Minute, time.Time{}), "retry must not reset the wait budget")
	retryDeadline := time.Now().Add(50 * time.Millisecond)
	require.Equal(t, retryDeadline, b.waitDeadline(time.Minute, retryDeadline))
	require.Equal(t, first, b.deadline, "same-account retry must not shorten the turn's account wait budget")
	require.Equal(t, first, b.waitDeadline(time.Minute, time.Time{}), "failover must retain the account wait budget")
	b.requestSent.Store(true)
	require.False(t, b.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseInitial), "sent request cannot become a fresh passthrough request")
	require.False(t, b.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseRetry))
	require.True(t, b.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseSubsequent), "a later turn is held by the gateway until admitted")
	b.nextTurn()
	require.True(t, b.deadline.IsZero())
	for _, phase := range []string{openAIWSAccountWaitPhaseInitial, openAIWSAccountWaitPhaseRetry, openAIWSAccountWaitPhaseSubsequent} {
		require.Equal(t, phase == openAIWSAccountWaitPhaseSubsequent, b.canWait(service.OpenAIWSIngressModePassthrough, phase), phase)
		require.True(t, b.canWait(service.OpenAIWSIngressModeCtxPool, phase), phase)
		require.True(t, b.canWait(service.OpenAIWSIngressModeHTTPBridge, phase), phase)
		require.False(t, b.canWait(service.OpenAIWSIngressModeOff, phase), phase)
	}
	continuation := &openAIWSAccountWaitBudget{continuation: true}
	require.False(t, continuation.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseInitial))
	require.True(t, continuation.canWait(service.OpenAIWSIngressModePassthrough, openAIWSAccountWaitPhaseSubsequent))
}

func TestOpenAIWSAccountWait_NoPlanAndAcquireCancellation(t *testing.T) {
	for _, scenario := range []string{"no_plan", "canceled_after_acquire", "canceled_after_wait_acquire", "cache_error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempts := 0
			cache := &concurrencyCacheMock{acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				attempts++
				if scenario == "canceled_after_acquire" || (scenario == "canceled_after_wait_acquire" && attempts > 1) {
					cancel()
					return true, nil
				}
				if scenario == "cache_error" {
					return false, errors.New("redis unavailable")
				}
				return false, nil
			}}
			h := &OpenAIGatewayHandler{concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)}
			var plan *service.AccountWaitPlan
			if scenario == "canceled_after_wait_acquire" {
				plan = &service.AccountWaitPlan{Timeout: time.Second, MaxWaiting: 2}
			}
			release, err := h.acquireWSAccountSlot(ctx, &service.Account{ID: 801, Platform: service.PlatformOpenAI}, 1, plan, &openAIWSAccountWaitBudget{turn: 1}, service.OpenAIWSIngressModeCtxPool, openAIWSAccountWaitPhaseInitial, time.Time{}, zap.NewNop())
			require.Nil(t, release)
			require.Error(t, err)
			if strings.HasPrefix(scenario, "canceled_after_") {
				require.ErrorIs(t, err, context.Canceled)
				require.EqualValues(t, 1, cache.releaseAccountCalled)
			} else {
				var closeErr *service.OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				if scenario == "cache_error" {
					require.Equal(t, coderws.StatusInternalError, closeErr.StatusCode())
				} else {
					require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
				}
			}
		})
	}
}

func TestOpenAIWSAccountWait_DeadlineAdmissionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name             string
		waitExpired      bool
		retryExpired     bool
		slotAvailable    bool
		wantAcquired     bool
		wantAcquireCalls int
	}{
		{name: "account_wait", waitExpired: true, slotAvailable: true},
		{name: "both", waitExpired: true, retryExpired: true, slotAvailable: true},
		{name: "retry_expired_free_slot", retryExpired: true, slotAvailable: true, wantAcquired: true, wantAcquireCalls: 1},
		{name: "retry_expired_busy_slot", retryExpired: true, wantAcquireCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			cache := &concurrencyCacheMock{acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				attempts++
				return tc.slotAvailable, nil
			}}
			h := &OpenAIGatewayHandler{concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)}
			budget := &openAIWSAccountWaitBudget{turn: 1}
			var retryDeadline time.Time
			if tc.waitExpired {
				budget.deadline = time.Now().Add(-time.Second)
			}
			if tc.retryExpired {
				retryDeadline = time.Now().Add(-time.Second)
			}
			release, err := h.acquireWSAccountSlot(context.Background(), &service.Account{ID: 801, Platform: service.PlatformOpenAI}, 1,
				&service.AccountWaitPlan{Timeout: time.Second, MaxWaiting: 2}, budget, service.OpenAIWSIngressModeCtxPool, openAIWSAccountWaitPhaseRetry, retryDeadline, zap.NewNop())
			if release != nil {
				release()
			}
			require.Equal(t, tc.wantAcquireCalls, attempts)
			if tc.wantAcquired {
				require.NoError(t, err)
				require.NotNil(t, release)
				require.EqualValues(t, 1, cache.releaseAccountCalled)
				return
			}
			require.Nil(t, release)
			var closeErr *service.OpenAIWSClientCloseError
			require.ErrorAs(t, err, &closeErr)
			require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
		})
	}
}

func TestOpenAIWSAccountWait_RetryDeadlineDoesNotExpireFailoverBudget(t *testing.T) {
	for _, acquireWhileWaiting := range []bool{true, false} {
		t.Run(fmt.Sprintf("acquired=%v", acquireWhileWaiting), func(t *testing.T) {
			var waitDeadline time.Time
			cache := &concurrencyCacheMock{acquireAccountSlotFn: func(ctx context.Context, accountID int64, _ int, _ string) (bool, error) {
				if accountID == 802 {
					return true, nil
				}
				if deadline, ok := ctx.Deadline(); ok {
					waitDeadline = deadline
					return acquireWhileWaiting, nil
				}
				return false, nil
			}}
			h := &OpenAIGatewayHandler{concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)}
			budget := &openAIWSAccountWaitBudget{turn: 1}
			plan := &service.AccountWaitPlan{Timeout: time.Minute, MaxWaiting: 2}
			retryDeadline := time.Now().Add(250 * time.Millisecond)
			release, err := h.acquireWSAccountSlot(context.Background(), &service.Account{ID: 801, Platform: service.PlatformOpenAI}, 1,
				plan, budget, service.OpenAIWSIngressModeCtxPool, openAIWSAccountWaitPhaseRetry, retryDeadline, zap.NewNop())
			if release != nil {
				release()
			}
			if acquireWhileWaiting {
				require.NoError(t, err)
				require.NotNil(t, release)
			} else {
				require.Nil(t, release)
				var closeErr *service.OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
			}
			require.Equal(t, retryDeadline, waitDeadline, "same-account retry deadline must still cap queue waiting")
			<-time.After(time.Until(retryDeadline))
			require.False(t, budget.expired(), "retry window expiry must not exhaust the turn's admission budget")
			release, err = h.acquireWSAccountSlot(context.Background(), &service.Account{ID: 802, Platform: service.PlatformOpenAI}, 1,
				plan, budget, service.OpenAIWSIngressModeCtxPool, openAIWSAccountWaitPhaseInitial, time.Time{}, zap.NewNop())
			require.NoError(t, err)
			require.NotNil(t, release)
			release()
		})
	}
}

func TestOpenAIWSAccountWait_LeaseLossReleasesResources(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge, service.OpenAIWSIngressModePassthrough} {
		for _, later := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/later=%v", mode, later), func(t *testing.T) {
				f := newOpenAIWSAccountWaitSession(t, mode, 2*time.Second)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if later {
					completeOpenAIWSAccountWaitTurn(t, ctx, f)
				}
				// 续聊轮（later=true）固定走续聊类，按 wait:account:cont 计数；首轮仍走旧键。
				getWaitingCount := f.cache.GetAccountWaitingCount
				if later {
					getWaitingCount = f.cache.GetAccountContinuationWaitingCount
				}
				ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other")
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"waiting"}`)))
				require.Eventually(t, func() bool { n, _ := getWaitingCount(ctx, 801); return n == 1 }, time.Second, time.Millisecond)
				(<-f.cancel)(service.ErrOpenAIWSIngressLeaseLost)
				_, _, err = f.conn.Read(ctx)
				require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
				require.Contains(t, err.Error(), "ingress capacity lease lost")
				select {
				case <-f.finished:
				case <-ctx.Done():
					t.Fatal("handler did not clean up")
				}
				n, err := getWaitingCount(ctx, 801)
				require.NoError(t, err)
				require.Zero(t, n)
				n, err = f.cache.GetUserConcurrency(ctx, 1702)
				require.NoError(t, err)
				require.Zero(t, n)
				n, err = f.cache.GetAccountConcurrency(ctx, 801)
				require.NoError(t, err)
				require.Equal(t, 1, n)
				require.Empty(t, f.requests, "canceled turn must not reach upstream")
			})
		}
	}
}

func newOpenAIWSAccountWaitSession(t *testing.T, mode string, timeout time.Duration) *openAIWSAccountWaitSession {
	t.Helper()
	return newOpenAIWSAccountWaitSessionWithHold(t, mode, timeout, 0)
}

func newOpenAIWSAccountWaitSessionWithHold(t *testing.T, mode string, timeout time.Duration, holdSeconds int) *openAIWSAccountWaitSession {
	t.Helper()
	return newOpenAIWSAccountWaitSessionWithDeps(t, mode, timeout, holdSeconds, nil, nil)
}

// newOpenAIWSAccountWaitSessionWithDeps 允许多个会话共用同一份粘性绑定与并发缓存，模拟独立连接命中同一绑定。
func newOpenAIWSAccountWaitSessionWithDeps(t *testing.T, mode string, timeout time.Duration, holdSeconds int, bindings service.GatewayCache, concurrencyCache service.ConcurrencyCache) *openAIWSAccountWaitSession {
	t.Helper()
	return newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: mode, timeout: timeout, holdSeconds: holdSeconds, bindings: bindings, concurrency: concurrencyCache})
}

func newOpenAIWSSessionWithOptions(t *testing.T, opts openAIWSSessionOptions) *openAIWSAccountWaitSession {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mode, timeout, holdSeconds, bindings, concurrencyCache := opts.mode, opts.timeout, opts.holdSeconds, opts.bindings, opts.concurrency
	f := &openAIWSAccountWaitSession{requests: make(chan []byte, 32), finished: make(chan struct{}), cancel: make(chan context.CancelCauseFunc, 1), groupID: 4202}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.failUpstream.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			f.requests <- body
			if n := f.rateLimitPOSTs.Load(); n > 0 && f.rateLimitPOSTs.CompareAndSwap(n, n-1) {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			if f.gate != nil {
				<-f.gate
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: "+accountWaitCompleted+"\n\n")
			return
		}
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			_, body, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			f.requests <- body
			if f.gate != nil {
				<-f.gate
			}
			if conn.Write(r.Context(), coderws.MessageText, []byte(accountWaitCompleted)) != nil {
				return
			}
		}
	}))
	t.Cleanup(upstream.Close)
	f.upstream = upstream
	accounts := []service.Account{{
		ID: 801, Name: "account-wait", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": upstream.URL},
		Extra:       map[string]any{"openai_apikey_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_mode": mode},
	}}
	var httpUpstream service.HTTPUpstream = openAIWSTurnBudgetHTTPClient{}
	if opts.oauth {
		accounts[0].Type = service.AccountTypeOAuth
		accounts[0].Credentials = map[string]any{"access_token": "oauth-token"}
		accounts[0].Extra = map[string]any{"openai_oauth_responses_websockets_v2_enabled": true, "openai_oauth_responses_websockets_v2_mode": mode}
		httpUpstream = openAIWSRewriteHTTPClient{target: upstream.URL}
	}
	groupPlatform := service.PlatformOpenAI
	if opts.platform != "" {
		groupPlatform = opts.platform
		accounts[0].Platform = opts.platform
		httpUpstream = openAIWSRewriteHTTPClient{target: upstream.URL}
	}
	for _, extra := range opts.extraAccounts {
		if extra.Credentials == nil {
			extra.Credentials = map[string]any{"api_key": "sk-test", "base_url": upstream.URL}
		}
		if extra.Extra == nil {
			extra.Extra = map[string]any{"openai_apikey_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_mode": mode}
		}
		accounts = append(accounts, extra)
	}
	repo := &openAIWSTurnBudgetAccountRepo{accounts: accounts}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = opts.oauth
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = timeout
	cfg.Gateway.Scheduling.FallbackWaitTimeout = timeout
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
	cfg.Gateway.Scheduling.ContinuationMaxWaiting = 2
	if opts.waitingLimit > 0 {
		cfg.Gateway.Scheduling.ContinuationMaxWaiting = opts.waitingLimit
	}
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 2
	cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds = holdSeconds
	cfg.Gateway.Scheduling.ContinuationBurstLimit = opts.burstLimit
	cfg.Gateway.Scheduling.LoadBatchEnabled = opts.loadBatch
	f.cache = concurrencyCache
	if f.cache == nil {
		f.cache = testutil.NewTestConcurrencyCache(t)
	}
	concurrency := service.NewConcurrencyService(f.cache)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, bindings, cfg, nil, concurrency,
		service.NewBillingService(cfg, nil), nil, billing, httpUpstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(gateway.CloseOpenAIWSPool)
	h := NewOpenAIGatewayHandler(gateway, concurrency, billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	f.handler = h
	groupID := f.groupID
	apiKey := &service.APIKey{ID: 1802, GroupID: &groupID, User: &service.User{ID: 1702, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: groupPlatform, Status: service.StatusActive, AllowMessagesDispatch: true}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1702, Concurrency: 1})
		c.Next()
	})
	router.GET("/v1/responses", func(c *gin.Context) {
		defer close(f.finished)
		ctx, cancel := context.WithCancelCause(c.Request.Context())
		defer cancel(nil)
		f.cancel <- cancel
		c.Request = c.Request.WithContext(ctx)
		h.ResponsesWebSocket(c)
	})
	router.POST("/v1/responses", h.Responses)
	router.POST("/v1/chat/completions", h.ChatCompletions)
	router.POST("/v1/messages", h.Messages)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	f.server = server
	if opts.skipDial {
		close(f.finished)
		return f
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var err error
	f.conn, _, err = coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &coderws.DialOptions{HTTPHeader: opts.dialHeader})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.conn.CloseNow()
		select {
		case <-f.finished:
		case <-time.After(3 * time.Second):
			t.Error("handler did not exit")
		}
	})
	return f
}

// openAIWSRewriteHTTPClient 把 OAuth 账号发往官方地址的请求改写到测试上游。
type openAIWSRewriteHTTPClient struct {
	service.HTTPUpstream
	target string
}

func (c openAIWSRewriteHTTPClient) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	target, err := url.Parse(c.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.Host = target.Host
	return http.DefaultClient.Do(req)
}

const accountWaitCompleted = `{"type":"response.completed","response":{"id":"resp_wait","model":"gpt-5.1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`

func completeOpenAIWSAccountWaitTurn(t *testing.T, ctx context.Context, f *openAIWSAccountWaitSession) {
	t.Helper()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, _, err := f.conn.Read(ctx)
	require.NoError(t, err)
	<-f.requests
	require.Eventually(t, func() bool { n, _ := f.cache.GetAccountConcurrency(ctx, 801); return n == 0 }, time.Second, time.Millisecond)
}

type openAIWSStickyBindings struct {
	service.GatewayCache
	mu        sync.Mutex
	accounts  map[string]int64
	refreshes int
}

// snapshot 返回当前全部绑定的副本，键是服务层传入的缓存键。
func (b *openAIWSStickyBindings) snapshot() map[string]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]int64, len(b.accounts))
	for k, v := range b.accounts {
		out[k] = v
	}
	return out
}

func (b *openAIWSStickyBindings) refreshCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.refreshes
}

func (b *openAIWSStickyBindings) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if id, ok := b.accounts[sessionHash]; ok {
		return id, nil
	}
	return 0, service.ErrStickySessionNotFound
}

func (b *openAIWSStickyBindings) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.accounts == nil {
		b.accounts = make(map[string]int64)
	}
	b.accounts[sessionHash] = accountID
	return nil
}

func (b *openAIWSStickyBindings) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshes++
	return nil
}

func (b *openAIWSStickyBindings) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.accounts, sessionHash)
	return nil
}

func (b *openAIWSStickyBindings) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.accounts)
}

func TestOpenAIWSFirstFrameContinuationEligibilityNeedsExplicitSession(t *testing.T) {
	svc := &service.OpenAIGatewayService{}
	newCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		return c
	}
	content := []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)
	require.NotEmpty(t, svc.GenerateSessionHash(newCtx(), content), "只有 model 与 input 的首帧也会走内容回退得到非空哈希")
	require.Empty(t, svc.ExtractSessionID(newCtx(), content), "内容回退哈希不是显式会话标识")
	withKey := []byte(`{"type":"response.create","model":"gpt-5.1","input":"first","prompt_cache_key":"pck-1"}`)
	require.Equal(t, "pck-1", svc.ExtractSessionID(newCtx(), withKey))
	withHeader := newCtx()
	withHeader.Request.Header.Set("session_id", "sess-1")
	require.Equal(t, "sess-1", svc.ExtractSessionID(withHeader, content))
}

func TestOpenAIWSAccountWait_ContentHashBindingYieldsToContinuationWaiters(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	first := newOpenAIWSAccountWaitSessionWithDeps(t, service.OpenAIWSIngressModeCtxPool, 250*time.Millisecond, 0, bindings, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completeOpenAIWSAccountWaitTurn(t, ctx, first)
	require.NotZero(t, bindings.count(), "无显式会话标识的首帧按内容回退哈希写入了共享绑定")
	require.NoError(t, first.conn.Close(coderws.StatusNormalClosure, ""))
	select {
	case <-first.finished:
	case <-ctx.Done():
		t.Fatal("first handler did not exit")
	}

	ok, err := first.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
	require.NoError(t, err)
	require.True(t, ok)

	second := newOpenAIWSAccountWaitSessionWithDeps(t, service.OpenAIWSIngressModeCtxPool, 250*time.Millisecond, 0, bindings, first.cache)
	require.NoError(t, second.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, _, err = second.conn.Read(ctx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), "同内容的独立连接命中共享绑定时不是续聊，有续聊等待者必须让出而不是抢到空槽")
	select {
	case <-second.requests:
		t.Fatal("request from a content-hash binding jumped the continuation queue")
	default:
	}
}

func completeOpenAIWSTurnKeepingSlot(t *testing.T, ctx context.Context, f *openAIWSAccountWaitSession, input string) {
	t.Helper()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"`+input+`"}`)))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
}

func TestOpenAIWSAccountWait_ReleaseContinuesSameConnection(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge, service.OpenAIWSIngressModePassthrough} {
		for _, later := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/later=%v", mode, later), func(t *testing.T) {
				f := newOpenAIWSAccountWaitSession(t, mode, 2*time.Second)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if later {
					completeOpenAIWSAccountWaitTurn(t, ctx, f)
				}
				// 续聊轮（later=true）固定走续聊类，按 wait:account:cont 计数；首轮仍走旧键。
				getWaitingCount := f.cache.GetAccountWaitingCount
				if later {
					getWaitingCount = f.cache.GetAccountContinuationWaitingCount
				}
				acquired, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other-request")
				require.NoError(t, err)
				require.True(t, acquired)
				require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"waiting"}`)))
				require.Eventually(t, func() bool { n, _ := getWaitingCount(ctx, 801); return n == 1 }, time.Second, time.Millisecond, "request should wait without closing")
				select {
				case <-f.requests:
					t.Fatal("upstream called before account admission")
				default:
				}
				require.NoError(t, f.cache.ReleaseAccountSlot(ctx, 801, "other-request"))
				_, payload, err := f.conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(payload, "type").String())
				select {
				case request := <-f.requests:
					require.Contains(t, string(request), "waiting")
				case <-ctx.Done():
					t.Fatal("upstream request missing")
				}
				require.Eventually(t, func() bool { n, _ := getWaitingCount(ctx, 801); return n == 0 }, time.Second, time.Millisecond)
			})
		}
	}
}

func TestOpenAIWSAccountWait_ContinuationQueueIgnoresLegacyWaiters(t *testing.T) {
	f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completeOpenAIWSAccountWaitTurn(t, ctx, f)

	ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other")
	require.NoError(t, err)
	require.True(t, ok)
	for range 2 {
		ok, err = f.cache.IncrementAccountWaitCount(ctx, 801, 2)
		require.NoError(t, err)
		require.True(t, ok)
	}

	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	require.Eventually(t, func() bool { n, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801); return n == 1 }, time.Second, time.Millisecond, "后续轮按续聊类入队，不被旧键上的 2 个等待者挡住")
	select {
	case <-f.requests:
		t.Fatal("等待期间不得有上游请求")
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, f.cache.ReleaseAccountSlot(ctx, 801, "other"))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
	require.Eventually(t, func() bool { n, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801); return n == 0 }, time.Second, time.Millisecond)
}

func TestOpenAIWSAccountWait_NewSessionYieldsToContinuationWaiters(t *testing.T) {
	f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	require.Eventually(t, func() bool { n, _ := f.cache.GetAccountWaitingCount(ctx, 801); return n == 1 }, time.Second, time.Millisecond, "首帧无会话标识的连接是新会话类，进旧键")

	require.NoError(t, f.cache.ReleaseAccountSlot(ctx, 801, "other"))
	select {
	case <-f.requests:
		t.Fatal("有续聊等待者时新会话不得抢到空出的槽并转发上游")
	case <-time.After(300 * time.Millisecond):
	}
	n, err := f.cache.GetAccountConcurrency(ctx, 801)
	require.NoError(t, err)
	require.Equal(t, 0, n, "有续聊等待者时新会话不得拿走空出的槽")

	require.NoError(t, f.cache.DecrementAccountContinuationWaitCount(ctx, 801))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
}

func TestOpenAIWSAccountWait_NewSessionYieldsAtEntryWhenSlotFree(t *testing.T) {
	f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	ok, err := f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	time.Sleep(400 * time.Millisecond)
	n, err := f.cache.GetAccountConcurrency(ctx, 801)
	require.NoError(t, err)
	require.Equal(t, 0, n, "账号有空槽但有续聊等待者，新会话在入口也不得快抢")
	require.Eventually(t, func() bool { c, _ := f.cache.GetAccountWaitingCount(ctx, 801); return c == 1 }, time.Second, time.Millisecond, "进旧键排队")

	require.NoError(t, f.cache.DecrementAccountContinuationWaitCount(ctx, 801))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
}
