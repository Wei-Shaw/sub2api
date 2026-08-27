package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAICompactFallbackTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func TestPrepareOpenAICompactFallbackRetryRequiresExplicitCompact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}}
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"}]}`)
	errorBody := []byte(`{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"}}`)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "maximum context length exceeded", errorBody, false,
	)

	require.False(t, retry)
	require.Empty(t, fallbackModel)
	require.Equal(t, body, retryBody)
}

func TestPrepareOpenAICompactFallbackRetryPreservesNativeTriggerAndContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}}
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	MarkOpenAINativeCompactionV2(c)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	errorBody := []byte(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"}}`)
	pathBefore := openAIResponsesRequestPathSuffix(c)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "context window exceeded", errorBody, false,
	)

	require.True(t, retry)
	require.Equal(t, "gpt-5.4", fallbackModel)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(retryBody, "model").String())
	require.True(t, HasCompactionTriggerInInput(retryBody))
	require.True(t, isOpenAINativeCompactionV2(c))
	require.Equal(t, pathBefore, openAIResponsesRequestPathSuffix(c))
}

func TestResolveOpenAICompactFallbackModelPrefersAccountMapping(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "global-compact"}}}
	account := &Account{Credentials: map[string]any{
		"compact_model_mapping": map[string]any{"gpt-5.5": "account-compact"},
	}}

	require.Equal(t, "account-compact", svc.resolveOpenAICompactFallbackModel(account, "gpt-5.5"))
	require.Equal(t, "global-compact", svc.resolveOpenAICompactFallbackModel(account, "unmapped-model"))
}

func TestOpenAIGatewayForwardUsesGlobalCompactModelOnInitialLegacyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[]}`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"global-compact","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "global-compact"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "global-compact", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Contains(t, upstream.requests[0].URL.Path, "/compact")
}

func TestPrepareOpenAICompactFallbackRetryLegacyPathAndSingleAttemptGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}}
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	errorBody := []byte(`{"response":{"status":"failed","error":null}}`)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "", errorBody, false,
	)
	require.True(t, retry)
	require.Equal(t, "gpt-5.4", fallbackModel)
	require.Equal(t, "/compact", openAIResponsesRequestPathSuffix(c))

	secondBody, secondModel, secondRetry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", retryBody, http.StatusBadRequest, "", errorBody, true,
	)
	require.False(t, secondRetry)
	require.Empty(t, secondModel)
	require.Equal(t, retryBody, secondBody)
}

func TestPrepareOpenAICompactFallbackRetryDoesNotHideSpecificBusinessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}}
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	errorBody := []byte(`{"response":{"status":"failed","error":{"type":"permission_error","message":"workspace denied"}}}`)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "workspace denied", errorBody, false,
	)

	require.False(t, retry)
	require.Empty(t, fallbackModel)
	require.Equal(t, body, retryBody)
}

func TestIsOpenAICompactModelFailureRequiresExplicitModelAvailabilityMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "explicit unsupported model", message: "The requested model is not supported", want: true},
		{name: "named missing model", message: "The model `gpt-5.5` does not exist", want: true},
		{name: "unsupported model code-like message", message: "unsupported model: gpt-5.5", want: true},
		{name: "unsupported model feature", message: "This model output format is not supported", want: false},
		{name: "unsupported parameter for model", message: "Parameter tools is not supported for this model", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isOpenAICompactModelFailure(
				http.StatusBadRequest,
				tt.message,
				[]byte(`{"error":{"message":`+strconv.Quote(tt.message)+`}}`),
			))
		})
	}
}

func TestPrepareOpenAICompactFallbackRetrySkipsSameModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.5"}}}
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	errorBody := []byte(`{"error":{"code":"model_not_found","message":"model not found"}}`)

	_, _, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusNotFound, "model not found", errorBody, false,
	)
	require.False(t, retry)
}

func TestOpenAIGatewayForwardRetriesExplicitNativeCompactHTTPFailureOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.True(t, HasCompactionTriggerInInput(upstream.bodies[1]))
	require.Equal(t, upstream.requests[0].URL.Path, upstream.requests[1].URL.Path)
	require.NotContains(t, upstream.requests[1].URL.Path, "/compact")
	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "retry", events[0].Kind)
	require.Equal(t, "compact_model_fallback", events[0].Reason)
	require.Equal(t, http.StatusBadRequest, events[0].UpstreamStatusCode)
}

func TestOpenAIGatewayForwardRetriesExplicitNativeCompactSSEFailureBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("event: response.failed\n" +
				`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"}}}` + "\n\n")),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.Equal(t, upstream.requests[0].URL.Path, upstream.requests[1].URL.Path)
	require.NotContains(t, upstream.requests[1].URL.Path, "/compact")
}

func TestOpenAIGatewayForwardRetriesStreamingCompactFailureBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	failed := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"}}}` + "\n\n"
	completed := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed))},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.NotContains(t, recorder.Body.String(), "context_length_exceeded")
	require.Contains(t, recorder.Body.String(), "response.completed")
}

func TestOpenAIGatewayForwardDoesNotRecurseWhenCompactFallbackAlsoFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	failed := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"model_not_found","message":"model not found"}}}` + "\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	var compactSignal *openAICompactFallbackSignal
	require.False(t, errors.As(err, &compactSignal))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "model not found")
	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Equal(t, "retry", events[0].Kind)
	require.Equal(t, "compact_model_fallback", events[0].Reason)
	require.Equal(t, "http_error", events[1].Kind)
}

func TestOpenAIPassthroughCompactFallbackSecondStreamFailureUsesStandardErrorPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	failed := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"}}}` + "\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failed))},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}

	result, err := svc.forwardOpenAIPassthrough(
		context.Background(), c, account, body, body, "gpt-5.5", false, nil, true, time.Now(),
	)

	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.bodies, 2)
	var compactSignal *openAICompactFallbackSignal
	require.False(t, errors.As(err, &compactSignal))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "context window exceeded")
	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Equal(t, "retry", events[0].Kind)
	require.Equal(t, "compact_model_fallback", events[0].Reason)
	require.Equal(t, "http_error", events[1].Kind)
	require.True(t, events[1].Passthrough)
}
