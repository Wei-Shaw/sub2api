package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyErrorPassthroughRule_NoBoundService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		PlatformAnthropic,
		http.StatusUnprocessableEntity,
		[]byte(`{"error":{"message":"invalid schema"}}`),
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)

	assert.False(t, matched)
	assert.Equal(t, http.StatusBadGateway, status)
	assert.Equal(t, "upstream_error", errType)
	assert.Equal(t, "Upstream request failed", errMsg)
}

func TestGatewayHandleErrorResponse_NoRuleKeepsDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GatewayService{}
	respBody := []byte(`{"error":{"message":"Invalid schema for field messages"}}`)
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{},
	}
	account := &Account{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadGateway, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "Upstream request failed", errField["message"])
}

func TestOpenAIHandleErrorResponse_NoRuleKeepsDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &OpenAIGatewayService{}
	respBody := []byte(`{"error":{"message":"Invalid schema for field messages"}}`)
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{},
	}
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "Invalid schema for field messages", errField["message"])

	upstreamField, ok := errField["upstream"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(http.StatusUnprocessableEntity), upstreamField["status"])
	assert.Equal(t, "Invalid schema for field messages", upstreamField["message"])
}

func TestGeminiWriteGeminiMappedError_NoRuleKeepsDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	respBody := []byte(`{"error":{"code":422,"message":"Invalid schema for field messages","status":"INVALID_ARGUMENT"}}`)
	account := &Account{ID: 13, Platform: PlatformGemini, Type: AccountTypeAPIKey}

	err := svc.writeGeminiMappedError(c, account, http.StatusUnprocessableEntity, "req-2", respBody)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "invalid_request_error", errField["type"])
	assert.Equal(t, "Upstream request failed", errField["message"])
}

func TestGatewayHandleErrorResponse_AppliesRuleFor422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{newNonFailoverPassthroughRule(http.StatusUnprocessableEntity, "invalid schema", http.StatusTeapot, "上游请求失败")})
	BindErrorPassthroughService(c, ruleSvc)

	svc := &GatewayService{}
	respBody := []byte(`{"error":{"message":"Invalid schema for field messages"}}`)
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{},
	}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account)
	require.Error(t, err)
	assert.Equal(t, http.StatusTeapot, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "上游请求失败", errField["message"])
}

func TestOpenAIHandleErrorResponse_AppliesRuleFor422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{newNonFailoverPassthroughRule(http.StatusUnprocessableEntity, "invalid schema", http.StatusTeapot, "OpenAI上游失败")})
	BindErrorPassthroughService(c, ruleSvc)

	svc := &OpenAIGatewayService{}
	respBody := []byte(`{"error":{"message":"Invalid schema for field messages"}}`)
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{},
	}
	account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusTeapot, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "OpenAI上游失败", errField["message"])
}

func TestOpenAIHandleErrorResponse_PreservesStructuredUpstream400(t *testing.T) {

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &OpenAIGatewayService{}
	respBody := []byte(`{"error":{"code":"invalid_value","message":"The image data you provided does not represent a valid image.","param":"input","type":"invalid_request_error"}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	account := &Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "The image data you provided does not represent a valid image.", errField["message"])

	upstreamField, ok := errField["upstream"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(http.StatusBadRequest), upstreamField["status"])
	assert.Equal(t, "invalid_value", upstreamField["code"])
	assert.Equal(t, "invalid_request_error", upstreamField["type"])
	assert.Equal(t, "input", upstreamField["param"])
	assert.Equal(t, "The image data you provided does not represent a valid image.", upstreamField["message"])

	rawField, ok := upstreamField["raw"].(map[string]any)
	require.True(t, ok)
	rawError, ok := rawField["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "invalid_value", rawError["code"])
}

func TestGeminiWriteGeminiMappedError_AppliesRuleFor422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{newNonFailoverPassthroughRule(http.StatusUnprocessableEntity, "invalid schema", http.StatusTeapot, "Gemini上游失败")})
	BindErrorPassthroughService(c, ruleSvc)

	svc := &GeminiMessagesCompatService{}
	respBody := []byte(`{"error":{"code":422,"message":"Invalid schema for field messages","status":"INVALID_ARGUMENT"}}`)
	account := &Account{ID: 3, Platform: PlatformGemini, Type: AccountTypeAPIKey}

	err := svc.writeGeminiMappedError(c, account, http.StatusUnprocessableEntity, "req-1", respBody)
	require.Error(t, err)
	assert.Equal(t, http.StatusTeapot, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errField["type"])
	assert.Equal(t, "Gemini上游失败", errField["message"])
}

func TestApplyErrorPassthroughRule_SkipMonitoringSetsContextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	rule := newNonFailoverPassthroughRule(http.StatusBadRequest, "prompt is too long", http.StatusBadRequest, "上下文超限")
	rule.SkipMonitoring = true

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, ruleSvc)

	_, _, _, matched := applyErrorPassthroughRule(
		c,
		PlatformAnthropic,
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"prompt is too long"}}`),
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)

	assert.True(t, matched)
	v, exists := c.Get(OpsSkipPassthroughKey)
	assert.True(t, exists, "OpsSkipPassthroughKey should be set when skip_monitoring=true")
	boolVal, ok := v.(bool)
	assert.True(t, ok, "value should be bool")
	assert.True(t, boolVal)
}

func TestApplyErrorPassthroughRule_RedactsGeneratedImagePayloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	rule := newNonFailoverPassthroughRule(http.StatusBadRequest, "invalid generated image", http.StatusBadRequest, "")
	rule.PassthroughBody = true
	rule.CustomMessage = nil

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, ruleSvc)

	_, _, errMsg, matched := applyErrorPassthroughRule(
		c,
		PlatformOpenAI,
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"invalid generated image `+imageURL+`"}}`),
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)

	require.True(t, matched)
	require.NotContains(t, errMsg, "data:image")
	require.NotContains(t, errMsg, sample)
	require.Contains(t, errMsg, "invalid generated image")
}

func TestApplyErrorPassthroughRule_NoSkipMonitoringDoesNotSetContextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	rule := newNonFailoverPassthroughRule(http.StatusBadRequest, "prompt is too long", http.StatusBadRequest, "上下文超限")
	rule.SkipMonitoring = false

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, ruleSvc)

	_, _, _, matched := applyErrorPassthroughRule(
		c,
		PlatformAnthropic,
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"prompt is too long"}}`),
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)

	assert.True(t, matched)
	_, exists := c.Get(OpsSkipPassthroughKey)
	assert.False(t, exists, "OpsSkipPassthroughKey should NOT be set when skip_monitoring=false")
}

func newNonFailoverPassthroughRule(statusCode int, keyword string, respCode int, customMessage string) *model.ErrorPassthroughRule {
	return &model.ErrorPassthroughRule{
		ID:              1,
		Name:            "non-failover-rule",
		Enabled:         true,
		Priority:        1,
		ErrorCodes:      []int{statusCode},
		Keywords:        []string{keyword},
		MatchMode:       model.MatchModeAll,
		PassthroughCode: false,
		ResponseCode:    &respCode,
		PassthroughBody: false,
		CustomMessage:   &customMessage,
	}
}
