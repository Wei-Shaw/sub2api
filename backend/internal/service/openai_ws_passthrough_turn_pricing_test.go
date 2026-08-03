package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// startPassthroughHookRecordingServer 与 startPassthroughLifecycleServer 相同，
// 但把一组会记录调用的 hooks 传给 ingress，用于观察透传路径的 turn 回调。
func startPassthroughHookRecordingServer(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	hooks *OpenAIWSIngressHooks,
) (*httptest.Server, <-chan error) {
	t.Helper()
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		msgType, firstMessage, err := ReadOpenAIWSClientMessage(
			controlCtx,
			conn,
			3*time.Second,
			coderws.StatusPolicyViolation,
			"missing first response.create message",
		)
		if err != nil {
			serverErr <- err
			return
		}
		if msgType != coderws.MessageText {
			serverErr <- errors.New("first message was not text")
			return
		}

		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		req := r.Clone(controlCtx)
		req.Header = req.Header.Clone()
		ginCtx.Request = req
		serverErr <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	return server, serverErr
}

// TestPassthroughIngressCallsBeforeTurn verifies that passthrough uses the
// same turn-start lifecycle as pooled ingress. The handler freezes pricing and
// rechecks terminal admission from this callback.
func TestPassthroughIngressCallsBeforeTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pricing","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)

	var hooksMu sync.Mutex
	beforeTurnCalls := 0
	afterTurnCalls := 0
	hooks := &OpenAIWSIngressHooks{
		BeforeTurn: func(int) error {
			hooksMu.Lock()
			beforeTurnCalls++
			hooksMu.Unlock()
			return nil
		},
		AfterTurn: func(int, *OpenAIForwardResult, error) {
			hooksMu.Lock()
			afterTurnCalls++
			hooksMu.Unlock()
		},
	}

	server, serverErr := startPassthroughHookRecordingServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
		hooks,
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	// 等待连接自然结束（inter-turn idle 超时），确保 AfterTurn 已提交。
	_, _ = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough ingress did not exit")
	}

	hooksMu.Lock()
	gotBefore, gotAfter := beforeTurnCalls, afterTurnCalls
	hooksMu.Unlock()

	require.Equal(t, 1, gotBefore, "首个 response.create 必须触发 BeforeTurn")
	require.Positive(t, gotAfter, "透传 ingress 仍应回调 AfterTurn 提交用量")
}

func TestPassthroughIngressBinaryFollowupUsesTurnLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_binary_turn_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)

	var hooksMu sync.Mutex
	beforeTurns := make([]int, 0, 2)
	afterTurns := make([]int, 0, 2)
	afterModels := make([]string, 0, 2)
	hooks := &OpenAIWSIngressHooks{
		BeforeTurn: func(turn int) error {
			hooksMu.Lock()
			beforeTurns = append(beforeTurns, turn)
			hooksMu.Unlock()
			return nil
		},
		AfterTurn: func(turn int, result *OpenAIForwardResult, _ error) {
			hooksMu.Lock()
			defer hooksMu.Unlock()
			afterTurns = append(afterTurns, turn)
			if result != nil {
				afterModels = append(afterModels, result.Model)
			}
		},
	}

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
	require.Equal(t, "gpt-5.1", gjson.GetBytes(firstUpstream, "model").String())
	first, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_binary_turn_1", gjson.GetBytes(first, "response.id").String())

	writeWSFrameType(t, client, coderws.MessageBinary, `{"type":"response.create","model":"gpt-5.2","stream":false,"previous_response_id":"resp_binary_turn_1"}`)
	secondUpstream := requirePassthroughUpstreamWrite(t, upstream, time.Second)
	require.Equal(t, "response.create", gjson.GetBytes(secondUpstream, "type").String())
	require.Equal(t, "gpt-5.2", gjson.GetBytes(secondUpstream, "model").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_binary_turn_2","model":"gpt-5.2","usage":{"input_tokens":2,"output_tokens":2}}}`)
	second, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_binary_turn_2", gjson.GetBytes(second, "response.id").String())

	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("binary follow-up passthrough did not exit")
	}

	hooksMu.Lock()
	gotBefore := append([]int(nil), beforeTurns...)
	gotAfter := append([]int(nil), afterTurns...)
	gotModels := append([]string(nil), afterModels...)
	hooksMu.Unlock()
	require.Equal(t, []int{1, 2}, gotBefore)
	require.Equal(t, []int{1, 2}, gotAfter)
	require.Equal(t, []string{"gpt-5.1", "gpt-5.2"}, gotModels)
}
