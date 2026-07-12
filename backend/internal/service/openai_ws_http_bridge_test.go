package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIWSHTTPBridgeBodyStripsWSFields(t *testing.T) {
	body, err := prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":false,"previous_response_id":"resp_prev","input":"hi"}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "type").Exists())
	require.False(t, gjson.GetBytes(body, "generate").Exists())
	require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
	require.Equal(t, "gpt-5", gjson.GetBytes(body, "model").String())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
	require.Equal(t, "hi", gjson.GetBytes(body, "input").String())
}

func TestOpenAIWSHTTPBridgeDecisionKeepsSmallFramesOnWS(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					HTTPBridgeEnabled:        true,
					HTTPBridgeThresholdBytes: 100,
				},
			},
		},
	}

	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 99, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(nil, 100, ""))
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 1000, "resp_existing"))

	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = false
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 1000, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformGrok}, 1, "resp_existing"))
}

func TestOpenAIWSHTTPBridgeRelaysSSEFramesAsWebSocketMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_bridge","model":"gpt-5"}}`,
		"",
		`data: {"type":"response.output_text.delta","response":{"id":"resp_bridge"},"delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_bridge","model":"gpt-5","usage":{"input_tokens":3,"output_tokens":2}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"x-request-id": []string{"rid_bridge"},
		},
		Body: io.NopCloser(strings.NewReader(sseBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
				OpenAIWS: config.GatewayOpenAIWSConfig{
					HTTPBridgeEnabled:        true,
					HTTPBridgeThresholdBytes: 1,
				},
			},
		},
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          7,
		Name:        "api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Status:      StatusActive,
	}
	payload := []byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":true,"input":"hi"}`)

	type bridgeResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan bridgeResult, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			resultCh <- bridgeResult{err: err}
			return
		}
		defer func() { _ = conn.CloseNow() }()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		ginCtx.Request = req

		writeClient := func(message []byte) error {
			writeCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			return conn.Write(writeCtx, coderws.MessageText, message)
		}
		result, bridgeErr := svc.proxyOpenAIWSHTTPBridgeTurn(
			r.Context(),
			ginCtx,
			account,
			"sk-test",
			payload,
			len(payload),
			"gpt-5",
			"",
			"",
			"",
			1,
			writeClient,
		)
		resultCh <- bridgeResult{result: result, err: bridgeErr}
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	readEvent := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		msgType, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
	}

	created := readEvent()
	delta := readEvent()
	completed := readEvent()

	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())

	select {
	case bridge := <-resultCh:
		require.NoError(t, bridge.err)
		require.NotNil(t, bridge.result)
		require.Equal(t, "resp_bridge", bridge.result.RequestID)
		require.Equal(t, 3, bridge.result.Usage.InputTokens)
		require.Equal(t, 2, bridge.result.Usage.OutputTokens)
		require.True(t, bridge.result.OpenAIWSMode)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for bridge result")
	}

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
}

func TestProxyOpenAIWSHTTPBridgeTurnForGrokDefaultsEmptyModelTo45(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_grok_default","model":"grok-4.5"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_grok_default","model":"grok-4.5","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          72,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"base_url": xai.DefaultCLIBaseURL},
	}
	payload := []byte(`{"type":"response.create","generate":true,"stream":true,"input":"hi"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload), "", "", "", "", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Len(t, events, 2)
}

func TestProxyResponsesWebSocketFromClientForGrokUsesXAIHTTPBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_grok_ws","model":"grok-4.3"}}`,
		"",
		`data: {"type":"response.output_text.delta","response":{"id":"resp_grok_ws"},"delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_grok_ws","model":"grok-4.3","usage":{"input_tokens":4,"output_tokens":2}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"text/event-stream"},
			"Xai-Request-Id": []string{"xai-ws-req"},
		},
		Body: io.NopCloser(strings.NewReader(sseBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          71,
		Name:        "grok",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Status:      StatusActive,
		Credentials: map[string]any{
			"base_url": xai.DefaultCLIBaseURL,
		},
	}

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
		}
		if msgType != coderws.MessageText {
			errCh <- errors.New("first message was not text")
			return
		}

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		ginCtx.Request = req

		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "access-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"grok","stream":true,"input":"hi","prompt_cache_retention":"24h"}`))
	cancelWrite()
	require.NoError(t, err)

	readEvent := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		msgType, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
	}

	created := readEvent()
	delta := readEvent()
	completed := readEvent()
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())

	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(3 * time.Second):
		require.Fail(t, "proxy did not finish after client close")
	}

	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "sub2api-grok/1.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_retention").Exists())
}

func TestOpenAIWSHTTPBridgeAcceptsFirstFrameAboveLegacy16MiB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_large_bridge","model":"gpt-5"}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_large_bridge","model":"gpt-5","usage":{"input_tokens":9,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"x-request-id": []string{"rid_large_bridge"},
		},
		Body: io.NopCloser(strings.NewReader(sseBody)),
	}}
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
			OpenAIWS: config.GatewayOpenAIWSConfig{
				Enabled:                  true,
				APIKeyEnabled:            true,
				ResponsesWebsocketsV2:    true,
				ClientReadLimitBytes:     64 * 1024 * 1024,
				HTTPBridgeEnabled:        true,
				HTTPBridgeThresholdBytes: 15 * 1024 * 1024,
			},
		},
	}
	svc := &OpenAIGatewayService{
		cfg:           cfg,
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          9,
		Name:        "api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
		Concurrency: 1,
		Status:      StatusActive,
	}

	payload := []byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":true,"input":"` + strings.Repeat("x", 17*1024*1024) + `"}`)
	require.Greater(t, len(payload), 16*1024*1024)
	require.Less(t, int64(len(payload)), ResolveOpenAIWSClientReadLimitBytes(cfg))

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		conn.SetReadLimit(ResolveOpenAIWSClientReadLimitBytes(cfg))

		readCtx, cancelRead := context.WithTimeout(r.Context(), 10*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			errCh <- NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "unexpected client websocket message type", nil)
			return
		}

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "codex_cli_rs/0.135.0")
		ginCtx.Request = req

		proxyCtx, cancelProxy := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancelProxy()
		errCh <- svc.ProxyResponsesWebSocketFromClient(proxyCtx, ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 5*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 20*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, payload)
	cancelWrite()
	require.NoError(t, err)

	var eventTypes []string
	for {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 10*time.Second)
		msgType, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)

		eventType := gjson.GetBytes(event, "type").String()
		eventTypes = append(eventTypes, eventType)
		if eventType == "response.completed" {
			break
		}
	}
	require.Contains(t, eventTypes, "response.created")
	require.Contains(t, eventTypes, "response.completed")

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Greater(t, len(upstream.lastBody), 16*1024*1024)
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "gpt-5", gjson.GetBytes(upstream.lastBody, "model").String())
}

func TestOpenAIWSHTTPBridgeKeepsContinuationFramesOnHTTPWithoutPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_first","model":"gpt-5.1","output":[{"type":"function_call","id":"fc_bridge_1","call_id":"call_bridge_1","name":"shell","arguments":"{}"}],"usage":{"input_tokens":9,"output_tokens":1}}}`,
		"",
	}, "\n")
	secondSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: io.NopCloser(strings.NewReader(firstSSEBody)),
		},
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: io.NopCloser(strings.NewReader(secondSSEBody)),
		},
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          19,
		Name:        "api-key-bridge-handoff",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
		Concurrency: 1,
		Status:      StatusActive,
		Schedulable: true,
	}

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			errCh <- NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "unexpected client websocket message type", nil)
			return
		}

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "codex_cli_rs/0.135.0")
		ginCtx.Request = req

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
		msgType, event, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
	}

	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"first"}`)
	firstTurnEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(firstTurnEvent, "type").String())
	require.Equal(t, "resp_bridge_first", gjson.GetBytes(firstTurnEvent, "response.id").String())

	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_bridge_first","input":[{"type":"function_call_output","call_id":"call_bridge_1","output":"ok"}]}`)
	secondTurnEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(secondTurnEvent, "type").String())
	require.Equal(t, "resp_bridge_second", gjson.GetBytes(secondTurnEvent, "response.id").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.Len(t, upstream.bodies, 2, "进入 HTTP bridge 后同一客户端 WS 连接内应保持 HTTP/SSE bridge")
	require.False(t, gjson.GetBytes(upstream.bodies[0], "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	secondInput := gjson.GetBytes(upstream.bodies[1], "input").Array()
	require.Len(t, secondInput, 3)
	require.Equal(t, "first", secondInput[0].String())
	require.Equal(t, "function_call", secondInput[1].Get("type").String())
	require.Equal(t, "call_bridge_1", secondInput[1].Get("call_id").String())
	require.Equal(t, "function_call_output", secondInput[2].Get("type").String())
	require.Equal(t, "call_bridge_1", secondInput[2].Get("call_id").String())
	require.Equal(t, 0, captureDialer.DialCount())
	require.Empty(t, captureConn.writes)
}
