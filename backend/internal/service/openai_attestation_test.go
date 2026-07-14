package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testOpenAIAttestationValue = `{"v":1,"s":0,"t":"v1.test-opaque+/=_-"}`

func newOpenAIAttestationTestContext(values ...string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if values != nil {
		c.Request.Header[openAIAttestationHeader] = append([]string(nil), values...)
	}
	return c
}

func requireNoOpenAIAttestationHeader(t *testing.T, headers http.Header) {
	t.Helper()
	for key := range headers {
		require.False(t, strings.EqualFold(key, openAIAttestationHeader), "不应残留证明头：%s", key)
	}
}

func TestCopyOpenAIAttestationHeaderScopeAndCardinality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	t.Run("OpenAI OAuth 保持原值", func(t *testing.T) {
		src := http.Header{"x-oai-attestation": []string{testOpenAIAttestationValue}}
		dst := http.Header{openAIAttestationHeader: []string{"stale"}}

		require.True(t, copyOpenAIAttestationHeader(dst, src, oauthAccount))
		require.Equal(t, []string{testOpenAIAttestationValue}, dst.Values(openAIAttestationHeader))
	})

	tests := []struct {
		name    string
		src     http.Header
		account *Account
	}{
		{name: "缺失时省略", src: http.Header{}, account: oauthAccount},
		{name: "空值时省略", src: http.Header{openAIAttestationHeader: []string{""}}, account: oauthAccount},
		{name: "多值时省略", src: http.Header{openAIAttestationHeader: []string{"first", "second"}}, account: oauthAccount},
		{name: "API Key 不透传", src: http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}}, account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}},
		{name: "非 OpenAI OAuth 不透传", src: http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}}, account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}},
		{name: "空账号不透传", src: http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}}, account: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := http.Header{openAIAttestationHeader: []string{"stale"}}
			require.False(t, copyOpenAIAttestationHeader(dst, tt.src, tt.account))
			requireNoOpenAIAttestationHeader(t, dst)
		})
	}
}

func TestOpenAIAttestationResponsesPathScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		path string
		want bool
	}{
		{path: "/v1/responses", want: true},
		{path: "/openai/v1/responses/", want: true},
		{path: "/v1/responses/compact", want: true},
		{path: "/openai/v1/responses/compact/detail", want: false},
		{path: "/v1/chat/completions", want: false},
		{path: "/v1/messages", want: false},
		{path: "/v1/images/generations", want: false},
		{path: "/v1/responses/resp_123", want: false},
		{path: "/v1/responses-compat", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			c := newOpenAIAttestationTestContext()
			c.Request.URL.Path = tt.path
			require.Equal(t, tt.want, isOpenAIAttestationResponsesPath(c))
		})
	}
}

func TestOpenAIAttestationHTTPBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg}
	body := []byte(`{"model":"gpt-5"}`)

	builders := []struct {
		name  string
		build func(*gin.Context, *Account) (*http.Request, error)
	}{
		{
			name: "普通 HTTP",
			build: func(c *gin.Context, account *Account) (*http.Request, error) {
				return svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", false, "", true)
			},
		},
		{
			name: "passthrough",
			build: func(c *gin.Context, account *Account) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, body, "token")
			},
		},
	}

	for _, builder := range builders {
		t.Run(builder.name+"/OpenAI OAuth 原样透传", func(t *testing.T) {
			c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
			account := &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"chatgpt_account_id": "test-account"},
			}

			req, err := builder.build(c, account)
			require.NoError(t, err)
			require.Equal(t, []string{testOpenAIAttestationValue}, req.Header.Values(openAIAttestationHeader))
			require.True(t, IsOpenAIAttestationForwarded(c))
		})

		t.Run(builder.name+"/API Key 隔离", func(t *testing.T) {
			c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
			account := &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": "https://example.com/v1"},
			}

			req, err := builder.build(c, account)
			require.NoError(t, err)
			requireNoOpenAIAttestationHeader(t, req.Header)
			require.False(t, IsOpenAIAttestationForwarded(c))
		})

		t.Run(builder.name+"/缺失时不新增", func(t *testing.T) {
			c := newOpenAIAttestationTestContext()
			account := &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"chatgpt_account_id": "test-account"},
			}

			req, err := builder.build(c, account)
			require.NoError(t, err)
			requireNoOpenAIAttestationHeader(t, req.Header)
			require.False(t, IsOpenAIAttestationForwarded(c))
		})
	}
}

func TestOpenAIAttestationWebSocketHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}

	t.Run("OpenAI OAuth 握手原样透传", func(t *testing.T) {
		c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{"chatgpt_account_id": "test-account"},
		}

		headers, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "token", decision, true, "", "", "")
		require.NoError(t, err)
		require.Equal(t, []string{testOpenAIAttestationValue}, headers.Values(openAIAttestationHeader))
		require.True(t, IsOpenAIAttestationForwarded(c))
	})

	t.Run("API Key 握手隔离", func(t *testing.T) {
		c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://example.com/v1"},
		}

		headers, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "token", decision, true, "", "", "")
		require.NoError(t, err)
		requireNoOpenAIAttestationHeader(t, headers)
		require.False(t, IsOpenAIAttestationForwarded(c))
	})
}

func TestOpenAIAttestationWebSocketRedirectIsBlocked(t *testing.T) {
	var targetHits atomic.Int32
	targetHeaders := make(chan http.Header, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		targetHeaders <- r.Header.Clone()
		http.Error(w, "not a websocket", http.StatusBadRequest)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	dialer := newDefaultOpenAIWSClientDialer()
	wsURL := "ws" + strings.TrimPrefix(redirect.URL, "http") + "/start"

	attestedHeaders := http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	conn, _, _, err := dialer.Dial(ctx, wsURL, attestedHeaders, "")
	cancel()
	require.Nil(t, conn)
	require.ErrorIs(t, err, ErrOpenAIAttestationRedirect)
	require.Zero(t, targetHits.Load(), "携带证明的握手不应访问重定向目标")

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	conn, _, _, err = dialer.Dial(ctx, wsURL, http.Header{}, "")
	cancel()
	require.Nil(t, conn)
	require.Error(t, err)
	require.Equal(t, int32(1), targetHits.Load(), "普通握手应保持既有重定向行为")
	select {
	case headers := <-targetHeaders:
		requireNoOpenAIAttestationHeader(t, headers)
	default:
		t.Fatal("普通握手未到达重定向目标")
	}
}

func TestOpenAIAttestationDoesNotLeakToImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg}
	c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://example.com/v1",
		},
	}

	req, err := svc.buildOpenAIImagesRequest(
		context.Background(),
		c,
		account,
		[]byte(`{"model":"gpt-image-1","prompt":"test"}`),
		"application/json",
		"token",
		openAIImagesGenerationsEndpoint,
	)
	require.NoError(t, err)
	requireNoOpenAIAttestationHeader(t, req.Header)
}

func TestOpenAIAttestationDoesNotLeakThroughCompatibilityPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}
	body := []byte(`{"model":"gpt-5"}`)
	paths := []string{
		"/v1/chat/completions",
		"/v1/messages",
		"/v1/images/generations",
	}

	for _, path := range paths {
		for _, passthrough := range []bool{false, true} {
			name := "普通"
			if passthrough {
				name = "passthrough"
			}
			t.Run(path+"/"+name, func(t *testing.T) {
				c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
				c.Request.URL.Path = path
				var req *http.Request
				var err error
				if passthrough {
					req, err = svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, body, "token")
				} else {
					req, err = svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", false, "", true)
				}
				require.NoError(t, err)
				requireNoOpenAIAttestationHeader(t, req.Header)
				require.False(t, IsOpenAIAttestationForwarded(c))
			})
		}
	}

	for _, passthrough := range []bool{false, true} {
		name := "普通"
		if passthrough {
			name = "passthrough"
		}
		t.Run("/v1/responses/消息兼容桥/"+name, func(t *testing.T) {
			c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
			setOpenAICompatMessagesBridgeContext(c, true)
			var req *http.Request
			var err error
			if passthrough {
				req, err = svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, body, "token")
			} else {
				req, err = svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", false, "", true)
			}
			require.NoError(t, err)
			requireNoOpenAIAttestationHeader(t, req.Header)
			require.False(t, IsOpenAIAttestationForwarded(c))
		})
	}
}

func TestOpenAIAttestationPassthroughBuilderAllowsNilGinContext(t *testing.T) {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(
		context.Background(),
		nil,
		account,
		[]byte(`{"model":"gpt-5"}`),
		"token",
	)
	require.NoError(t, err)
	requireNoOpenAIAttestationHeader(t, req.Header)
}

func TestOpenAIAttestationWebSocketEphemeralConnections(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIAttestationCaptureDialer{}
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)

	account := &Account{ID: 701, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	values := []string{
		testOpenAIAttestationValue,
		`{"v":1,"s":0,"t":"v1.second-client"}`,
	}
	for _, value := range values {
		requestHeaders := http.Header{}
		requestHeaders.Set(openAIAttestationHeader, value)
		lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
			Account:   account,
			WSURL:     "wss://example.com/v1/responses",
			Headers:   requestHeaders,
			Ephemeral: true,
		})
		require.NoError(t, err)
		require.False(t, lease.Reused())
		inflight, _, conns := pool.AccountPoolLoad(account.ID)
		require.Equal(t, 1, inflight)
		require.Equal(t, 1, conns)

		lease.Release()
		inflight, _, conns = pool.AccountPoolLoad(account.ID)
		require.Equal(t, 0, inflight)
		require.Equal(t, 0, conns)
	}

	lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
		Headers: http.Header{},
	})
	require.NoError(t, err)
	lease.Release()

	headers, connections := dialer.snapshot()
	require.Len(t, headers, 3)
	require.Equal(t, []string{values[0]}, headers[0].Values(openAIAttestationHeader))
	require.Equal(t, []string{values[1]}, headers[1].Values(openAIAttestationHeader))
	require.Empty(t, headers[2].Values(openAIAttestationHeader))
	require.True(t, connections[0].isClosed())
	require.True(t, connections[1].isClosed())
	require.False(t, connections[2].isClosed())

	ap, ok := pool.getAccountPool(account.ID)
	require.True(t, ok)
	ap.mu.Lock()
	require.NotNil(t, ap.lastAcquire)
	require.Empty(t, ap.lastAcquire.Headers.Values(openAIAttestationHeader))
	ap.mu.Unlock()
}

func TestOpenAIAttestationEphemeralFailureRestoresSharedPool(t *testing.T) {
	tests := []struct {
		name           string
		waitForContext bool
		dialErr        error
	}{
		{name: "拨号失败", dialErr: errors.New("attestation dial failed")},
		{name: "请求取消", waitForContext: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.PrewarmCooldownMS = 300

			sharedConn := &openAIAttestationFakeConn{}
			restoredConn := &openAIAttestationFakeConn{}
			dialer := &openAIAttestationScriptedDialer{results: []openAIAttestationDialResult{
				{conn: sharedConn},
				{err: tt.dialErr, waitForContext: tt.waitForContext},
				{conn: restoredConn},
			}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			t.Cleanup(pool.Close)

			account := &Account{ID: 710, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			sharedLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
				Account: account,
				WSURL:   "wss://example.com/v1/responses",
				Headers: http.Header{},
			})
			require.NoError(t, err)
			sharedLease.Release()

			ap, ok := pool.getAccountPool(account.ID)
			require.True(t, ok)
			ap.mu.Lock()
			ap.prewarmUntil = time.Now().Add(time.Hour)
			ap.mu.Unlock()

			acquireCtx := context.Background()
			if tt.waitForContext {
				canceledCtx, cancel := context.WithCancel(context.Background())
				cancel()
				acquireCtx = canceledCtx
			}
			_, err = pool.Acquire(acquireCtx, openAIWSAcquireRequest{
				Account:   account,
				WSURL:     "wss://example.com/v1/responses",
				Headers:   http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}},
				Ephemeral: true,
			})
			require.Error(t, err)

			require.Eventually(t, func() bool {
				_, _, conns := pool.AccountPoolLoad(account.ID)
				return conns == 1 && dialer.DialCount() == 3
			}, 2*time.Second, 10*time.Millisecond)
			require.True(t, sharedConn.isClosed(), "让出容量的旧共享连接应关闭")
			require.False(t, restoredConn.isClosed(), "恢复后的共享连接应保持空闲")

			ap.mu.Lock()
			require.Zero(t, ap.prewarmFails, "临时拨号失败不应污染共享预热熔断")
			require.NotNil(t, ap.lastAcquire)
			requireNoOpenAIAttestationHeader(t, ap.lastAcquire.Headers)
			ap.mu.Unlock()
		})
	}
}

func TestOpenAIAttestationEphemeralConcurrencyAndReleaseAreBounded(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	started := make(chan struct{})
	releaseDial := make(chan struct{})
	sharedConn := &openAIAttestationFakeConn{}
	ephemeralConn := &openAIAttestationFakeConn{}
	restoredConn := &openAIAttestationFakeConn{}
	dialer := &openAIAttestationScriptedDialer{results: []openAIAttestationDialResult{
		{conn: sharedConn},
		{conn: ephemeralConn, started: started, release: releaseDial},
		{conn: restoredConn},
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)

	account := &Account{ID: 711, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	sharedLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
	})
	require.NoError(t, err)
	sharedLease.Release()

	type acquireResult struct {
		lease *openAIWSConnLease
		err   error
	}
	resultCh := make(chan acquireResult, 1)
	go func() {
		lease, acquireErr := pool.Acquire(context.Background(), openAIWSAcquireRequest{
			Account:   account,
			WSURL:     "wss://example.com/v1/responses",
			Headers:   http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}},
			Ephemeral: true,
		})
		resultCh <- acquireResult{lease: lease, err: acquireErr}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("等待临时连接拨号开始超时")
	}
	_, err = pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:   account,
		WSURL:     "wss://example.com/v1/responses",
		Headers:   http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}},
		Ephemeral: true,
	})
	require.ErrorIs(t, err, errOpenAIWSConnQueueFull)
	require.Equal(t, 2, dialer.DialCount(), "超过账号上限时不应继续拨号")

	close(releaseDial)
	var first acquireResult
	select {
	case first = <-resultCh:
		require.NoError(t, first.err)
		require.NotNil(t, first.lease)
	case <-time.After(2 * time.Second):
		t.Fatal("等待临时连接获取完成超时")
	}
	first.lease.MarkBroken()
	first.lease.Release()
	first.lease.Release()

	require.Eventually(t, func() bool {
		_, _, conns := pool.AccountPoolLoad(account.ID)
		return conns == 1 && dialer.DialCount() == 3
	}, 2*time.Second, 10*time.Millisecond)
	require.True(t, sharedConn.isClosed())
	require.True(t, ephemeralConn.isClosed())
	require.False(t, restoredConn.isClosed())
}

func TestOpenAIAttestationEphemeralReleaseAfterCloseDoesNotPrewarm(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	sharedConn := &openAIAttestationFakeConn{}
	ephemeralConn := &openAIAttestationFakeConn{}
	dialer := &openAIAttestationScriptedDialer{results: []openAIAttestationDialResult{
		{conn: sharedConn},
		{conn: ephemeralConn},
		{conn: &openAIAttestationFakeConn{}},
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 712, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	sharedLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
	})
	require.NoError(t, err)
	sharedLease.Release()

	ephemeralLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:   account,
		WSURL:     "wss://example.com/v1/responses",
		Headers:   http.Header{openAIAttestationHeader: []string{testOpenAIAttestationValue}},
		Ephemeral: true,
	})
	require.NoError(t, err)
	pool.Close()
	ephemeralLease.Release()
	ephemeralLease.Release()

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 2, dialer.DialCount(), "连接池关闭后不应重新预热")
	_, _, conns := pool.AccountPoolLoad(account.ID)
	require.Zero(t, conns)
	require.True(t, sharedConn.isClosed())
	require.True(t, ephemeralConn.isClosed())
}

func TestOpenAIAttestationWebSocketIngressOverridesHTTPBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	upstreamConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_attestation_ws","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	dialer := &openAIWSCaptureDialer{conn: upstreamConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)

	httpUpstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_unexpected_http_bridge\"}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     httpUpstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	stateStore := NewOpenAIWSStateStore(&stubGatewayCache{})
	svc.openaiWSStateStore = stateStore
	account := &Account{
		ID:          702,
		Name:        "oauth-attestation-http-bridge",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "test-account",
		},
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeHTTPBridge,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}

		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(
			r.Context(),
			ginCtx,
			conn,
			account,
			"oauth-token",
			firstMessage,
			nil,
		)
	}))
	defer wsServer.Close()

	clientHeaders := http.Header{"User-Agent": []string{"codex_cli_rs/0.98.0"}}
	clientHeaders[openAIAttestationHeader] = []string{testOpenAIAttestationValue}
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(
		dialCtx,
		"ws"+strings.TrimPrefix(wsServer.URL, "http"),
		&coderws.DialOptions{
			HTTPHeader:      clientHeaders,
			CompressionMode: coderws.CompressionContextTakeover,
		},
	)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(
		writeCtx,
		coderws.MessageText,
		[]byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`),
	)
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	messageType, event, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr)
	require.Equal(t, coderws.MessageText, messageType)
	require.Equal(t, "resp_attestation_ws", gjson.GetBytes(event, "response.id").String())

	inflight, _, activeConns := pool.AccountPoolLoad(account.ID)
	require.Equal(t, 1, inflight)
	require.Equal(t, 1, activeConns)

	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	select {
	case serverErr := <-serverErrCh:
		if serverErr != nil {
			require.Contains(t, serverErr.Error(), "StatusNormalClosure")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待 attestation websocket 会话结束超时")
	}

	dialer.mu.Lock()
	handshakeHeaders := cloneHeader(dialer.lastHeaders)
	dialer.mu.Unlock()
	require.Equal(t, 1, dialer.DialCount())
	require.Equal(t, []string{testOpenAIAttestationValue}, handshakeHeaders.Values(openAIAttestationHeader))
	require.Nil(t, httpUpstream.lastReq, "携带证明时不应进入 HTTP bridge")

	_, _, remainingConns := pool.AccountPoolLoad(account.ID)
	require.Zero(t, remainingConns, "会话结束后不应残留临时连接")
	upstreamConn.mu.Lock()
	writeCount := len(upstreamConn.writes)
	closed := upstreamConn.closed
	upstreamConn.mu.Unlock()
	require.Equal(t, 1, writeCount)
	require.True(t, closed)
	_, bound := stateStore.GetResponseConn("resp_attestation_ws")
	require.False(t, bound, "临时连接不应写入 response 到 conn 的粘性映射")
}

func TestOpenAIAttestationDisablesInternalWebSocketReconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	firstConn := &openAIWSCaptureConn{}
	secondConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"unexpected_retry","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          703,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "test-account",
		},
		Extra: map[string]any{"responses_websockets_v2_enabled": true},
	}
	c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
	c.Request.URL.Path = "/openai/v1/responses"

	result, err := svc.Forward(
		context.Background(),
		c,
		account,
		[]byte(`{"model":"gpt-5.1","stream":false,"input":"test"}`),
	)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, dialer.DialCount(), "新握手需要新证明，不应复用入站证明自动重连")
}

func TestOpenAIAttestationWebSocketV2DoesNotBindEphemeralConn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	upstreamConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_attestation_v2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	dialer := &openAIWSCaptureDialer{conn: upstreamConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)
	stateStore := NewOpenAIWSStateStore(&stubGatewayCache{})
	svc := &OpenAIGatewayService{
		cfg:                cfg,
		httpUpstream:       &httpUpstreamRecorder{},
		cache:              &stubGatewayCache{},
		openaiWSResolver:   NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:      NewCodexToolCorrector(),
		openaiWSPool:       pool,
		openaiWSStateStore: stateStore,
	}
	account := &Account{
		ID:          713,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "test-account",
		},
		Extra: map[string]any{"responses_websockets_v2_enabled": true},
	}
	c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
	c.Request.URL.Path = "/openai/v1/responses"
	body := []byte(`{"model":"gpt-5.1","stream":false,"store":false,"prompt_cache_key":"attestation-sticky-test","input":"test"}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_attestation_v2", result.RequestID)
	_, bound := stateStore.GetResponseConn("resp_attestation_v2")
	require.False(t, bound, "临时连接不应写入 response 到 conn 的粘性映射")
	sessionHash, _ := openAIWSSessionHashesFromID("attestation-sticky-test")
	_, bound = stateStore.GetSessionConn(getOpenAIGroupIDFromContext(c), sessionHash)
	require.False(t, bound, "临时连接不应写入 session 到 conn 的粘性映射")
	_, _, conns := pool.AccountPoolLoad(account.ID)
	require.Zero(t, conns)
	upstreamConn.mu.Lock()
	closed := upstreamConn.closed
	upstreamConn.mu.Unlock()
	require.True(t, closed)
}

func TestOpenAIAttestationDisablesBodyMutatingHTTPRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"bad encrypted content"}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
			},
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{
		cfg:           cfg,
		httpUpstream:  upstream,
		cache:         &stubGatewayCache{},
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          704,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "test-account",
		},
	}
	c := newOpenAIAttestationTestContext(testOpenAIAttestationValue)
	c.Request.URL.Path = "/openai/v1/responses"
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"reasoning","encrypted_content":"gAAA"},{"type":"input_text","text":"hello"}]}`)

	_, _ = svc.Forward(context.Background(), c, account, body)
	require.Len(t, upstream.requests, 1, "修改请求体后的重试需要新证明，应交还客户端重新发起")
}

func TestOpenAIAttestationIsRedacted(t *testing.T) {
	require.Equal(t, "[redacted]", safeHeaderValueForLog(openAIAttestationHeader, testOpenAIAttestationValue))
	require.True(t, isSensitiveKey(openAIAttestationHeader))
}

func TestOpenAIAttestationHeaderOverrideDirtyDataIsIgnored(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides: map[string]any{
				openAIAttestationHeader: testOpenAIAttestationValue,
			},
		},
	}

	require.Empty(t, account.GetHeaderOverrides())
	headers := http.Header{}
	account.ApplyHeaderOverrides(headers)
	requireNoOpenAIAttestationHeader(t, headers)
}

type openAIAttestationCaptureDialer struct {
	mu          sync.Mutex
	headers     []http.Header
	connections []*openAIAttestationFakeConn
}

type openAIAttestationDialResult struct {
	conn           openAIWSClientConn
	err            error
	waitForContext bool
	started        chan struct{}
	release        <-chan struct{}
}

type openAIAttestationScriptedDialer struct {
	mu        sync.Mutex
	results   []openAIAttestationDialResult
	dialCount int
}

func (d *openAIAttestationScriptedDialer) Dial(
	ctx context.Context,
	_ string,
	_ http.Header,
	_ string,
) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	d.dialCount++
	if len(d.results) == 0 {
		d.mu.Unlock()
		return nil, http.StatusServiceUnavailable, nil, errors.New("no scripted dial result")
	}
	result := d.results[0]
	d.results = d.results[1:]
	d.mu.Unlock()

	if result.started != nil {
		close(result.started)
	}
	if result.waitForContext {
		<-ctx.Done()
		return nil, http.StatusServiceUnavailable, nil, ctx.Err()
	}
	if result.release != nil {
		select {
		case <-result.release:
		case <-ctx.Done():
			return nil, http.StatusServiceUnavailable, nil, ctx.Err()
		}
	}
	if result.err != nil {
		return nil, http.StatusServiceUnavailable, nil, result.err
	}
	return result.conn, 0, nil, nil
}

func (d *openAIAttestationScriptedDialer) DialCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dialCount
}

func (d *openAIAttestationCaptureDialer) Dial(
	_ context.Context,
	_ string,
	headers http.Header,
	_ string,
) (openAIWSClientConn, int, http.Header, error) {
	conn := &openAIAttestationFakeConn{}
	d.mu.Lock()
	d.headers = append(d.headers, cloneHeader(headers))
	d.connections = append(d.connections, conn)
	d.mu.Unlock()
	return conn, 0, nil, nil
}

func (d *openAIAttestationCaptureDialer) snapshot() ([]http.Header, []*openAIAttestationFakeConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	headers := make([]http.Header, len(d.headers))
	for i := range d.headers {
		headers[i] = cloneHeader(d.headers[i])
	}
	return headers, append([]*openAIAttestationFakeConn(nil), d.connections...)
}

type openAIAttestationFakeConn struct {
	mu     sync.Mutex
	closed bool
}

func (c *openAIAttestationFakeConn) WriteJSON(context.Context, any) error { return nil }

func (c *openAIAttestationFakeConn) ReadMessage(context.Context) ([]byte, error) { return nil, nil }

func (c *openAIAttestationFakeConn) Ping(context.Context) error { return nil }

func (c *openAIAttestationFakeConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *openAIAttestationFakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
