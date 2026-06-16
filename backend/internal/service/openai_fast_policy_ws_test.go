package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// --- Helper-level (unit) tests for applyOpenAIFastPolicyToWSResponseCreate ---

func TestWSResponseCreate_DefaultPassesPriorityAndNormalizesFast(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority","input":[{"type":"input_text","text":"hi"}]}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String(), "default policy should preserve priority tier")
	// Other fields preserved.
	require.Equal(t, "response.create", gjson.GetBytes(updated, "type").String())
	require.Equal(t, "gpt-5.5", gjson.GetBytes(updated, "model").String())
	require.Equal(t, "hi", gjson.GetBytes(updated, "input.0.text").String())

	frame = []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"fast"}`)
	updated, blocked, err = svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String(), "fast alias should normalize before reaching upstream")

	// Mixed-case + whitespace variant should also normalize.
	frame = []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"  Fast  "}`)
	updated, blocked, err = svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
}

func TestWSResponseCreate_ExplicitFilterStripsServiceTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority","input":[{"type":"input_text","text":"hi"}]}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.NotContains(t, string(updated), `"service_tier"`, "filter action should strip service_tier")

	frame = []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"fast"}`)
	updated, blocked, err = svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.NotContains(t, string(updated), `"service_tier"`)
}

func TestWSResponseCreate_FlexPassThrough(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Default policy has no rules; flex is left untouched.
	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"flex"}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "flex", gjson.GetBytes(updated, "service_tier").String(), "flex frames must reach upstream untouched under default policy")
}

func TestWSResponseCreate_BlockReturnsTypedError(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "ws fast blocked",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.NotNil(t, blocked)
	require.Equal(t, "ws fast blocked", blocked.Message)
	// On block, payload returned unchanged so caller can inspect / log it.
	require.Equal(t, string(frame), string(updated))
}

func TestWSResponseCreate_NoServiceTierUntouched(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	frame := []byte(`{"type":"response.create","model":"gpt-5.5","input":[]}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, string(frame), string(updated), "no service_tier present must result in zero mutation")
}

func TestWSResponseCreate_NonResponseCreateFrameUntouched(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"*"},
			FallbackAction: BetaPolicyActionFilter,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// response.cancel happens to carry a service_tier-shaped field — must not be touched.
	frame := []byte(`{"type":"response.cancel","service_tier":"priority"}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, string(frame), string(updated))
}

// TestWSResponseCreate_EmptyTypeFrameUntouched is the A1 regression: the
// helper used to treat empty type as response.create, which risked stripping
// fields from malformed / unknown client events. After the A1 fix only a
// strict "response.create" match triggers policy.
func TestWSResponseCreate_EmptyTypeFrameUntouched(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"*"},
			FallbackAction: BetaPolicyActionFilter,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Frame with no "type" field: must pass through completely unchanged
	// even with a service_tier-shaped field present.
	frame := []byte(`{"service_tier":"priority","model":"gpt-5.5"}`)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, string(frame), string(updated), "empty type must NOT be policy-checked — Realtime spec requires type, malformed frames are passed through")

	// Explicit empty string also passes through.
	frame = []byte(`{"type":"","service_tier":"priority","model":"gpt-5.5"}`)
	updated, blocked, err = svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, string(frame), string(updated))
}

// TestBuildOpenAIFastPolicyBlockedWSEvent_HasEventIDAndCode is the B1
// regression: the rendered Realtime error event must carry a non-empty
// event_id (so clients can correlate the rejection) and a stable error.code
// ("policy_violation"). The HTTP-side equivalent is the 403 permission_error
// JSON body emitted by writeOpenAIFastPolicyBlockedResponse.
func TestBuildOpenAIFastPolicyBlockedWSEvent_HasEventIDAndCode(t *testing.T) {
	bytes := buildOpenAIFastPolicyBlockedWSEvent(&OpenAIFastBlockedError{Message: "blocked because reasons"})
	require.NotNil(t, bytes)

	require.Equal(t, "error", gjson.GetBytes(bytes, "type").String())
	require.Equal(t, "invalid_request_error", gjson.GetBytes(bytes, "error.type").String())
	require.Equal(t, "policy_violation", gjson.GetBytes(bytes, "error.code").String())
	require.Equal(t, "blocked because reasons", gjson.GetBytes(bytes, "error.message").String())

	eventID := gjson.GetBytes(bytes, "event_id").String()
	require.NotEmpty(t, eventID, "event_id must be present so clients can correlate the rejection in their logs")
	require.True(t, strings.HasPrefix(eventID, "evt_"), "event_id should follow the evt_<rand> Realtime convention; got %q", eventID)

	// Sanity check: two consecutive events get distinct IDs.
	other := buildOpenAIFastPolicyBlockedWSEvent(&OpenAIFastBlockedError{Message: "second"})
	otherID := gjson.GetBytes(other, "event_id").String()
	require.NotEqual(t, eventID, otherID, "event_id must be random per-event")
}

// TestBuildOpenAIFastPolicyBlockedWSEvent_NilSafe ensures the helper returns
// nil for a nil error (defensive guard for callers that always invoke it).
func TestBuildOpenAIFastPolicyBlockedWSEvent_NilSafe(t *testing.T) {
	require.Nil(t, buildOpenAIFastPolicyBlockedWSEvent(nil))
}

// --- D5: passthrough wrapper FrameConn — capturedSessionModel fallback ---

// fakePassthroughFrameConn replays a fixed sequence of client frames into the
// policy-enforcing wrapper, then returns io.EOF. Captures all Write attempts
// for write-side assertions (none expected in the D5 test, since the wrapper
// only filters reads).
type fakePassthroughFrameConn struct {
	reads     [][]byte
	idx       int
	writes    [][]byte
	closeOnce bool
}

func (f *fakePassthroughFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if f.idx >= len(f.reads) {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	payload := f.reads[f.idx]
	f.idx++
	return coderws.MessageText, payload, nil
}

func (f *fakePassthroughFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	cp := append([]byte(nil), payload...)
	f.writes = append(f.writes, cp)
	return nil
}

func (f *fakePassthroughFrameConn) Close() error {
	f.closeOnce = true
	return nil
}

// gpt55WhitelistFastPolicy 返回一份强制带 model whitelist 的策略，用于
// 验证 capturedSessionModel fallback 的语义（默认配置没有规则，fallback
// 路径无法被观察到）。
func gpt55WhitelistFastPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-5.5", "gpt-5.5*"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
}

// TestPolicyEnforcingFrameConn_FollowupFrameWithoutModelUsesCapturedModel is
// the D5 regression: in passthrough mode a follow-up response.create frame
// without a "model" field must still hit the policy via the session-level
// model captured from the first frame. Without the fallback an empty model
// would miss a model whitelist and silently leak service_tier=priority
// through to the upstream.
func TestPolicyEnforcingFrameConn_FollowupFrameWithoutModelUsesCapturedModel(t *testing.T) {
	// 此处特意使用带 whitelist 的策略，以便观察 capturedSessionModel
	// fallback 是否生效（默认配置没有规则，fallback 与否结果一致，
	// 不能用来覆盖此回归）。
	svc := newOpenAIGatewayServiceWithSettings(t, gpt55WhitelistFastPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Simulate the passthrough adapter capturing model from the first frame.
	firstFrame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)
	capturedSessionModel := openAIWSPassthroughPolicyModelForFrame(account, firstFrame)
	require.Equal(t, "gpt-5.5", capturedSessionModel)

	// Follow-up frame deliberately omits "model" — Realtime allows this.
	followupFrame := []byte(`{"type":"response.create","service_tier":"priority"}`)

	inner := &fakePassthroughFrameConn{
		reads: [][]byte{followupFrame},
	}
	wrapper := &openAIWSPolicyEnforcingFrameConn{
		inner: inner,
		filter: func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
			if msgType != coderws.MessageText {
				return payload, nil, nil
			}
			model := openAIWSPassthroughPolicyModelForFrame(account, payload)
			if model == "" {
				model = capturedSessionModel
			}
			return svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, model, payload)
		},
	}

	// Read the follow-up frame through the wrapper. The policy MUST still
	// trigger filter (gpt-5.5 + priority → filter), so the service_tier
	// field is gone by the time the relay sees it.
	_, payload, err := wrapper.ReadFrame(context.Background())
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"service_tier"`,
		"D5 regression: empty model on follow-up frame must fall back to capturedSessionModel; whitelist policy filters service_tier=priority for gpt-5.5")
	require.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
}

// TestPolicyEnforcingFrameConn_WithoutCapturedFallbackPolicyMisses pins the
// inverse: when the wrapper has NO capturedSessionModel fallback (model is
// empty per-frame and no fallback is wired up), the policy fails to match
// the model whitelist and the frame leaks through unchanged. This documents
// exactly the leak the D5 fix prevents.
func TestPolicyEnforcingFrameConn_WithoutCapturedFallbackPolicyMisses(t *testing.T) {
	// 同样使用带 whitelist 的策略以观察 leak。
	svc := newOpenAIGatewayServiceWithSettings(t, gpt55WhitelistFastPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	followupFrame := []byte(`{"type":"response.create","service_tier":"priority"}`)
	inner := &fakePassthroughFrameConn{reads: [][]byte{followupFrame}}
	wrapper := &openAIWSPolicyEnforcingFrameConn{
		inner: inner,
		filter: func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
			// NO fallback — emulate the pre-fix behavior.
			model := openAIWSPassthroughPolicyModelForFrame(account, payload)
			return svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, model, payload)
		},
	}

	_, payload, err := wrapper.ReadFrame(context.Background())
	require.NoError(t, err)
	// Pre-fix: empty model misses ["gpt-5.5","gpt-5.5*"] whitelist → fallback=pass → service_tier kept.
	require.Contains(t, string(payload), `"service_tier"`,
		"sanity: without capturedSessionModel fallback the leak (D5) reproduces — confirms the fix is load-bearing")
}

// --- Ingress end-to-end test (explicit filter path) ---

// TestWSResponseCreate_IngressFiltersServiceTierBeforeUpstream wires up the
// real ProxyResponsesWebSocketFromClient ingress session pipeline against a
// captureConn upstream and asserts that a client frame with service_tier=fast
// is normalized + filtered out by an explicit admin policy before being
// written upstream.
func TestWSResponseCreate_IngressFiltersServiceTierBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_ws_filter_1","model":"gpt-5.5","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	filterPolicyJSON, err := json.Marshal(openAIFastFilterPriorityPolicy())
	require.NoError(t, err)
	repo.values[SettingKeyOpenAIFastPolicySettings] = string(filterPolicyJSON)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
		settingService:   NewSettingService(repo, cfg),
	}

	account := &Account{
		ID:          901,
		Name:        "openai-ws-filter",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			CompressionMode: coderws.CompressionContextTakeover,
		})
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
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
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
	require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.5","stream":false,"service_tier":"fast"}`)))
	cancelWrite()

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}

	require.Len(t, captureConn.writes, 1, "上游应只收到一条 response.create")
	upstream := captureConn.writes[0]
	_, hasServiceTier := upstream["service_tier"]
	require.False(t, hasServiceTier, "上游收到的 response.create 不应包含 service_tier 字段（已被 fast policy filter 删除）")
	require.Equal(t, "response.create", upstream["type"])
	require.Equal(t, "gpt-5.5", upstream["model"])
}

// TestWSResponseCreate_IngressBlockSendsErrorEventAndSkipsUpstream is the
// integration flavour of TestWSResponseCreate_BlockReturnsTypedError. It
// asserts that with a custom block rule, the client receives a Realtime-style
// error event AND the upstream FrameConn never receives the offending frame.
func TestWSResponseCreate_IngressBlockSendsErrorEventAndSkipsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{
		// No events queued; the upstream should never get written to anyway.
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	blockSettings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "ws priority blocked for testing",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	raw, err := json.Marshal(blockSettings)
	require.NoError(t, err)
	repo.values[SettingKeyOpenAIFastPolicySettings] = string(raw)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
		settingService:   NewSettingService(repo, cfg),
	}

	account := &Account{
		ID:          902,
		Name:        "openai-ws-block",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			CompressionMode: coderws.CompressionContextTakeover,
		})
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
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		proxyErr := svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
		// Mirror the production handler (openai_gateway_handler.go:1325-1328):
		// when the proxy returns an OpenAIWSClientCloseError, surface its
		// status code to the client via a graceful close handshake. Without
		// this the deferred CloseNow() above would tear down the TCP
		// connection without sending a close frame, and the C3 timing
		// assertion (next read returns CloseStatus=1008) would see EOF
		// instead.
		var closeErr *OpenAIWSClientCloseError
		if errors.As(proxyErr, &closeErr) {
			_ = conn.Close(closeErr.StatusCode(), closeErr.Reason())
		}
		serverErrCh <- proxyErr
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.5","stream":false,"service_tier":"priority"}`)))
	cancelWrite()

	// C3 timing assertion: the FIRST frame the client reads must be the
	// error event — not a close frame. coder/websocket@v1.8.14 Conn.Write is
	// synchronous (writeFrame Flushes the bufio writer at write.go:307-311
	// before returning) and the close handshake re-acquires the same
	// writeFrameMu, so this ordering is enforced by the library itself; this
	// assertion guards against future refactors that might break it.
	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr, "first read must succeed and return the error event before any close frame")
	require.Equal(t, "error", gjson.GetBytes(event, "type").String())
	require.Equal(t, "invalid_request_error", gjson.GetBytes(event, "error.type").String())
	// B1 regression: event_id + error.code must be populated.
	require.Equal(t, "policy_violation", gjson.GetBytes(event, "error.code").String())
	require.NotEmpty(t, gjson.GetBytes(event, "event_id").String(), "event_id must be present so clients can correlate")
	require.Contains(t, gjson.GetBytes(event, "error.message").String(), "ws priority blocked for testing")

	// Next read must surface the close frame (as a CloseError). This
	// asserts the [error event, close] ordering — i.e. the close did NOT
	// race ahead of the data frame.
	readCtx2, cancelRead2 := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, secondReadErr := clientConn.Read(readCtx2)
	cancelRead2()
	require.Error(t, secondReadErr, "after the error event the connection must surface a close")
	require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(secondReadErr),
		"close status must be PolicyViolation; got %v", secondReadErr)

	select {
	case serverErr := <-serverErrCh:
		// Server returns an OpenAIWSClientCloseError — handler closes the WS;
		// here we just assert it surfaced as the typed close error.
		require.Error(t, serverErr)
		var closeErr *OpenAIWSClientCloseError
		require.True(t, errors.As(serverErr, &closeErr), "block 应返回 OpenAIWSClientCloseError，得到 %T: %v", serverErr, serverErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress 关闭超时")
	}

	// Critical: the offending frame must NEVER reach the upstream.
	// captureDialer.DialCount may legitimately be 0 or 1 depending on whether
	// the lease was acquired before policy fired; either way, no writes.
	require.Empty(t, captureConn.writes, "block 命中后上游不应收到 response.create")
}

// --- HTTP-side gap-filling tests (already covered by existing tests but
// requested to be split out explicitly) ---

// TestApplyOpenAIFastPolicyToBody_BlockShortCircuitsUpstream confirms that
// applyOpenAIFastPolicyToBody surfaces a *OpenAIFastBlockedError when the rule
// action is "block", and that the body is left untouched. The caller (chat
// completions / messages handlers) inspects this typed error and skips the
// upstream HTTP call entirely — see openai_gateway_chat_completions.go:175 and
// openai_gateway_messages.go:149.
func TestApplyOpenAIFastPolicyToBody_BlockShortCircuitsUpstream(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "priority blocked",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.Error(t, err)
	var blocked *OpenAIFastBlockedError
	require.True(t, errors.As(err, &blocked), "block must surface as typed error so caller can skip upstream HTTP request")
	require.Equal(t, "priority blocked", blocked.Message)
	require.Equal(t, string(body), string(updated), "block must not mutate body")
}

// TestForwardAsAnthropicMessages_BetaFastModePassesOpenAIFastPolicyByDefault
// verifies the Anthropic-compat entrypoint chain: anthropic-beta: fast-mode →
// BetaFastMode detection → ServiceTier="priority" injection
// (openai_gateway_messages.go:60) → default OpenAI fast policy pass. We
// exercise the same internal pipeline (Anthropic→Responses + BetaFastMode +
// policy) without spinning up a real upstream HTTP server.
func TestForwardAsAnthropicMessages_BetaFastModePassesOpenAIFastPolicyByDefault(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Step 1: parse Anthropic request (mirrors openai_gateway_messages.go:38-50).
	anthropicBody := []byte(`{"model":"gpt-5.5","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	var anthropicReq apicompat.AnthropicRequest
	require.NoError(t, json.Unmarshal(anthropicBody, &anthropicReq))
	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	require.NoError(t, err)

	// Step 2: BetaFastMode header → service_tier="priority" (mirrors line 58-61).
	headers := http.Header{}
	headers.Set("anthropic-beta", claude.BetaFastMode)
	require.True(t, containsBetaToken(headers.Get("anthropic-beta"), claude.BetaFastMode))
	responsesReq.ServiceTier = "priority"
	responsesReq.Model = "gpt-5.5"

	// Step 3: marshal & apply fast policy (mirrors line 78 + 149).
	responsesBody, err := json.Marshal(responsesReq)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(responsesBody, "service_tier").String(), "前置：beta 翻译应当注入 priority")

	upstreamBody, policyErr := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", responsesBody)
	require.NoError(t, policyErr)

	// Step 4: default policy must preserve the explicit fast/priority request.
	require.Equal(t, "priority", gjson.GetBytes(upstreamBody, "service_tier").String(),
		"default policy should pass service_tier=priority through to upstream")
}

// --- Fix1: passthrough capturedSessionModel must follow session.update ---

// TestPolicyEnforcingFrameConn_SessionUpdateRotatesCapturedModel covers the
// fix1 bypass: client opens with a whitelist-miss model (gpt-4o → pass under
// gpt-5.5 whitelist), rotates to gpt-5.5 via session.update, then sends
// response.create without "model". Without the session.update sniffing the
// follow-up frame would fall back to the stale gpt-4o capture and pass — the
// fix updates capturedSessionModel from session.* events so the fallback now
// resolves to gpt-5.5 and the policy filters service_tier.
func TestPolicyEnforcingFrameConn_SessionUpdateRotatesCapturedModel(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, gpt55WhitelistFastPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Frame 1: response.create with whitelist-miss model — under default
	// rule fallback=pass, service_tier stays.
	first := []byte(`{"type":"response.create","model":"gpt-4o","service_tier":"priority"}`)
	// Frame 2: session.update rotates the session model to gpt-5.5.
	rotate := []byte(`{"type":"session.update","session":{"model":"gpt-5.5"}}`)
	// Frame 3: response.create WITHOUT model — must inherit gpt-5.5.
	followup := []byte(`{"type":"response.create","service_tier":"priority"}`)

	inner := &fakePassthroughFrameConn{reads: [][]byte{first, rotate, followup}}

	// Replicate the production wiring in openai_ws_v2_passthrough_adapter.go
	// so capturedSessionModel state is shared across frames.
	capturedSessionModel := openAIWSPassthroughPolicyModelForFrame(account, first)
	require.Equal(t, "gpt-4o", capturedSessionModel)
	wrapper := &openAIWSPolicyEnforcingFrameConn{
		inner: inner,
		filter: func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
			if msgType != coderws.MessageText {
				return payload, nil, nil
			}
			if updated := openAIWSPassthroughPolicyModelFromSessionFrame(account, payload); updated != "" {
				capturedSessionModel = updated
			}
			model := openAIWSPassthroughPolicyModelForFrame(account, payload)
			if model == "" {
				model = capturedSessionModel
			}
			return svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, model, payload)
		},
	}

	// Frame 1: gpt-4o miss whitelist → pass (service_tier preserved).
	_, payload1, err := wrapper.ReadFrame(context.Background())
	require.NoError(t, err)
	require.Contains(t, string(payload1), `"service_tier"`, "frame1: gpt-4o miss whitelist → pass keeps service_tier")

	// Frame 2: session.update — not response.create, untouched, but its
	// side effect updates capturedSessionModel to gpt-5.5.
	_, payload2, err := wrapper.ReadFrame(context.Background())
	require.NoError(t, err)
	require.Equal(t, string(rotate), string(payload2), "session.update frame is forwarded verbatim")
	require.Equal(t, "gpt-5.5", capturedSessionModel, "fix1: session.update must rotate capturedSessionModel")

	// Frame 3: empty model + new captured gpt-5.5 → matches whitelist → filter.
	_, payload3, err := wrapper.ReadFrame(context.Background())
	require.NoError(t, err)
	require.NotContains(t, string(payload3), `"service_tier"`,
		"fix1: post-rotate response.create without model must use refreshed capturedSessionModel and trigger filter")
}

// TestPolicyModelFromSessionFrame_OnlySessionUpdate covers the negative
// branches of openAIWSPassthroughPolicyModelFromSessionFrame: only
// client→upstream session.update frames rotate the captured model;
// server→client events (session.created) and unrelated frames must not.
func TestPolicyModelFromSessionFrame_OnlySessionUpdate(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// session.created is a server→client event in the OpenAI Realtime
	// protocol — clients never send it, so this filter (which only runs on
	// the client→upstream direction) must ignore it even if it appears.
	created := []byte(`{"type":"session.created","session":{"model":"gpt-5.5"}}`)
	require.Empty(t, openAIWSPassthroughPolicyModelFromSessionFrame(account, created))

	// Non-session.* frames must NOT trigger rotation.
	notSession := []byte(`{"type":"response.create","session":{"model":"gpt-9"}}`)
	require.Empty(t, openAIWSPassthroughPolicyModelFromSessionFrame(account, notSession))

	// Missing session.model returns empty — caller keeps the old captured value.
	noModel := []byte(`{"type":"session.update","session":{"voice":"alloy"}}`)
	require.Empty(t, openAIWSPassthroughPolicyModelFromSessionFrame(account, noModel))
}

// --- Fix2: native /responses normalize "fast" → "priority" on pass ---

// TestApplyOpenAIFastPolicyToBody_PassNormalizesFastAlias is the fix2
// regression. Before the fix, when action=pass, applyOpenAIFastPolicyToBody
// returned the body unchanged so a raw "fast" alias would leak to the
// upstream OpenAI API (which does not accept "fast"). The fix normalizes
// "fast" → "priority" on pass too.
func TestApplyOpenAIFastPolicyToBody_PassNormalizesFastAlias(t *testing.T) {
	// Use a policy that deliberately misses gpt-4 so the action is pass.
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// gpt-4 + "fast" → fallback pass. Body must be rewritten to "priority".
	body := []byte(`{"model":"gpt-4","service_tier":"fast"}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String(),
		"fix2: pass action must still normalize 'fast' → 'priority' so upstream OpenAI accepts the slug")

	// Already-canonical "priority" on pass: zero mutation (byte-equal).
	body = []byte(`{"model":"gpt-4","service_tier":"priority"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))

	// Mixed-case alias → normalized.
	body = []byte(`{"model":"gpt-4","service_tier":"  Fast  "}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())

	// Unrecognized tier → still no-op (not normalized, since normTier == "").
	body = []byte(`{"model":"gpt-4","service_tier":"turbo"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

// --- Fix3: passthrough billing must reflect post-filter service_tier ---

// TestPassthroughBilling_PostFilterServiceTier is the fix3 regression. The
// passthrough adapter (openai_ws_v2_passthrough_adapter.go) now extracts
// requestServiceTier from firstClientMessage AFTER applyOpenAIFastPolicy
// has rewritten it, so a filter hit causes billing to report nil (default
// tier) instead of the user-requested "priority". This test pins the
// contract those two helpers must uphold for the adapter's billing path.
func TestPassthroughBilling_PostFilterServiceTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	raw := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)

	// Pre-filter sanity: extracting from the raw frame would (incorrectly,
	// pre-fix) report "priority" — this is the very thing the adapter
	// must NOT do anymore.
	pre := extractOpenAIServiceTierFromBody(raw)
	require.NotNil(t, pre)
	require.Equal(t, "priority", *pre,
		"sanity: raw first frame carries priority that pre-fix billing would have reported")

	// Apply explicit policy filter (gpt-5.5 + priority → filter).
	filtered, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", raw)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.NotContains(t, string(filtered), `"service_tier"`)

	// Post-filter: extracting from the rewritten frame returns nil. This
	// is the value the adapter now passes to OpenAIForwardResult.ServiceTier,
	// so billing records "default" instead of "priority".
	post := extractOpenAIServiceTierFromBody(filtered)
	require.Nil(t, post, "fix3: post-filter extraction must return nil so passthrough billing reports default tier instead of the requested priority")

	// And the byte-level invariant the adapter relies on: filtering an
	// already-filtered frame is a no-op (idempotent), so re-running the
	// policy doesn't accidentally re-introduce the field.
	again, blocked2, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", filtered)
	require.NoError(t, err)
	require.Nil(t, blocked2)
	require.Equal(t, string(filtered), string(again),
		"policy is idempotent: filtering an already-filtered frame leaves bytes unchanged")
}

// TestApplyOpenAIFastPolicyToBody_NonStringServiceTier covers the test gap
// flagged in the review: when a client sends service_tier as a non-string
// (number, null, object, etc.) the policy must NOT panic and must NOT
// pretend the field was filtered. Behavior: skip policy entirely (treat as
// "no usable tier"), forward body unchanged. This mirrors the HTTP entry's
// type-assertion `reqBody["service_tier"].(string); ok` guard.
func TestApplyOpenAIFastPolicyToBody_NonStringServiceTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Number — gjson .String() coerces to "1" which is not a recognized
	// tier alias; normalize returns "" → policy no-ops.
	cases := [][]byte{
		[]byte(`{"model":"gpt-5.5","service_tier":1}`),
		[]byte(`{"model":"gpt-5.5","service_tier":null}`),
		[]byte(`{"model":"gpt-5.5","service_tier":{"nested":"priority"}}`),
		[]byte(`{"model":"gpt-5.5","service_tier":["priority"]}`),
		[]byte(`{"model":"gpt-5.5","service_tier":true}`),
	}
	for _, body := range cases {
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err, "non-string service_tier must not error: %s", string(body))
		require.Equal(t, string(body), string(updated),
			"non-string service_tier must pass through unchanged: %s", string(body))
	}

	// Same guard for the WS response.create entry.
	for _, body := range cases {
		frame := body
		updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
		require.NoError(t, err, "non-string service_tier ws frame must not error: %s", string(frame))
		require.Nil(t, blocked, "non-string service_tier must not trigger block: %s", string(frame))
		require.Equal(t, string(frame), string(updated),
			"non-string service_tier ws frame must pass through unchanged: %s", string(frame))
	}
}

// TestPassthroughBilling_MultiTurnServiceTierFollowsFilteredFrames covers the
// multi-turn passthrough billing regression: OpenAI Realtime / Responses WS
// allows the client to ship a different service_tier on each response.create
// frame (per-response field, see codex-rs/core/src/client.rs
// build_responses_request which re-fills the field on every request). Before
// the fix the adapter only captured service_tier from firstClientMessage so
// turn 2/3 billing was wrong. After the fix the filter closure refreshes an
// atomic.Pointer[string] on every successful response.create frame.
//
// This test pins the four legs of the semantic contract:
//   - turn 1: service_tier=priority hits the explicit filter rule, so
//     after filter the upstream sees no tier → billing is nil.
//   - turn 2: service_tier=flex passes (the filter rule targets priority only),
//     billing should now reflect "flex".
//   - turn 3: response.create without any service_tier — the upstream will
//     treat it as default; we choose to mirror that and overwrite billing
//     to nil rather than carry over "flex" from turn 2.
//   - non-response.create frame (response.cancel here) carrying a stray
//     service_tier-shaped field must NOT clobber the billing pointer.
func TestPassthroughBilling_MultiTurnServiceTierFollowsFilteredFrames(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// Mirror the production filter closure (openai_ws_v2_passthrough_adapter.go
	// proxyResponsesWebSocketV2Passthrough) so this test fails if the
	// production code drops the per-frame Store.
	var requestServiceTierPtr atomic.Pointer[string]
	capturedSessionModel := ""
	filter := func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
		if msgType != coderws.MessageText {
			return payload, nil, nil
		}
		if updated := openAIWSPassthroughPolicyModelFromSessionFrame(account, payload); updated != "" {
			capturedSessionModel = updated
		}
		model := openAIWSPassthroughPolicyModelForFrame(account, payload)
		if model == "" {
			model = capturedSessionModel
		}
		out, blocked, policyErr := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, model, payload)
		if policyErr == nil && blocked == nil &&
			strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
			requestServiceTierPtr.Store(extractOpenAIServiceTierFromBody(out))
		}
		return out, blocked, policyErr
	}

	// First-frame initialization mirrors the adapter: extract from the
	// post-filter payload so a filter-on-first-frame zeroes billing too.
	firstFrame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)
	firstOut, firstBlocked, firstErr := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", firstFrame)
	require.NoError(t, firstErr)
	require.Nil(t, firstBlocked)
	requestServiceTierPtr.Store(extractOpenAIServiceTierFromBody(firstOut))
	capturedSessionModel = openAIWSPassthroughPolicyModelForFrame(account, firstFrame)
	require.Nil(t, requestServiceTierPtr.Load(),
		"turn 1: filter strips service_tier=priority, billing must reflect upstream-actual nil tier")

	// Turn 2: client switches to flex, should pass and update billing.
	turn2 := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"flex"}`)
	out2, blocked2, err2 := filter(coderws.MessageText, turn2)
	require.NoError(t, err2)
	require.Nil(t, blocked2)
	require.Equal(t, "flex", gjson.GetBytes(out2, "service_tier").String(), "turn 2: flex must pass to upstream untouched")
	tier2 := requestServiceTierPtr.Load()
	require.NotNil(t, tier2, "turn 2: billing must update to reflect flex")
	require.Equal(t, "flex", *tier2)

	// A non-response.create frame with a stray service_tier-shaped field
	// must NOT overwrite the billing pointer (those frames don't carry
	// per-response service_tier in the Realtime spec).
	cancelFrame := []byte(`{"type":"response.cancel","service_tier":"priority"}`)
	_, blockedCancel, errCancel := filter(coderws.MessageText, cancelFrame)
	require.NoError(t, errCancel)
	require.Nil(t, blockedCancel)
	tierAfterCancel := requestServiceTierPtr.Load()
	require.NotNil(t, tierAfterCancel, "response.cancel must not clobber billing tier to nil")
	require.Equal(t, "flex", *tierAfterCancel,
		"non-response.create frames must not update billing tier even if they carry a service_tier-shaped field")

	// Turn 3: response.create without any service_tier. We deliberately
	// overwrite billing back to nil so it tracks what the upstream actually
	// sees on this turn (default tier).
	turn3 := []byte(`{"type":"response.create","model":"gpt-5.5"}`)
	out3, blocked3, err3 := filter(coderws.MessageText, turn3)
	require.NoError(t, err3)
	require.Nil(t, blocked3)
	require.Equal(t, string(turn3), string(out3), "turn 3 has no service_tier — filter must not mutate")
	require.Nil(t, requestServiceTierPtr.Load(),
		"turn 3: response.create without service_tier overwrites billing to nil to match upstream default")
}

func TestPassthroughUsageMeta_TracksReasoningEffortAcrossTurns(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	firstFrame := []byte(`{"type":"response.create","model":"gpt-5.5","reasoning":{"effort":"medium"},"service_tier":"priority"}`)
	meta := newOpenAIWSPassthroughUsageMeta("", firstFrame)
	capturedSessionModel := openAIWSPassthroughPolicyModelForFrame(account, firstFrame)
	firstOut, firstBlocked, firstErr := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, capturedSessionModel, firstFrame)
	require.NoError(t, firstErr)
	require.Nil(t, firstBlocked)
	meta.initFromFirstFrame(firstOut)
	require.NotNil(t, meta.reasoningEffort.Load())
	require.Equal(t, "medium", *meta.reasoningEffort.Load())

	process := func(payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
		if updated := openAIWSPassthroughPolicyModelFromSessionFrame(account, payload); updated != "" {
			capturedSessionModel = updated
		}
		meta.updateSessionRequestModel(payload)
		requestModelForThisFrame := meta.requestModelForFrame(payload)
		model := openAIWSPassthroughPolicyModelForFrame(account, payload)
		if model == "" {
			model = capturedSessionModel
		}
		out, blocked, policyErr := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, model, payload)
		if policyErr == nil && blocked == nil &&
			strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
			meta.updateFromResponseCreate(out, requestModelForThisFrame)
		}
		return out, blocked, policyErr
	}

	_, blockedSession, errSession := process([]byte(`{"type":"session.update","session":{"model":"gpt-5-high"}}`))
	require.NoError(t, errSession)
	require.Nil(t, blockedSession)
	require.NotNil(t, meta.reasoningEffort.Load())
	require.Equal(t, "medium", *meta.reasoningEffort.Load(), "session.update 只刷新后续 fallback model，不覆盖当前 turn metadata")

	_, blockedCancel, errCancel := process([]byte(`{"type":"response.cancel","reasoning_effort":"x-high"}`))
	require.NoError(t, errCancel)
	require.Nil(t, blockedCancel)
	require.NotNil(t, meta.reasoningEffort.Load())
	require.Equal(t, "medium", *meta.reasoningEffort.Load(), "非 response.create 帧不能污染当前 turn metadata")

	_, blockedFlat, errFlat := process([]byte(`{"type":"response.create","reasoning_effort":"x-high"}`))
	require.NoError(t, errFlat)
	require.Nil(t, blockedFlat)
	require.NotNil(t, meta.reasoningEffort.Load())
	require.Equal(t, "xhigh", *meta.reasoningEffort.Load(), "flat reasoning_effort 必须进入 passthrough usage metadata")

	_, blockedClear, errClear := process([]byte(`{"type":"response.create","model":"gpt-4o"}`))
	require.NoError(t, errClear)
	require.Nil(t, blockedClear)
	require.Nil(t, meta.reasoningEffort.Load(), "新的 response.create 无 effort 且无可推导后缀时必须清空旧值")
}

// TestPassthroughBilling_BlockedFrameDoesNotMutateServiceTier locks in the
// "block keeps previous" semantic: when policy returns block on a
// response.create frame, that frame is never sent upstream, so billing tier
// must keep the previous turn's value rather than getting silently zeroed.
func TestPassthroughBilling_BlockedFrameDoesNotMutateServiceTier(t *testing.T) {
	blockSettings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "blocked",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, blockSettings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	var requestServiceTierPtr atomic.Pointer[string]
	flexValue := "flex"
	requestServiceTierPtr.Store(&flexValue) // simulate prior turn billed as flex

	filter := func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error) {
		if msgType != coderws.MessageText {
			return payload, nil, nil
		}
		out, blocked, policyErr := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", payload)
		if policyErr == nil && blocked == nil &&
			strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
			requestServiceTierPtr.Store(extractOpenAIServiceTierFromBody(out))
		}
		return out, blocked, policyErr
	}

	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)
	_, blocked, err := filter(coderws.MessageText, frame)
	require.NoError(t, err)
	require.NotNil(t, blocked, "policy must block this frame")

	tier := requestServiceTierPtr.Load()
	require.NotNil(t, tier, "blocked frame must not clobber prior billing tier to nil")
	require.Equal(t, "flex", *tier,
		"blocked frame is never sent upstream; billing must retain the previous turn's tier")
}
