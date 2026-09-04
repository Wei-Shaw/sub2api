package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type stagedPassthroughFrame struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type stagedPassthroughConn struct {
	frames    chan stagedPassthroughFrame
	writes    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newStagedPassthroughConn() *stagedPassthroughConn {
	return &stagedPassthroughConn{
		frames: make(chan stagedPassthroughFrame, 4),
		writes: make(chan []byte, 4),
		closed: make(chan struct{}),
	}
}

func (c *stagedPassthroughConn) Send(payload string) {
	c.frames <- stagedPassthroughFrame{messageType: coderws.MessageText, payload: []byte(payload)}
}

func (c *stagedPassthroughConn) Fail(err error) {
	c.frames <- stagedPassthroughFrame{err: err}
}

func (c *stagedPassthroughConn) WriteJSON(context.Context, any) error { return nil }

func (c *stagedPassthroughConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_, payload, err := c.ReadFrame(ctx)
	return payload, err
}

func (c *stagedPassthroughConn) Ping(context.Context) error { return nil }

func (c *stagedPassthroughConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	case <-c.closed:
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	case frame := <-c.frames:
		return frame.messageType, append([]byte(nil), frame.payload...), frame.err
	}
}

func (c *stagedPassthroughConn) WriteFrame(ctx context.Context, _ coderws.MessageType, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return errOpenAIWSConnClosed
	default:
	}
	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return err
	}
	select {
	case c.writes <- append([]byte(nil), payload...):
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return errOpenAIWSConnClosed
	}
	return nil
}

func (c *stagedPassthroughConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type stagedPassthroughDialer struct {
	mu          sync.Mutex
	conn        openAIWSClientConn
	lastHeaders http.Header
}

type stagedPassthroughSequenceDialer struct {
	mu      sync.Mutex
	conns   []openAIWSClientConn
	headers []http.Header
}

func (d *stagedPassthroughSequenceDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.conns) == 0 {
		return nil, http.StatusServiceUnavailable, nil, errors.New("no staged passthrough connection")
	}
	conn := d.conns[0]
	d.conns = d.conns[1:]
	d.headers = append(d.headers, headers.Clone())
	return conn, http.StatusSwitchingProtocols, http.Header{}, nil
}

func (d *stagedPassthroughDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	d.lastHeaders = headers.Clone()
	d.mu.Unlock()
	return d.conn, http.StatusSwitchingProtocols, http.Header{}, nil
}

func (d *stagedPassthroughDialer) LastHeaders() http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastHeaders.Clone()
}

func newPassthroughLifecycleService(cfg *config.Config, upstream *stagedPassthroughConn) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg:                       cfg,
		httpUpstream:              &httpUpstreamRecorder{},
		cache:                     &stubGatewayCache{},
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: &stagedPassthroughDialer{conn: upstream},
	}
}

func passthroughLifecycleConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

func passthroughLifecycleAccount() *Account {
	return &Account{
		ID:          901,
		Name:        "passthrough-lifecycle",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}
}

func startPassthroughLifecycleServer(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
) (*httptest.Server, <-chan error) {
	return startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, account, nil)
}

func startPassthroughLifecycleServerWithHooks(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	hooksFactory func(*gin.Context) *OpenAIWSIngressHooks,
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
		var hooks *OpenAIWSIngressHooks
		if hooksFactory != nil {
			hooks = hooksFactory(ginCtx)
		}
		serverErr <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	return server, serverErr
}

func TestPassthroughLifecycle_CyberTerminalEventsMarkBeforeAfterTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		events      []string
		wantBody    string
		wantMessage string
		wantInput   int
		wantOutput  int
	}{
		{
			name: "error",
			events: []string{
				`{"type":"error","error":{"code":"cyber_policy","message":"blocked by error event"},"usage":{"input_tokens":5,"output_tokens":1}}`,
				`{"type":"response.failed","response":{"id":"resp_error","error":{"code":"cyber_policy","message":"blocked by paired failed event"},"usage":{"input_tokens":9,"output_tokens":2}}}`,
			},
			wantBody:    `"type":"error"`,
			wantMessage: "blocked by error event",
			wantInput:   5,
			wantOutput:  1,
		},
		{
			name: "response_failed",
			events: []string{
				`{"type":"response.failed","response":{"id":"resp_failed","error":{"code":"cyber_policy","message":"blocked by failed event"},"usage":{"input_tokens":9,"output_tokens":2}}}`,
			},
			wantBody:    `"type":"response.failed"`,
			wantMessage: "blocked by failed event",
			wantInput:   9,
			wantOutput:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)
			upstream := newStagedPassthroughConn()
			for _, event := range tt.events {
				upstream.Send(event)
			}

			markSeen := make(chan CyberPolicyMark, 1)
			afterTurnCalls := atomic.Int32{}
			server, serverErr := startPassthroughLifecycleServerWithHooks(
				t,
				controlCtx,
				newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
				passthroughLifecycleAccount(),
				func(c *gin.Context) *OpenAIWSIngressHooks {
					return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) {
						afterTurnCalls.Add(1)
						if mark := GetOpsCyberPolicy(c); mark != nil {
							select {
							case markSeen <- *mark:
							default:
							}
						}
					}}
				},
			)
			defer server.Close()
			clientConn := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = clientConn.CloseNow() }()

			for range tt.events {
				_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
				require.NoError(t, err)
			}

			select {
			case mark := <-markSeen:
				require.Equal(t, "cyber_policy", mark.Code)
				require.Equal(t, tt.wantMessage, mark.Message)
				require.Contains(t, mark.Body, tt.wantBody)
				require.Equal(t, http.StatusOK, mark.UpstreamStatus)
				require.Equal(t, tt.wantInput, mark.UpstreamInTok)
				require.Equal(t, tt.wantOutput, mark.UpstreamOutTok)
			case <-time.After(3 * time.Second):
				t.Fatal("cyber mark was not visible to AfterTurn")
			}
			require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("cyber passthrough test did not exit")
			}
			require.Equal(t, int32(1), afterTurnCalls.Load(), "error/response.failed pair must complete and record once")
		})
	}
}

func TestPassthroughLifecycle_NonCyberFailureKeepsAccountSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.failed","response":{"id":"resp_non_cyber","error":{"type":"authentication_error","code":"invalid_api_key","status_code":401,"message":"credential rejected"},"usage":{"input_tokens":3,"output_tokens":1}}}`)
	repo := &openAIStream403AccountRepo{}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	account := passthroughLifecycleAccount()

	markSeen := make(chan *CyberPolicyMark, 1)
	server, serverErr := startPassthroughLifecycleServerWithHooks(
		t,
		controlCtx,
		svc,
		account,
		func(c *gin.Context) *OpenAIWSIngressHooks {
			return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) {
				markSeen <- GetOpsCyberPolicy(c)
			}}
		},
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	select {
	case mark := <-markSeen:
		require.Nil(t, mark)
	case <-time.After(3 * time.Second):
		t.Fatal("non-cyber terminal event did not complete its turn")
	}
	require.Equal(t, 1, repo.setErrorCalls, "non-cyber credential failure must retain account failure side effects")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("non-cyber passthrough test did not exit")
	}
}

func TestPassthroughLifecycle_CyberSkipsFailureAccountSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.failed","response":{"id":"resp_cyber_auth","error":{"type":"authentication_error","code":"cyber_policy","status_code":401,"message":"request blocked"}}}`)
	repo := &openAIStream403AccountRepo{}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	account := passthroughLifecycleAccount()

	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	require.Zero(t, repo.setErrorCalls, "cyber_policy is request-scoped and must not cool down the account")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("cyber side-effect test did not exit")
	}
}

func TestPassthroughLifecycle_CloseReasonTruncationPreservesUTF8(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	originalReason := strings.Repeat("a", 119) + "界"
	upstream.Fail(NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, originalReason, errors.New("policy rejected")))

	server, serverErr := startPassthroughLifecycleServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.True(t, utf8.ValidString(closeErr.Reason))
	require.LessOrEqual(t, len(closeErr.Reason), 120)
	require.Equal(t, strings.Repeat("a", 119), closeErr.Reason)

	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough close reason test did not exit")
	}
}

func dialPassthroughLifecycleClient(t *testing.T, server *httptest.Server) *coderws.Conn {
	t.Helper()
	return dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.1","stream":false}`)
}

func dialPassthroughLifecycleClientWithPayload(t *testing.T, server *httptest.Server, payload string) *coderws.Conn {
	t.Helper()
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(payload))
	cancelWrite()
	require.NoError(t, err)
	return clientConn
}

func readPassthroughLifecycleFrame(t *testing.T, clientConn *coderws.Conn, timeout time.Duration) ([]byte, error) {
	t.Helper()
	readCtx, cancelRead := context.WithTimeout(context.Background(), timeout)
	_, payload, err := clientConn.Read(readCtx)
	cancelRead()
	return payload, err
}

func requirePassthroughUpstreamWrite(t *testing.T, upstream *stagedPassthroughConn, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload := <-upstream.writes:
		return payload
	case <-time.After(timeout):
		t.Fatal("passthrough request was not forwarded upstream")
		return nil
	}
}

func TestPassthroughLifecycle_ResponsesLiteFirstFramePinsParallelToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_lite","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClientWithPayload(t, server, `{
		"type":"response.create","model":"gpt-5.1","stream":false,
		"parallel_tool_calls":true,
		"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"}
	}`)
	defer func() { _ = clientConn.CloseNow() }()

	upstreamBody := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, gjson.False, gjson.GetBytes(upstreamBody, "parallel_tool_calls").Type, string(upstreamBody))

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("Lite 首帧测试等待 passthrough 退出超时")
	}
}

func TestOpenAIWSPassthroughTurnLifecycle_SerializesTerminalCommitAndNextTurn(t *testing.T) {
	clientFrameConn := &openAIWSClientFrameConn{interTurnStarted: make(chan struct{}, 1)}
	clientFrameConn.markTurnCompleted()
	lifecycle := newOpenAIWSPassthroughTurnLifecycle(true)
	lifecycle.beginTerminalWrite()

	admitted := make(chan bool, 1)
	go func() {
		admitted <- lifecycle.beginResponseCreate(clientFrameConn.markTurnStarted)
	}()
	select {
	case <-admitted:
		t.Fatal("next response.create was admitted before terminal commit completed")
	case <-time.After(50 * time.Millisecond):
	}

	lifecycle.finishTerminalWrite(true, clientFrameConn.markTurnCompleted)
	select {
	case ok := <-admitted:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("next response.create remained blocked after terminal commit")
	}
	require.False(t, clientFrameConn.waitingForNextTurn.Load(), "accepted next turn must win over terminal idle state")

	lifecycle = newOpenAIWSPassthroughTurnLifecycle(true)
	lifecycle.beginTerminalWrite()
	admitted = make(chan bool, 1)
	go func() {
		admitted <- lifecycle.beginResponseCreate(nil)
	}()
	lifecycle.finishTerminalWrite(false, func() {
		t.Error("failed terminal write must not commit idle state")
	})
	require.False(t, <-admitted, "failed terminal write must keep the current turn in flight")
}

func TestCodexProfileWSPassthroughTransformsEveryTurnAndRestoresStructuredResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	account := codexProfileTestAccount(t, 902, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			account.ID: {
				AccountID: account.ID, SlotID: 90201, ProfileID: 90200,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	upstream := newStagedPassthroughConn()
	dialer := &stagedPassthroughDialer{conn: upstream}
	svc := &OpenAIGatewayService{
		accountRepo:               repo,
		cache:                     &codexProfileGatewayCache{values: map[string]int64{}},
		cfg:                       cfg,
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: dialer,
	}

	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = client.CloseNow() }()
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := client.Read(readCtx)
		cancel()
		if err != nil {
			serverErr <- err
			return
		}
		ginCtx := newCodexProfileGatewayContext(t, 7, 101, firstMessage)
		if svc.GenerateSessionHash(ginCtx, firstMessage) == "" {
			serverErr <- errors.New("missing passthrough Profile session hash")
			return
		}
		prepared, err := svc.PrepareCodexProfileAttempt(ginCtx.Request.Context(), ginCtx, account, firstMessage)
		if err != nil {
			serverErr <- err
			return
		}
		err = svc.proxyResponsesWebSocketV2Passthrough(
			ginCtx.Request.Context(), ginCtx, client, prepared, "test-token", firstMessage, nil,
			OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		)
		svc.ReleaseCodexProfileAttempt(ginCtx, prepared)
		serverErr <- err
	}))
	defer server.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	// The request headers identify a Windows Desktop client; keep the explicit
	// body surface consistent so profile classification is not ambiguous.
	first := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"installation_id":"client-install","session_id":"client-session","turn_id":"client-turn-1","os":"windows","arch":"arm64","surface":"desktop"},"input":[{"role":"user","content":"first"}]}`)
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, first))
	cancel()
	select {
	case serverErrValue := <-serverErr:
		t.Fatalf("passthrough server exited before upstream write: %v", serverErrValue)
	default:
	}
	firstUpstream := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	expectedProfile, err := ResolveCodexRuntimeProfile(account.CodexIdentityPolicy.Profiles[0])
	require.NoError(t, err)
	handshakeHeaders := dialer.LastHeaders()
	require.Equal(t, expectedProfile.UserAgent, handshakeHeaders.Get("user-agent"))
	require.Equal(t, "Codex Desktop", handshakeHeaders.Get("originator"))
	require.Equal(t, expectedProfile.Version, handshakeHeaders.Get("version"))
	installationAlias := gjson.GetBytes(firstUpstream, "client_metadata.installation_id").String()
	sessionAlias := gjson.GetBytes(firstUpstream, "client_metadata.session_id").String()
	firstTurnAlias := gjson.GetBytes(firstUpstream, "client_metadata.turn_id").String()
	require.NotEmpty(t, installationAlias)
	require.NotEmpty(t, sessionAlias)
	require.NotEmpty(t, firstTurnAlias)
	require.NotEqual(t, "client-install", installationAlias)
	require.NotEqual(t, "client-session", sessionAlias)
	require.NotEqual(t, "client-turn-1", firstTurnAlias)
	require.Equal(t, "windows", gjson.GetBytes(firstUpstream, "client_metadata.os").String())
	require.Equal(t, "x86_64", gjson.GetBytes(firstUpstream, "client_metadata.arch").String())
	require.Equal(t, "desktop", gjson.GetBytes(firstUpstream, "client_metadata.surface").String())

	upstream.Send(`{"type":"response.completed","response":{"id":"resp_profile_pt_1","client_metadata":{"installation_id":"` + installationAlias + `","session_id":"` + sessionAlias + `","turn_id":"` + firstTurnAlias + `","os":"windows","arch":"x86_64","surface":"desktop"},"output":[{"type":"message","content":[{"type":"output_text","text":"` + sessionAlias + `"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	firstResponse, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "client-install", gjson.GetBytes(firstResponse, "response.client_metadata.installation_id").String())
	require.Equal(t, "client-session", gjson.GetBytes(firstResponse, "response.client_metadata.session_id").String())
	require.Equal(t, "client-turn-1", gjson.GetBytes(firstResponse, "response.client_metadata.turn_id").String())
	require.Equal(t, "arm64", gjson.GetBytes(firstResponse, "response.client_metadata.arch").String())
	require.Equal(t, "desktop", gjson.GetBytes(firstResponse, "response.client_metadata.surface").String())
	require.Equal(t, sessionAlias, gjson.GetBytes(firstResponse, "response.output.0.content.0.text").String(), "ordinary model output must not be globally restored")

	second := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"installation_id":"client-install","session_id":"client-session","turn_id":"client-turn-2","os":"windows","arch":"arm64","surface":"desktop"},"input":[{"role":"user","content":"second"}]}`)
	writeCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, second))
	cancel()
	secondUpstream := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	secondTurnAlias := gjson.GetBytes(secondUpstream, "client_metadata.turn_id").String()
	require.Equal(t, installationAlias, gjson.GetBytes(secondUpstream, "client_metadata.installation_id").String())
	require.Equal(t, sessionAlias, gjson.GetBytes(secondUpstream, "client_metadata.session_id").String())
	require.NotEmpty(t, secondTurnAlias)
	require.NotEqual(t, firstTurnAlias, secondTurnAlias)

	upstream.Send(`{"type":"response.completed","response":{"id":"resp_profile_pt_2","client_metadata":{"installation_id":"` + installationAlias + `","session_id":"` + sessionAlias + `","turn_id":"` + secondTurnAlias + `","os":"windows","arch":"x86_64","surface":"desktop"},"usage":{"input_tokens":1,"output_tokens":1}}}`)
	secondResponse, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "client-turn-2", gjson.GetBytes(secondResponse, "response.client_metadata.turn_id").String())
	require.Equal(t, "client-session", gjson.GetBytes(secondResponse, "response.client_metadata.session_id").String())
	_ = client.Close(coderws.StatusNormalClosure, "done")

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		if err != nil && (!errors.As(err, &closeErr) || closeErr.StatusCode() != coderws.StatusNormalClosure) {
			require.NoError(t, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Profile passthrough relay shutdown")
	}
}

func TestCodexProfileWSPassthroughLaterTurn429RebindsAndReplaysCurrentTurnPerAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	firstAccount := codexProfileTestAccount(t, 912, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	firstAccount.Name = "passthrough-limited"
	firstAccount.Credentials["model_mapping"] = map[string]any{"client-model": "upstream-a"}
	firstAccount.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
	secondAccount := codexProfileTestAccount(t, 913, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	secondAccount.Name = "passthrough-replacement"
	secondAccount.Credentials["model_mapping"] = map[string]any{"client-model": "upstream-b"}
	secondAccount.Extra[codexFingerprintSeedExtraKey] = "33333333-3333-4333-8333-333333333333"
	secondAccount.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{firstAccount.ID: firstAccount, secondAccount.ID: secondAccount},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			firstAccount.ID: {
				AccountID: firstAccount.ID, SlotID: 91201, ProfileID: 91200,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
			secondAccount.ID: {
				AccountID: secondAccount.ID, SlotID: 91301, ProfileID: 91300,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	profileCache := &codexProfileGatewayCache{values: map[string]int64{}}
	upstreamFirst := newStagedPassthroughConn()
	upstreamSecond := newStagedPassthroughConn()
	dialer := &stagedPassthroughSequenceDialer{conns: []openAIWSClientConn{upstreamFirst, upstreamSecond}}
	svc := &OpenAIGatewayService{
		accountRepo:               repo,
		cache:                     profileCache,
		cfg:                       cfg,
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: dialer,
	}
	firstHooks := &OpenAIWSIngressHooks{
		MapRequestModel: func(_ int, originalModel string) (string, error) {
			if originalModel == "client-model" {
				return "upstream-a", nil
			}
			return originalModel, nil
		},
	}
	secondHooks := &OpenAIWSIngressHooks{
		MapRequestModel: func(_ int, originalModel string) (string, error) {
			if originalModel == "client-model" {
				return "upstream-b", nil
			}
			return originalModel, nil
		},
	}

	type isolationProbe struct {
		indexKey   string
		bindingKey string
	}
	retryPayloadCh := make(chan []byte, 1)
	isolationProbeCh := make(chan isolationProbe, 1)
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = client.CloseNow() }()
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := client.Read(readCtx)
		cancel()
		if err != nil {
			serverErr <- err
			return
		}

		ginCtx := newCodexProfileGatewayContext(t, 7, 101, firstMessage)
		sessionHash := svc.GenerateSessionHash(ginCtx, firstMessage)
		if sessionHash == "" {
			serverErr <- errors.New("missing passthrough failover session hash")
			return
		}
		requestB := codexProfileTestContext(7, 202, CodexClientProfile{
			OSClass: CodexOSWindows, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64,
		}, sessionHash)
		profileRequestB, ok := codexProfileRequestFromContext(requestB)
		if !ok {
			serverErr <- errors.New("missing API key B Profile request")
			return
		}
		probe := isolationProbe{
			indexKey:   codexProfileAffinityKey(profileRequestB, sessionHash, 0, true),
			bindingKey: codexProfileAffinityKey(profileRequestB, sessionHash, 1, false),
		}
		profileCache.values[probe.indexKey] = 999
		profileCache.values[probe.bindingKey] = 999
		isolationProbeCh <- probe

		groupID := int64(3)
		preparedFirst, err := svc.PrepareCodexProfileAttempt(ginCtx.Request.Context(), ginCtx, firstAccount, firstMessage)
		if err != nil {
			serverErr <- err
			return
		}
		if _, err = svc.setCodexProfileAffinityAccountID(ginCtx.Request.Context(), &groupID, sessionHash, preparedFirst.ID); err != nil {
			svc.ReleaseCodexProfileAttempt(ginCtx, preparedFirst)
			serverErr <- err
			return
		}
		proxyErr := svc.proxyResponsesWebSocketV2Passthrough(
			ginCtx.Request.Context(), ginCtx, client, preparedFirst, "test-token", firstMessage, firstHooks,
			OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		)
		svc.ReleaseCodexProfileAttempt(ginCtx, preparedFirst)
		var failoverErr *UpstreamFailoverError
		if !errors.As(proxyErr, &failoverErr) || failoverErr.StatusCode != http.StatusTooManyRequests {
			serverErr <- fmt.Errorf("expected passthrough 429 failover, got %w", proxyErr)
			return
		}
		retryPayload, retryCurrentTurn := OpenAIWSCurrentTurnRetryPayload(proxyErr)
		if !retryCurrentTurn || len(retryPayload) == 0 {
			serverErr <- errors.New("missing passthrough current-turn retry payload")
			return
		}
		retryPayload, err = svc.RestoreCodexProfileRetryPayload(ginCtx, preparedFirst, retryPayload)
		if err != nil {
			serverErr <- err
			return
		}
		retryPayloadCh <- append([]byte(nil), retryPayload...)

		preparedSecond, err := svc.PrepareCodexProfileAttempt(ginCtx.Request.Context(), ginCtx, secondAccount, retryPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if _, err = svc.setCodexProfileAffinityAccountID(ginCtx.Request.Context(), &groupID, sessionHash, preparedSecond.ID); err != nil {
			svc.ReleaseCodexProfileAttempt(ginCtx, preparedSecond)
			serverErr <- err
			return
		}
		err = svc.proxyResponsesWebSocketV2Passthrough(
			ginCtx.Request.Context(), ginCtx, client, preparedSecond, "test-token", retryPayload, secondHooks,
			OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		)
		svc.ReleaseCodexProfileAttempt(ginCtx, preparedSecond)
		serverErr <- err
	}))
	defer server.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	firstMessage := []byte(`{"type":"response.create","model":"client-model","client_metadata":{"installation_id":"client-install","session_id":"client-session","turn_id":"client-turn-1","os":"windows","arch":"arm64","surface":"desktop"},"input":[{"role":"user","content":"first"}]}`)
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, firstMessage))
	cancel()
	select {
	case serverErrValue := <-serverErr:
		t.Fatalf("passthrough failover server exited before upstream write: %v", serverErrValue)
	default:
	}
	firstWire := requirePassthroughUpstreamWrite(t, upstreamFirst, 3*time.Second)
	require.Equal(t, "upstream-a", gjson.GetBytes(firstWire, "model").String())
	firstInstallAlias := gjson.GetBytes(firstWire, "client_metadata.installation_id").String()
	firstSessionAlias := gjson.GetBytes(firstWire, "client_metadata.session_id").String()
	firstTurnAlias := gjson.GetBytes(firstWire, "client_metadata.turn_id").String()
	require.NotEmpty(t, firstInstallAlias)
	require.NotEmpty(t, firstSessionAlias)

	upstreamFirst.Send(`{"type":"response.completed","response":{"id":"resp_passthrough_first","client_metadata":{"installation_id":"` + firstInstallAlias + `","session_id":"` + firstSessionAlias + `","turn_id":"` + firstTurnAlias + `"},"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"first-ok"}]},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"inspect","arguments":"{}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	firstResponse, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_passthrough_first", gjson.GetBytes(firstResponse, "response.id").String())
	require.Equal(t, "client-session", gjson.GetBytes(firstResponse, "response.client_metadata.session_id").String())

	secondMessage := []byte(`{"type":"response.create","model":"client-model","previous_response_id":"resp_passthrough_first","client_metadata":{"installation_id":"client-install","session_id":"client-session","turn_id":"client-turn-2","os":"windows","arch":"arm64","surface":"desktop"},"input":[{"type":"function_call_output","call_id":"call_1","output":"second"}]}`)
	writeCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, secondMessage))
	cancel()
	secondWireFirstAccount := requirePassthroughUpstreamWrite(t, upstreamFirst, 3*time.Second)
	require.Equal(t, "upstream-a", gjson.GetBytes(secondWireFirstAccount, "model").String())
	require.Equal(t, "resp_passthrough_first", gjson.GetBytes(secondWireFirstAccount, "previous_response_id").String())
	upstreamFirst.Send(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"limited"}}`)

	retryWire := requirePassthroughUpstreamWrite(t, upstreamSecond, 5*time.Second)
	require.False(t, gjson.GetBytes(retryWire, "previous_response_id").Exists())
	require.Equal(t, "upstream-b", gjson.GetBytes(retryWire, "model").String())
	require.Len(t, gjson.GetBytes(retryWire, "input").Array(), 4)
	require.Contains(t, gjson.GetBytes(retryWire, "input").Raw, "first")
	require.Contains(t, gjson.GetBytes(retryWire, "input").Raw, "first-ok")
	require.Contains(t, gjson.GetBytes(retryWire, "input").Raw, "second")
	secondInstallAlias := gjson.GetBytes(retryWire, "client_metadata.installation_id").String()
	secondSessionAlias := gjson.GetBytes(retryWire, "client_metadata.session_id").String()
	secondTurnAlias := gjson.GetBytes(retryWire, "client_metadata.turn_id").String()
	require.NotEqual(t, firstInstallAlias, secondInstallAlias)
	require.NotEqual(t, firstSessionAlias, secondSessionAlias)
	require.NotEmpty(t, secondTurnAlias)

	upstreamSecond.Send(`{"type":"response.completed","response":{"id":"resp_passthrough_second","client_metadata":{"installation_id":"` + secondInstallAlias + `","session_id":"` + secondSessionAlias + `","turn_id":"` + secondTurnAlias + `"},"output":[{"id":"msg_2","type":"message","role":"assistant","content":[{"type":"output_text","text":"second-ok"}]}],"usage":{"input_tokens":4,"output_tokens":1}}}`)
	secondResponse, err := readPassthroughLifecycleFrame(t, client, 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_passthrough_second", gjson.GetBytes(secondResponse, "response.id").String())
	require.Equal(t, "client-turn-2", gjson.GetBytes(secondResponse, "response.client_metadata.turn_id").String())
	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))

	retryPayload := <-retryPayloadCh
	require.False(t, gjson.GetBytes(retryPayload, "previous_response_id").Exists())
	require.Equal(t, "client-model", gjson.GetBytes(retryPayload, "model").String(), "retry payload must retain client model semantics for the replacement account")
	require.Equal(t, [][2]int64{{firstAccount.ID, secondAccount.ID}}, repo.rebinds)
	probe := <-isolationProbeCh
	require.Equal(t, int64(999), profileCache.values[probe.indexKey], "API key A failover must not overwrite API key B affinity index")
	require.Equal(t, int64(999), profileCache.values[probe.bindingKey], "API key A failover must not overwrite API key B affinity binding")

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		if err != nil && (!errors.As(err, &closeErr) || closeErr.StatusCode() != coderws.StatusNormalClosure) {
			require.NoError(t, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for passthrough failover relay shutdown")
	}
}

func TestOpenAIWSPassthroughReplayStateFailsClosedForOrphanToolOutput(t *testing.T) {
	state := &openAIWSPassthroughReplayState{}
	require.NoError(t, state.BeginTurn(
		[]byte(`{"type":"response.create","model":"client-model","input":[{"role":"user","content":"first"}]}`),
		"client-model",
	))
	state.ObserveUpstream([]byte(`{"type":"response.completed","response":{"id":"resp_first","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}}`))
	require.NoError(t, state.BeginTurn(
		[]byte(`{"type":"response.create","model":"client-model","previous_response_id":"resp_first","input":[{"type":"function_call_output","call_id":"missing_call","output":"done"}]}`),
		"client-model",
	))

	retryPayload, retrySafe, err := state.CurrentTurnRetryPayload()
	require.NoError(t, err)
	require.False(t, retrySafe)
	require.Nil(t, retryPayload)
}

func TestPassthroughLifecycle_LeaseLossSendsRetryClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_lease","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(event, "type").String())
	cancelControl(ErrOpenAIWSIngressLeaseLost)

	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
	require.Equal(t, "websocket ingress capacity lease lost; please reconnect", closeErr.Reason)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough lease-loss reader did not exit")
	}
}

func TestPassthroughLifecycle_CompletedTurnStartsInterTurnIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusNormalClosure, closeErr.Code)
	require.Equal(t, "websocket idle timeout", closeErr.Reason)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough idle reader did not exit")
	}
}

func TestPassthroughLifecycle_ActiveTurnInactivityUsesReadTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.output_text.delta","response_id":"resp_active","delta":"hello"}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	delta, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream websocket read timeout; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
		require.Equal(t, "upstream websocket read timeout; please reconnect", closeErr.Reason())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("passthrough active turn remained unbounded after upstream activity stopped")
	}
}

func TestPassthroughLifecycle_PreambleAllowsPromptClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_cancel","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())

	created, err := readPassthroughLifecycleFrame(t, clientConn, time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.cancel","response_id":"resp_cancel"}`))
	cancelWrite()
	require.NoError(t, err)
	cancelFrame := requirePassthroughUpstreamWrite(t, upstream, 500*time.Millisecond)
	require.Equal(t, "response.cancel", gjson.GetBytes(cancelFrame, "type").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough cancel test did not exit")
	}
}

func TestPassthroughLifecycle_RejectsOverlappingResponseCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_overlap_first","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())

	created, err := readPassthroughLifecycleFrame(t, clientConn, time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`))
	cancelWrite()
	require.NoError(t, err)

	_, err = readPassthroughLifecycleFrame(t, clientConn, time.Second)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusPolicyViolation, websocketCloseErr.Code)
	require.Equal(t, "overlapping response.create is not supported", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
		require.Equal(t, "overlapping response.create is not supported", closeErr.Reason())
	case <-time.After(3 * time.Second):
		t.Fatal("overlapping response.create did not terminate passthrough")
	}
}

func TestPassthroughLifecycle_ActiveTurnActivityRefreshesReadTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"one"}`)
	go func() {
		for _, event := range []string{
			`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"two"}`,
			`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"three"}`,
			`{"type":"response.completed","response":{"id":"resp_active_refresh","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":3}}}`,
		} {
			timer := time.NewTimer(600 * time.Millisecond)
			<-timer.C
			timer.Stop()
			upstream.Send(event)
		}
	}()
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	for _, wantType := range []string{
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.completed",
	} {
		frame, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, wantType, gjson.GetBytes(frame, "type").String())
	}
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough active-turn refresh test did not exit")
	}
}

func TestPassthroughLifecycle_TerminalSwitchesToInterTurnIdleTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 2
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())

	completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_idle_first", gjson.GetBytes(completed, "response.id").String())
	time.Sleep(1300 * time.Millisecond)
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_idle_first"}`))
	cancelWrite()
	require.NoError(t, err)
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	completed, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_idle_second", gjson.GetBytes(completed, "response.id").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusNormalClosure, websocketCloseErr.Code)
	require.Equal(t, "websocket idle timeout", websocketCloseErr.Reason)

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
		require.Equal(t, "websocket idle timeout", closeErr.Reason())
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough terminal turn did not use inter-turn idle timeout")
	}
}

func TestPassthroughLifecycle_FirstOutputTimeoutRemainsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
		require.Contains(t, string(failoverErr.ResponseBody), "first_output_timeout")
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("passthrough first output was left unbounded")
	}
}

func TestPassthroughLifecycle_ResponseCreatedTimeoutClosesWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_preamble","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	created, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream produced no semantic output; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr)
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
		require.Equal(t, "upstream produced no semantic output; please reconnect", closeErr.Reason())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("response.created timeout did not close the passthrough connection")
	}
}

func TestPassthroughLifecycle_RateLimitAfterCurrentTurnOutputDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_partial","model":"gpt-5.1"}}`)
	upstream.Send(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"limited"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	created, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	errorEvent, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	require.NoError(t, upstream.Close())

	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr, "a turn that already produced downstream output must never be replayed")
	case <-time.After(3 * time.Second):
		t.Fatal("same-turn rate-limit relay did not exit")
	}
}

func TestPassthroughLifecycle_SecondTurnTimeoutIsNotFailoverSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_first"}`))
	cancelWrite()
	require.NoError(t, err)
	upstream.Send(`{"type":"response.created","response":{"id":"resp_second","model":"gpt-5.1"}}`)

	created, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream produced no semantic output; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr, "handler must not replay the initial request on another account for a later-turn timeout")
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("second turn first semantic output was left unbounded")
	}
}
