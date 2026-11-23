package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc"
)

const wsIdentityWarmup = `{"type":"response.create","model":"gpt-5.1","generate":false,"input":[{"role":"user","content":"synthetic prefix"}],"client_metadata":{"session_id":"fixture-session","turn_id":"warmup-turn","parent_thread_id":"fixture-parent","marker":"untouched"}}`
const wsIdentityContinuation = `{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_native_1","input":[],"client_metadata":{"session_id":"fixture-session","turn_id":"next-turn","marker":"untouched"}}`

type wsIdentityFixtureDialer struct {
	server *httptest.Server
}

func (d wsIdentityFixtureDialer) Dial(ctx context.Context, target string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
	if !strings.HasPrefix(target, "wss://") || proxyURL != "" {
		return nil, 0, nil, errors.New("expected native WSS on the synthetic route")
	}
	conn, response, err := coderws.Dial(ctx, "wss"+strings.TrimPrefix(d.server.URL, "https"), &coderws.DialOptions{
		HTTPClient: d.server.Client(), HTTPHeader: headers,
	})
	if err != nil {
		return nil, 0, nil, err
	}
	return &coderOpenAIWSClientConn{conn: conn}, response.StatusCode, response.Header, nil
}

type wsIdentityWireFixture struct {
	svc        *OpenAIGatewayService
	cfg        *config.Config
	account    *Account
	http       *httpUpstreamRecorder
	handshakes chan http.Header
	frames     chan []byte
}

func newWSIdentityWireFixture(t *testing.T) *wsIdentityWireFixture {
	t.Helper()
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 3
	account := passthroughLifecycleAccount()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"chatgpt_account_id": "synthetic-oauth-account"}
	account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough}
	fixture := &wsIdentityWireFixture{
		cfg: cfg, account: account,
		http:       &httpUpstreamRecorder{err: errors.New("unexpected HTTP bridge")},
		handshakes: make(chan http.Header, 4), frames: make(chan []byte, 4),
	}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		fixture.handshakes <- r.Header.Clone()
		for turn := 1; turn <= 2; turn++ {
			_, payload, err := conn.Read(ctx)
			if err != nil {
				return
			}
			fixture.frames <- payload
			terminal := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_native_%d","model":"gpt-5.1","output":[],"usage":{"input_tokens":0,"output_tokens":0}}}`, turn)
			if err := conn.Write(ctx, coderws.MessageText, []byte(terminal)); err != nil {
				return
			}
		}
		_, _, _ = conn.Read(ctx)
	}))
	t.Cleanup(upstream.Close)
	fixture.svc = newPassthroughLifecycleService(cfg, nil)
	fixture.svc.httpUpstream = fixture.http
	fixture.svc.openaiWSPassthroughDialer = wsIdentityFixtureDialer{server: upstream}
	return fixture
}

func (f *wsIdentityWireFixture) start(t *testing.T) (*coderws.Conn, <-chan error) {
	t.Helper()
	return f.startForKey(t, 101)
}

func (f *wsIdentityWireFixture) startForKey(t *testing.T, keyID int64) (*coderws.Conn, <-chan error) {
	t.Helper()
	server, errCh := startPassthroughLifecycleServerWithHooks(t, t.Context(), f.svc, f.account, func(c *gin.Context) *OpenAIWSIngressHooks {
		c.Set("api_key", &APIKey{ID: keyID})
		return nil
	})
	t.Cleanup(server.Close)
	client := dialPassthroughLifecycleClientWithPayload(t, server, wsIdentityWarmup)
	t.Cleanup(func() { _ = client.CloseNow() })
	return client, errCh
}

func waitWSIdentityFixtureValue[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the synthetic WebSocket")
		var zero T
		return zero
	}
}

func requireWSIdentityCompletion(t *testing.T, client *coderws.Conn, id string) {
	t.Helper()
	payload, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(payload, "type").String())
	require.Equal(t, id, gjson.GetBytes(payload, "response.id").String())
}

func TestResponsesWSIdentity_NativeOAuthBaseline(t *testing.T) {
	fixture := newWSIdentityWireFixture(t)
	client, errCh := fixture.start(t)
	requireWSIdentityCompletion(t, client, "resp_native_1")
	headers := waitWSIdentityFixtureValue(t, fixture.handshakes)
	require.Equal(t, "Bearer sk-test", headers.Get("Authorization"))
	require.NotEmpty(t, headers.Get("OpenAI-Beta"))
	warmup := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, gjson.False, gjson.GetBytes(warmup, "generate").Type)
	require.Equal(t, "synthetic prefix", gjson.GetBytes(warmup, "input.0.content").String())

	require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
	requireWSIdentityCompletion(t, client, "resp_native_2")
	continuation := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, "resp_native_1", gjson.GetBytes(continuation, "previous_response_id").String())
	require.JSONEq(t, "[]", gjson.GetBytes(continuation, "input").Raw)
	require.Empty(t, fixture.handshakes, "prewarm and continuation must share one upstream connection")
	_ = client.Close(coderws.StatusNormalClosure, "done")
	_ = waitWSIdentityFixtureValue(t, errCh)
	require.Empty(t, fixture.http.requests)
}

type wsIdentityRPCFunc func(context.Context, *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error)

func (f wsIdentityRPCFunc) RewriteWSIdentity(ctx context.Context, req *pluginv1.RewriteWSIdentityRequest, _ ...grpc.CallOption) (*pluginv1.RewriteWSIdentityResponse, error) {
	return f(ctx, req)
}

type wsIdentityHTTPPlugin struct {
	pluginv1.TransportPluginClient
	calls atomic.Int32
}

func (p *wsIdentityHTTPPlugin) Forward(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[pluginv1.ForwardRequest, pluginv1.ForwardResponse], error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic HTTP plugin reached")
}

func (f *wsIdentityWireFixture) addPlugin(rewriter pluginv1.WSIdentityPluginClient, rollout int) (*pluginRuntime, *wsIdentityHTTPPlugin) {
	httpPlugin := &wsIdentityHTTPPlugin{}
	runtime := &pluginRuntime{client: &hcplugin.Client{}, api: httpPlugin, wsIdentity: rewriter}
	f.svc.pluginManager = &PluginManager{}
	f.svc.pluginManager.route.Store(&pluginRoute{runtime: runtime, rolloutPercent: rollout})
	return runtime, httpPlugin
}

func TestResponsesWSIdentity_RewritesOnlyAuthorizedFields(t *testing.T) {
	fixture := newWSIdentityWireFixture(t)
	fixture.cfg.Plugins.OpenAIWSIdentityEnabled = true
	requests := make(chan *pluginv1.RewriteWSIdentityRequest, 2)
	_, legacy := fixture.addPlugin(wsIdentityRPCFunc(func(_ context.Context, req *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
		requests <- req
		if req.Handshake != nil {
			return &pluginv1.RewriteWSIdentityResponse{
				Handshake:      &pluginv1.WSIdentityMetadata{Fields: map[string]string{"user-agent": "SyntheticCodex/2.0", "originator": "synthetic-codex"}},
				ResponseCreate: &pluginv1.WSIdentityMetadata{Fields: map[string]string{"session_id": "mapped-session", "turn_id": "mapped-warmup"}},
			}, nil
		}
		return &pluginv1.RewriteWSIdentityResponse{
			ResponseCreate: &pluginv1.WSIdentityMetadata{
				Fields:       map[string]string{"turn_id": "mapped-next"},
				TurnMetadata: map[string]string{"turn_id": "mapped-next"},
			},
		}, nil
	}), 100)
	client, errCh := fixture.start(t)
	requireWSIdentityCompletion(t, client, "resp_native_1")
	initial := waitWSIdentityFixtureValue(t, requests)
	require.Equal(t, fixture.account.ID, initial.AccountId)
	require.NotEmpty(t, initial.ConnectionId)
	require.NotContains(t, initial.Handshake.Fields, "authorization")
	require.NotContains(t, initial.Handshake.Fields, "openai-beta")
	require.NotContains(t, initial.ResponseCreate.Fields, "marker")
	require.Equal(t, scopeCodexAccountIdentityValue(fixture.account, 101, "turn", "warmup-turn"), initial.ResponseCreate.Fields["turn_id"])
	require.Equal(t, scopeCodexAccountIdentityValue(fixture.account, 101, "thread", "fixture-parent"), initial.ResponseCreate.Fields["parent_thread_id"])
	headers := waitWSIdentityFixtureValue(t, fixture.handshakes)
	require.Equal(t, "Bearer sk-test", headers.Get("Authorization"))
	require.NotEmpty(t, headers.Get("OpenAI-Beta"))
	require.Equal(t, "SyntheticCodex/2.0", headers.Get("User-Agent"))
	require.Equal(t, "synthetic-codex", headers.Get("Originator"))
	warmup := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, gjson.False, gjson.GetBytes(warmup, "generate").Type)
	require.Equal(t, "synthetic prefix", gjson.GetBytes(warmup, "input.0.content").String())
	require.Equal(t, "mapped-session", gjson.GetBytes(warmup, "client_metadata.session_id").String())
	require.Equal(t, "mapped-warmup", gjson.GetBytes(warmup, "client_metadata.turn_id").String())

	continuation := `{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_native_1","input":[],"prompt_cache_key":"independent-cache-key","client_metadata":{"session_id":"fixture-session","turn_id":"next-turn","marker":"untouched","x-codex-turn-metadata":"{\"turn_id\":\"next-turn\",\"tool_namespaces_info\":[\"private fixture\"]}"}}`
	require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(continuation)))
	requireWSIdentityCompletion(t, client, "resp_native_2")
	followup := waitWSIdentityFixtureValue(t, requests)
	require.Equal(t, initial.ConnectionId, followup.ConnectionId)
	require.Nil(t, followup.Handshake)
	require.Equal(t, map[string]string{"turn_id": scopeCodexAccountIdentityValue(fixture.account, 101, "turn", "next-turn")}, followup.ResponseCreate.TurnMetadata)
	frame := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, "resp_native_1", gjson.GetBytes(frame, "previous_response_id").String())
	require.JSONEq(t, "[]", gjson.GetBytes(frame, "input").Raw)
	require.Equal(t, scopeCodexAccountIdentityValue(fixture.account, 101, "prompt-cache", "independent-cache-key"), gjson.GetBytes(frame, "prompt_cache_key").String())
	require.Equal(t, "mapped-session", gjson.GetBytes(frame, "client_metadata.session_id").String())
	require.Equal(t, "mapped-next", gjson.GetBytes(frame, "client_metadata.turn_id").String())
	require.Equal(t, "untouched", gjson.GetBytes(frame, "client_metadata.marker").String())
	nested := gjson.GetBytes(frame, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, "mapped-next", gjson.Get(nested, "turn_id").String())
	require.Equal(t, "private fixture", gjson.Get(nested, "tool_namespaces_info.0").String())
	require.Empty(t, fixture.handshakes)
	_ = client.Close(coderws.StatusNormalClosure, "done")
	_ = waitWSIdentityFixtureValue(t, errCh)
	require.Zero(t, legacy.calls.Load())
}

func TestResponsesWSIdentity_RejectsInvalidPatchBeforeDialOrWrite(t *testing.T) {
	for _, followup := range []bool{false, true} {
		t.Run(fmt.Sprintf("followup=%t", followup), func(t *testing.T) {
			fixture := newWSIdentityWireFixture(t)
			fixture.cfg.Plugins.OpenAIWSIdentityEnabled = true
			_, legacy := fixture.addPlugin(wsIdentityRPCFunc(func(_ context.Context, req *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
				if followup && req.Handshake != nil {
					return &pluginv1.RewriteWSIdentityResponse{}, nil
				}
				return &pluginv1.RewriteWSIdentityResponse{
					ResponseCreate: &pluginv1.WSIdentityMetadata{Fields: map[string]string{"previous_response_id": "forbidden"}},
				}, nil
			}), 100)
			client, errCh := fixture.start(t)
			if followup {
				requireWSIdentityCompletion(t, client, "resp_native_1")
				_ = waitWSIdentityFixtureValue(t, fixture.frames)
				_ = waitWSIdentityFixtureValue(t, fixture.handshakes)
				require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
				_, closeErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.Equal(t, coderws.StatusInternalError, coderws.CloseStatus(closeErr))
			}
			err := waitWSIdentityFixtureValue(t, errCh)
			require.ErrorIs(t, err, ErrOpenAIWSIdentityRewrite)
			var failover *UpstreamFailoverError
			require.NotErrorAs(t, err, &failover)
			require.Empty(t, fixture.handshakes)
			require.Empty(t, fixture.frames)
			require.Zero(t, legacy.calls.Load())
		})
	}
}

func TestResponsesWSIdentity_PreservesLegacyBridge(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		enabled, capable, forceHTTP, large, unscoped bool
	}{
		{name: "default off", capable: true},
		{name: "legacy plugin", enabled: true},
		{name: "force HTTP", enabled: true, capable: true, forceHTTP: true},
		{name: "large first message", enabled: true, capable: true, large: true},
		{name: "no credential scope", enabled: true, capable: true, unscoped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newWSIdentityWireFixture(t)
			fixture.cfg.Plugins.OpenAIWSIdentityEnabled = tc.enabled
			fixture.cfg.Gateway.OpenAIWS.ForceHTTP = tc.forceHTTP
			if tc.unscoped {
				fixture.account.Credentials = nil
			}
			if tc.large {
				fixture.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
				fixture.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
			}
			var identityCalls atomic.Int32
			var rewriter pluginv1.WSIdentityPluginClient
			if tc.capable {
				rewriter = wsIdentityRPCFunc(func(context.Context, *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
					identityCalls.Add(1)
					return &pluginv1.RewriteWSIdentityResponse{}, nil
				})
			}
			_, legacy := fixture.addPlugin(rewriter, 100)
			_, errCh := fixture.start(t)
			require.Error(t, waitWSIdentityFixtureValue(t, errCh))
			require.EqualValues(t, 1, legacy.calls.Load())
			require.Zero(t, identityCalls.Load())
			require.Empty(t, fixture.handshakes)
		})
	}
}

func TestResponsesWSIdentity_CanaryScopeKeepsOtherRequestsOnHTTPBridge(t *testing.T) {
	for _, tc := range []struct {
		name             string
		account, key     int64
		flag, wantNative bool
	}{
		{"selected pair", 901, 101, true, true},
		{"other key on the selected account", 901, 102, true, false},
		{"same key on a different account", 902, 101, true, false},
		{"flag off with retained scope", 901, 101, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newWSIdentityWireFixture(t)
			fixture.account.ID = tc.account
			fixture.cfg.Plugins.OpenAIWSIdentityEnabled = tc.flag
			fixture.cfg.Plugins.OpenAIWSIdentityCanaryAccountID = 901
			fixture.cfg.Plugins.OpenAIWSIdentityCanaryAPIKeyID = 101
			var calls atomic.Int32
			_, legacy := fixture.addPlugin(wsIdentityRPCFunc(func(context.Context, *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
				calls.Add(1)
				return &pluginv1.RewriteWSIdentityResponse{}, nil
			}), 100)
			client, done := fixture.startForKey(t, tc.key)
			if !tc.wantNative {
				require.Error(t, waitWSIdentityFixtureValue(t, done))
				require.EqualValues(t, 1, legacy.calls.Load())
				require.Zero(t, calls.Load())
				require.Empty(t, fixture.handshakes)
				return
			}
			requireWSIdentityCompletion(t, client, "resp_native_1")
			_ = waitWSIdentityFixtureValue(t, fixture.handshakes)
			_ = waitWSIdentityFixtureValue(t, fixture.frames)
			require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
			requireWSIdentityCompletion(t, client, "resp_native_2")
			frame := waitWSIdentityFixtureValue(t, fixture.frames)
			require.Equal(t, "resp_native_1", gjson.GetBytes(frame, "previous_response_id").String())
			require.Empty(t, fixture.handshakes)
			require.EqualValues(t, 2, calls.Load())
			require.Zero(t, legacy.calls.Load())
			_ = client.Close(coderws.StatusNormalClosure, "done")
			_ = waitWSIdentityFixtureValue(t, done)
		})
	}
}

func TestResponsesWSIdentity_ReceivesHostScopedMetadata(t *testing.T) {
	seen := map[string]bool{}
	for _, scope := range []struct {
		keyID     int64
		accountID string
	}{
		{11, "synthetic-oauth-a"}, {12, "synthetic-oauth-a"}, {11, "synthetic-oauth-b"},
	} {
		fixture := newWSIdentityWireFixture(t)
		fixture.account.Credentials = map[string]any{"chatgpt_account_id": scope.accountID}
		fixture.cfg.Plugins.OpenAIWSIdentityEnabled = true
		projected := make(chan string, 1)
		fixture.addPlugin(wsIdentityRPCFunc(func(_ context.Context, req *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
			projected <- req.ResponseCreate.GetFields()["session_id"]
			return &pluginv1.RewriteWSIdentityResponse{}, nil
		}), 100)
		client, errCh := fixture.startForKey(t, scope.keyID)
		requireWSIdentityCompletion(t, client, "resp_native_1")
		id := waitWSIdentityFixtureValue(t, projected)
		require.NotEmpty(t, id)
		require.NotEqual(t, "fixture-session", id)
		require.False(t, seen[id], "the plugin must not receive the same ID for different host scopes")
		seen[id] = true
		_ = client.Close(coderws.StatusNormalClosure, "done")
		_ = waitWSIdentityFixtureValue(t, errCh)
	}
}

func TestResponsesWSIdentity_TimeoutIsLocalBeforeDial(t *testing.T) {
	fixture := newWSIdentityWireFixture(t)
	fixture.cfg.Plugins.OpenAIWSIdentityEnabled = true
	runtime, legacy := fixture.addPlugin(wsIdentityRPCFunc(func(ctx context.Context, _ *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}), 100)
	_, errCh := fixture.start(t)
	require.ErrorIs(t, waitWSIdentityFixtureValue(t, errCh), ErrOpenAIWSIdentityRewrite)
	require.Empty(t, fixture.handshakes)
	require.Empty(t, fixture.frames)
	require.Zero(t, legacy.calls.Load())
	require.Zero(t, runtime.inFlight.Load())
}

func TestResponsesWSIdentity_ConfigChangeCannotRewriteActiveConnection(t *testing.T) {
	fixture := newWSIdentityWireFixture(t)
	fixture.cfg.Plugins.OpenAIWSIdentityEnabled = true
	var calls atomic.Int32
	runtime, legacy := fixture.addPlugin(wsIdentityRPCFunc(func(context.Context, *pluginv1.RewriteWSIdentityRequest) (*pluginv1.RewriteWSIdentityResponse, error) {
		calls.Add(1)
		return &pluginv1.RewriteWSIdentityResponse{}, nil
	}), 100)
	client, errCh := fixture.start(t)
	requireWSIdentityCompletion(t, client, "resp_native_1")
	_ = waitWSIdentityFixtureValue(t, fixture.frames)
	runtime.wsConfigEpoch.Add(2)
	require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
	_, closeErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.Equal(t, coderws.StatusInternalError, coderws.CloseStatus(closeErr))
	require.ErrorIs(t, waitWSIdentityFixtureValue(t, errCh), ErrOpenAIWSIdentityRewrite)
	require.EqualValues(t, 1, calls.Load())
	require.Zero(t, legacy.calls.Load())
	require.Empty(t, fixture.frames)
}
