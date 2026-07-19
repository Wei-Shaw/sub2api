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

	cfg := &config.Config{REDACTED
	require.Equal(t, defaultTimeout, ResolveOpenAIWSClientFirstMessageTimeout(cfg))

	cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds = 120
	require.Equal(t, 120*time.Second, ResolveOpenAIWSClientFirstMessageTimeout(cfg))
REDACTED

func TestPrepareOpenAIWSHTTPBridgeBodyStripsWSFields(t *testing.T) {
	body, err := prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":false,"previous_response_id":"resp_prev","input":"hi"REDACTED`))
REDACTED
	require.False(t, gjson.GetBytes(body, "type").Exists())
	require.False(t, gjson.GetBytes(body, "generate").Exists())
	require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
	require.Equal(t, "gpt-5", gjson.GetBytes(body, "model").String())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
	require.Equal(t, "hi", gjson.GetBytes(body, "input").String())
REDACTED

func TestOpenAIWSHTTPBridgeDecisionKeepsSmallFramesOnWS(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					HTTPBridgeEnabled:        true,
					HTTPBridgeThresholdBytes: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 99, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(nil, 100, ""))
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 1000, "resp_existing"))

	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = false
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(nil, 1000, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformGrokREDACTED, 1, "resp_existing"))
REDACTED

func TestOpenAIWSHTTPBridgeRelaysSSEFramesAsWebSocketMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_bridge","model":"gpt-5"REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_text.delta","response":{"id":"resp_bridge"REDACTED,"delta":"ok"REDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_bridge","model":"gpt-5","usage":{"input_tokens":3,"output_tokens":2REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"REDACTED,
			"x-request-id": []string{"rid_bridge"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(sseBody)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
				OpenAIWS: config.GatewayOpenAIWSConfig{
					HTTPBridgeEnabled:        true,
					HTTPBridgeThresholdBytes: 1,
			REDACTED,
		REDACTED,
	REDACTED,
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Status:      StatusActive,
REDACTED
	payload := []byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":true,"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"REDACTED,"input":"hi"REDACTED`)

	type bridgeResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan bridgeResult, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			resultCh <- bridgeResult{err: errREDACTED
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		ginCtx.Request = req

		writeClient := func(message []byte) error {
			writeCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			return conn.Write(writeCtx, coderws.MessageText, message)
	REDACTED
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
		resultCh <- bridgeResult{result: result, err: bridgeErrREDACTED
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()

	readEvent := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		msgType, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
REDACTED

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
REDACTED

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
REDACTED

func TestProxyOpenAIWSHTTPBridgeTurnForGrokDefaultsEmptyModelTo45(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_grok_default","model":"grok-4.5"REDACTEDREDACTED`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_grok_default","model":"grok-4.5","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          72,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED"base_url": xai.DefaultCLIBaseURLREDACTED,
REDACTED
	payload := []byte(`{"type":"response.create","generate":true,"stream":true,"input":"hi"REDACTED`)
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
	REDACTED,
	)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, grokDefaultResponsesModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Len(t, events, 2)
REDACTED

func TestProxyOpenAIWSHTTPBridgeTurnPromotesCodexAdditionalToolsForMixedCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_grok_codex_lite","model":"grok-4.5"REDACTEDREDACTED`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_grok_codex_lite","model":"grok-4.5","usage":{"input_tokens":4,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          73,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"base_url":          xai.DefaultCLIBaseURL,
			"subscription_tier": "free",
	REDACTED,
REDACTED
	payload := []byte(`{
		"type":"response.create","generate":true,"model":"grok","stream":true,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED,
				{"type":"function","name":"web_search","parameters":{"type":"object"REDACTEDREDACTED,
				{"type":"custom","name":"apply_patch"REDACTED,
				{"type":"namespace","name":"collaboration"REDACTED
			]REDACTED,
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"REDACTED]REDACTED
		]
REDACTED`)
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
	REDACTED,
	)

REDACTED
	require.NotNil(t, result)
	require.Len(t, events, 2)
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools")`).Exists())
	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="custom")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="namespace")`).Exists())
	require.Equal(t, "isolated-ws-cache-id", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "isolated-ws-cache-id", upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Empty(t, upstream.lastReq.Header.Get(grokClientToolCacheOptInHeader))
REDACTED

func TestProxyResponsesWebSocketFromClientForGrokUsesXAIHTTPBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bridgeResponse := func(responseID, requestID string, cachedTokens int) *http.Response {
		sseBody := strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"` + responseID + `","model":"grok-4.3"REDACTEDREDACTED`,
			"",
			`data: {"type":"response.output_text.delta","response":{"id":"` + responseID + `"REDACTED,"delta":"ok"REDACTED`,
			"",
			`data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"grok-4.3","usage":{"input_tokens":4,"output_tokens":2,"input_tokens_details":{"cached_tokens":` + fmt.Sprintf("%d", cachedTokens) + `REDACTEDREDACTEDREDACTEDREDACTED`,
			"",
	REDACTED, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":   []string{"text/event-stream"REDACTED,
				"Xai-Request-Id": []string{requestIDREDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(sseBody)),
	REDACTED
REDACTED
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		bridgeResponse("resp_grok_ws_1", "xai-ws-req-1", 0),
		bridgeResponse("resp_grok_ws_2", "xai-ws-req-2", 3),
		bridgeResponse("resp_grok_ws_3", "xai-ws-req-3", 0),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          71,
		Name:        "grok",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Status:      StatusActive,
REDACTED
			"base_url": xai.DefaultCLIBaseURL,
	REDACTED,
REDACTED

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			errCh <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
	REDACTED
		if msgType != coderws.MessageText {
			errCh <- errors.New("first message was not text")
			return
	REDACTED

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		ginCtx.Request = req
		ginCtx.Set("api_key", &APIKey{ID: 7101REDACTED)

		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "access-token", firstMessage, nil)
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"grok","stream":true,"input":"hi","prompt_cache_retention":"24h"REDACTED`))
	cancelWrite()
REDACTED

	readEvent := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		msgType, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
REDACTED

	created := readEvent()
	delta := readEvent()
	completed := readEvent()
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	require.Equal(t, "resp_grok_ws_1", gjson.GetBytes(completed, "response.id").String())

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"grok","stream":true,"previous_response_id":"resp_grok_ws_1","input":"second turn"REDACTED`))
	cancelWrite()
REDACTED

	created = readEvent()
	delta = readEvent()
	completed = readEvent()
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	require.Equal(t, "resp_grok_ws_2", gjson.GetBytes(completed, "response.id").String())

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","generate":true,"model":"grok-4.3","stream":true,"previous_response_id":"resp_grok_ws_2","input":"third turn with a different model"REDACTED`))
	cancelWrite()
REDACTED

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
REDACTED

	require.Len(t, upstream.requests, 3)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "sub2api-grok/1.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.bodies[1], "model").String())
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
	require.Equal(t, firstIdentity, secondIdentity)
	require.NotEmpty(t, thirdIdentity)
	require.NotEqual(t, firstIdentity, thirdIdentity)
	require.Equal(t, firstIdentity, upstream.requests[0].Header.Get(grokConversationIDHeader))
	require.Equal(t, secondIdentity, upstream.requests[1].Header.Get(grokConversationIDHeader))
	require.Equal(t, thirdIdentity, upstream.requests[2].Header.Get(grokConversationIDHeader))
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_retention").Exists())
REDACTED

func TestOpenAIWSHTTPBridgeAcceptsFirstFrameAboveLegacy16MiB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_large_bridge","model":"gpt-5"REDACTEDREDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_large_bridge","model":"gpt-5","usage":{"input_tokens":9,"output_tokens":1REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"REDACTED,
			"x-request-id": []string{"rid_large_bridge"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(sseBody)),
REDACTEDREDACTED
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
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{
		cfg:           cfg,
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
REDACTED
	account := &Account{
		ID:          9,
		Name:        "api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"api_key": "sk-upstream"REDACTED,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
	REDACTED,
		Concurrency: 1,
		Status:      StatusActive,
REDACTED

	payload := []byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":true,"input":"` + strings.Repeat("x", 17*1024*1024) + `"REDACTED`)
	require.Greater(t, len(payload), 16*1024*1024)
	require.Less(t, int64(len(payload)), ResolveOpenAIWSClientReadLimitBytes(cfg))

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			errCh <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()
		conn.SetReadLimit(ResolveOpenAIWSClientReadLimitBytes(cfg))

		readCtx, cancelRead := context.WithTimeout(r.Context(), 10*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
	REDACTED
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			errCh <- NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "unexpected client websocket message type", nil)
			return
	REDACTED

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "codex_cli_rs/0.135.0")
		ginCtx.Request = req

		proxyCtx, cancelProxy := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancelProxy()
		errCh <- svc.ProxyResponsesWebSocketFromClient(proxyCtx, ginCtx, conn, account, "sk-test", firstMessage, nil)
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 5*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 20*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, payload)
	cancelWrite()
REDACTED

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
	REDACTED
REDACTED
	require.Contains(t, eventTypes, "response.created")
	require.Contains(t, eventTypes, "response.completed")

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
REDACTED

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Greater(t, len(upstream.lastBody), 16*1024*1024)
	require.False(t, gjson.GetBytes(upstream.lastBody, "type").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "generate").Exists())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "gpt-5", gjson.GetBytes(upstream.lastBody, "model").String())
REDACTED

func TestOpenAIWSHTTPBridgeKeepsContinuationFramesOnHTTPWithoutPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_first","model":"gpt-5.1","output":[{"type":"function_call","id":"fc_bridge_1","call_id":"call_bridge_1","name":"shell","arguments":"{REDACTED"REDACTED],"usage":{"input_tokens":9,"output_tokens":1REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	secondSSEBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(firstSSEBody)),
	REDACTED,
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(secondSSEBody)),
	REDACTED,
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
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

	captureConn := &openAIWSCaptureConn{REDACTED
	captureDialer := &openAIWSCaptureDialer{conn: captureConnREDACTED
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{REDACTED,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
REDACTED
	account := &Account{
		ID:          19,
		Name:        "api-key-bridge-handoff",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"api_key": "sk-upstream"REDACTED,
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
	REDACTED,
		Concurrency: 1,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			errCh <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
	REDACTED
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			errCh <- NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "unexpected client websocket message type", nil)
			return
	REDACTED

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "codex_cli_rs/0.135.0")
		ginCtx.Request = req

		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()

	writeMessage := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWrite()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
REDACTED
	readMessage := func() []byte {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelRead()
		msgType, event, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return event
REDACTED

	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"first"REDACTED`)
	firstTurnEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(firstTurnEvent, "type").String())
	require.Equal(t, "resp_bridge_first", gjson.GetBytes(firstTurnEvent, "response.id").String())

	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_bridge_first","input":[{"type":"function_call_output","call_id":"call_bridge_1","output":"ok"REDACTED]REDACTED`)
	secondTurnEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(secondTurnEvent, "type").String())
	require.Equal(t, "resp_bridge_second", gjson.GetBytes(secondTurnEvent, "response.id").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-errCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket bridge proxy to finish")
REDACTED

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
REDACTED

func TestOpenAIWSHTTPBridge_IdleTimeoutClosesClientSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sseBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_bridge_idle","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(sseBody)),
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
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
		cache:            &stubGatewayCache{REDACTED,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
REDACTED
	account := &Account{
		ID:          20,
		Name:        "api-key-bridge-idle-timeout",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"api_key": "sk-upstream"REDACTED,
		Extra:       map[string]any{"responses_websockets_v2_enabled": trueREDACTED,
		Concurrency: 1,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			errCh <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			errCh <- err
			return
	REDACTED
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		ginCtx.Request = r.Clone(r.Context())
		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"input":"hello"REDACTED`))
	cancelWrite()
REDACTED

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, err := clientConn.Read(readCtx)
	cancelRead()
REDACTED
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
REDACTED
	require.Len(t, upstream.bodies, 1, "an idle client must not leave a continuation request running")
REDACTED
