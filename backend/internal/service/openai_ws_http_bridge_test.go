package service

import (
	"context"
	"errors"
	"fmt"
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

func TestResolveOpenAIWSClientFirstMessageTimeout(t *testing.T) {
	defaultTimeout := time.Duration(config.DefaultOpenAIWSClientFirstMessageTimeoutSeconds) * time.Second
	require.Equal(t, defaultTimeout, ResolveOpenAIWSClientFirstMessageTimeout(nil))

	cfg := &config.Config{}
	require.Equal(t, defaultTimeout, ResolveOpenAIWSClientFirstMessageTimeout(cfg))

	cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds = 120
	require.Equal(t, 120*time.Second, ResolveOpenAIWSClientFirstMessageTimeout(cfg))
}

func TestPrepareOpenAIWSHTTPBridgeBodyStripsWSFields(t *testing.T) {
	body, err := prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":false,"previous_response_id":"resp_prev","input":"hi","sequence":900719925474099312345}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "type").Exists())
	require.False(t, gjson.GetBytes(body, "generate").Exists())
	require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
	require.Equal(t, "gpt-5", gjson.GetBytes(body, "model").String())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
	require.Equal(t, "hi", gjson.GetBytes(body, "input").String())
	require.Equal(t, "900719925474099312345", gjson.GetBytes(body, "sequence").Raw)
	_, err = prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create"}{"trailing":true}`))
	require.Error(t, err)
}

func TestProxyOpenAIWSHTTPBridgeTurn_UpstreamDefaultServiceTierWinsOverRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// proxyOpenAIWSHTTPBridgeTurn 是 client WS→HTTP bridge，本身不 canonicalize
	// fast→priority；生产入口的归一化在 openai_ws_forwarder_ingress.go 的 fast
	// policy。本测试只覆盖局部 observer：canonical 请求 priority 被上游
	// response.completed service_tier=default 覆盖。
	sse := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_tier","model":"gpt-5.5","status":"completed","service_tier":"default","usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{ID: 5881, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	payload := []byte(`{"type":"response.create","model":"gpt-5.5","stream":true,"service_tier":"priority","input":"hi"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "test-token", payload, len(payload),
		"gpt-5.5", "", "", "", "", 1,
		func([]byte) error { return nil },
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "default", *result.ServiceTier)
}

func TestProxyOpenAIWSHTTPBridgeTurn_UpstreamDefaultWinsOverFastAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 客户端别名 fast 同样被上游回显的 default 覆盖：局部 observer 的
	// ServiceTier() 是唯一计费依据，绝不回退到请求侧 fast。
	sse := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_tier2","model":"gpt-5.5","status":"completed","service_tier":"default","usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{ID: 5882, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	payload := []byte(`{"type":"response.create","model":"gpt-5.5","stream":true,"service_tier":"fast","input":"hi"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "test-token", payload, len(payload),
		"gpt-5.5", "", "", "", "", 1,
		func([]byte) error { return nil },
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "default", *result.ServiceTier,
		"local observer's upstream-echoed default must win over the fast alias")
}

func TestProxyOpenAIWSHTTPBridgeTurnAPIKeyAdaptsClientTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","status":"in_progress"}}`,
		``,
		`data: {"type":"response.function_call_arguments.done","sequence_number":1,"output_index":0,"item_id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}"}`,
		``,
		`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}","status":"completed"}}`,
		``,
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_tools","status":"completed","output":[{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{ID: 5659, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	payload := []byte(`{
		"type":"response.create","model":"gpt-5","stream":true,
		"tools":[{"type":"custom","name":"exec","description":"Run a command"}],
		"input":[
			{"type":"custom_tool_call","id":"previous_item","call_id":"previous_call","name":"exec","input":"echo ready"},
			{"type":"custom_tool_call_output","call_id":"previous_call","output":"ready"}
		]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "test-token", payload, len(payload),
		"gpt-5", "", "", "", "", 2,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.JSONEq(t, `{"input":"echo ready"}`, gjson.GetBytes(upstream.lastBody, "input.0.arguments").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.input").Exists())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())

	var outputDone, completed []byte
	for _, event := range events {
		switch gjson.GetBytes(event, "type").String() {
		case "response.output_item.done":
			outputDone = event
		case "response.completed":
			completed = event
		}
	}
	require.NotEmpty(t, outputDone)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(outputDone, "item.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(outputDone, "item.input").String())
	require.False(t, gjson.GetBytes(outputDone, "item.arguments").Exists())
	require.NotEmpty(t, completed)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(completed, "response.output.0.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(completed, "response.output.0.input").String())
	require.True(t, result.wsReplayInputExists)
	require.Len(t, result.wsReplayInput, 1)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(result.wsReplayInput[0], "type").String())
	require.Equal(t, "pwd", gjson.GetBytes(result.wsReplayInput[0], "input").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnAPIKeyRestoresClientToolsInResponseDone(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","status":"in_progress"}}`,
		``,
		`data: {"type":"response.function_call_arguments.done","sequence_number":1,"output_index":0,"item_id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}"}`,
		``,
		`data: {"type":"response.done","sequence_number":2,"response":{"id":"resp_tools","status":"completed","output":[{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{ID: 5764, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	payload := []byte(`{
		"type":"response.create","model":"gpt-5","stream":true,
		"tools":[{"type":"custom","name":"exec","description":"Run a command"}],
		"input":"run pwd"
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "test-token", payload, len(payload),
		"gpt-5", "", "", "", "", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, events, 4)
	terminal := events[len(events)-1]
	require.Equal(t, "response.done", gjson.GetBytes(terminal, "type").String())
	require.Equal(t, int64(3), gjson.GetBytes(terminal, "sequence_number").Int())
	require.Equal(t, "custom_tool_call", gjson.GetBytes(terminal, "response.output.0.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(terminal, "response.output.0.input").String())
	require.False(t, gjson.GetBytes(terminal, "response.output.0.arguments").Exists())
	require.True(t, result.wsReplayInputExists)
	require.Len(t, result.wsReplayInput, 1)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(result.wsReplayInput[0], "type").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnGrokPromotesDiscoveryAndRestoresNamespaceSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"item_spawn","call_id":"call_spawn","name":"multi_agent_v1__spawn_agent","status":"in_progress"}}`,
		"",
		`data: {"type":"response.function_call_arguments.done","sequence_number":1,"output_index":0,"item_id":"item_spawn","call_id":"call_spawn","name":"multi_agent_v1__spawn_agent","arguments":"{\"message\":\"work\"}"}`,
		"",
		`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"function_call","id":"item_spawn","call_id":"call_spawn","name":"multi_agent_v1__spawn_agent","arguments":"{\"message\":\"work\"}","status":"completed"}}`,
		"",
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_spawn","status":"completed","output":[{"type":"function_call","id":"item_spawn","call_id":"call_spawn","name":"multi_agent_v1__spawn_agent","arguments":"{\"message\":\"work\"}","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 5765, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"base_url": xai.DefaultCLIBaseURL},
	}
	payload := []byte(`{
		"type":"response.create","model":"grok-4.5","stream":true,
		"tools":[{"type":"tool_search"}],
		"input":[
			{"type":"tool_search_call","call_id":"call_search","arguments":{"query":"subagent"},"status":"completed"},
			{"type":"tool_search_output","call_id":"call_search","execution":"client","status":"completed","tools":[
				{"type":"namespace","name":"multi_agent_v1","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
			]}
		]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok-4.5", "", "", "", "", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "multi_agent_v1__spawn_agent", gjson.GetBytes(upstream.lastBody, "tools.1.name").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.1.tools").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.1.status").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.1.execution").Exists())
	state, ok := openAIWSHTTPBridgeToolStateFromContext(c)
	require.True(t, ok)
	require.Equal(t, "multi_agent_v1", state.ClientMapping.NamespaceTools["multi_agent_v1__spawn_agent"].Namespace)
	require.Len(t, events, 4)
	require.Equal(t, "spawn_agent", gjson.GetBytes(events[0], "item.name").String())
	require.Equal(t, "multi_agent_v1", gjson.GetBytes(events[0], "item.namespace").String())
	require.Equal(t, "spawn_agent", gjson.GetBytes(events[2], "item.name").String())
	require.Equal(t, "multi_agent_v1", gjson.GetBytes(events[2], "item.namespace").String())
	require.Equal(t, "spawn_agent", gjson.GetBytes(events[3], "response.output.0.name").String())
	require.Equal(t, "multi_agent_v1", gjson.GetBytes(events[3], "response.output.0.namespace").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnGrokInheritsToolSearchAndPromotesFollowupDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstSSE := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_first\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	secondSSE := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_second\",\"output\":[{\"type\":\"function_call\",\"id\":\"item_spawn\",\"call_id\":\"call_spawn\",\"name\":\"multi_agent_v1__spawn_agent\",\"arguments\":\"{}\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstSSE))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondSSE))},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 5766, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"base_url": xai.DefaultCLIBaseURL, "subscription_tier": "free"},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	first := []byte(`{"type":"response.create","model":"grok-4.5","stream":true,"tools":[{"type":"tool_search"}],"input":"discover tools"}`)
	_, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", first, len(first),
		"grok-4.5", "", "", "", "grok-ws-cache", 1, func([]byte) error { return nil },
	)
	require.NoError(t, err)

	second := []byte(`{
		"type":"response.create","model":"grok-4.5","stream":true,
		"input":[{"type":"tool_search_output","call_id":"call_search","status":"completed","tools":[
			{"type":"namespace","name":"multi_agent_v1","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
		]}]
	}`)
	var events [][]byte
	_, err = svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", second, len(second),
		"grok-4.5", "", "", "", "grok-ws-cache", 2,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.bodies[1], "tools.0.name").String())
	require.Equal(t, "multi_agent_v1__spawn_agent", gjson.GetBytes(upstream.bodies[1], "tools.1.name").String())
	require.NotEqual(t, grokFreeCacheDisabledToolChoice, gjson.GetBytes(upstream.bodies[1], "tool_choice").String())
	require.Equal(t, "grok-ws-cache", gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.bodies[1], "input.0.type").String())
	require.Len(t, events, 1)
	require.Equal(t, "spawn_agent", gjson.GetBytes(events[0], "response.output.0.name").String())
	require.Equal(t, "multi_agent_v1", gjson.GetBytes(events[0], "response.output.0.namespace").String())
}

func TestOpenAIWSHTTPBridgeAPIKeyReusesClientToolMappingWhenFollowupOmitsTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_custom_first","model":"gpt-5.6-sol","output":[{"type":"function_call","id":"fc_custom_1","call_id":"call_custom_1","name":"exec","arguments":"{\"input\":\"pwd\"}"}],"usage":{"input_tokens":9,"output_tokens":1}}}`,
		"",
	}, "\n")
	secondSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_custom_second","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstSSEBody))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondSSEBody))},
	}}
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

	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 9001, Name: "api-key-custom-followup", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"}, Extra: map[string]any{"responses_websockets_v2_enabled": true},
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

	writeMessage(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"tools":[{"type":"custom","name":"exec"}],"input":"run pwd"}`)
	firstEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(firstEvent, "type").String())
	require.Equal(t, "custom_tool_call", gjson.GetBytes(firstEvent, "response.output.0.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(firstEvent, "response.output.0.input").String())

	writeMessage(`{"type":"response.create","model":"gpt-5.6-sol","stream":true,"previous_response_id":"resp_custom_first","input":[{"type":"custom_tool_call_output","id":"ctco_client_output_1","call_id":"call_custom_1","output":"ok"}]}`)
	secondEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(secondEvent, "type").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.Len(t, upstream.bodies, 2)
	firstTools := gjson.GetBytes(upstream.bodies[0], "tools").Array()
	require.Len(t, firstTools, 1)
	require.Equal(t, "function", firstTools[0].Get("type").String())
	secondTools := gjson.GetBytes(upstream.bodies[1], "tools").Array()
	require.Len(t, secondTools, 1)
	require.Equal(t, "function", secondTools[0].Get("type").String())
	require.Equal(t, "exec", secondTools[0].Get("name").String())
	secondInput := gjson.GetBytes(upstream.bodies[1], "input").Array()
	require.Len(t, secondInput, 3)
	require.Equal(t, "run pwd", secondInput[0].String())
	require.Equal(t, "function_call", secondInput[1].Get("type").String())
	require.Equal(t, "fc_custom_1", secondInput[1].Get("id").String())
	require.JSONEq(t, `{"input":"pwd"}`, secondInput[1].Get("arguments").String())
	require.False(t, secondInput[1].Get("input").Exists())
	require.Equal(t, "function_call_output", secondInput[2].Get("type").String())
	require.False(t, secondInput[2].Get("id").Exists())
}

func TestOpenAIWSHTTPBridgeFullCustomToolHistoryWithoutPreviousResponseIDDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	completed := func(responseID string, output string) string {
		return "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"" + responseID + "\",\"model\":\"gpt-5.1\",\"output\":" + output + ",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_1", `[{"type":"custom_tool_call","id":"item_1","call_id":"call_1","name":"exec","input":"pwd"}]`)))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_2", `[]`)))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_3", `[]`)))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_4", `[]`)))},
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 9002, Name: "oauth-full-context", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{"responses_websockets_v2_enabled": true},
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
		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "test-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeAndRead := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
		cancelWrite()
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		_, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	}

	writeAndRead(`{"type":"response.create","model":"gpt-5.1","input":"run pwd"}`)
	writeAndRead(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_1","input":[{"role":"user","content":"continue without tool output"}]}`)
	fullContext := `{"type":"response.create","model":"gpt-5.1","input":[{"type":"custom_tool_call","id":"item_1","call_id":"call_1","name":"exec","input":"pwd"},{"type":"custom_tool_call_output","call_id":"call_1","output":"/tmp"},{"role":"user","content":"continue"}]}`
	writeAndRead(fullContext)
	writeAndRead(fullContext)

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.Len(t, upstream.bodies, 4)
	orphanInput := gjson.GetBytes(upstream.bodies[1], "input").Array()
	require.Len(t, orphanInput, 2)
	require.Equal(t, "run pwd", orphanInput[0].String())
	require.Equal(t, "user", orphanInput[1].Get("role").String())
	for _, body := range upstream.bodies[2:] {
		input := gjson.GetBytes(body, "input").Array()
		require.Len(t, input, 3)
		require.Equal(t, "custom_tool_call", input[0].Get("type").String())
		require.Equal(t, "call_1", input[0].Get("call_id").String())
		require.Equal(t, "custom_tool_call_output", input[1].Get("type").String())
		require.Equal(t, "call_1", input[1].Get("call_id").String())
	}
}

func TestOpenAIWSHTTPBridgeObjectToolOutputWithoutPreviousResponseIDReplaysMatchingCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	completed := func(responseID string, output string) string {
		return "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"" + responseID + "\",\"model\":\"gpt-5.1\",\"output\":" + output + ",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_1", `[{"type":"custom_tool_call","id":"item_1","call_id":"call_1","name":"exec","input":"pwd"}]`)))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed("resp_2", `[]`)))},
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 9003, Name: "oauth-output-only", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{"responses_websockets_v2_enabled": true},
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
		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "test-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeAndRead := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
		cancelWrite()
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		_, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	}

	writeAndRead(`{"type":"response.create","model":"gpt-5.1","input":"run pwd"}`)
	writeAndRead(`{"type":"response.create","model":"gpt-5.1","input":{"type":"custom_tool_call_output","call_id":"call_1","output":"/tmp"}}`)

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
	}

	require.Len(t, upstream.bodies, 2)
	secondInput := gjson.GetBytes(upstream.bodies[1], "input").Array()
	require.Len(t, secondInput, 3)
	require.Equal(t, "custom_tool_call", secondInput[1].Get("type").String())
	require.Equal(t, "call_1", secondInput[1].Get("call_id").String())
	require.Equal(t, "custom_tool_call_output", secondInput[2].Get("type").String())
	require.Equal(t, "call_1", secondInput[2].Get("call_id").String())
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

func TestProxyOpenAIWSHTTPBridgeTurnTransportErrorFailoverSafety(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		turn         int
		wantFailover bool
		wantWrites   int
	}{
		{name: "first_turn_fails_over_before_downstream_event", turn: 1, wantFailover: true},
		{name: "later_turn_does_not_replay_completed_turns", turn: 2, wantWrites: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: io.EOF}
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{},
				httpUpstream: upstream,
			}
			account := &Account{
				ID:          8,
				Name:        "api-key",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
			var writes [][]byte

			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5", "", "", "", "", tt.turn,
				func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				},
			)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			if tt.wantFailover {
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
				require.JSONEq(t, string(openAITransportFailoverBody), string(failoverErr.ResponseBody))
			} else {
				require.Error(t, err)
				require.False(t, errors.As(err, &failoverErr))
			}
			require.Len(t, writes, tt.wantWrites)
			if tt.wantWrites > 0 {
				require.Equal(t, "error", gjson.GetBytes(writes[0], "type").String())
				require.Equal(t, int64(http.StatusBadGateway), gjson.GetBytes(writes[0], "status").Int())
			}
		})
	}
}

func TestProxyOpenAIWSHTTPBridgeTurnHTTPStatusFailoverSafety(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		turn         int
		status       int
		wantFailover bool
		wantWrites   int
	}{
		{name: "first_turn_401", turn: 1, status: http.StatusUnauthorized, wantFailover: true},
		{name: "first_turn_429", turn: 1, status: http.StatusTooManyRequests, wantFailover: true},
		{name: "first_turn_500", turn: 1, status: http.StatusInternalServerError, wantFailover: true},
		{name: "later_turn_500_does_not_replay", turn: 2, status: http.StatusInternalServerError, wantWrites: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"temporary upstream failure"}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
			var writes [][]byte

			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5", "", "", "", "", tt.turn,
				func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				},
			)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			if tt.wantFailover {
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, tt.status, failoverErr.StatusCode)
			} else {
				require.Error(t, err)
				require.False(t, errors.As(err, &failoverErr))
			}
			require.Len(t, writes, tt.wantWrites)
		})
	}
}

func TestProxyOpenAIWSHTTPBridgeTurnRetriesRejectedFieldBeforeClientOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"type":"invalid_request_error","code":"invalid_parameter","param":"truncation","message":"Unsupported parameter: truncation"}}`,
			)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_retry\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
			)),
		},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi","truncation":"auto"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "sk-test", payload, len(payload),
		"gpt-5", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, upstream.callCount)
	require.Len(t, upstream.bodies, 2)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "truncation").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "truncation").Exists())
	require.Len(t, writes, 1)
	require.Equal(t, "response.completed", gjson.GetBytes(writes[0], "type").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnSSEErrorFailoverSafety(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, turn := range []int{1, 2} {
		t.Run(fmt.Sprintf("turn_%d", turn), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"code\":\"rate_limit_exceeded\",\"message\":\"limited\"}}\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
			var writes [][]byte

			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5", "", "", "", "", turn,
				func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				},
			)

			var failoverErr *UpstreamFailoverError
			require.Nil(t, result)
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
			require.Empty(t, writes)
		})
	}
}

// 桥接转发 error / response.failed 给 WS 客户端前必须把容量降载码改写为可重试
// 的 server_error：Codex 对 server_is_overloaded/slow_down 判致命并终止会话。
// 账号状态判定使用改写前的原始事件，不受影响。
func TestProxyOpenAIWSHTTPBridgeTurnRewritesCapacityShedCodeForClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		turn    int
		body    string
		wantErr bool
	}{
		{
			name:    "turn2_error_frame",
			turn:    2,
			body:    "data: {\"type\":\"error\",\"error\":{\"type\":\"service_unavailable_error\",\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}\n\n",
			wantErr: true,
		},
		{
			// 后续 turn 不允许 replay，容量错误必须改写后交给客户端重试。
			name: "turn2_bare_response_failed",
			turn: 2,
			body: "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_shed\",\"status\":\"failed\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}}\n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
			var writes [][]byte

			_, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5", "", "", "", "", tt.turn,
				func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				},
			)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, writes, 1)
			require.Contains(t, string(writes[0]), `"code":"server_error"`)
			require.NotContains(t, string(writes[0]), "server_is_overloaded")
			require.Contains(t, string(writes[0]), "Our servers are currently overloaded")
		})
	}
}

func TestProxyOpenAIWSHTTPBridgeTurnBareErrorUsesAuthoritativeFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","response_id":"resp_failed","delta":"partial"}`,
		``,
		`data: {"type":"error","error":{"status_code":403,"code":"workspace_suspended","message":"workspace is suspended"}}`,
		``,
		`data: {"type":"response.failed","response":{"id":"resp_failed","status":"failed","usage":{"input_tokens":9,"output_tokens":2},"error":{"status_code":403,"code":"workspace_suspended","message":"workspace is suspended"}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
	repo := &openAIStream403AccountRepo{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: &RateLimitService{accountRepo: repo}}
	account := &Account{ID: 111, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", payload, len(payload), "gpt-5", "", "", "", "", 2, func(message []byte) error {
		writes = append(writes, append([]byte(nil), message...))
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Len(t, writes, 2)
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(writes[0], "type").String())
	require.Equal(t, "response.failed", gjson.GetBytes(writes[1], "type").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnBareErrorEOFSynthesizesFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_eof\",\"status\":\"in_progress\"}}\n\n" +
		"data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request\",\"message\":\"bad request\"}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 112, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", payload, len(payload), "gpt-5", "", "", "", "", 2, func(message []byte) error {
		writes = append(writes, append([]byte(nil), message...))
		return nil
	})

	require.EqualError(t, err, "bad request")
	require.NotNil(t, result)
	require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
	require.Len(t, writes, 2)
	require.Equal(t, "response.failed", gjson.GetBytes(writes[1], "type").String())
	require.Equal(t, "failed", gjson.GetBytes(writes[1], "response.status").String())
	require.Equal(t, "resp_eof", gjson.GetBytes(writes[1], "response.id").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnBareErrorFollowedByCompletedUsesCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_recovered","status":"in_progress"}}`,
		``,
		`data: {"type":"error","error":{"code":"transient","message":"retrying"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_recovered","status":"completed","output":[],"usage":{"input_tokens":8,"output_tokens":4}}}`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 113, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", payload, len(payload), "gpt-5", "", "", "", "", 2, func(message []byte) error {
		writes = append(writes, append([]byte(nil), message...))
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Len(t, writes, 2)
	require.Equal(t, "response.created", gjson.GetBytes(writes[0], "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(writes[1], "type").String())
}

func TestProxyOpenAIWSHTTPBridgeTurnStagesMetadataBeforeCapacityFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_shed"}}`,
		"",
		`data: {"type":"response.in_progress","response":{"id":"resp_shed"}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_shed","status":"failed","error":{"message":"Our servers are currently overloaded. Please try again later."}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-ws-bridge-capacity"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "sk-test", payload, len(payload),
		"gpt-5", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.RequestScopedTransient)
	require.Empty(t, writes)
}

func TestProxyOpenAIWSHTTPBridgeTurnDoesNotReplayCapacityAfterSemanticOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_partial"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_partial","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-ws-bridge-post-output"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "sk-test", payload, len(payload),
		"gpt-5", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.NotNil(t, result)
	require.NoError(t, err)
	require.Len(t, writes, 3)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, string(writes[2]), `"code":"server_error"`)
	require.NotContains(t, string(writes[2]), "server_is_overloaded")
	require.True(t, logSink.ContainsMessage("gateway.failover_suppressed_after_semantic_output"))
	require.True(t, logSink.ContainsFieldValue("path", "ws_http_bridge"))
}

func TestProxyOpenAIWSHTTPBridgeTurnRequiresTerminalEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		body         string
		wantFailover bool
		wantWrites   int
	}{
		{name: "done_without_events_fails_over", body: "data: [DONE]\n\n", wantFailover: true},
		{
			name: "created_then_done_fails_over_before_semantic_output",
			body: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_truncated\"}}\n\n" +
				"data: [DONE]\n\n",
			wantFailover: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
			var writes [][]byte

			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5", "", "", "", "", 1,
				func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				},
			)

			var failoverErr *UpstreamFailoverError
			if tt.wantFailover {
				require.Nil(t, result)
				require.ErrorAs(t, err, &failoverErr)
			} else {
				require.NotNil(t, result)
				require.Error(t, err)
				require.False(t, errors.As(err, &failoverErr))
			}
			require.Len(t, writes, tt.wantWrites)
		})
	}
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
	payload := []byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":true,"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"},"input":"hi"}`)

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
	require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
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
		context.Background(), c, account, "access-token", payload, len(payload),
		"", "", "", "", "", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, grokDefaultResponsesModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Len(t, events, 2)
}

func TestProxyOpenAIWSHTTPBridgeTurnPromotesCodexAdditionalToolsForMixedCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_grok_codex_lite","model":"grok-4.5"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_grok_codex_lite","model":"grok-4.5","usage":{"input_tokens":4,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          73,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"base_url":          xai.DefaultCLIBaseURL,
			"subscription_tier": "free",
		},
	}
	payload := []byte(`{
		"type":"response.create","generate":true,"model":"grok","stream":true,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"lookup","parameters":{"type":"object"}},
				{"type":"function","name":"web_search","parameters":{"type":"object"}},
				{"type":"custom","name":"apply_patch"},
				{"type":"namespace","name":"collaboration"}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
		]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set(grokClientToolCacheOptInHeader, "prefer-cache")
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok", "", "", "", "isolated-ws-cache-id", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, events, 2)
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools")`).Exists())
	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 4)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "function", tools[2].Get("type").String())
	require.Equal(t, "apply_patch", tools[2].Get("name").String())
	require.Equal(t, "x_search", tools[3].Get("type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="custom")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="namespace")`).Exists())
	require.Equal(t, "isolated-ws-cache-id", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "isolated-ws-cache-id", upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Empty(t, upstream.lastReq.Header.Get(grokClientToolCacheOptInHeader))
}

func TestProxyResponsesWebSocketFromClientForGrokUsesXAIHTTPBridgeAndPreservesMappedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bridgeResponse := func(responseID, requestID string, cachedTokens int) *http.Response {
		sseBody := strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"` + responseID + `","model":"grok-4.3"}}`,
			"",
			`data: {"type":"response.output_text.delta","response":{"id":"` + responseID + `"},"delta":"ok"}`,
			"",
			`data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"grok-4.3","usage":{"input_tokens":4,"output_tokens":2,"input_tokens_details":{"cached_tokens":` + fmt.Sprintf("%d", cachedTokens) + `}}}}`,
			"",
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":   []string{"text/event-stream"},
				"Xai-Request-Id": []string{requestID},
			},
			Body: io.NopCloser(strings.NewReader(sseBody)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		bridgeResponse("resp_grok_ws_1", "xai-ws-req-1", 0),
		bridgeResponse("resp_grok_ws_2", "xai-ws-req-2", 3),
		bridgeResponse("resp_grok_ws_3", "xai-ws-req-3", 0),
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
		ginCtx.Set("api_key", &APIKey{ID: 7101})

		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "access-token", firstMessage, &OpenAIWSIngressHooks{
			MapRequestModel: func(_ int, originalModel string) (string, error) {
				if originalModel == "channel-alias" {
					return "grok-4.3", nil
				}
				return originalModel, nil
			},
		})
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
	require.Equal(t, "resp_grok_ws_1", gjson.GetBytes(completed, "response.id").String())

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"channel-alias","stream":true,"previous_response_id":"resp_grok_ws_1","input":"second turn"}`))
	cancelWrite()
	require.NoError(t, err)

	created = readEvent()
	delta = readEvent()
	completed = readEvent()
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	require.Equal(t, "resp_grok_ws_2", gjson.GetBytes(completed, "response.id").String())
	require.Equal(t, "channel-alias", gjson.GetBytes(completed, "response.model").String())

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"grok-4.3","stream":true,"previous_response_id":"resp_grok_ws_2","input":"third turn with a different model"}`))
	cancelWrite()
	require.NoError(t, err)

	created = readEvent()
	delta = readEvent()
	completed = readEvent()
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	require.Equal(t, "resp_grok_ws_3", gjson.GetBytes(completed, "response.id").String())

	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(3 * time.Second):
		require.Fail(t, "proxy did not finish after client close")
	}

	require.Len(t, upstream.requests, 3)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "grok-4.6", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "grok-4.3", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.Equal(t, "grok-4.3", gjson.GetBytes(upstream.bodies[2], "model").String())
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String(), upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "x_search", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
	require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	firstIdentity := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	secondIdentity := gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String()
	thirdIdentity := gjson.GetBytes(upstream.bodies[2], "prompt_cache_key").String()
	require.NotEmpty(t, firstIdentity)
	require.NotEmpty(t, secondIdentity)
	require.NotEqual(t, firstIdentity, secondIdentity)
	require.NotEmpty(t, thirdIdentity)
	require.Equal(t, secondIdentity, thirdIdentity)
	require.Equal(t, firstIdentity, upstream.requests[0].Header.Get(grokConversationIDHeader))
	require.Equal(t, secondIdentity, upstream.requests[1].Header.Get(grokConversationIDHeader))
	require.Equal(t, thirdIdentity, upstream.requests[2].Header.Get(grokConversationIDHeader))
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

func TestOpenAIWSHTTPBridge_IdleTimeoutClosesClientSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_idle","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sseBody)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          20,
		Name:        "api-key-bridge-idle-timeout",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
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

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"input":"hello"}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	closeReadCtx, cancelCloseRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(closeReadCtx)
	cancelCloseRead()
	var clientClose coderws.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, coderws.StatusNormalClosure, clientClose.Code)
	require.Equal(t, "websocket idle timeout", clientClose.Reason)

	select {
	case proxyErr := <-errCh:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, proxyErr, &closeErr)
		require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
		require.Equal(t, "websocket idle timeout", closeErr.Reason())
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for idle HTTP bridge session to close")
	}
	require.Len(t, upstream.bodies, 1, "an idle client must not leave a continuation request running")
}
