package service

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
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIWSFailingDialer struct {
	err   error
	calls atomic.Int32
}

func (d *openAIWSFailingDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	d.calls.Add(1)
	return nil, 0, nil, d.err
}

// 场景：账号所绑代理断开，WS 拨号在传输层失败（无 HTTP 状态码）。
// 首次拒连即返回可换号的错误交给 handler 切账号，并临时禁调度 10 分钟。
func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_DialTransportFailureFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

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

	dialer := &openAIWSFailingDialer{
		err: &openAIWSHandshakeError{Err: errors.New("failed to WebSocket dial: dial tcp 127.0.0.1:10809: connect: connection refused")},
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	defer pool.Close()

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          452,
		Name:        "openai-ingress-dial-refused",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
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
			serverErrCh <- errors.New("unsupported websocket client message type")
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	runFailedSession := func() *UpstreamFailoverError {
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
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, serverErr, &failoverErr, "拨号传输失败应返回可换号的错误")
			return failoverErr
		case <-time.After(5 * time.Second):
			t.Fatal("等待 ingress websocket 结束超时")
			return nil
		}
	}

	failoverErr := runFailedSession()
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.JSONEq(t, string(openAITransportFailoverBody), string(failoverErr.ResponseBody))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "首次拒连应立即临时禁调度")
	require.Equal(t, int32(1), dialer.calls.Load())
}

type openAIWSDialThenFailDialer struct {
	mu    sync.Mutex
	conns []openAIWSClientConn
	err   error
	calls int
}

func (d *openAIWSDialThenFailDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if len(d.conns) == 0 {
		return nil, 0, nil, d.err
	}
	conn := d.conns[0]
	d.conns = d.conns[1:]
	return conn, 0, nil, nil
}

func (d *openAIWSDialThenFailDialer) Calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// 两轮会话：首轮在上游正常完成（助手输出 first-ok），第二轮带 previous_response_id 续链，
// 上游按 turnTwoEvents 回应后重试新建连接，此时拨号在传输层失败。返回 handler 将收到的错误与拨号次数。
func runOpenAIWSLaterTurnDialFailureSession(t *testing.T, cfg *config.Config, turnTwoEvents [][]byte) (int, error) {
	t.Helper()
	return runOpenAIWSLaterTurnDialFailureSessionWithFirstFrame(
		t,
		cfg,
		[]byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"first"}]}`),
		turnTwoEvents,
	)
}

func runOpenAIWSLaterTurnDialFailureSessionWithFirstFrame(t *testing.T, cfg *config.Config, firstFrame []byte, turnTwoEvents [][]byte) (int, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

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

	events := [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"first-ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`),
	}
	events = append(events, turnTwoEvents...)
	staleConn := &openAIWSCaptureConn{events: events}
	dialer := &openAIWSDialThenFailDialer{
		conns: []openAIWSClientConn{staleConn},
		err:   &openAIWSHandshakeError{Err: errors.New("failed to WebSocket dial: dial tcp 127.0.0.1:10809: connect: connection refused")},
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	defer pool.Close()

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          453,
		Name:        "openai-ingress-later-turn-dial-refused",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
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
			serverErrCh <- errors.New("unsupported websocket client message type")
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, firstFrame)
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, completed, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr)
	require.Equal(t, "resp_first", gjson.GetBytes(completed, "response.id").String())

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_first","input":[{"role":"user","content":"second"}]}`))
	cancelWrite()
	require.NoError(t, err)

	select {
	case serverErr := <-serverErrCh:
		return dialer.Calls(), serverErr
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
		return 0, nil
	}
}

// 后续轮次的换号错误必须携带当前轮完整请求：去掉 previous_response_id、模型为原始模型，
// input 含首轮输入、首轮助手输出与第二轮输入。
func requireOpenAIWSLaterTurnRetryPayload(t *testing.T, serverErr error) {
	t.Helper()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, serverErr, &failoverErr, "后续轮次拨号传输失败应返回可换号的错误")
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)

	retryPayload, retryCurrentTurn := OpenAIWSCurrentTurnRetryPayload(serverErr)
	require.True(t, retryCurrentTurn, "后续轮次的换号错误必须携带当前轮请求，禁止退回首包")
	require.NotEmpty(t, retryPayload)
	require.False(t, gjson.GetBytes(retryPayload, "previous_response_id").Exists())
	require.Equal(t, "gpt-5.1", gjson.GetBytes(retryPayload, "model").String())
	input := gjson.GetBytes(retryPayload, "input")
	require.True(t, input.IsArray())
	require.Len(t, input.Array(), 3, "当前轮重放应包含首轮输入、首轮助手输出与第二轮输入")
	require.Contains(t, input.Raw, `"first"`)
	require.Contains(t, input.Raw, "first-ok")
	require.Contains(t, input.Raw, `"second"`)
}

// 场景：首轮已在上游完成，第二轮首读失败触发重试新建连接，此时拨号在传输层失败。
// 返回给 handler 的换号错误必须携带当前轮完整请求，否则 handler 只能退回首包重试：
// 首轮被重复生成并计费，第二轮请求丢失。
func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_LaterTurnDialFailureCarriesCurrentTurnPayload(t *testing.T) {
	dialCalls, serverErr := runOpenAIWSLaterTurnDialFailureSession(t, &config.Config{}, nil)

	require.Equal(t, 2, dialCalls, "第二轮首读失败后的重试应新建连接")
	requireOpenAIWSLaterTurnRetryPayload(t, serverErr)
}

// 场景：第二轮上游回 previous_response_not_found，续链恢复去掉 previous_response_id 后重建连接，
// 此时拨号在传输层失败。恢复重试不得用普通 replay 覆盖已构建的换号上下文，首轮助手输出必须保留。
func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_LaterTurnPrevResponseRecoveryKeepsFailoverContext(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.IngressPreviousResponseRecoveryEnabled = true
	dialCalls, serverErr := runOpenAIWSLaterTurnDialFailureSession(t, cfg, [][]byte{
		[]byte(`{"type":"error","error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"Previous response not found."}}`),
	})

	require.Equal(t, 2, dialCalls, "续链恢复后应重建连接")
	requireOpenAIWSLaterTurnRetryPayload(t, serverErr)
}

// 场景：首帧用 previous_response_id 续接连接建立之前的响应、只带增量输入，网关手里没有此前的历史。
// 第二轮拨号失败时，本连接内累积的 first / first-ok / second 不是完整对话，
// 不得生成换号重放载荷：载荷置空，由 handler 关闭连接，避免替代账号静默丢失连接前的上下文。
func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_LaterTurnDialFailureRefusesFailoverWithoutFullHistory(t *testing.T) {
	dialCalls, serverErr := runOpenAIWSLaterTurnDialFailureSessionWithFirstFrame(
		t,
		&config.Config{},
		[]byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_before_connection","input":[{"role":"user","content":"first"}]}`),
		nil,
	)

	require.Equal(t, 2, dialCalls, "第二轮首读失败后的重试应新建连接")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, serverErr, &failoverErr)
	retryPayload, retryCurrentTurn := OpenAIWSCurrentTurnRetryPayload(serverErr)
	require.True(t, retryCurrentTurn, "后续轮次的换号错误仍须标记为当前轮，禁止退回首包")
	require.Empty(t, string(retryPayload), "历史不完整时不得携带跨账号重放载荷")
}

func TestOpenAIWSAccountFailoverHistoryComplete(t *testing.T) {
	cases := []struct {
		name               string
		previousComplete   bool
		previousResponseID string
		lastTurnResponseID string
		want               bool
	}{
		{name: "首帧自带全量 input", want: true},
		{name: "首帧续接连接建立之前的响应", previousResponseID: "resp_before_connection"},
		{name: "续接上一轮且此前完整", previousComplete: true, previousResponseID: " resp_1 ", lastTurnResponseID: "resp_1", want: true},
		{name: "续接上一轮但此前不完整", previousResponseID: "resp_1", lastTurnResponseID: "resp_1"},
		{name: "续接的不是上一轮响应", previousComplete: true, previousResponseID: "resp_other", lastTurnResponseID: "resp_1"},
		{name: "不完整之后客户端重发全量 input", lastTurnResponseID: "resp_1", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, openAIWSAccountFailoverHistoryComplete(tc.previousComplete, tc.previousResponseID, tc.lastTurnResponseID))
		})
	}
}
