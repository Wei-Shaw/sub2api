package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestPassthroughIngressFollowUpCallsBeforeTurnAfterBeforeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pricing_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)

	var hooksMu sync.Mutex
	var callbacks []string
	afterTurnCalls := 0
	hooks := &OpenAIWSIngressHooks{
		BeforeRequest: func(int, []byte, string) error {
			hooksMu.Lock()
			callbacks = append(callbacks, "before_request")
			hooksMu.Unlock()
			return nil
		},
		BeforeTurn: func(int) error {
			hooksMu.Lock()
			callbacks = append(callbacks, "before_turn")
			hooksMu.Unlock()
			return nil
		},
		AfterTurn: func(int, *OpenAIForwardResult, error) {
			hooksMu.Lock()
			afterTurnCalls++
			hooksMu.Unlock()
		},
	}

	server, _ := startPassthroughHookRecordingServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
		hooks,
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`))
	cancelWrite()
	require.NoError(t, err)
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pricing_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	event, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	hooksMu.Lock()
	gotCallbacks := append([]string(nil), callbacks...)
	gotAfter := afterTurnCalls
	hooksMu.Unlock()

	require.Equal(t, []string{"before_request", "before_turn"}, gotCallbacks)
	require.Equal(t, 2, gotAfter)
}

func TestPassthroughIngressBeforeTurnRejectionDoesNotForwardFollowUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_reject_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	rejection := errors.New("turn rejected")
	var hooksMu sync.Mutex
	afterTurnCalls := 0
	var finalErr error
	beforeTurnTurn := 0
	hooks := &OpenAIWSIngressHooks{
		BeforeTurn: func(turn int) error {
			hooksMu.Lock()
			beforeTurnTurn = turn
			hooksMu.Unlock()
			return rejection
		},
		AfterTurn: func(_ int, _ *OpenAIForwardResult, turnErr error) {
			hooksMu.Lock()
			afterTurnCalls++
			if turnErr != nil {
				finalErr = turnErr
			}
			hooksMu.Unlock()
		},
	}

	server, serverErr := startPassthroughHookRecordingServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount(), hooks)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	requirePassthroughUpstreamWrite(t, upstream, time.Second)
	_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`))
	cancelWrite()
	require.NoError(t, err)
	select {
	case payload := <-upstream.writes:
		t.Fatalf("rejected response.create was forwarded upstream: %s", payload)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case err = <-serverErr:
		require.ErrorIs(t, err, rejection)
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough ingress did not exit after BeforeTurn rejection")
	}

	hooksMu.Lock()
	gotBeforeTurn, gotAfter, gotFinalErr := beforeTurnTurn, afterTurnCalls, finalErr
	hooksMu.Unlock()
	require.Equal(t, 2, gotBeforeTurn)
	require.Equal(t, 2, gotAfter, "each started turn must be finalized exactly once")
	require.ErrorIs(t, gotFinalErr, rejection)
}

func TestPassthroughIngressFreezesSubsequentTurnBeforeRequestPolicy(t *testing.T) {
	testPassthroughIngressFreezesSubsequentTurnBeforeRequestPolicy(t, coderws.MessageText)
}

func TestPassthroughIngressFreezesBinarySubsequentTurnBeforeRequestPolicy(t *testing.T) {
	testPassthroughIngressFreezesSubsequentTurnBeforeRequestPolicy(t, coderws.MessageBinary)
}

func testPassthroughIngressFreezesSubsequentTurnBeforeRequestPolicy(t *testing.T, secondMessageType coderws.MessageType) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)

	type turnStart struct {
		turn      int
		startedAt time.Time
	}
	turnStarts := make(chan turnStart, 2)
	beforeRequestEntered := make(chan time.Time, 1)
	releaseBeforeRequest := make(chan struct{})
	hooks := &OpenAIWSIngressHooks{
		InitialTurnStartedAt: time.Now(),
		TurnStarted: func(turn int, startedAt time.Time) {
			turnStarts <- turnStart{turn: turn, startedAt: startedAt}
		},
		BeforeRequest: func(turn int, _ []byte, _ string) error {
			if turn == 2 {
				beforeRequestEntered <- time.Now()
				<-releaseBeforeRequest
			}
			return nil
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

	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())
	firstCompleted, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_first", gjson.GetBytes(firstCompleted, "response.id").String())
	select {
	case first := <-turnStarts:
		require.Equal(t, 1, first.turn)
	case <-time.After(time.Second):
		t.Fatal("first turn start was not reported")
	}

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, secondMessageType, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_first"}`))
	cancelWrite()
	require.NoError(t, err)

	var policyEnteredAt time.Time
	select {
	case policyEnteredAt = <-beforeRequestEntered:
	case <-time.After(time.Second):
		t.Fatal("second turn did not enter BeforeRequest")
	}
	close(releaseBeforeRequest)
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	secondCompleted, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_second", gjson.GetBytes(secondCompleted, "response.id").String())

	select {
	case second := <-turnStarts:
		require.Equal(t, 2, second.turn)
		require.False(t, second.startedAt.After(policyEnteredAt), "第二轮开始时刻必须在 BeforeRequest 策略执行前冻结")
	case <-time.After(time.Second):
		t.Fatal("second turn start was not reported")
	}

	_ = clientConn.CloseNow()
	cancelControl(context.Canceled)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough ingress did not exit")
	}
}

func TestOpenAIWSIngressOAuthDefaultEchoMatchesCurrentTurnBilling(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModePassthrough, OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			upstream := newStagedPassthroughConn()
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			account := passthroughLifecycleAccount()
			account.Type = AccountTypeOAuth
			account.Credentials = map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-account"}
			account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": mode}
			results := make(chan *OpenAIForwardResult, 3)
			hooks := &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
				if err == nil {
					results <- result
				}
			}}
			svc := newPassthroughLifecycleService(cfg, upstream)
			svc.openaiWSPool = newOpenAIWSConnPool(cfg)
			svc.openaiWSPool.setClientDialerForTest(&stagedPassthroughDialer{conn: &serviceTierRecordingWSConn{upstream}})
			server, _ := startPassthroughHookRecordingServer(t, ctx, svc, account, hooks)
			defer server.Close()
			client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.1","service_tier":"priority"}`)
			defer client.CloseNow()
			for i, want := range []string{"priority", "default", "ultrafast"} {
				if i > 0 {
					payload := `{"type":"response.create","model":"gpt-5.1"}`
					if want == "ultrafast" {
						payload = `{"type":"response.create","model":"gpt-5.1","service_tier":"ultrafast"}`
					}
					writeCtx, stop := context.WithTimeout(ctx, time.Second)
					err := client.Write(writeCtx, coderws.MessageText, []byte(payload))
					stop()
					require.NoError(t, err)
				}
				sent := requirePassthroughUpstreamWrite(t, upstream, time.Second)
				if want == "default" {
					require.False(t, gjson.GetBytes(sent, "service_tier").Exists())
				} else {
					require.Equal(t, want, gjson.GetBytes(sent, "service_tier").String())
				}
				upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_tier_%d","model":"gpt-5.1","status":"completed","service_tier":"default","usage":{"input_tokens":1,"output_tokens":1}}}`, i))
				frame, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
				require.Equal(t, want, gjson.GetBytes(frame, "response.service_tier").String())
				select {
				case result := <-results:
					require.Equal(t, "default", result.UpstreamResponseServiceTier)
					expected := want
					if want == "default" {
						expected = ""
					}
					require.Equal(t, expected, ApplyOpenAIServiceTierBillingResolution(account, result).Billing)
				case <-time.After(3 * time.Second):
					t.Fatal("missing turn usage result")
				}
			}
		})
	}
}

type serviceTierRecordingWSConn struct{ *stagedPassthroughConn }

func (c *serviceTierRecordingWSConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}
