//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func geminiEmptyCompletionHandlerErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{
		StatusCode:       http.StatusBadGateway,
		ResponseBody:     []byte(`{"error":{"code":502,"message":"Gemini upstream returned no content (finishReason=STOP)","status":"EMPTY_RESPONSE"}}`),
		Reason:           service.GatewayFailureReason("gemini_empty_completion"),
		ClientStatusCode: http.StatusBadGateway,
		ClientMessage:    "Gemini upstream returned an empty response",
	}
}

func TestHandleFailoverExhausted_GeminiEmptyCompletionUsesClientMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	(&GatewayHandler{}).handleFailoverExhausted(c, geminiEmptyCompletionHandlerErr(), service.PlatformGemini, false)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), `"type":"api_error"`)
	require.Contains(t, rec.Body.String(), "Gemini upstream returned an empty response")
}

func TestHandleCCFailoverExhausted_StreamStartedGeminiEmptyWritesErrorChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	(&GatewayHandler{}).handleCCFailoverExhausted(c, geminiEmptyCompletionHandlerErr(), true)
	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, `data: {"error":{"type":"server_error","code":"gemini_empty_completion"`), body)
	require.Contains(t, body, "Gemini upstream returned an empty response")

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	(&GatewayHandler{}).handleCCFailoverExhausted(c2, &service.UpstreamFailoverError{StatusCode: 502}, true)
	require.Empty(t, rec2.Body.String(), "other exhausted errors keep the existing stream behaviour")
}
