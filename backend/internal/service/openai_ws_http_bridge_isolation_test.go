package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type httpBridgeIsolationBody struct {
	ctx     context.Context
	prefix  *bytes.Reader
	suffix  *bytes.Reader
	release <-chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (b *httpBridgeIsolationBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	select {
	case <-b.release:
		return b.suffix.Read(p)
	case <-b.closed:
		return 0, io.EOF
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

func (b *httpBridgeIsolationBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type httpBridgeIsolationRequest struct {
	input []string
	state string
}

type httpBridgeIsolationUpstream struct {
	ctx          context.Context
	mu           sync.Mutex
	requests     []httpBridgeIsolationRequest
	firstRelease chan struct{}
}

func (u *httpBridgeIsolationUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	input := gjson.GetBytes(body, "input")
	var texts []string
	if input.Type == gjson.String {
		texts = append(texts, input.String())
	} else {
		for _, item := range input.Array() {
			if item.Type == gjson.String {
				texts = append(texts, item.String())
				continue
			}
			if item.Get("type").String() == "function_call" {
				texts = append(texts, item.Get("call_id").String())
				continue
			}
			if item.Get("type").String() == "function_call_output" {
				texts = append(texts, item.Get("output").String())
				continue
			}
			content := item.Get("content")
			if content.Type == gjson.String {
				texts = append(texts, content.String())
			} else {
				for _, part := range content.Array() {
					if text := part.Get("text"); text.Exists() {
						texts = append(texts, text.String())
					}
				}
			}
		}
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("missing bridge input")
	}
	u.mu.Lock()
	u.requests = append(u.requests, httpBridgeIsolationRequest{input: texts, state: req.Header.Get(openAIWSTurnStateHeader)})
	u.mu.Unlock()

	id := texts[0]
	turn := 1
	if input.IsArray() {
		turn = 2
	}
	responseID := fmt.Sprintf("resp_%s_%d", id, turn)
	prefix := fmt.Sprintf("data: {\"type\":\"response.created\",\"response\":{\"id\":%q,\"status\":\"in_progress\"}}\n\n", responseID)
	if turn == 1 {
		prefix += "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"rs_test\",\"type\":\"reasoning\",\"encrypted_content\":\"opaque\"}}\n\n"
	}
	output := "[]"
	if turn == 1 {
		output = fmt.Sprintf(`[{"type":"function_call","id":%q,"call_id":%q,"name":"lookup","arguments":"{}","status":"completed"}]`, "fc_"+id, "call_"+id)
	}
	suffix := fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"id\":%q,\"status\":\"completed\",\"output\":%s,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n", responseID, output)
	headers := http.Header{"Content-Type": []string{"text/event-stream"}}
	headers.Set(openAIWSTurnStateHeader, "turn-"+id)
	responseBody := io.NopCloser(strings.NewReader(prefix + suffix))
	if turn == 1 {
		responseBody = &httpBridgeIsolationBody{ctx: u.ctx, prefix: bytes.NewReader([]byte(prefix)), suffix: bytes.NewReader([]byte(suffix)), release: u.firstRelease, closed: make(chan struct{})}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: responseBody}, nil
}

func (u *httpBridgeIsolationUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestOpenAIWSHTTPBridgeSessionIsolationAcrossSameSessionHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeHTTPBridge
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 5
	upstream := &httpBridgeIsolationUpstream{ctx: ctx, firstRelease: make(chan struct{})}
	stateStore := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), openaiWSStateStore: stateStore}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeHTTPBridge},
		Concurrency: 2, Status: StatusActive, Schedulable: true}
	groupID := int64(7)
	newContext := func(r *http.Request) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r.Clone(ctx)
		c.Request.Header.Set("session-id", "shared-session")
		c.Set("api_key", &APIKey{ID: 11, GroupID: &groupID})
		return c
	}
	seedHash := svc.GenerateSessionHash(newContext(httptest.NewRequest(http.MethodGet, "/v1/responses", nil)), nil)
	stateStore.BindSessionTurnState(groupID, seedHash, "sentinel-turn-state", time.Hour)
	stateStore.BindSessionConn(groupID, seedHash, "sentinel-native-conn", time.Hour)

	serverResults := make(chan error, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverResults <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, firstMessage, err := conn.Read(ctx)
		if err != nil {
			serverResults <- err
			return
		}
		c := newContext(r)
		// Match the handler-owned registration and nested forwarding call.
		preemptCtx, cleanup, _ := svc.BeginOpenAIWSIngressSessionPreemption(ctx, c, account, firstMessage)
		defer cleanup()
		hooks := &OpenAIWSIngressHooks{ClientLifecycleContext: ctx}
		serverResults <- svc.ProxyResponsesWebSocketFromClient(preemptCtx, c, conn, account, "test-token", firstMessage, hooks)
	}))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(upstream.firstRelease) }) }
	defer func() {
		release()
		cancel()
		server.Close()
	}()

	readEvent := func(conn *coderws.Conn, wantType string) []byte {
		t.Helper()
		_, event, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, wantType, gjson.GetBytes(event, "type").String(), "event: %s", event)
		return event
	}
	clients := make([]*coderws.Conn, 0, 2)
	for _, id := range []string{"alpha", "beta"} {
		conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()
		clients = append(clients, conn)
		payload := fmt.Sprintf(`{"type":"response.create","model":"gpt-5","prompt_cache_key":%q,"client_metadata":{"thread_id":%q},"input":%q}`, id+"-cache", id+"-thread", id)
		require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
		readEvent(conn, "response.created")
		readEvent(conn, "response.output_item.added")
	}
	// Both HTTP streams are now in flight. Neither may replace the other.
	release()
	for i, id := range []string{"alpha", "beta"} {
		event := readEvent(clients[i], "response.completed")
		require.Equal(t, "resp_"+id+"_1", gjson.GetBytes(event, "response.id").String())
	}
	for i, id := range []string{"alpha", "beta"} {
		payload := fmt.Sprintf(`{"type":"response.create","model":"gpt-5","previous_response_id":%q,"input":[{"type":"function_call_output","call_id":%q,"output":%q}]}`, "resp_"+id+"_1", "call_"+id, id+"-result")
		require.NoError(t, clients[i].Write(ctx, coderws.MessageText, []byte(payload)))
		readEvent(clients[i], "response.created")
		event := readEvent(clients[i], "response.completed")
		require.Equal(t, "resp_"+id+"_2", gjson.GetBytes(event, "response.id").String())
		require.NoError(t, clients[i].Close(coderws.StatusNormalClosure, "done"))
	}
	for range clients {
		select {
		case err := <-serverResults:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	upstream.mu.Lock()
	requests := append([]httpBridgeIsolationRequest(nil), upstream.requests...)
	upstream.mu.Unlock()
	require.ElementsMatch(t, []httpBridgeIsolationRequest{
		{input: []string{"alpha"}},
		{input: []string{"beta"}},
		{input: []string{"alpha", "call_alpha", "alpha-result"}, state: "turn-alpha"},
		{input: []string{"beta", "call_beta", "beta-result"}, state: "turn-beta"},
	}, requests)
	gotState, ok := stateStore.GetSessionTurnState(groupID, seedHash)
	require.True(t, ok)
	require.Equal(t, "sentinel-turn-state", gotState)
	gotConn, ok := stateStore.GetSessionConn(groupID, seedHash)
	require.True(t, ok)
	require.Equal(t, "sentinel-native-conn", gotConn)
}
