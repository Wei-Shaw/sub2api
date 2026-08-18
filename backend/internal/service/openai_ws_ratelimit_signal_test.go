package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type openAIWSRateLimitSignalRepo struct {
	stubOpenAIAccountRepo
	rateLimitCalls []time.Time
	updateExtra    []map[string]any
}

type openAICodexSnapshotAsyncRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
}

type orderedOpenAICodexSnapshotRepo struct {
	stubOpenAIAccountRepo
	firstStarted chan struct{}
	releaseFirst chan struct{}
	mu           sync.Mutex
	calls        int
	used5hWrites []float64
}

type blockingCodexQuotaBody struct {
	payload     []byte
	readStarted chan struct{}
	release     <-chan struct{}
	offset      int
	started     bool
}

func (b *blockingCodexQuotaBody) Read(dst []byte) (int, error) {
	if !b.started {
		b.started = true
		close(b.readStarted)
		<-b.release
	}
	if b.offset >= len(b.payload) {
		return 0, io.EOF
	}
	n := copy(dst, b.payload[b.offset:])
	b.offset += n
	return n, nil
}

func (b *blockingCodexQuotaBody) Close() error { return nil }

type openAICodexExtraListRepo struct {
	stubOpenAIAccountRepo
	rateLimitCh chan time.Time
}

func (r *openAIWSRateLimitSignalRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls = append(r.rateLimitCalls, resetAt)
	return nil
}

func (r *openAIWSRateLimitSignalRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
	}
	r.updateExtra = append(r.updateExtra, copied)
	return nil
}

func (r *openAICodexSnapshotAsyncRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func (r *openAICodexSnapshotAsyncRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *orderedOpenAICodexSnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.firstStarted)
		<-r.releaseFirst
	}
	used5h, _ := updates["codex_5h_used_percent"].(float64)
	r.mu.Lock()
	r.used5hWrites = append(r.used5hWrites, used5h)
	r.mu.Unlock()
	return nil
}

func (r *openAICodexExtraListRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func (r *openAICodexExtraListRepo) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	_ = platform
	_ = accountType
	_ = status
	_ = search
	_ = groupID
	_ = privacyMode
	return r.accounts, &pagination.PaginationResult{Total: int64(len(r.accounts)), Page: params.Page, PageSize: params.PageSize}, nil
}

func TestOpenAIGatewayService_Forward_WSv2ErrorEventUsageLimitPersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resetAt := time.Now().Add(2 * time.Hour).Unix()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":      "rate_limit_exceeded",
				"type":      "usage_limit_reached",
				"message":   "The usage limit has been reached",
				"resets_at": resetAt,
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_should_not_run"}`)),
		},
	}

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	account := Account{
		ID:          501,
		Name:        "openai-ws-rate-limit-event",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	rateSvc := &RateLimitService{accountRepo: repo}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, &account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Nil(t, upstream.lastReq, "WS 限流 error event 不应回退到同账号 HTTP")
	require.Len(t, repo.rateLimitCalls, 1)
	require.WithinDuration(t, time.Unix(resetAt, 0), repo.rateLimitCalls[0], 2*time.Second)
}

func TestOpenAIGatewayService_Forward_WSv2Handshake429PersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-codex-primary-used-percent", "100")
		w.Header().Set("x-codex-primary-reset-after-seconds", "7200")
		w.Header().Set("x-codex-primary-window-minutes", "10080")
		w.Header().Set("x-codex-secondary-used-percent", "3")
		w.Header().Set("x-codex-secondary-reset-after-seconds", "1800")
		w.Header().Set("x-codex-secondary-window-minutes", "300")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_exceeded","message":"rate limited"}}`))
	}))
	defer server.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_should_not_run"}`)),
		},
	}

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	account := Account{
		ID:          502,
		Name:        "openai-ws-rate-limit-handshake",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	rateSvc := &RateLimitService{accountRepo: repo}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, &account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Nil(t, upstream.lastReq, "WS 握手 429 不应回退到同账号 HTTP")
	require.Len(t, repo.rateLimitCalls, 1)
	require.NotEmpty(t, repo.updateExtra, "握手 429 的 x-codex 头应立即落库")
	require.Contains(t, repo.updateExtra[0], "codex_usage_updated_at")
}

func TestOpenAIGatewayService_Forward_WSv2Handshake502RecordsModelTransient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req-ws-502")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"bad gateway"}}`))
	}))
	defer server.Close()

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	account := Account{
		ID:          504,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": server.URL},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		rateLimitService: NewRateLimitService(transientCooldownAccountRepo{}, nil, cfg, nil, nil),
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`)

	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
		result, err := svc.Forward(context.Background(), c, &account, body)
		require.Error(t, err)
		require.Nil(t, result)
	}

	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(&account, "gpt-5.5"))
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_ErrorEventUsageLimitPersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	resetAt := time.Now().Add(90 * time.Minute).Unix()
	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":PLACEHOLDER}}`),
		},
	}
	captureConn.events[0] = []byte(strings.ReplaceAll(string(captureConn.events[0]), "PLACEHOLDER", strconv.FormatInt(resetAt, 10)))
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	account := Account{
		ID:          503,
		Name:        "openai-ingress-rate-limit",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	rateSvc := &RateLimitService{accountRepo: repo}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- io.ErrUnexpectedEOF
			return
		}

		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, &account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	select {
	case serverErr := <-serverErrCh:
		require.Error(t, serverErr)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, serverErr, &failoverErr)
		require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
		require.Len(t, repo.rateLimitCalls, 1)
		require.WithinDuration(t, time.Unix(resetAt, 0), repo.rateLimitCalls[0], 2*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}
}

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_ExhaustedSnapshotDoesNotSetRateLimit(t *testing.T) {
	repo := &openAICodexSnapshotAsyncRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &OpenAIGatewayService{accountRepo: repo}
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         ptrFloat64WS(100),
		PrimaryResetAfterSeconds:   ptrIntWS(3600),
		PrimaryWindowMinutes:       ptrIntWS(10080),
		SecondaryUsedPercent:       ptrFloat64WS(12),
		SecondaryResetAfterSeconds: ptrIntWS(1200),
		SecondaryWindowMinutes:     ptrIntWS(300),
	}
	svc.updateCodexUsageSnapshot(context.Background(), &Account{ID: 601}, snapshot)

	select {
	case updates := <-repo.updateExtraCh:
		require.Equal(t, 100.0, updates["codex_7d_used_percent"])
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 快照落库超时")
	}

	select {
	case resetAt := <-repo.rateLimitCh:
		t.Fatalf("不应因仅写入快照而生成运行时限流时间: %v", resetAt)
	case <-time.After(2 * time.Second):
	}
}

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_NonExhaustedSnapshotDoesNotSetRateLimit(t *testing.T) {
	repo := &openAICodexSnapshotAsyncRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &OpenAIGatewayService{accountRepo: repo}
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         ptrFloat64WS(94),
		PrimaryResetAfterSeconds:   ptrIntWS(3600),
		PrimaryWindowMinutes:       ptrIntWS(10080),
		SecondaryUsedPercent:       ptrFloat64WS(22),
		SecondaryResetAfterSeconds: ptrIntWS(1200),
		SecondaryWindowMinutes:     ptrIntWS(300),
	}
	svc.updateCodexUsageSnapshot(context.Background(), &Account{ID: 602}, snapshot)

	select {
	case <-repo.updateExtraCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 快照落库超时")
	}

	select {
	case resetAt := <-repo.rateLimitCh:
		t.Fatalf("不应写入运行时限流时间: %v", resetAt)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_ThrottlesExtraWrites(t *testing.T) {
	repo := &openAICodexSnapshotAsyncRepo{
		updateExtraCh: make(chan map[string]any, 2),
	}
	svc := &OpenAIGatewayService{
		accountRepo:           repo,
		codexSnapshotThrottle: newAccountWriteThrottle(time.Hour),
	}
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         ptrFloat64WS(94),
		PrimaryResetAfterSeconds:   ptrIntWS(3600),
		PrimaryWindowMinutes:       ptrIntWS(10080),
		SecondaryUsedPercent:       ptrFloat64WS(22),
		SecondaryResetAfterSeconds: ptrIntWS(1200),
		SecondaryWindowMinutes:     ptrIntWS(300),
	}

	account := &Account{ID: 777}
	svc.updateCodexUsageSnapshot(context.Background(), account, snapshot)
	svc.updateCodexUsageSnapshot(context.Background(), account, snapshot)

	select {
	case <-repo.updateExtraCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待第一次 codex 快照落库超时")
	}

	select {
	case updates := <-repo.updateExtraCh:
		t.Fatalf("unexpected second codex snapshot write: %v", updates)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_ThresholdCrossingBypassesThrottle(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		extra     map[string]any
		accountID int64
	}{
		{
			name:      "account threshold",
			ctx:       context.Background(),
			extra:     map[string]any{"auto_pause_5h_threshold": 0.95},
			accountID: 778,
		},
		{
			name:      "global threshold",
			ctx:       withOpenAIQuotaAutoPauseSettings(context.Background(), OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.95}),
			accountID: 779,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &openAICodexSnapshotAsyncRepo{updateExtraCh: make(chan map[string]any, 2)}
			svc := &OpenAIGatewayService{
				accountRepo:           repo,
				codexSnapshotThrottle: newAccountWriteThrottle(time.Hour),
			}
			account := &Account{
				ID:       tt.accountID,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    tt.extra,
			}
			snapshot := func(used5h float64) *OpenAICodexUsageSnapshot {
				return &OpenAICodexUsageSnapshot{
					PrimaryUsedPercent:         ptrFloat64WS(used5h),
					PrimaryResetAfterSeconds:   ptrIntWS(3600),
					PrimaryWindowMinutes:       ptrIntWS(300),
					SecondaryUsedPercent:       ptrFloat64WS(20),
					SecondaryResetAfterSeconds: ptrIntWS(86400),
					SecondaryWindowMinutes:     ptrIntWS(10080),
				}
			}

			svc.updateCodexUsageSnapshot(tt.ctx, account, snapshot(94))
			select {
			case updates := <-repo.updateExtraCh:
				require.Equal(t, 94.0, updates["codex_5h_used_percent"])
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the pre-threshold snapshot")
			}

			// The 95% observation is inside the ordinary one-hour throttle window,
			// but it must be persisted synchronously because it pauses scheduling.
			svc.updateCodexUsageSnapshot(tt.ctx, account, snapshot(95))
			select {
			case updates := <-repo.updateExtraCh:
				require.Equal(t, 95.0, updates["codex_5h_used_percent"])
			default:
				t.Fatal("threshold-crossing snapshot was not persisted synchronously")
			}

			// Already in-flight responses above the threshold are deduplicated.
			svc.updateCodexUsageSnapshot(tt.ctx, account, snapshot(96))
			select {
			case updates := <-repo.updateExtraCh:
				t.Fatalf("unexpected duplicate critical snapshot write: %v", updates)
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_ThresholdWriteCannotBeOverwrittenByOlderAsyncWrite(t *testing.T) {
	repo := &orderedOpenAICodexSnapshotRepo{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	svc := &OpenAIGatewayService{
		accountRepo:           repo,
		codexSnapshotThrottle: newAccountWriteThrottle(time.Hour),
	}
	account := &Account{
		ID:       780,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"auto_pause_5h_threshold": 0.95},
	}
	snapshot := func(used5h float64) *OpenAICodexUsageSnapshot {
		return &OpenAICodexUsageSnapshot{
			PrimaryUsedPercent:         ptrFloat64WS(used5h),
			PrimaryResetAfterSeconds:   ptrIntWS(3600),
			PrimaryWindowMinutes:       ptrIntWS(300),
			SecondaryUsedPercent:       ptrFloat64WS(20),
			SecondaryResetAfterSeconds: ptrIntWS(86400),
			SecondaryWindowMinutes:     ptrIntWS(10080),
		}
	}

	svc.updateCodexUsageSnapshot(context.Background(), account, snapshot(94))
	select {
	case <-repo.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("older async snapshot did not start")
	}

	criticalDone := make(chan struct{})
	go func() {
		svc.updateCodexUsageSnapshot(context.Background(), account, snapshot(95))
		close(criticalDone)
	}()
	select {
	case <-criticalDone:
		t.Fatal("critical write returned before the older snapshot finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(repo.releaseFirst)
	select {
	case <-criticalDone:
	case <-time.After(2 * time.Second):
		t.Fatal("critical snapshot write did not finish")
	}

	repo.mu.Lock()
	writes := append([]float64(nil), repo.used5hWrites...)
	repo.mu.Unlock()
	require.Equal(t, []float64{94, 95}, writes)
}

func TestAccountWriteLockMap_ThrottleDecisionCannotBeOvertaken(t *testing.T) {
	var locks accountWriteLockMap
	firstDecisionStarted := make(chan struct{})
	releaseFirstDecision := make(chan struct{})
	firstAcquired := make(chan struct{})
	releaseFirstWrite := make(chan struct{})
	secondDecisionStarted := make(chan struct{})
	secondAcquired := make(chan struct{})

	go func() {
		unlock, allowed := locks.LockIfAllowed(1, func() bool {
			close(firstDecisionStarted)
			<-releaseFirstDecision
			return true
		})
		if !allowed {
			return
		}
		close(firstAcquired)
		<-releaseFirstWrite
		unlock()
	}()
	<-firstDecisionStarted

	go func() {
		unlock, allowed := locks.LockIfAllowed(1, func() bool {
			close(secondDecisionStarted)
			return true
		})
		if !allowed {
			return
		}
		close(secondAcquired)
		unlock()
	}()

	select {
	case <-secondDecisionStarted:
		t.Fatal("newer decision overtook the older decision before it claimed write order")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstDecision)
	select {
	case <-firstAcquired:
	case <-time.After(2 * time.Second):
		t.Fatal("older write did not acquire its ordered lock")
	}
	select {
	case <-secondDecisionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("newer throttle decision did not run after the older write was ordered")
	}
	select {
	case <-secondAcquired:
		t.Fatal("newer write acquired before the older write completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstWrite)
	select {
	case <-secondAcquired:
	case <-time.After(2 * time.Second):
		t.Fatal("newer write did not acquire after the older write completed")
	}
}

func TestAccountWriteLockMap_DeniedWriteDoesNotWaitForActiveWrite(t *testing.T) {
	var locks accountWriteLockMap
	unlockFirst, allowed := locks.LockIfAllowed(1, func() bool { return true })
	require.True(t, allowed)
	defer unlockFirst()

	type deniedResult struct {
		unlock  func()
		allowed bool
	}
	deniedDone := make(chan deniedResult, 1)
	go func() {
		unlock, secondAllowed := locks.LockIfAllowed(1, func() bool { return false })
		deniedDone <- deniedResult{unlock: unlock, allowed: secondAllowed}
	}()

	select {
	case result := <-deniedDone:
		require.False(t, result.allowed)
		require.Nil(t, result.unlock)
	case <-time.After(2 * time.Second):
		t.Fatal("throttled write waited for an unrelated active database write")
	}
}

func TestOpenAIGatewayService_ForwardPersistsThresholdSnapshotBeforeStreamingBodyCompletes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestBody := []byte(`{"model":"gpt-5.4","stream":true,"input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	releaseBody := make(chan struct{})
	upstreamBody := &blockingCodexQuotaBody{
		payload:     []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_quota\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"),
		readStarted: make(chan struct{}),
		release:     releaseBody,
	}
	headers := http.Header{"Content-Type": []string{"text/event-stream"}}
	headers.Set("x-codex-primary-used-percent", "95")
	headers.Set("x-codex-primary-reset-after-seconds", "3600")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-used-percent", "20")
	headers.Set("x-codex-secondary-reset-after-seconds", "86400")
	headers.Set("x-codex-secondary-window-minutes", "10080")

	repo := &openAICodexSnapshotAsyncRepo{updateExtraCh: make(chan map[string]any, 1)}
	svc := &OpenAIGatewayService{
		cfg:                   &config.Config{},
		accountRepo:           repo,
		httpUpstream:          &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: headers, Body: upstreamBody}},
		codexSnapshotThrottle: newAccountWriteThrottle(time.Hour),
	}
	account := &Account{
		ID:          781,
		Name:        "quota-stream-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Extra: map[string]any{
			"openai_passthrough":      true,
			"auto_pause_5h_threshold": 0.95,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	resultCh := make(chan *OpenAIForwardResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.Forward(context.Background(), c, account, requestBody)
		resultCh <- result
		errCh <- err
	}()

	select {
	case <-upstreamBody.readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming body read did not start")
	}
	select {
	case updates := <-repo.updateExtraCh:
		require.Equal(t, 95.0, updates["codex_5h_used_percent"])
	default:
		t.Fatal("quota threshold snapshot was not persisted before streaming body read")
	}
	select {
	case <-resultCh:
		t.Fatal("forward returned before the blocked streaming body was released")
	default:
	}

	close(releaseBody)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("forward did not finish after releasing the streaming body")
	}
	select {
	case result := <-resultCh:
		require.NotNil(t, result)
	case <-time.After(2 * time.Second):
		t.Fatal("forward result was not returned")
	}
}

func ptrFloat64WS(v float64) *float64 { return &v }
func ptrIntWS(v int) *int             { return &v }

func TestOpenAIGatewayService_GetSchedulableAccount_ExhaustedCodexExtraDoesNotSetRateLimit(t *testing.T) {
	resetAt := time.Now().Add(6 * 24 * time.Hour)
	account := Account{
		ID:          701,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.UTC().Format(time.RFC3339),
		},
	}
	repo := &openAICodexExtraListRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}, rateLimitCh: make(chan time.Time, 1)}
	svc := &OpenAIGatewayService{accountRepo: repo}

	fresh, err := svc.getSchedulableAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, fresh)
	require.Nil(t, fresh.RateLimitResetAt)
	select {
	case persisted := <-repo.rateLimitCh:
		t.Fatalf("不应将已耗尽的 codex extra 提升为运行时限流状态: %v", persisted)
	case <-time.After(2 * time.Second):
	}
}

func TestAdminService_ListAccounts_ExhaustedCodexExtraDoesNotSetRateLimit(t *testing.T) {
	resetAt := time.Now().Add(4 * 24 * time.Hour)
	repo := &openAICodexExtraListRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          702,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Extra: map[string]any{
				"codex_7d_used_percent": 100.0,
				"codex_7d_reset_at":     resetAt.UTC().Format(time.RFC3339),
			},
		}}},
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &adminServiceImpl{accountRepo: repo}

	accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformOpenAI, AccountTypeOAuth, "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, accounts, 1)
	require.Nil(t, accounts[0].RateLimitResetAt)
	select {
	case persisted := <-repo.rateLimitCh:
		t.Fatalf("不应在账号列表查询时将 codex extra 持久化为运行时限流状态: %v", persisted)
	case <-time.After(2 * time.Second):
	}
}

func TestOpenAIWSErrorHTTPStatusFromRaw_UsageLimitReachedIs429(t *testing.T) {
	require.Equal(t, http.StatusTooManyRequests, openAIWSErrorHTTPStatusFromRaw("", "usage_limit_reached"))
	require.Equal(t, http.StatusTooManyRequests, openAIWSErrorHTTPStatusFromRaw("rate_limit_exceeded", ""))
}
