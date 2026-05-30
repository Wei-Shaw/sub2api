package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "error", parsed["type"])
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

// Writer 已写后 ensureForwardErrorResponse 必须把错误以 SSE 形式追加，
// 而不是 silent EOF。非 /responses 路径走 legacy data:{"type":"error"} 分支。
func TestGatewayEnsureForwardErrorResponse_AppendsSSEAfterWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Contains(t, w.Body.String(), "already written")
	assert.Contains(t, w.Body.String(), `data: {"type":"error"`)
}

// case B 回归：Anthropic-backed /responses，Writer 已被写过时
// ensureForwardErrorResponse 仍要发 response.failed。
func TestGatewayEnsureForwardErrorResponse_ResponsesRouteAfterWrittenEmitsResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	_, _ = c.Writer.WriteString(":\n\n")

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	body := w.Body.String()
	assert.Contains(t, body, ":\n\n")
	assert.Contains(t, body, "event: response.failed\n")
	assert.Contains(t, body, `"type":"response.failed"`)
}

// 回归：当 Forward 内部已经通过 handleErrorResponse 写完完整 JSON 错误响应
// 并显式标记了 ForwardResponseFinalizedKey，外层 handler 的
// ensureForwardErrorResponse 必须不再追加 SSE 错误帧。
// 否则 client 会同时收到 application/json 响应体和 `data: {"type":"error",...}` SSE 行
// （历史现象：HTML 400 + SSE 帧混合）。
func TestGatewayEnsureForwardErrorResponse_SkipsWhenResponseFinalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 模拟 handleErrorResponse 已经完整写出一份 JSON 错误响应。
	c.JSON(http.StatusBadRequest, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": "Upstream returned 400 with non-JSON body",
		},
	})
	service.MarkForwardResponseFinalized(c)
	bodyBefore := w.Body.String()
	codeBefore := w.Code

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote, "must report a response was already written")
	assert.Equal(t, codeBefore, w.Code, "status code must not change")
	assert.Equal(t, bodyBefore, w.Body.String(), "body must not be appended to")
	assert.False(t, strings.Contains(w.Body.String(), `data: {"type":"error"`),
		"finalized JSON response must not be followed by an SSE error frame")
}
