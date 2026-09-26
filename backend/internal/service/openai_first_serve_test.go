package service

import (
	"context"
	"encoding/json"
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

func TestFirstServeFixedExpiry(t *testing.T) {
	now := time.Now()
	s := newOpenAIFirstServeState(&Account{ID: 98701}, "route", now)
	for _, ms := range []int{100, 15000, 15001, 90000} {
		s.observe(ms, now.Add(time.Minute))
		require.False(t, s.due(now.Add(time.Minute)))
		require.Equal(t, now.Add(240*time.Second), s.status.ExpiresAt, "samples do not renew expiry")
	}
	require.False(t, s.due(now.Add(240*time.Second-time.Nanosecond)))
	require.True(t, s.due(now.Add(240*time.Second)))
}

func TestFirstServeReplayPreservesConversationAndTools(t *testing.T) {
	s := &openAIFirstServeState{}
	s.input([]byte(`{"input":"remember 42"}`))
	s.finish("resp_1", []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Remembered."}]}`),
		json.RawMessage(`{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}`),
	}, true)
	payload := []byte(`{"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	replay, ok := s.replay(payload)
	require.True(t, ok)
	require.False(t, gjson.GetBytes(replay, "previous_response_id").Exists())
	require.Equal(t, int64(4), gjson.GetBytes(replay, "input.#").Int())
	require.Equal(t, "remember 42", gjson.GetBytes(replay, "input.0.content").String())
	require.Equal(t, "Remembered.", gjson.GetBytes(replay, "input.1.content.0.text").String())
	_, ok = s.replay([]byte(`{"previous_response_id":"resp_unknown","input":[{"role":"user","content":"next"}]}`))
	require.False(t, ok, "unknown parents must not be silently dropped")
	_, ok = s.replay([]byte(`{"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"missing","output":"ok"}]}`))
	require.False(t, ok, "unmatched tool outputs cannot move to a new connection")
	_, ok = s.replay([]byte(`{"previous_response_id":"resp_1","input":[{"type":"item_reference","id":"remote"}]}`))
	require.False(t, ok, "remote references cannot be replayed locally")
}

func TestFirstServePoolIsolation(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 3
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 3
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&firstServeTestDialer{conns: []openAIWSClientConn{
		&openAIWSCaptureConn{}, &openAIWSCaptureConn{}, &openAIWSCaptureConn{},
	}})
	a := &Account{ID: 98703, Concurrency: 3}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/ws", FirstServeScope: "user-a"}
	first, err := pool.Acquire(ctx, req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()
	req.FirstServeScope = "user-b"
	req.PreferredConnID = firstID
	req.ForcePreferredConn = true
	_, err = pool.Acquire(ctx, req)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)
	req.PreferredConnID = ""
	req.ForcePreferredConn = false
	second, err := pool.Acquire(ctx, req)
	require.NoError(t, err)
	require.NotEqual(t, firstID, second.ConnID())
	second.Release()
	req.FirstServeScope = ""
	normal, err := pool.Acquire(ctx, req)
	require.NoError(t, err)
	require.NotEqual(t, firstID, normal.ConnID(), "ordinary requests cannot borrow first-serve connections")
	normal.Release()
}

func TestFirstServeAccountValidation(t *testing.T) {
	group := int64(1)
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Name: "test", Extra: map[string]any{
		"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe,
	}}
	require.ErrorContains(t, validateOpenAIFirstServeRouting(a), "test")
	a.ProxyGroupID = &group
	require.NoError(t, validateOpenAIFirstServeRouting(a))
	a.Extra["openai_ws_force_http"] = true
	require.NoError(t, validateOpenAIFirstServeRouting(a), "HTTP/SSE first serve does not require WebSocket")
	a.Extra["openai_apikey_responses_websockets_v2_mode"] = OpenAIWSIngressModeCtxPool
	require.NoError(t, validateOpenAIFirstServeRouting(a), "existing modes keep their behavior")
}

type firstServeTestResolver struct {
	first, next *Proxy
}

func (r firstServeTestResolver) ResolveAccountProxy(_ context.Context, a *Account) error {
	a.Proxy, a.ProxyID = r.first, &r.first.ID
	a.proxyGroupResolved = true
	return nil
}

func (r firstServeTestResolver) SelectNextProxy(_ context.Context, _, previous int64, _ []int64) (*Proxy, error) {
	if previous == 0 {
		return r.first, nil
	}
	if r.next == nil || r.next.ID == previous {
		return nil, ErrProxyGroupNoProxy
	}
	return r.next, nil
}

type firstServeTestDialer struct {
	mu      sync.Mutex
	conns   []openAIWSClientConn
	proxies []string
	headers []http.Header
}

func (d *firstServeTestDialer) Dial(_ context.Context, _ string, headers http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.proxies = append(d.proxies, proxy)
	d.headers = append(d.headers, headers.Clone())
	if len(d.conns) == 0 {
		return nil, 503, nil, ErrProxyGroupNoProxy
	}
	conn := d.conns[0]
	d.conns = d.conns[1:]
	return conn, 0, nil, nil
}

func TestFirstServeIngressRotation(t *testing.T) {
	// The global proxy resolver is shared with production entry points; these
	// cases deliberately run serially and restore it before returning.
	defaultProxyGroupResolver.RLock()
	oldResolver := defaultProxyGroupResolver.resolver
	defaultProxyGroupResolver.RUnlock()
	defer SetDefaultProxyGroupResolver(oldResolver)
	for _, tc := range []struct {
		name                                                                          string
		slow, expire, missingParent, noProxy, reconnect, custom, session, fingerprint bool
		wantRotation                                                                  bool
	}{
		{name: "slow does not rotate", slow: true},
		{name: "legacy threshold ignored", custom: true},
		{name: "expired", expire: true, wantRotation: true},
		{name: "healthy"},
		{name: "fingerprint healthy", fingerprint: true},
		{name: "fingerprint expires", fingerprint: true, expire: true, wantRotation: true},
		{name: "fingerprint reconnect expires", fingerprint: true, reconnect: true, expire: true, wantRotation: true},
		{name: "fingerprint no alternative", fingerprint: true, noProxy: true, expire: true},
		{name: "session without client ID", session: true, expire: true, wantRotation: true},
		{name: "reconnect_expired", expire: true, reconnect: true, wantRotation: true},
		{name: "reconnect_healthy", reconnect: true},
		{name: "unknown parent", expire: true, missingParent: true, wantRotation: true},
		{name: "no alternative", expire: true, noProxy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 30
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			firstProxy := &Proxy{ID: 101, Name: "A", Protocol: "http", Host: "proxy-a", Port: 8080}
			nextProxy := &Proxy{ID: 102, Name: "B", Protocol: "http", Host: "proxy-b", Port: 8080}
			resolver := firstServeTestResolver{first: firstProxy, next: nextProxy}
			if tc.noProxy {
				resolver.next = nil
			}
			SetDefaultProxyGroupResolver(resolver)
			completed := []byte(`{"type":"response.completed","response":{"id":"resp_a","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer A"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
			nextCompleted := []byte(`{"type":"response.completed","response":{"id":"resp_b","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)
			firstConn := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.output_text.delta","delta":"answer A"}`), completed}}
			if tc.slow {
				firstConn.readDelays = []time.Duration{15100 * time.Millisecond}
			}
			if tc.custom {
				firstConn.readDelays = []time.Duration{1100 * time.Millisecond}
			}
			secondConn := &openAIWSCaptureConn{events: [][]byte{nextCompleted}}
			if !tc.wantRotation {
				firstConn.events = append(firstConn.events, nextCompleted)
			}
			dialer := &firstServeTestDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			defer pool.Close()
			group := int64(10)
			account := &Account{ID: 98702, Name: "first serve test", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, ProxyGroupID: &group,
				Credentials: map[string]any{"api_key": "sk-test"}, Extra: map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe}}
			if tc.fingerprint {
				account.Type = AccountTypeOAuth
				account.Credentials = map[string]any{"access_token": "sk-test", "chatgpt_account_id": "account-test"}
				account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModeFirstServe
				account.Extra[codexFingerprintModeExtraKey] = "full"
				account.Extra[codexFingerprintSeedExtraKey] = testCodexFingerprintSeed
			}
			if tc.session {
				account.Extra["openai_first_serve"] = map[string]any{"reuse_scope": "session"}
			}
			if tc.custom {
				account.Extra["openai_first_serve"] = map[string]any{"ttl_minutes": 12, "ttft_seconds": 1, "proxy_mode": "selected", "proxy_ids": []int64{101, 102}}
			}
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector(), openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
			var reused []bool
			hooks := &OpenAIWSIngressHooks{AfterTurn: func(turn int, result *OpenAIForwardResult, _ error) {
				if result != nil {
					reused = append(reused, result.FirstServeActive)
				}
				if turn != 1 || !tc.expire {
					return
				}
				for _, entry := range svc.openaiFirstServeHTTP.items {
					entry.mu.Lock()
					entry.state.status.ExpiresAt = time.Now().Add(-time.Second)
					entry.mu.Unlock()
				}
				ap := pool.getOrCreateAccountPool(account.ID)
				ap.mu.Lock()
				defer ap.mu.Unlock()
				for _, conn := range ap.conns {
					if conn.firstServe != nil {
						conn.firstServe.status.ExpiresAt = time.Now().Add(-time.Second)
					}
				}
			}}
			errCh := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					errCh <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				_, first, err := conn.Read(r.Context())
				if err != nil {
					errCh <- err
					return
				}
				errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "sk-test", first, hooks)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			dialOptions := &coderws.DialOptions{HTTPHeader: http.Header{"X-Codex-Installation-Id": []string{"client-installation"}}}
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), dialOptions)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			first := `{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"role":"user","content":"question A"}]}`
			if tc.missingParent {
				first = `{"type":"response.create","model":"gpt-5.1","store":true,"previous_response_id":"resp_external","input":[{"role":"user","content":"question A"}]}`
			}
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(first)))
			for {
				_, event, readErr := client.Read(ctx)
				require.NoError(t, readErr)
				if gjson.GetBytes(event, "type").String() == "response.completed" {
					break
				}
			}
			if tc.reconnect {
				_ = client.CloseNow()
				require.NoError(t, <-errCh)
				client, _, err = coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), dialOptions)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
			}
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"previous_response_id":"resp_a","input":[{"role":"user","content":"question B"}]}`)))
			_, _, err = client.Read(ctx)
			require.NoError(t, err)
			_ = client.CloseNow()
			require.NoError(t, <-errCh)
			require.Equal(t, []bool{false, !tc.wantRotation}, reused)
			dialer.mu.Lock()
			proxies := append([]string(nil), dialer.proxies...)
			headers := append([]http.Header(nil), dialer.headers...)
			dialer.mu.Unlock()
			routeID := headers[0].Get("session_id")
			require.NotEmpty(t, routeID)
			for _, header := range headers {
				require.Equal(t, routeID, header.Get("session_id"), "proxy changes retain the wire routing ID")
				if !tc.fingerprint {
					require.Equal(t, "client-installation", header.Get("x-codex-installation-id"), "convergence off preserves the client fingerprint without repeated reconnects")
				}
			}
			if tc.fingerprint {
				firstID := headers[0].Get("x-codex-installation-id")
				require.NotEmpty(t, firstID)
				lastID := headers[len(headers)-1].Get("x-codex-installation-id")
				if tc.wantRotation {
					require.NotEqual(t, firstID, lastID)
				} else {
					require.Equal(t, firstID, lastID)
				}
				for i, conn := range []*openAIWSCaptureConn{firstConn, secondConn} {
					expected := firstID
					if i == 1 {
						expected = lastID
					}
					conn.mu.Lock()
					writes := append([]map[string]any(nil), conn.writes...)
					conn.mu.Unlock()
					for _, payload := range writes {
						raw, err := json.Marshal(payload)
						require.NoError(t, err)
						require.Equal(t, expected, gjson.GetBytes(raw, "client_metadata.x-codex-installation-id").String())
					}
				}
			}
			if !tc.session {
				_, routed, route, routeErr := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(42, "another conversation"), account, nil)
				require.NoError(t, routeErr)
				require.Equal(t, routeID, route.id, "HTTP and WS share the account route")
				if tc.fingerprint {
					require.Equal(t, headers[len(headers)-1].Get("x-codex-installation-id"), route.fingerprint)
				}
				require.Equal(t, proxies[len(proxies)-1], routed.Proxy.URL())
				route.finish(nil, nil)
			}
			if !tc.wantRotation {
				require.Equal(t, []string{firstProxy.URL()}, proxies)
				statuses := GetOpenAIFirstServeStatuses(account.ID)
				require.NotEmpty(t, statuses)
				if tc.noProxy {
					require.Equal(t, "proxy_unavailable", statuses[0].Reason)
				}
				if tc.missingParent {
					require.Equal(t, "context_incomplete", statuses[0].Reason)
				}
				return
			}
			require.Equal(t, []string{firstProxy.URL(), nextProxy.URL()}, proxies)
			secondConn.mu.Lock()
			payload, err := json.Marshal(secondConn.lastWrite)
			secondConn.mu.Unlock()
			require.NoError(t, err)
			require.Equal(t, routeID, gjson.GetBytes(payload, "prompt_cache_key").String())
			if tc.missingParent {
				require.Equal(t, "resp_a", gjson.GetBytes(payload, "previous_response_id").String())
				return
			}
			require.False(t, gjson.GetBytes(payload, "previous_response_id").Exists())
			require.Equal(t, int64(3), gjson.GetBytes(payload, "input.#").Int())
			require.Equal(t, "question A", gjson.GetBytes(payload, "input.0.content").String())
			require.Equal(t, "answer A", gjson.GetBytes(payload, "input.1.content.0.text").String())
			require.Equal(t, "question B", gjson.GetBytes(payload, "input.2.content").String())
		})
	}
}
