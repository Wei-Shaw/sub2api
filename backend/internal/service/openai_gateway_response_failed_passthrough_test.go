//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func buildContextLengthFailedSSE() string {
	failed := `{"type":"response.failed","response":{"id":"resp_err","object":"response","status":"failed","error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"Your input exceeds the context window of this model. Please adjust your input and try again."REDACTED,"output":[],"usage":{"input_tokens":100000,"output_tokens":0,"total_tokens":100000REDACTEDREDACTEDREDACTED`
	return fmt.Sprintf("data: %s\n\n", failed)
REDACTED

func bindPassthroughRule(c *gin.Context, platform string, keywords []string, responseCode int) {
	svc := &ErrorPassthroughService{REDACTED
	rules := make([]*cachedPassthroughRule, 0, len(keywords))
	for i, kw := range keywords {
		code := responseCode
		rules = append(rules, &cachedPassthroughRule{
			ErrorPassthroughRule: &model.ErrorPassthroughRule{
				ID:              int64(i + 1),
				Enabled:         true,
				Platforms:       []string{platformREDACTED,
				MatchMode:       model.MatchModeAny,
				Keywords:        []string{kwREDACTED,
				ResponseCode:    &code,
				PassthroughBody: true,
		REDACTED,
			lowerKeywords:  []string{strings.ToLower(kw)REDACTED,
			lowerPlatforms: []string{strings.ToLower(platform)REDACTED,
	REDACTED)
REDACTED
	svc.localCacheMu.Lock()
	svc.localCache = rules
	svc.localCacheMu.Unlock()
	BindErrorPassthroughService(c, svc)
REDACTED

func TestForwardAsChatCompletions_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"REDACTED, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

REDACTED
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")

	respBody := rec.Body.String()
	errType := gjson.Get(respBody, "error.type").String()
	require.Equal(t, "upstream_error", errType)
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
	require.Contains(t, errMsg, "context window")
REDACTED

func TestForwardAsAnthropic_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"REDACTED, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

REDACTED
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")
	respBody := rec.Body.String()
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
REDACTED

func TestForwardAsChatCompletions_ResponseFailed_NoRule_Still502(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

REDACTED
	require.Equal(t, http.StatusBadGateway, rec.Code, "without passthrough rule should still be 502")
REDACTED
