package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAIWSExecutionScopeTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

func dialOpenAIWSExecutionScopeClient(t *testing.T, wsURL, threadID string) *coderws.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("session-id", "root-session")
	if threadID != "" {
		header.Set(openAIWSTurnMetadataHeader, `{"session_id":"root-session","thread_id":"`+threadID+`"}`)
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsURL, "http"), &coderws.DialOptions{HTTPHeader: header})
	require.NoError(t, err)
	return conn
}

func writeOpenAIWSExecutionScopeRequest(t *testing.T, conn *coderws.Conn, body string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(body)))
}

// 场景：codex 子智能体与父线程共用 session-id 头，只有 x-codex-turn-metadata 的 thread_id 不同。
// ctx_pool 下的 turn state 绑定与 store=false 的上游连接绑定必须落在执行作用域键下，
// 不能落在按 session-id 算出的会话哈希下，否则子智能体会覆盖父线程的绑定。
func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_StateBoundToExecutionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSExecutionScopeTestConfig()

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_exec_scope","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	handshake := http.Header{}
	handshake.Set(openAIWSTurnStateHeader, "turn-state-from-upstream")
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn, handshake: handshake})
	defer pool.Close()

	stateStore := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{
		cfg:                cfg,
		httpUpstream:       &httpUpstreamRecorder{},
		cache:              &stubGatewayCache{},
		openaiWSResolver:   NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:      NewCodexToolCorrector(),
		openaiWSPool:       pool,
		openaiWSStateStore: stateStore,
	}
	groupID := int64(9)
	account := &Account{
		ID:          454,
		Name:        "openai-ingress-exec-scope",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	serverErrCh := make(chan error, 1)
	keysCh := make(chan [2]string, 1)
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
		ginCtx.Set("api_key", &APIKey{ID: 21, GroupID: &groupID})
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		scope, _ := resolveOpenAIWSExecutionScope(ginCtx, firstMessage, 21)
		keysCh <- [2]string{svc.GenerateSessionHash(ginCtx, firstMessage), scope}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	clientConn := dialOpenAIWSExecutionScopeClient(t, wsServer.URL, "child-thread")
	defer func() { _ = clientConn.CloseNow() }()
	writeOpenAIWSExecutionScopeRequest(t, clientConn, `{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"input":[{"role":"user","content":"first"}]}`)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, completed, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr)
	require.Equal(t, "resp_exec_scope", gjson.GetBytes(completed, "response.id").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}
	keys := <-keysCh
	legacyHash, scope := keys[0], keys[1]
	require.NotEmpty(t, legacyHash)
	require.Len(t, scope, 16)
	require.NotEqual(t, legacyHash, scope)

	_, connBoundToLegacy := stateStore.GetSessionConn(groupID, legacyHash)
	require.False(t, connBoundToLegacy, "上游连接不得绑定到按 session-id 算出的会话哈希")
	_, connBoundToScope := stateStore.GetSessionConn(groupID, scope)
	require.True(t, connBoundToScope, "上游连接应绑定到执行作用域")
	_, turnStateOnLegacy := stateStore.GetSessionTurnState(groupID, legacyHash)
	require.False(t, turnStateOnLegacy, "turn state 不得绑定到会话哈希")
	turnState, turnStateOnScope := stateStore.GetSessionTurnState(groupID, scope)
	require.True(t, turnStateOnScope, "turn state 应绑定到执行作用域")
	require.Equal(t, "turn-state-from-upstream", turnState)
}

// openAIWSGatedConn 是只回一个事件的假上游连接：收到请求后关闭 sent，
// 事件要等 gate 打开才吐出，让用例精确控制“A 在飞时 B 接入”的先后。
type openAIWSGatedConn struct {
	mu       sync.Mutex
	event    []byte
	sent     chan struct{}
	gate     chan struct{}
	closed   bool
	consumed bool
}

func newOpenAIWSGatedConn(event string) *openAIWSGatedConn {
	return &openAIWSGatedConn{event: []byte(event), sent: make(chan struct{}), gate: make(chan struct{})}
}

func (c *openAIWSGatedConn) WriteJSON(ctx context.Context, value any) error {
	_ = ctx
	_ = value
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errOpenAIWSConnClosed
	}
	select {
	case <-c.sent:
	default:
		close(c.sent)
	}
	return nil
}

func (c *openAIWSGatedConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errOpenAIWSConnClosed
	}
	if c.consumed {
		c.mu.Unlock()
		return nil, io.EOF
	}
	c.consumed = true
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.gate:
	}
	return c.event, nil
}

func (c *openAIWSGatedConn) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (c *openAIWSGatedConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// runOpenAIWSCodexThreadPair 用 OAuth 账号（抢占只对 OAuth ctx_pool 生效）跑两条并发接入：
// A 的请求发到上游后 B 才接入并立即完成，B 完成后才放行 A 的上游事件。
// 返回 A 与 B 的服务端返回值、A 客户端读结果的错误。
func runOpenAIWSCodexThreadPair(t *testing.T, threadA, threadB string) (serverErrs []error, aReadErr error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSExecutionScopeTestConfig()

	gatedConn := newOpenAIWSGatedConn(`{"type":"response.completed","response":{"id":"resp_thread_a","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	fastConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_thread_b","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSQueueDialer{conns: []openAIWSClientConn{gatedConn, fastConn}})
	defer pool.Close()

	svc := &OpenAIGatewayService{
		cfg:                cfg,
		httpUpstream:       &httpUpstreamRecorder{},
		cache:              &stubGatewayCache{},
		openaiWSResolver:   NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:      NewCodexToolCorrector(),
		openaiWSPool:       pool,
		openaiWSStateStore: NewOpenAIWSStateStore(nil),
	}
	groupID := int64(9)
	account := &Account{
		ID:          455,
		Name:        "openai-ingress-thread-pair",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra:       map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
	}

	serverErrCh := make(chan error, 2)
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
		ginCtx.Set("api_key", &APIKey{ID: 21, GroupID: &groupID})
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "test-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	connA := dialOpenAIWSExecutionScopeClient(t, wsServer.URL, threadA)
	defer func() { _ = connA.CloseNow() }()
	writeOpenAIWSExecutionScopeRequest(t, connA, `{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"thread a"}]}`)
	select {
	case <-gatedConn.sent:
	case <-time.After(3 * time.Second):
		t.Fatal("A 的请求应先到达上游")
	}

	connB := dialOpenAIWSExecutionScopeClient(t, wsServer.URL, threadB)
	defer func() { _ = connB.CloseNow() }()
	writeOpenAIWSExecutionScopeRequest(t, connB, `{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"thread b"}]}`)
	readCtxB, cancelB := context.WithTimeout(context.Background(), 3*time.Second)
	_, completedB, readErrB := connB.Read(readCtxB)
	cancelB()
	require.NoError(t, readErrB, "B 必须正常完成")
	require.Equal(t, "resp_thread_b", gjson.GetBytes(completedB, "response.id").String())
	close(gatedConn.gate)

	readCtxA, cancelA := context.WithTimeout(context.Background(), 5*time.Second)
	_, completedA, aReadErr := connA.Read(readCtxA)
	cancelA()
	if aReadErr == nil {
		require.Equal(t, "resp_thread_a", gjson.GetBytes(completedA, "response.id").String())
		require.NoError(t, connA.Close(coderws.StatusNormalClosure, "done"))
	}
	require.NoError(t, connB.Close(coderws.StatusNormalClosure, "done"))

	for i := 0; i < 2; i++ {
		select {
		case err := <-serverErrCh:
			serverErrs = append(serverErrs, err)
		case <-time.After(5 * time.Second):
			t.Fatal("等待 ingress websocket 结束超时")
		}
	}
	return serverErrs, aReadErr
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexThreadsDoNotPreemptEachOther(t *testing.T) {
	serverErrs, aReadErr := runOpenAIWSCodexThreadPair(t, "thread-a", "thread-b")
	require.NoError(t, aReadErr, "子智能体接入后父线程在飞的请求必须继续完成")
	for _, err := range serverErrs {
		require.NoError(t, err)
	}
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_SameCodexThreadStillPreempts(t *testing.T) {
	serverErrs, aReadErr := runOpenAIWSCodexThreadPair(t, "thread-a", "thread-a")
	require.Error(t, aReadErr, "同线程重连必须取代旧连接")
	var closeErr coderws.CloseError
	require.True(t, errors.As(aReadErr, &closeErr), "被取代的连接应收到关闭帧而不是裸断开: %v", aReadErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
	require.Equal(t, openAIWSSessionPreemptedCloseReason, closeErr.Reason)
	preempted := 0
	for _, err := range serverErrs {
		if IsOpenAIWSSessionPreemptedError(err) {
			preempted++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, preempted, "应恰好有一条会话被抢占")
}
