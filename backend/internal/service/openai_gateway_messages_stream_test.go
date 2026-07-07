package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIMessagesAnthropicStreamContextLengthBeforeVisibleOutputReturnsClientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-context-window"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_context","model":"gpt-5.5","status":"in_progress"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_context","status":"in_progress"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","response":{"id":"resp_context","status":"failed","usage":{"input_tokens":271533,"output_tokens":0,"total_tokens":271533},"error":{"code":"context_length_exceeded","message":"Your input exceeds the context window of this model. Please adjust your input and try again."}}}`,
			"",
		}, "\n"))),
	}

	result, err := svc.handleAnthropicStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, "gpt-5.5[400k]", "gpt-5.5", "gpt-5.5", time.Now(), 272051)

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.ClientOutputStarted)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	assert.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	assert.False(t, failoverErr.RetryableOnSameAccount)
	assert.Contains(t, string(failoverErr.ResponseBody), "invalid_request_error")
	assert.Contains(t, string(failoverErr.ResponseBody), "context window")
	assert.False(t, c.Writer.Written())
	assert.Empty(t, rec.Body.String())
}

func TestOpenAIMessagesAnthropicStreamBuffersStartUntilVisibleOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-visible-output"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_ok","model":"gpt-5.5","status":"in_progress"}}`,
			"",
			"event: response.output_item.added",
			`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`,
			"",
			"event: response.output_text.delta",
			`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello"}`,
			"",
			"event: response.output_text.done",
			`data: {"type":"response.output_text.done","output_index":0,"content_index":0}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","usage":{"input_tokens":12345,"output_tokens":3,"total_tokens":12348},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}}`,
			"",
		}, "\n"))),
	}

	result, err := svc.handleAnthropicStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, "gpt-5.5[400k]", "gpt-5.5", "gpt-5.5", time.Now(), 12345)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.ClientOutputStarted)

	body := rec.Body.String()
	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, `"input_tokens":12345`)
	require.Contains(t, body, "event: content_block_delta")
	assert.Less(t, strings.Index(body, "event: message_start"), strings.Index(body, "event: content_block_delta"))
}
