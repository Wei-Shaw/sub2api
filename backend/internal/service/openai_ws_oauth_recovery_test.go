//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIOAuthStagedConn struct{ *stagedPassthroughConn }

func (c *openAIOAuthStagedConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

type openAIOAuthRecoveryDialer struct {
	mu          sync.Mutex
	headers     []http.Header
	conn        openAIWSClientConn
	rejectAgain bool
	body        string
}

func (d *openAIOAuthRecoveryDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.headers = append(d.headers, headers.Clone())
	if len(d.headers) == 1 || d.rejectAgain {
		body := d.body
		if body == "" {
			body = `{"error":{"code":"invalid_api_key","message":"token expired"}}`
		}
		return nil, http.StatusUnauthorized, nil, &openAIWSHandshakeError{Body: []byte(body), Err: errors.New("synthetic unauthorized handshake")}
	}
	return d.conn, http.StatusSwitchingProtocols, http.Header{}, nil
}

func installOpenAIOAuthRecovery(t *testing.T, svc *OpenAIGatewayService, account *Account) *refreshAPIExecutorStub {
	t.Helper()
	repo := &refreshAPIAccountRepo{account: account}
	cache := newOpenAITokenCacheStub()
	executor := &refreshAPIExecutorStub{credentials: map[string]any{
		"access_token":  "fresh-at",
		"refresh_token": "rotated-rt",
		"expires_at":    time.Now().Add(time.Hour).Format(time.RFC3339),
	}}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
	svc.openAITokenProvider = provider
	return executor
}

func TestOpenAIOAuth401WSHandshakeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		again, noRT      bool
		dials, refreshes int
	}{
		{name: "success", dials: 2, refreshes: 1},
		{name: "second_401", again: true, dials: 2, refreshes: 1},
		{name: "revoked", body: `{"error":{"code":"token_revoked"}}`, dials: 1},
		{name: "scope", body: `{"error":{"code":"insufficient_scope"}}`, dials: 1},
		{name: "missing_refresh_token", noRT: true, dials: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := passthroughLifecycleConfig()
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			conn := newStagedPassthroughConn()
			dialer := &openAIOAuthRecoveryDialer{conn: conn, rejectAgain: tc.again, body: tc.body}
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool}
			account := newOpenAIOAuthRecoveryAccount()
			if tc.noRT {
				delete(account.Credentials, "refresh_token")
			}
			executor := installOpenAIOAuthRecovery(t, svc, account)
			headers := http.Header{"Authorization": []string{"Bearer rejected-at"}, "Session_id": []string{"synthetic-session"}}
			req := openAIWSAcquireRequest{Account: account, WSURL: "wss://synthetic.invalid/responses", Headers: headers}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			lease, err := svc.acquireOpenAIWSWithOAuthRecovery(ctx, &req)
			if tc.name == "success" {
				require.NoError(t, err)
				lease.MarkBroken()
				lease.Release()
			} else {
				require.Error(t, err)
			}
			require.Len(t, dialer.headers, tc.dials)
			require.Equal(t, tc.refreshes, executor.refreshCalls)
			require.Equal(t, "Bearer rejected-at", headers.Get("Authorization"), "original headers must remain immutable")
			if tc.dials == 2 {
				require.Equal(t, "Bearer fresh-at", dialer.headers[1].Get("Authorization"))
				require.Equal(t, headers.Get("Session_id"), dialer.headers[1].Get("Session_id"))
			}
			require.Empty(t, conn.writes, "authentication recovery must not send or replay generation frames")
		})
	}
}

func TestOpenAIOAuth401WSIngressRecovery(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
			upstream := newStagedPassthroughConn()
			svc := newPassthroughLifecycleService(cfg, upstream)
			dialer := &openAIOAuthRecoveryDialer{conn: &openAIOAuthStagedConn{upstream}}
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(dialer)
			svc.openaiWSPool = pool
			svc.openaiWSPassthroughDialer = dialer
			account := newOpenAIOAuthRecoveryAccount()
			account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": mode}
			executor := installOpenAIOAuthRecovery(t, svc, account)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
			defer server.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			frame := []byte(`{"type":"response.create","model":"gpt-5.4","input":[],"store":false}`)
			require.NoError(t, client.Write(ctx, coderws.MessageText, frame))
			select {
			case written := <-upstream.writes:
				require.Equal(t, "response.create", gjson.GetBytes(written, "type").String())
				require.Equal(t, "gpt-5.4", gjson.GetBytes(written, "model").String())
			case <-ctx.Done():
				t.Fatal("no generation frame after recovered handshake")
			}
			upstream.Send(`{"type":"response.completed","response":{"id":"resp_synthetic","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`)
			_, payload, err := client.Read(ctx)
			require.NoError(t, err)
			require.Contains(t, string(payload), "response.completed")
			_ = client.CloseNow()
			cancel()
			select {
			case <-serverErr:
			case <-time.After(time.Second):
				t.Fatal("relay did not stop")
			}
			dialer.mu.Lock()
			headers := append([]http.Header(nil), dialer.headers...)
			dialer.mu.Unlock()
			require.Len(t, headers, 2)
			require.Equal(t, "Bearer fresh-at", headers[1].Get("Authorization"))
			require.Equal(t, 1, executor.refreshCalls)
			require.Empty(t, upstream.writes, "generation must be sent once")
		})
	}
}

func TestOpenAIOAuth401ForwardWSV2Recovery(t *testing.T) {
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	conn := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_synthetic","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`)}}
	dialer := &openAIOAuthRecoveryDialer{conn: conn}
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(dialer)
	svc := newPassthroughLifecycleService(cfg, newStagedPassthroughConn())
	svc.openaiWSPool = pool
	account := newOpenAIOAuthRecoveryAccount()
	account.Extra = map[string]any{"responses_websockets_v2_enabled": true, "openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool}
	executor := installOpenAIOAuthRecovery(t, svc, account)
	body := []byte(`{"model":"gpt-5.4","instructions":"synthetic","stream":false,"input":[{"role":"user","content":"synthetic"}]}`)
	c := newOpenAIRejectedFieldTestContext(body)
	SetOpenAIClientTransport(c, OpenAIClientTransportWS)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, dialer.headers, 2)
	require.Equal(t, "Bearer rejected-at", dialer.headers[0].Get("Authorization"))
	require.Equal(t, "Bearer fresh-at", dialer.headers[1].Get("Authorization"))
	require.Equal(t, 1, executor.refreshCalls)
	conn.mu.Lock()
	defer conn.mu.Unlock()
	require.Len(t, conn.writes, 1, "handshake retry must send generation once")
}

func TestOpenAIOAuth401WSIngressStopsAfterSecondRejection(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			upstream := newStagedPassthroughConn()
			svc := newPassthroughLifecycleService(cfg, upstream)
			dialer := &openAIOAuthRecoveryDialer{conn: upstream, rejectAgain: true}
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(dialer)
			svc.openaiWSPool = pool
			svc.openaiWSPassthroughDialer = dialer
			account := newOpenAIOAuthRecoveryAccount()
			account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": mode}
			executor := installOpenAIOAuthRecovery(t, svc, account)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
			defer server.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":[],"store":false}`)))
			select {
			case err := <-serverErr:
				require.Error(t, err)
			case <-ctx.Done():
				t.Fatal("rejection did not terminate handshake")
			}
			require.Len(t, dialer.headers, 2)
			require.Equal(t, 1, executor.refreshCalls)
			require.Empty(t, upstream.writes)
		})
	}
}

func TestOpenAIOAuth401WSStrictAffinityIsPreserved(t *testing.T) {
	cfg := passthroughLifecycleConfig()
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &openAIOAuthRecoveryDialer{}
	pool.setClientDialerForTest(dialer)
	svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool}
	account := newOpenAIOAuthRecoveryAccount()
	executor := installOpenAIOAuthRecovery(t, svc, account)
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://synthetic.invalid/responses", Headers: http.Header{"Authorization": []string{"Bearer rejected-at"}}, PreferredConnID: "missing-previous-connection", ForcePreferredConn: true}
	_, err := svc.acquireOpenAIWSWithOAuthRecovery(context.Background(), &req)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)
	require.Empty(t, dialer.headers, "must not bypass continuation affinity to open a different connection")
	require.Zero(t, executor.refreshCalls)
}
