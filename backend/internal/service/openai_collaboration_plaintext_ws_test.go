package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
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

func collabPlaintextWSCompletedEvent(callName, responseID string) string {
	return `data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5.6-sol","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"` + callName + `","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextIngressLocalRejectAtomic(t *testing.T) {
	for _, optIn := range []bool{false, true} {
		name := "off"
		if optIn {
			name = "on"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			completed := func(id string) *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
					`data: {"type":"response.completed","response":{"id":"` + id + `","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"))}
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{completed("resp_before"), completed("resp_after")}}
			cfg := collabPlaintextWSBridgeConfig()
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector()}
			a := collabPlaintextAPIKeyAccount()
			a.Extra["responses_websockets_v2_enabled"] = true
			if !optIn {
				delete(a.Extra, "openai_responses_plaintext_collaboration")
			}
			b := collabPlaintextAPIKeyAccount()
			b.ID = a.ID + 1
			b.Extra["responses_websockets_v2_enabled"] = true
			type state struct {
				tools, mirror, lowered, plaintextMapping, genericMapping [32]byte
				accountID                                                int64
				plaintext                                                bool
			}
			snapshot := func(c *gin.Context) (state, bool) {
				session := openAIWSCollabPlaintextSessionFromContext(c)
				bridge, exists := openAIWSHTTPBridgeToolStateFromContext(c)
				if session == nil || !exists {
					return state{}, false
				}
				var result state
				result.tools = sha256.Sum256(session.declaredTools())
				session.mu.Lock()
				plain, plainErr := json.Marshal(session.mapping)
				session.mu.Unlock()
				generic, genericErr := json.Marshal(bridge.ClientMapping)
				if plainErr != nil || genericErr != nil {
					return state{}, false
				}
				result.plaintextMapping = sha256.Sum256(plain)
				result.genericMapping = sha256.Sum256(generic)
				result.mirror = sha256.Sum256(bridge.OriginalTools)
				result.lowered = sha256.Sum256(bridge.LoweredTools)
				result.accountID, result.plaintext = bridge.AccountID, bridge.Plaintext
				return result, true
			}
			type rejection struct{ localError, statePresent, unchanged bool }
			checks := make(chan rejection, 1)
			beforeState := make(chan state, 1)
			serverErr := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverErr <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				readCtx, stop := context.WithTimeout(r.Context(), 3*time.Second)
				_, first, err := conn.Read(readCtx)
				stop()
				if err != nil {
					serverErr <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				hooks := &OpenAIWSIngressHooks{AfterTurn: func(turn int, result *OpenAIForwardResult, turnErr error) {
					if turn == 1 && result != nil && turnErr == nil {
						before, ok := snapshot(c)
						if ok {
							beforeState <- before
						}
					}
				}}
				err = svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, a, "sk-test", first, hooks)
				var before state
				var present bool
				select {
				case before = <-beforeState:
					present = true
				default:
				}
				after, afterPresent := snapshot(c)
				checks <- rejection{localError: err != nil, statePresent: present && afterPresent, unchanged: present && afterPresent && before == after}
				if err == nil {
					serverErr <- errors.New("locally conflicting declaration was accepted")
					return
				}
				retry := []byte(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"input":"after-rejection"}`)
				serverErr <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, b, "sk-test", retry, nil)
			}))
			defer server.Close()
			dialCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			stop()
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			write := func(payload string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(payload)))
			}
			read := func(want string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_, event, readErr := client.Read(ctx)
				require.NoError(t, readErr)
				require.Equal(t, want, gjson.GetBytes(event, "response.id").String())
			}
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":` + collabPlaintextToolsJSON + `,"input":"before"}`)
			read("resp_before")
			conflict := `[{"type":"custom","name":"exec"},{"type":"function","name":"exec","parameters":{"type":"object"}}]`
			if optIn {
				conflict = `[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}]},` + conflict[1:]
			}
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":` + conflict + `,"input":"reject"}`)
			select {
			case check := <-checks:
				require.True(t, check.localError, "generic adapter must reject the conflict")
				require.True(t, check.statePresent, "both state owners must be observable before and after rejection")
				require.True(t, check.unchanged, "local rejection must preserve the committed declaration and both mappings")
			case <-time.After(5 * time.Second):
				t.Fatal("local rejection was not observed")
			}
			read("resp_after")
			_ = client.Close(coderws.StatusNormalClosure, "done")
			select {
			case proxyErr := <-serverErr:
				require.NoError(t, proxyErr)
			case <-time.After(5 * time.Second):
				t.Fatal("replacement account did not finish")
			}
			require.Len(t, upstream.bodies, 2, "locally rejected request must not reach upstream")
			flat := gjson.GetBytes(upstream.bodies[1], `tools.#(name=="collaboration__send_message")`)
			require.True(t, flat.Exists(), "reassigned account must inherit the earlier accepted declaration")
			require.False(t, flat.Get("parameters.properties.message.encrypted").Exists())
			require.False(t, gjson.GetBytes(upstream.bodies[1], `tools.#(name=="collaboration__spawn_agent")`).Exists())
		})
	}
}

// The retry enters the real WS ingress again with the same gin context. The
// client's original tool declaration survives the attempt; A's lowered form
// must not become B's declaration when B has plaintext collaboration off.
func TestOpenAIWSHTTPBridgeCollaborationPlaintextFailoverToOptOut(t *testing.T) {
	testOpenAIWSHTTPBridgeCollaborationPlaintextFailover(t, false, false, false)
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextFailoverToOptIn(t *testing.T) {
	testOpenAIWSHTTPBridgeCollaborationPlaintextFailover(t, true, false, false)
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextSameIDOptionChange(t *testing.T) {
	testOpenAIWSHTTPBridgeCollaborationPlaintextFailover(t, false, true, false)
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextOffFirstToOptIn(t *testing.T) {
	testOpenAIWSHTTPBridgeCollaborationPlaintextFailover(t, true, false, true)
}

func testOpenAIWSHTTPBridgeCollaborationPlaintextFailover(t *testing.T, optIn, sameID, firstOff bool) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"))},
		{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader(
			`{"error":{"type":"usage_limit_reached","message":"limited"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"resp_retried","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"))},
	}}
	cfg := collabPlaintextWSBridgeConfig()
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
	}
	accountA := collabPlaintextAPIKeyAccount()
	accountA.Extra["responses_websockets_v2_enabled"] = true
	if firstOff {
		delete(accountA.Extra, "openai_responses_plaintext_collaboration")
	}
	accountB := collabPlaintextAPIKeyAccount()
	if !sameID {
		accountB.ID++
	}
	accountB.Extra["responses_websockets_v2_enabled"] = true
	if !optIn {
		delete(accountB.Extra, "openai_responses_plaintext_collaboration")
	}

	serverErrCh := make(chan error, 1)
	retrySeen := make(chan bool, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, first, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			serverErrCh <- err
			return
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r.Clone(r.Context())
		err = svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, accountA, "sk-test", first, nil)
		var failover *UpstreamFailoverError
		if !errors.As(err, &failover) {
			serverErrCh <- err
			return
		}
		retryPayload, ok := OpenAIWSCurrentTurnRetryPayload(err)
		retrySeen <- ok && len(retryPayload) > 0 && !gjson.GetBytes(retryPayload, "tools").Exists()
		if !ok || len(retryPayload) == 0 {
			serverErrCh <- errors.New("missing retry payload")
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, accountB, "sk-test", retryPayload, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()
	write := func(payload string) {
		ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(payload)))
	}
	read := func() string {
		ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_, msg, readErr := client.Read(ctx)
		require.NoError(t, readErr)
		return gjson.GetBytes(msg, "response.id").String()
	}
	write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":` + collabPlaintextToolsJSON + `,"input":"first"}`)
	require.Equal(t, "resp_first", read())
	write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"previous_response_id":"resp_first","input":"continue"}`)
	require.Equal(t, "resp_retried", read())
	_ = client.Close(coderws.StatusNormalClosure, "done")
	select {
	case ok := <-retrySeen:
		require.True(t, ok, "retry must omit tools and retain a current-turn payload")
	case <-time.After(5 * time.Second):
		t.Fatal("retry not observed")
	}
	select {
	case proxyErr := <-serverErrCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("replacement attempt not completed")
	}
	require.Len(t, upstream.bodies, 3)
	first := gjson.GetBytes(upstream.bodies[0], `tools.#(name=="collaboration__send_message")`)
	require.True(t, first.Exists(), "A must retain its declaration")
	require.Equal(t, firstOff, first.Get("parameters.properties.message.encrypted").Bool(), "A must honor its own option")
	second := gjson.GetBytes(upstream.bodies[2], `tools.#(name=="collaboration__send_message")`)
	require.True(t, second.Exists(), "generic namespace declaration must survive account reassignment")
	require.Equal(t, !optIn, second.Get("parameters.properties.message.encrypted").Bool(), "B must adapt the original client declaration under its own option")
	require.True(t, gjson.GetBytes(upstream.bodies[2], `tools.#(name=="collaboration__wait")`).Exists())
	require.True(t, gjson.GetBytes(upstream.bodies[2], `tools.#(name=="exec")`).Exists())
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextThreeAccountDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name       string
		key        string
		tools      string
		want       string
		wantMarker bool
		sameID     bool
	}{
		{"empty", "tools", `[]`, "", false, false},
		{"null", "tools", `null`, "", false, false},
		{"escaped key empty", `to\u006fls`, `[]`, "", false, false},
		{"replacement target", "tools", `[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}},{"type":"function","name":"wait","parameters":{"type":"object"}}]}]`, "collaboration__spawn_agent", false, false},
		{"generic namespace and custom same ID", "tools", `[{"type":"namespace","name":"ops","tools":[{"type":"function","name":"review","parameters":{"type":"object"}}]},{"type":"custom","name":"command","format":{"type":"text"}}]`, "ops__review", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			completed := func(id string) *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
					`data: {"type":"response.completed","response":{"id":"` + id + `","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"))}
			}
			limited := func() *http.Response {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"usage_limit_reached","message":"limited"}}`))}
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				completed("resp_a"), limited(), completed("resp_b"), completed("resp_replace"), limited(), completed("resp_c"),
			}}
			cfg := collabPlaintextWSBridgeConfig()
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector()}
			a := collabPlaintextAPIKeyAccount()
			a.Extra["responses_websockets_v2_enabled"] = true
			b := collabPlaintextAPIKeyAccount()
			cAccount := collabPlaintextAPIKeyAccount()
			if !tc.sameID {
				b.ID = a.ID + 1
				cAccount.ID = a.ID + 2
			}
			b.Extra["responses_websockets_v2_enabled"] = true
			cAccount.Extra["responses_websockets_v2_enabled"] = true
			delete(b.Extra, "openai_responses_plaintext_collaboration")
			serverErr := make(chan error, 1)
			retries := make(chan bool, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverErr <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				readCtx, stop := context.WithTimeout(r.Context(), 3*time.Second)
				_, first, err := conn.Read(readCtx)
				stop()
				if err != nil {
					serverErr <- err
					return
				}
				gc, _ := gin.CreateTestContext(httptest.NewRecorder())
				gc.Request = r.Clone(r.Context())
				for _, account := range []*Account{a, b, cAccount} {
					err = svc.ProxyResponsesWebSocketFromClient(r.Context(), gc, conn, account, "sk-test", first, nil)
					if account == cAccount {
						serverErr <- err
						return
					}
					var failover *UpstreamFailoverError
					if !errors.As(err, &failover) {
						serverErr <- errors.New("attempt did not produce retryable failover")
						return
					}
					var ok bool
					first, ok = OpenAIWSCurrentTurnRetryPayload(err)
					retries <- ok && len(first) > 0 && !gjson.GetBytes(first, "tools").Exists()
					if !ok || len(first) == 0 {
						serverErr <- errors.New("missing retry frame")
						return
					}
				}
			}))
			defer server.Close()
			dialCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			stop()
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			write := func(frame string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(frame)))
			}
			read := func(wantID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_, event, readErr := client.Read(ctx)
				require.NoError(t, readErr)
				require.Equal(t, wantID, gjson.GetBytes(event, "response.id").String())
			}
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":` + collabPlaintextToolsJSON + `,"input":"first"}`)
			read("resp_a")
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"previous_response_id":"resp_a","input":"continue"}`)
			read("resp_b")
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"` + tc.key + `":` + tc.tools + `,"input":"replace"}`)
			read("resp_replace")
			write(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"previous_response_id":"resp_replace","input":"follow-up"}`)
			read("resp_c")
			_ = client.Close(coderws.StatusNormalClosure, "done")
			for range 2 {
				select {
				case ok := <-retries:
					require.True(t, ok, "current-turn retry must omit tools")
				case <-time.After(5 * time.Second):
					t.Fatal("failover not observed")
				}
			}
			select {
			case proxyErr := <-serverErr:
				require.NoError(t, proxyErr)
			case <-time.After(5 * time.Second):
				t.Fatal("final attempt not completed")
			}
			require.Len(t, upstream.bodies, 6)
			require.True(t, gjson.GetBytes(upstream.bodies[0], `tools.#(name=="collaboration__send_message")`).Exists(), "A must lower the initial target")
			bTools := gjson.GetBytes(upstream.bodies[3], "tools")
			if tc.tools == `[]` {
				require.Len(t, bTools.Array(), 0)
			} else if tc.tools == `null` {
				require.Equal(t, gjson.Null, bTools.Type)
			}
			cTools := upstream.bodies[5]
			require.False(t, gjson.GetBytes(cTools, `tools.#(name=="collaboration__send_message")`).Exists(), "C must not inherit A's stale declaration")
			if tc.want != "" {
				require.True(t, gjson.GetBytes(cTools, `tools.#(name=="`+tc.want+`")`).Exists(), "C must inherit B's declaration")
			}
			if tc.want == "collaboration__spawn_agent" {
				require.False(t, gjson.GetBytes(cTools, `tools.#(name=="collaboration__spawn_agent").parameters.properties.message.encrypted`).Exists())
				require.True(t, gjson.GetBytes(cTools, `tools.#(name=="collaboration__wait")`).Exists())
			}
			if tc.want == "ops__review" {
				require.True(t, gjson.GetBytes(cTools, `tools.#(name=="command")`).Exists(), "generic custom tool must survive")
			}
		})
	}
}

func TestOpenAIWSHTTPBridgeCollaborationPlaintextRejectedOffDeclarationAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_one","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")),
	}}
	cfg := collabPlaintextWSBridgeConfig()
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{}}
	account := collabPlaintextAPIKeyAccount()
	delete(account.Extra, "openai_responses_plaintext_collaboration")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	first := []byte(`{"type":"response.create","model":"gpt-5.6-sol","tools":` + collabPlaintextToolsJSON + `,"input":"first"}`)
	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", first, len(first), "gpt-5.6-sol", "", "", "", "", 1, func([]byte) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, result)
	session := openAIWSCollabPlaintextSessionFromContext(c)
	require.NotNil(t, session)
	accepted := session.declaredTools()
	require.NotEmpty(t, accepted)
	rejected := []byte(`{"type":"response.create","model":"gpt-5.6-sol","tools":[{"type":"custom","name":"exec"},{"type":"function","name":"exec","parameters":{"type":"object"}}],"input":"reject"}`)
	result, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", rejected, len(rejected), "gpt-5.6-sol", "", "", "", "", 2, func([]byte) error { return nil })
	require.Error(t, err)
	require.Nil(t, result)
	require.True(t, bytes.Equal(accepted, session.declaredTools()), "rejected generic declaration must not replace accepted client tools")
}

func collabPlaintextWSBridgeConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

// WS→HTTP bridge 端到端：经真实入口 ProxyResponsesWebSocketFromClient，
// 覆盖 parseClientPayload 降级、多轮 tools 继承/reset、writeClientMessage 还原。
func TestOpenAIWSHTTPBridgeCollaborationPlaintextRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(collabPlaintextWSCompletedEvent("collaboration__send_message", "resp_collab_1")))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(collabPlaintextWSCompletedEvent("collaboration__send_message", "resp_collab_2")))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_collab_3","model":"gpt-5.6-sol","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"))},
	}}
	cfg := collabPlaintextWSBridgeConfig()
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 9901, Name: "apikey-collab-bridge", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"},
		Extra: map[string]any{
			"responses_websockets_v2_enabled":          true,
			"openai_responses_plaintext_collaboration": true,
		},
		Concurrency: 1, Status: StatusActive, Schedulable: true,
	}

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
		}
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		ginCtx.Request = r.Clone(r.Context())
		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeMessage := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWrite()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	readMessage := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelRead()
		messageType, event, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, messageType)
		return event
	}

	// turn 1：显式 collaboration tools 声明 → 降级后发往上游。
	writeMessage(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":` + collabPlaintextToolsJSON + `,"input":"go"}`)
	first := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(first, "type").String())
	firstCall := gjson.GetBytes(first, "response.output.0")
	require.Equal(t, "send_message", firstCall.Get("name").String())
	require.Equal(t, "collaboration", firstCall.Get("namespace").String())
	require.Len(t, firstCall.Get("encrypted_function_args").Array(), 0)

	// turn 2：省略 tools → 继承 turn 1 客户端原始声明并重新降级。
	writeMessage(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"previous_response_id":"resp_collab_1","input":[
		{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"c1","arguments":"{\"message\":\"ok\"}"},
		{"type":"function_call_output","call_id":"c1","output":"done"}
	]}`)
	second := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(second, "type").String())
	require.Equal(t, "send_message", gjson.GetBytes(second, "response.output.0.name").String())

	// turn 3：显式空 tools → reset；本帧 input 里的 namespaced 调用不再改写。
	writeMessage(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":[],"input":[
		{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"c9","arguments":"{}"}
	]}`)
	third := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(third, "type").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.Len(t, upstream.bodies, 3)

	// turn 1 出站体：API-key 透传形态下 wait 由既有 client-tool 摊平器处理，
	// send_message 由本适配器降级（encrypted 已移除）。
	flat1 := gjson.GetBytes(upstream.bodies[0], `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat1.Exists())
	require.False(t, flat1.Get("parameters.properties.message.encrypted").Exists())
	require.True(t, gjson.GetBytes(upstream.bodies[0], `tools.#(name=="collaboration__wait")`).Exists())

	// turn 2：tools 省略但继承声明 → 上游仍看到降级后的 tools；
	// input 里 namespaced 调用被改写为别名。
	require.True(t, gjson.GetBytes(upstream.bodies[1], `tools.#(name=="collaboration__send_message")`).Exists(),
		"follow-up without tools must inherit the original declaration")
	inputs := gjson.GetBytes(upstream.bodies[1], "input").Array()
	sawAliasCall := false
	for _, item := range inputs {
		if item.Get("type").String() == "function_call" && item.Get("name").String() == "collaboration__send_message" {
			sawAliasCall = true
		}
	}
	require.True(t, sawAliasCall, "namespaced input call must be rewritten to the alias on the follow-up frame")

	// turn 3：显式空 tools → 上游 tools 为空，input 调用不再改写。
	require.Len(t, gjson.GetBytes(upstream.bodies[2], "tools").Array(), 0)
	require.Equal(t, "send_message", gjson.GetBytes(upstream.bodies[2], "input.#(type==\"function_call\").name").String())
}

func TestOpenAIWSPassthroughCollaborationPlaintextOffObservesTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	account := passthroughLifecycleAccount()
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	contexts := make(chan *gin.Context, 1)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, account, func(c *gin.Context) *OpenAIWSIngressHooks {
		contexts <- c
		return nil
	})
	defer server.Close()
	client := dialPassthroughLifecycleClientWithPayload(t, server,
		`{"type":"response.create","model":"gpt-5.1","to\u006fls":`+collabPlaintextToolsJSON+`,"input":"first"}`)
	defer func() { _ = client.CloseNow() }()
	first := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "collaboration", gjson.GetBytes(first, "tools.0.name").String())
	require.True(t, gjson.GetBytes(first, "tools.0.tools.0.parameters.properties.message.encrypted").Bool(), "off payload must retain the marker")
	require.False(t, gjson.GetBytes(first, `tools.#(name=="collaboration__send_message")`).Exists())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pt_1","model":"gpt-5.1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","tools":[],"input":"reset"}`)))
	cancelWrite()
	second := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Len(t, gjson.GetBytes(second, "tools").Array(), 0)
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pt_2","model":"gpt-5.1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	_ = client.Close(coderws.StatusNormalClosure, "done")
	select {
	case proxyErr := <-serverErr:
		if proxyErr != nil {
			var closeErr *OpenAIWSClientCloseError
			require.ErrorAs(t, proxyErr, &closeErr)
			require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("passthrough session did not finish")
	}
	var c *gin.Context
	select {
	case c = <-contexts:
	default:
		t.Fatal("missing passthrough context")
	}
	session := openAIWSCollabPlaintextSessionFromContext(c)
	require.NotNil(t, session, "off declaration must be recorded without adapting the payload")
	require.Equal(t, "[]", string(session.declaredTools()))
	on := *account
	on.Extra = map[string]any{"openai_responses_plaintext_collaboration": true}
	onSession, err := openAIWSCollabPlaintextSessionForAccount(c, &on)
	require.NoError(t, err)
	adapted, err := onSession.adaptPayload([]byte(`{"type":"response.create","model":"gpt-5.1","input":"next"}`))
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(adapted, "tools").Array(), 0)
	require.False(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`).Exists())
}

// WS passthrough：staged upstream conn 验证 filter 降级 + WriteFrame 还原。
func TestOpenAIWSPassthroughCollaborationPlaintextRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	account := passthroughLifecycleAccount()
	account.Extra["openai_responses_plaintext_collaboration"] = true
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)

	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
	defer server.Close()
	// 首帧必须带 tools 声明，作为 firstClientMessage 进入 passthrough 入口。
	clientConn := dialPassthroughLifecycleClientWithPayload(t, server,
		`{"type":"response.create","model":"gpt-5.1","tools":`+collabPlaintextToolsJSON+`,"input":"go"}`)
	defer func() { _ = clientConn.CloseNow() }()

	firstRequest := requirePassthroughUpstreamWrite(t, upstream, time.Second)
	flat := gjson.GetBytes(firstRequest, `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists(), "passthrough filter must lower the marked collaboration tool")
	require.False(t, flat.Get("parameters.properties.message.encrypted").Exists())
	require.Equal(t, "collaboration", gjson.GetBytes(firstRequest, `tools.#(type=="namespace").name`).String())

	// 上游别名事件 → 客户端收到还原后的 namespace/name。
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pt_1","model":"gpt-5.1","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	event, err := readPassthroughLifecycleFrame(t, clientConn, time.Second)
	require.NoError(t, err)
	require.Equal(t, "send_message", gjson.GetBytes(event, "response.output.0.name").String())
	require.Equal(t, "collaboration", gjson.GetBytes(event, "response.output.0.namespace").String())
	require.Len(t, gjson.GetBytes(event, "response.output.0.encrypted_function_args").Array(), 0)

	// follow-up 省略 tools → 继承客户端原始声明，重新降级。
	writeCtx2, cancelWrite2 := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, clientConn.Write(writeCtx2, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_pt_1","input":[{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"c1","arguments":"{\"message\":\"again\"}"},{"type":"function_call_output","call_id":"c1","output":"done"}]}`)))
	cancelWrite2()
	secondRequest := requirePassthroughUpstreamWrite(t, upstream, time.Second)
	require.True(t, gjson.GetBytes(secondRequest, `tools.#(name=="collaboration__send_message")`).Exists(),
		"passthrough follow-up without tools must inherit the original declaration")
	require.Equal(t, "collaboration__send_message",
		gjson.GetBytes(secondRequest, `input.#(type=="function_call").name`).String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-serverErr:
		if proxyErr != nil {
			var closeErr *OpenAIWSClientCloseError
			require.ErrorAs(t, proxyErr, &closeErr)
			require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for passthrough proxy to finish")
	}
}

// opt-in + 不支持的出站模式（force_chat_completions）→ passthrough 入口在
// 拨号上游之前明确拒绝，而不是静默按原样转发。
func TestOpenAIWSPassthroughCollaborationPlaintextRejectsUnsupportedOutbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	account := passthroughLifecycleAccount()
	account.Extra["openai_responses_plaintext_collaboration"] = true
	account.Extra["openai_responses_mode"] = "force_chat_completions"
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)

	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
	defer server.Close()
	// 出站模式校验在 passthrough 入口执行，与首帧内容无关。
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	select {
	case proxyErr := <-serverErr:
		require.Error(t, proxyErr)
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, proxyErr, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
	case <-time.After(5 * time.Second):
		t.Fatal("unsupported outbound account was not rejected at passthrough entry")
	}
	select {
	case replay := <-upstream.writes:
		t.Fatalf("rejected account must not reach upstream: %s", replay)
	default:
	}
}

// HTTP POST → WSv2（ctx_pool）：Forward 里完成的降级经 wsReqBody 进入
// response.create 帧，上游别名事件在写回客户端前还原。
func TestForwardOpenAIWSV2CollaborationPlaintextRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_ws2_collab","model":"gpt-5.5","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := collabPlaintextEnabledAccount()
	account.Extra["responses_websockets_v2_enabled"] = true

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"model":"gpt-5.5","stream":false,"tools":` + collabPlaintextToolsJSON + `,"input":"go"}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.OpenAIWSMode)
	rawWrite, marshalErr := json.Marshal(captureConn.lastWrite)
	require.NoError(t, marshalErr)
	flat := gjson.GetBytes(rawWrite, `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists(), "response.create frame must carry the lowered alias tool")
	require.False(t, flat.Get("parameters.properties.message.encrypted").Exists())

	out := recorder.Body.String()
	assertCollabPlaintextRestored(t, out)
}
