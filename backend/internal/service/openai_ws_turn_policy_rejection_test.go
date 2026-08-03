package service

import (
	"context"
	"errors"
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

const testWSTurnPolicyRejectionReason = "client policy no longer allows this websocket turn"

func wsTurnPolicyRejectOnSecond(hookCalls *[]int, mu *sync.Mutex) *OpenAIWSIngressHooks {
	return &OpenAIWSIngressHooks{
		BeforeTurn: func(turn int) error {
			mu.Lock()
			*hookCalls = append(*hookCalls, turn)
			mu.Unlock()
			if turn > 1 {
				return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, testWSTurnPolicyRejectionReason, nil)
			}
			return nil
		},
	}
}

func TestOpenAIWSIngressRejectsSubsequentTurnBeforeUpstreamWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	upstream := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_policy_turn_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	dialer := &openAIWSCaptureDialer{conn: upstream}
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
		ID:          951,
		Name:        "pooled-turn-policy",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	var hooksMu sync.Mutex
	var hookCalls []int
	hooks := wsTurnPolicyRejectOnSecond(&hookCalls, &hooksMu)

	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancelRead()
		if readErr != nil {
			serverErr <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErr <- errors.New("unsupported websocket client message type")
			return
		}
		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		ginCtx.Request = r.Clone(r.Context())
		serverErr <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	writeWSFrame(t, client, `{"type":"response.create","model":"gpt-5.1","stream":false}`)
	first := readWSFrame(t, client)
	require.Equal(t, "resp_policy_turn_1", gjson.GetBytes(first, "response.id").String())

	writeWSFrame(t, client, `{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_policy_turn_1"}`)
	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = client.Read(readCtx)
	cancelRead()
	require.Error(t, err, "服务端拒绝第二轮后客户端连接必须结束")
	// ProxyResponsesWebSocketFromClient 把精确的 policy close 作为 typed error
	// 交给外层 handler 写帧；本测试 server 直接调用 service 并 CloseNow，因此
	// 客户端可能只看到 EOF。精确状态码/文案在下方 serverErr 上断言。

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
		require.Equal(t, testWSTurnPolicyRejectionReason, closeErr.Reason())
	case <-time.After(3 * time.Second):
		t.Fatal("pooled ingress policy rejection did not terminate")
	}

	upstream.mu.Lock()
	writes := len(upstream.writes)
	upstream.mu.Unlock()
	require.Equal(t, 1, writes, "第二轮策略拒绝必须发生在任何上游写入之前")
	hooksMu.Lock()
	gotCalls := append([]int(nil), hookCalls...)
	hooksMu.Unlock()
	require.Equal(t, []int{1, 2}, gotCalls)
}

func TestOpenAIWSPassthroughRejectsSubsequentTurnBeforeUpstreamWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		msgType coderws.MessageType
	}{
		{name: "text", msgType: coderws.MessageText},
		{name: "binary", msgType: coderws.MessageBinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)

			upstream := newStagedPassthroughConn()
			upstream.Send(`{"type":"response.completed","response":{"id":"resp_passthrough_policy_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
			var hooksMu sync.Mutex
			var hookCalls []int
			hooks := wsTurnPolicyRejectOnSecond(&hookCalls, &hooksMu)
			server, serverErr := startPassthroughHookRecordingServer(
				t,
				controlCtx,
				newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
				passthroughLifecycleAccount(),
				hooks,
			)
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = client.CloseNow() }()

			firstUpstream := requirePassthroughUpstreamWrite(t, upstream, time.Second)
			require.Equal(t, "response.create", gjson.GetBytes(firstUpstream, "type").String())
			first, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
			require.NoError(t, err)
			require.Equal(t, "resp_passthrough_policy_1", gjson.GetBytes(first, "response.id").String())

			writeWSFrameType(t, client, tc.msgType, `{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_passthrough_policy_1"}`)
			_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
			var websocketCloseErr coderws.CloseError
			require.ErrorAs(t, err, &websocketCloseErr)
			require.Equal(t, coderws.StatusPolicyViolation, websocketCloseErr.Code)
			require.Equal(t, testWSTurnPolicyRejectionReason, websocketCloseErr.Reason)

			select {
			case err := <-serverErr:
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
				require.Equal(t, testWSTurnPolicyRejectionReason, closeErr.Reason())
			case <-time.After(3 * time.Second):
				t.Fatal("passthrough policy rejection did not terminate")
			}

			select {
			case unexpected := <-upstream.writes:
				t.Fatalf("第二轮策略拒绝后不应再写上游，got %s", unexpected)
			case <-time.After(150 * time.Millisecond):
			}
			hooksMu.Lock()
			gotCalls := append([]int(nil), hookCalls...)
			hooksMu.Unlock()
			require.Equal(t, []int{1, 2}, gotCalls)
		})
	}
}

func writeWSFrame(t *testing.T, conn *coderws.Conn, payload string) {
	writeWSFrameType(t, conn, coderws.MessageText, payload)
}

func writeWSFrameType(t *testing.T, conn *coderws.Conn, msgType coderws.MessageType, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, msgType, []byte(payload)))
}

func readWSFrame(t *testing.T, conn *coderws.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msgType, payload, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	return payload
}
