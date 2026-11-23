package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const wsIdentityWarmup = `{"type":"response.create","model":"gpt-5.1","generate":false,"input":[{"role":"user","content":"synthetic prefix"}],"client_metadata":{"session_id":"fixture-session","turn_id":"warmup-turn","marker":"untouched"}}`
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
	account.Credentials = nil
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
	server, errCh := startPassthroughLifecycleServer(t, t.Context(), f.svc, f.account)
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
