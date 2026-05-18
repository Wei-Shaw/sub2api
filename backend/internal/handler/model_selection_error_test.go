package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandlerHandleSelectionError_UnsupportedModelReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}
	require.True(t, h.handleSelectionError(c, &service.UnsupportedRequestedModelError{Model: "gpt-5"}, false))
	require.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errObj["type"])
	require.Contains(t, errObj["message"], "gpt-5")
}

func TestOpenAIGatewayHandlerHandleSelectionError_ModelAccessDeniedReturns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	require.True(t, h.handleSelectionError(c, &service.ModelAccessDeniedError{Model: "gpt-4.1", Reason: "channel pricing restriction"}, false))
	require.Equal(t, http.StatusForbidden, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "permission_error", errObj["type"])
	require.Contains(t, errObj["message"], "channel pricing restriction")
}

func TestOpenAIGatewayHandlerHandleSelectionErrorAnthropic_UnsupportedModelReturnsAnthropicShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &OpenAIGatewayHandler{}
	require.True(t, h.handleSelectionErrorAnthropic(c, &service.UnsupportedRequestedModelError{Model: "gpt-5"}, false))
	require.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "error", body["type"])
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errObj["type"])
	require.Contains(t, errObj["message"], "gpt-5")
}
