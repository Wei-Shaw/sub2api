package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const openAIWSHandshakeCyberBody = `{"error":{"type":"invalid_request_error","code":"cyber_policy","message":"blocked by cyber policy"}}`

type openAIWSHandshakeCyberDialer struct{}

func (d *openAIWSHandshakeCyberDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	body := []byte(openAIWSHandshakeCyberBody)
	return nil, http.StatusBadRequest, http.Header{"X-Request-Id": []string{"req_handshake_cyber"}}, &openAIWSHandshakeError{
		Body: body,
		Err:  errors.New("websocket handshake rejected"),
	}
}

type openAIWSHandshakeCyberObservation struct {
	mark    *CyberPolicyMark
	turnErr error
}

func TestOpenAIWSIngressCtxPoolHandshakeCyberMarkVisibleToAfterTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSHandshakeCyberDialer{})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := passthroughLifecycleAccount()
	account.ID = 902
	account.Extra["openai_apikey_responses_websockets_v2_mode"] = OpenAIWSIngressModeCtxPool

	observed := make(chan openAIWSHandshakeCyberObservation, 1)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, account, func(c *gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, turnErr error) {
			observed <- openAIWSHandshakeCyberObservation{mark: GetOpsCyberPolicy(c), turnErr: turnErr}
		}}
	})
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	select {
	case got := <-observed:
		require.Error(t, got.turnErr)
		requireOpenAIWSHandshakeCyberMark(t, got.mark)
	case <-time.After(3 * time.Second):
		t.Fatal("ctx_pool handshake failure did not reach AfterTurn")
	}
	select {
	case err := <-serverErr:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("ctx_pool handshake failure did not exit")
	}
}

func TestForwardOpenAIWSV2HandshakeBodyMarksCyberPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "req_http_ws_cyber")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(openAIWSHandshakeCyberBody))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 903, Name: "http-ws-handshake-cyber", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": server.URL},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`))

	require.Error(t, err)
	require.Nil(t, result)
	requireOpenAIWSHandshakeCyberMark(t, GetOpsCyberPolicy(c))
}

func TestOpenAIWSV2PassthroughHandshakeCyberMarkVisibleToAfterTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	cfg := passthroughLifecycleConfig()
	svc := newPassthroughLifecycleService(cfg, newStagedPassthroughConn())
	svc.openaiWSPassthroughDialer = &openAIWSHandshakeCyberDialer{}
	account := passthroughLifecycleAccount()
	account.ID = 904

	observed := make(chan openAIWSHandshakeCyberObservation, 1)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, account, func(c *gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, turnErr error) {
			observed <- openAIWSHandshakeCyberObservation{mark: GetOpsCyberPolicy(c), turnErr: turnErr}
		}}
	})
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	select {
	case got := <-observed:
		require.Error(t, got.turnErr)
		requireOpenAIWSHandshakeCyberMark(t, got.mark)
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough handshake failure did not reach AfterTurn")
	}
	select {
	case err := <-serverErr:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough handshake failure did not exit")
	}
}

func TestFailoverOpenAIUpstreamHTTPErrorCyberBypassesCustomPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	repo := &tempUnschedulableOpenAIAccountRepo{}
	svc := &OpenAIGatewayService{
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	account := &Account{
		ID: 905, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusBadRequest),
				"keywords":         []any{"blocked by cyber policy"},
				"duration_minutes": float64(1),
			}},
		},
	}
	resp := &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{}}

	got := svc.failoverOpenAIUpstreamHTTPError(
		context.Background(), c, account, resp, []byte(openAIWSHandshakeCyberBody),
		"blocked by cyber policy", "gpt-5.5",
	)

	require.Nil(t, got, "cyber policy rejection must not fail over even when a custom rule matches")
	require.Zero(t, repo.modelRateLimitAccountID, "cyber policy rejection must skip custom temp-unscheduled state")
	require.Empty(t, repo.modelRateLimitKey)
	requireOpenAIWSHandshakeCyberMark(t, GetOpsCyberPolicy(c))
}

func requireOpenAIWSHandshakeCyberMark(t *testing.T, mark *CyberPolicyMark) {
	t.Helper()
	require.NotNil(t, mark)
	require.Equal(t, "cyber_policy", mark.Code)
	require.Equal(t, "blocked by cyber policy", mark.Message)
	require.Contains(t, mark.Body, `"code":"cyber_policy"`)
	require.Equal(t, http.StatusBadRequest, mark.UpstreamStatus)
	require.Zero(t, mark.UpstreamInTok)
	require.Zero(t, mark.UpstreamOutTok)
}
