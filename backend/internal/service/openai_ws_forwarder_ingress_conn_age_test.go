package service

import (
	"context"
	"errors"
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

type openAIWSIngressConnAgeScenario struct {
	dialer     *openAIWSQueueDialer
	firstConn  *openAIWSCaptureConn
	secondConn *openAIWSCaptureConn
	secondTurn []byte
}

// runOpenAIWSIngressConnAgeScenario 跑两轮 ingress 会话，并在第二轮前把池里连接的建立时间
// 回拨到 51 分钟前，模拟会话持有的上游连接到龄。
func runOpenAIWSIngressConnAgeScenario(
	t *testing.T,
	accountID int64,
	firstConnEvents [][]byte,
	secondConnEvents [][]byte,
	firstTurnPayload string,
	secondTurnPayload string,
) openAIWSIngressConnAgeScenario {
	t.Helper()
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

	firstConn := &openAIWSCaptureConn{events: firstConnEvents}
	secondConn := &openAIWSCaptureConn{events: secondConnEvents}
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
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
		ID:          accountID,
		Name:        "openai-ingress-conn-age",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	if previousResponseID := gjson.Get(firstTurnPayload, "previous_response_id").String(); previousResponseID != "" {
		conn := newOpenAIWSConn("resumed", accountID, firstConn, nil)
		ap := pool.getOrCreateAccountPool(accountID)
		ap.mu.Lock()
		ap.conns[conn.id] = conn
		ap.mu.Unlock()
		svc.getOpenAIWSStateStore().BindResponseConn(previousResponseID, conn.id, time.Hour)
		dialer.conns = []openAIWSClientConn{secondConn}
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() {
			_ = conn.CloseNow()
		}()
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
	defer func() {
		_ = clientConn.CloseNow()
	}()
	writeMessage := func(payload string) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	readMessage := func() []byte {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		msgType, message, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return message
	}

	writeMessage(firstTurnPayload)
	_ = readMessage()

	ap := pool.getOrCreateAccountPool(account.ID)
	ap.mu.Lock()
	for _, conn := range ap.conns {
		conn.createdAtNano.Store(time.Now().Add(-51 * time.Minute).UnixNano())
	}
	ap.mu.Unlock()

	writeMessage(secondTurnPayload)
	secondTurn := readMessage()

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}
	return openAIWSIngressConnAgeScenario{dialer: dialer, firstConn: firstConn, secondConn: secondConn, secondTurn: secondTurn}
}

func (s openAIWSIngressConnAgeScenario) writes(conn *openAIWSCaptureConn) []map[string]any {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return append([]map[string]any(nil), conn.writes...)
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_ConnAgeRotatesBeforeTurn(t *testing.T) {
	sc := runOpenAIWSIngressConnAgeScenario(t, 131,
		[][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)},
		[][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)},
		`{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`,
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_turn_age_1","input":[{"type":"input_text","text":"world"}]}`,
	)
	require.Equal(t, "resp_turn_age_2", gjson.GetBytes(sc.secondTurn, "response.id").String())
	require.Equal(t, 2, sc.dialer.DialCount(), "连接到龄后第二轮应换新连接")
	require.Len(t, sc.writes(sc.firstConn), 1, "到龄连接不应再承接第二轮")
	secondWrites := sc.writes(sc.secondConn)
	require.Len(t, secondWrites, 1)
	require.Equal(t, "resp_turn_age_1", gjson.Get(requestToJSONString(secondWrites[0]), "previous_response_id").String(), "store 未禁用时续链锚点在服务端有效，换连应原样保留")
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_StoreDisabledConnAgeRotatesWithFullReplay(t *testing.T) {
	sc := runOpenAIWSIngressConnAgeScenario(t, 132,
		[][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_strict_1","model":"gpt-5.1","output":[{"type":"message","id":"msg_age_1","role":"assistant","content":[{"type":"output_text","text":"secret-829"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`)},
		[][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_strict_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)},
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"input":[{"type":"input_text","text":"hello"}]}`,
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"previous_response_id":"resp_turn_age_strict_1","input":[{"type":"input_text","text":"world"}]}`,
	)
	require.Equal(t, "resp_turn_age_strict_2", gjson.GetBytes(sc.secondTurn, "response.id").String())
	require.Equal(t, 2, sc.dialer.DialCount(), "store=false 会话连接到龄后应换连重放")
	require.Len(t, sc.writes(sc.firstConn), 1)
	secondWrites := sc.writes(sc.secondConn)
	require.Len(t, secondWrites, 1)
	secondWrite := requestToJSONString(secondWrites[0])
	require.False(t, gjson.Get(secondWrite, "previous_response_id").Exists(), "换连重放应移除只在旧连接有效的 previous_response_id")
	require.Len(t, gjson.Get(secondWrite, "input").Array(), 3, "换连重放应携带用户输入和助手输出的完整历史")
	require.Equal(t, "hello", gjson.Get(secondWrite, "input.0.text").String())
	require.Equal(t, "assistant", gjson.Get(secondWrite, "input.1.role").String())
	require.Equal(t, "secret-829", gjson.Get(secondWrite, "input.1.content.0.text").String())
	require.Equal(t, "world", gjson.Get(secondWrite, "input.2.text").String())
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_StoreDisabledConnAgeRotateSkipsIncompleteHistory(t *testing.T) {
	sc := runOpenAIWSIngressConnAgeScenario(t, 134,
		[][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_age_resumed_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_age_resumed_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
		[][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_age_resumed_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)},
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"previous_response_id":"resp_before_session","input":[{"type":"input_text","text":"resumed delta"}]}`,
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"previous_response_id":"resp_age_resumed_1","input":[{"type":"input_text","text":"next delta"}]}`,
	)
	require.Equal(t, "resp_age_resumed_2", gjson.GetBytes(sc.secondTurn, "response.id").String())
	firstWrites := sc.writes(sc.firstConn)
	require.Len(t, firstWrites, 2, "连接建立前的历史无法重建时应保留原连接")
	require.Equal(t, "resp_age_resumed_1", gjson.Get(requestToJSONString(firstWrites[1]), "previous_response_id").String())
	require.Empty(t, sc.writes(sc.secondConn))
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_StoreDisabledConnAgeRotateSkipsWhenFunctionCallOutputNeedsPreviousResponseID(t *testing.T) {
	sc := runOpenAIWSIngressConnAgeScenario(t, 133,
		[][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_fc_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_turn_age_fc_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
		nil,
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"input":[{"type":"function_call","call_id":"call_other","name":"shell","arguments":"{}"},{"type":"function_call_output","call_id":"call_replay_1","output":"ok"}]}`,
		`{"type":"response.create","model":"gpt-5.1","stream":false,"store":false,"previous_response_id":"resp_turn_age_fc_1","input":[{"type":"function_call_output","call_id":"call_replay_1","output":"ok"}]}`,
	)
	require.Equal(t, "resp_turn_age_fc_2", gjson.GetBytes(sc.secondTurn, "response.id").String())
	require.Equal(t, 1, sc.dialer.DialCount(), "重放上下文不完整时不应换连")
	firstWrites := sc.writes(sc.firstConn)
	require.Len(t, firstWrites, 2, "第二轮应继续走原连接")
	require.Equal(t, "resp_turn_age_fc_1", gjson.Get(requestToJSONString(firstWrites[1]), "previous_response_id").String())
	require.Empty(t, sc.writes(sc.secondConn))
}
