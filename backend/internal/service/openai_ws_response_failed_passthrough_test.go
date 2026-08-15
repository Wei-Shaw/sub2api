package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForwardWSV2ResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	bindResponseFailedKeywordPassthroughRule(c)

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.failed","response":{"id":"resp_ws_failed","model":"gpt-test","error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          1701,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-test","stream":true,"input":"hello"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
	require.Contains(t, rec.Body.String(), responseFailedCustomMessage)
	require.NotContains(t, rec.Body.String(), responseFailedUpstreamMessage)
	streamErr, marked := GetOpsStreamError(c)
	require.True(t, marked)
	require.Equal(t, responseFailedCustomStatus, streamErr.IntendedStatus)
	require.Equal(t, responseFailedCustomMessage, streamErr.Message)
}

func TestProxyResponsesWebSocketFromClientCtxPoolResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	upstreamConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.failed","response":{"id":"resp_ingress_failed","model":"gpt-test","error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: upstreamConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          1702,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
		},
	}

	type proxyResult struct {
		err       error
		streamErr OpsStreamError
		marked    bool
		skipped   bool
	}
	resultCh := make(chan proxyResult, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			resultCh <- proxyResult{err: err}
			return
		}
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancelRead()
		if readErr != nil {
			resultCh <- proxyResult{err: readErr}
			return
		}
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		ginCtx.Request = r.Clone(r.Context())
		bindResponseFailedKeywordPassthroughRule(ginCtx)
		proxyErr := svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
		streamErr, marked := GetOpsStreamError(ginCtx)
		skipped, _ := ginCtx.Get(OpsSkipPassthroughKey)
		resultCh <- proxyResult{err: proxyErr, streamErr: streamErr, marked: marked, skipped: skipped == true}
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-test","stream":false}`))
	cancelWrite()
	require.NoError(t, err)
	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	require.Equal(t, responseFailedCustomMessage, gjson.GetBytes(event, "response.error.message").String())
	require.NotContains(t, string(event), responseFailedUpstreamMessage)
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case proxy := <-resultCh:
		require.NoError(t, proxy.err)
		require.True(t, proxy.marked)
		require.Equal(t, responseFailedCustomStatus, proxy.streamErr.IntendedStatus)
		require.Equal(t, responseFailedCustomMessage, proxy.streamErr.Message)
		require.True(t, proxy.skipped)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ctx_pool websocket proxy to exit")
	}
}

func TestProxyOpenAIWSHTTPBridgeTurnResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	failedEvent := `{"type":"response.failed","response":{"id":"resp_bridge_failed","error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + failedEvent + "\n\n")),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
	account := &Account{
		ID:          1703,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	bindResponseFailedKeywordPassthroughRule(c)
	payload := []byte(`{"type":"response.create","model":"gpt-test","stream":false,"input":"hello"}`)
	var frames [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(),
		c,
		account,
		"sk-test",
		payload,
		len(payload),
		"gpt-test",
		"",
		"",
		"",
		"",
		1,
		func(frame []byte) error {
			frames = append(frames, append([]byte(nil), frame...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
	require.Len(t, frames, 1)
	require.Equal(t, responseFailedCustomMessage, gjson.GetBytes(frames[0], "response.error.message").String())
	require.NotContains(t, string(frames[0]), responseFailedUpstreamMessage)
	streamErr, marked := GetOpsStreamError(c)
	require.True(t, marked)
	require.Equal(t, responseFailedCustomStatus, streamErr.IntendedStatus)
}

func TestOpenAIWSPassthroughResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.failed","response":{"id":"resp_passthrough_failed","error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`)
	server, serverErr := startPassthroughLifecycleServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
		newResponseFailedKeywordPassthroughService(),
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	require.Equal(t, responseFailedCustomMessage, gjson.GetBytes(event, "response.error.message").String())
	require.NotContains(t, string(event), responseFailedUpstreamMessage)
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for passthrough websocket proxy to exit")
	}
}
