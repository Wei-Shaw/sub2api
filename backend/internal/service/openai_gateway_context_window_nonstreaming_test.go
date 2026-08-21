package service

import (
	"errors"
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

func TestOpenAINonStreamingContextWindowFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIContextWindowFailedSSEResponse()

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI}, "gpt-5.5", "gpt-5.5")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "context window")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestOpenAINonStreamingPassthroughContextWindowFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIContextWindowFailedSSEResponse()

	_, err := svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, "gpt-5.5", "gpt-5.5")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "context window")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestOpenAINonStreamingContextWindowFailureAfterKeepaliveCommitUsesBadRequestEvent(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIContextWindowFailedSSEResponse()

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI}, "gpt-5.5", "gpt-5.5")

	require.Error(t, err)
	require.Equal(t, http.StatusOK, rec.Code, "心跳已提交后 HTTP 状态不可再修改")
	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 1)
	require.Equal(t, "response.failed", events[0][0])
	require.Equal(t, "invalid_request_error", gjson.Get(events[0][1], "response.error.code").String())
	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, streamErr.IntendedStatus)
}

func openAIContextWindowFailedSSEResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.failed",
			`data: {"type":"response.failed","error":{"code":"context_length_exceeded","message":"Your input exceeds the context window of this model."}}`,
			"",
		}, "\n"))),
	}
}
