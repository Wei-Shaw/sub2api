package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAICompactFallbackTestContext(t *testing.T, path string) *gin.Context {
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
REDACTED

func TestPrepareOpenAICompactFallbackRetryRequiresExplicitCompact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTEDREDACTED
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"REDACTED]REDACTED`)
	errorBody := []byte(`{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"REDACTEDREDACTED`)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "maximum context length exceeded", errorBody, false,
	)

	require.False(t, retry)
	require.Empty(t, fallbackModel)
	require.Equal(t, body, retryBody)
REDACTED

func TestPrepareOpenAICompactFallbackRetryPreservesNativeTriggerAndContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTEDREDACTED
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	MarkOpenAINativeCompactionV2(c)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	errorBody := []byte(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"REDACTEDREDACTED`)
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
REDACTED

func TestResolveOpenAICompactFallbackModelPrefersAccountMapping(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "global-compact"REDACTEDREDACTEDREDACTED
	account := &Account{Credentials: map[string]any{
		"compact_model_mapping": map[string]any{"gpt-5.5": "account-compact"REDACTED,
REDACTEDREDACTED

	require.Equal(t, "account-compact", svc.resolveOpenAICompactFallbackModel(account, "gpt-5.5"))
	require.Equal(t, "global-compact", svc.resolveOpenAICompactFallbackModel(account, "unmapped-model"))
REDACTED

func TestOpenAIGatewayForwardUsesGlobalCompactModelOnInitialLegacyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[]REDACTED`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"global-compact","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "global-compact"REDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"REDACTED,
		Status:      StatusActive, Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "global-compact", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Contains(t, upstream.requests[0].URL.Path, "/compact")
REDACTED

func TestPrepareOpenAICompactFallbackRetryLegacyPathAndSingleAttemptGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTEDREDACTED
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]REDACTED`)
	errorBody := []byte(`{"response":{"status":"failed","error":nullREDACTEDREDACTED`)

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
REDACTED

func TestPrepareOpenAICompactFallbackRetryDoesNotHideSpecificBusinessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTEDREDACTED
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]REDACTED`)
	errorBody := []byte(`{"response":{"status":"failed","error":{"type":"permission_error","message":"workspace denied"REDACTEDREDACTEDREDACTED`)

	retryBody, fallbackModel, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusBadRequest, "workspace denied", errorBody, false,
	)

	require.False(t, retry)
	require.Empty(t, fallbackModel)
	require.Equal(t, body, retryBody)
REDACTED

func TestPrepareOpenAICompactFallbackRetrySkipsSameModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.5"REDACTEDREDACTEDREDACTED
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
	body := []byte(`{"model":"gpt-5.5","input":[]REDACTED`)
	errorBody := []byte(`{"error":{"code":"model_not_found","message":"model not found"REDACTEDREDACTED`)

	_, _, retry := svc.prepareOpenAICompactFallbackRetry(
		c, nil, "gpt-5.5", body, http.StatusNotFound, "model not found", errorBody, false,
	)
	require.False(t, retry)
REDACTED

func TestOpenAIGatewayForwardRetriesExplicitNativeCompactHTTPFailureOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"REDACTEDREDACTED`)),
	REDACTED,
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
	REDACTED,
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"REDACTED,
		Status:      StatusActive, Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.True(t, HasCompactionTriggerInInput(upstream.bodies[1]))
	require.Equal(t, upstream.requests[0].URL.Path, upstream.requests[1].URL.Path)
	require.NotContains(t, upstream.requests[1].URL.Path, "/compact")
REDACTED

func TestOpenAIGatewayForwardRetriesExplicitNativeCompactSSEFailureBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader("event: response.failed\n" +
				`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"REDACTEDREDACTEDREDACTED` + "\n\n")),
	REDACTED,
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
	REDACTED,
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"REDACTED,
		Status:      StatusActive, Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.Equal(t, upstream.requests[0].URL.Path, upstream.requests[1].URL.Path)
	require.NotContains(t, upstream.requests[1].URL.Path, "/compact")
REDACTED

func TestOpenAIGatewayForwardRetriesStreamingCompactFailureBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)

	failed := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"REDACTEDREDACTEDREDACTED` + "\n\n"
	completed := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED` + "\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(failed))REDACTED,
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(completed))REDACTED,
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"REDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"REDACTED,
		Status:      StatusActive, Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.NotContains(t, recorder.Body.String(), "context_length_exceeded")
	require.Contains(t, recorder.Body.String(), "response.completed")
REDACTED
