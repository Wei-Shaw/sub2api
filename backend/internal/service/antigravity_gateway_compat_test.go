package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type antigravityCompatTokenCache struct {
	token string
REDACTED

type antigravityCompatErrorReader struct {
	data []byte
	off  int
	err  error
REDACTED

func (r *antigravityCompatErrorReader) Read(p []byte) (int, error) {
	if r.off < len(r.data) {
		n := copy(p, r.data[r.off:])
		r.off += n
		return n, nil
REDACTED
	return 0, r.err
REDACTED

func (r *antigravityCompatErrorReader) Close() error { return nil REDACTED

func (c *antigravityCompatTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return c.token, nil
REDACTED

func (c *antigravityCompatTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (c *antigravityCompatTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
REDACTED

func (c *antigravityCompatTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *antigravityCompatTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
REDACTED

func newAntigravityCompatService(cfg config.GatewayConfig, upstream HTTPUpstream) *AntigravityGatewayService {
	tokenProvider := NewAntigravityTokenProvider(
		nil,
		&antigravityCompatTokenCache{token: "fresh-oauth-token"REDACTED,
		nil,
	)
	return NewAntigravityGatewayService(
		nil,
		nil,
		nil,
		tokenProvider,
		nil,
		upstream,
		NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: cfgREDACTED),
		nil,
	)
REDACTED

func newAntigravityCompatAccount(accountType string) *Account {
REDACTED
		ID:          3757,
		Name:        "antigravity-compat",
		Platform:    PlatformAntigravity,
		Type:        accountType,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "stale-account-token",
			"project_id":   "project-3757",
			"model_mapping": map[string]any{
				"gemini-3.1-pro-high": "gemini-3.1-pro-high",
				"claude-sonnet-4-5":   "claude-sonnet-4-5",
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func newAntigravityCompatContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	return c, recorder
REDACTED

func antigravityCompatSuccessResponse() *http.Response {
	body := `data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"ok"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3REDACTEDREDACTEDREDACTED` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"REDACTED,
			"X-Request-Id": []string{"request-3757"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED

func TestAntigravityCompatOAuthUsesNativeTokenAndRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body []byte
		call func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
REDACTED{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"Reply exactly: ok"REDACTED]REDACTED`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"gemini-3.1-pro-high","input":"Reply exactly: ok"REDACTED`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var authorization string
			var upstreamPath string
			var upstreamAlt string
			upstream := &queuedHTTPUpstreamStub{
				responses: []*http.Response{antigravityCompatSuccessResponse()REDACTED,
				onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
					authorization = req.Header.Get("Authorization")
					upstreamPath = req.URL.Path
					upstreamAlt = req.URL.Query().Get("alt")
			REDACTED,
		REDACTED
			svc := newAntigravityCompatService(
				config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
				upstream,
			)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, tt.body)

			result, err := tt.call(svc, context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), tt.body)

		REDACTED
			require.NotNil(t, result)
			require.Equal(t, "Bearer fresh-oauth-token", authorization)
			require.Equal(t, "/v1internal:streamGenerateContent", upstreamPath)
			require.Equal(t, "sse", upstreamAlt)
			require.Equal(t, "request-3757", result.RequestID)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "ok")
			if tt.name == "chat completions" {
				require.Equal(t, "stop", gjson.Get(recorder.Body.String(), "choices.0.finish_reason").String())
				require.Equal(t, int64(8), gjson.Get(recorder.Body.String(), "usage.prompt_tokens").Int())
				require.Equal(t, int64(3), gjson.Get(recorder.Body.String(), "usage.completion_tokens").Int())
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatRejectsUnsupportedAccountType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		path        string
		accountType string
		call        func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
REDACTED{
		{
			name:        "chat completions upstream",
			path:        "/v1/chat/completions",
			accountType: AccountTypeUpstream,
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
		{
			name:        "responses setup token",
			path:        "/v1/responses",
			accountType: AccountTypeSetupToken,
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high"REDACTED`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, body)

			result, err := tt.call(&AntigravityGatewayService{REDACTED, context.Background(), c, newAntigravityCompatAccount(tt.accountType), body)

		REDACTED
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "native OAuth account required for antigravity compatibility mode")
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatPreservesChatTokenLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
		want int64
REDACTED{
		{
			name: "legacy max_tokens below bridge floor",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"REDACTED],"max_tokens":8REDACTED`,
			want: 8,
	REDACTED,
		{
			name: "max_completion_tokens takes precedence",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"REDACTED],"max_tokens":8,"max_completion_tokens":13REDACTED`,
			want: 13,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()REDACTEDREDACTED
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, upstream)
			body := []byte(tt.body)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			result, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				body,
				nil,
			)

		REDACTED
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.want, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.maxOutputTokens").Int())
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatRoutesByMappedModelFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		model         string
		wantSessionID bool
REDACTED{
		{model: "gemini-3.1-pro-high", wantSessionID: falseREDACTED,
		{model: "claude-sonnet-4-5", wantSessionID: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()REDACTEDREDACTED
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, upstream)
			body := []byte(`{"model":"` + tt.model + `","messages":[{"role":"user","content":"ok"REDACTED],"max_tokens":8REDACTED`)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			result, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				body,
				nil,
			)

		REDACTED
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.model, gjson.GetBytes(upstream.requestBodies[0], "model").String())
			require.Equal(t, tt.wantSessionID, gjson.GetBytes(upstream.requestBodies[0], "request.sessionId").Exists())
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatUnauthorizedIsCredentialFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"X-Request-Id": []string{"auth-3757"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid bearer token"REDACTEDREDACTED`)),
REDACTEDREDACTEDREDACTED
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, upstream)
	body := []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"REDACTED]REDACTED`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		newAntigravityCompatAccount(AccountTypeOAuth),
		body,
		nil,
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureStageAccountAuth, failoverErr.Stage)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.Equal(t, AntigravityCredentialRejectedReason, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Equal(t, http.StatusBadGateway, failoverErr.ClientStatusCode)
	require.Equal(t, AntigravityCredentialRejectedClientMessage, failoverErr.ClientMessage)
	require.Equal(t, "auth-3757", failoverErr.ResponseHeaders.Get("X-Request-Id"))
	require.Empty(t, recorder.Body.String())
REDACTED

func TestAntigravityCompatEmptyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*AntigravityGatewayService, *gin.Context, *http.Response) (*antigravityStreamResult, error)
REDACTED{
		{
			name: "chat completions",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", true)
		REDACTED,
	REDACTED,
		{
			name: "responses",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader("data: malformed\n\ndata: [DONE]\n\n")),
		REDACTED

			result, err := tt.run(svc, c, resp)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatUsageOnlyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*AntigravityGatewayService, *gin.Context, *http.Response) (*antigravityStreamResult, error)
REDACTED{
		{
			name: "chat completions",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", true)
		REDACTED,
	REDACTED,
		{
			name: "responses",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
				Body: io.NopCloser(strings.NewReader(
					`data: {"response":{"responseId":"resp_3757","usageMetadata":{"promptTokenCount":8REDACTEDREDACTEDREDACTED` + "\n\n",
				)),
		REDACTED

			result, err := tt.run(svc, c, resp)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatUsageOnlyNonStreamingTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body []byte
		call func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
REDACTED{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"REDACTED]REDACTED`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"gemini-3.1-pro-high","input":"ok"REDACTED`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
				Body: io.NopCloser(strings.NewReader(
					`data: {"response":{"responseId":"resp_3757","usageMetadata":{"promptTokenCount":8REDACTEDREDACTEDREDACTED` + "\n\n",
				)),
		REDACTEDREDACTEDREDACTED
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, upstream)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, tt.body)

			result, err := tt.call(
				svc,
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				tt.body,
			)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
	REDACTED)
REDACTED
REDACTED

func TestAntigravityCompatChatStreamMapsToolCallAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	body := `data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"functionCall":{"id":"call_3757","name":"get_weather","args":{"city":"Tokyo"REDACTEDREDACTEDREDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3REDACTEDREDACTEDREDACTED` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED

	result, err := svc.handleChatCompletionsStreamingFromAntigravity(
		c,
		resp,
		time.Now(),
		"gemini-3.1-pro-high",
		true,
	)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, 8, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), `"tool_calls"`)
	require.Contains(t, recorder.Body.String(), `"get_weather"`)
	require.Contains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
	require.Contains(t, recorder.Body.String(), `"prompt_tokens":8`)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
REDACTED

func TestAntigravityCompatFirstEventTimeoutTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(
		config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1REDACTED,
		nil,
	)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: readerREDACTED
	type outcome struct {
		result *antigravityStreamResult
		err    error
REDACTED
	done := make(chan outcome, 1)

	go func() {
		result, err := svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", false)
		done <- outcome{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-done:
		require.Nil(t, got.result)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, got.err, &failoverErr)
		require.True(t, failoverErr.RetryableOnSameAccount)
		require.Empty(t, recorder.Header().Get("Content-Type"))
	case <-time.After(2 * time.Second):
		_ = writer.Close()
		_ = reader.Close()
		t.Fatal("compat stream ignored StreamDataIntervalTimeout")
REDACTED
	_ = writer.Close()
	_ = reader.Close()
REDACTED

func TestAntigravityCompatClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, nil)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := strings.Join([]string{
		`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1REDACTEDREDACTEDREDACTED`,
		"",
		`data: {"response":{"responseId":"resp_3757","candidates":[{"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":15REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED

	result, err := svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", false)

REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 15, result.usage.OutputTokens)
REDACTED

func TestAntigravityCompatStreamErrorCommitsSingleTerminalFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	body := []byte(`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1REDACTEDREDACTEDREDACTED` + "\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: &antigravityCompatErrorReader{
			data: body,
			err:  io.ErrUnexpectedEOF,
	REDACTED,
REDACTED

	result, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")

REDACTED
	require.Nil(t, result)
	require.True(t, IsResponseCommitted(c))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error"))
REDACTED

func TestAntigravityCompatKeepaliveAfterFirstEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(
		config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1REDACTED,
		nil,
	)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: readerREDACTED
	done := make(chan error, 1)

	go func() {
		_, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
		done <- err
REDACTED()
	_, err := io.WriteString(
		writer,
		`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1REDACTEDREDACTEDREDACTED`+"\n\n",
	)
REDACTED
	time.Sleep(1200 * time.Millisecond)
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.NoError(t, reader.Close())
REDACTED
