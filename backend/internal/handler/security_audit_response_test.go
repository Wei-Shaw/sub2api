package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type capturedPromptResponse struct {
	request  securityaudit.Request
	response securityaudit.PromptResponse
}

type promptResponseRecorderStub struct {
	captured chan capturedPromptResponse
	disabled bool
}

func (*promptResponseRecorderStub) RecordPrompt(context.Context, securityaudit.Request) {}
func (r *promptResponseRecorderStub) PromptRecordingEnabled() bool                      { return !r.disabled }
func (r *promptResponseRecorderStub) RecordResponse(_ context.Context, request securityaudit.Request, response securityaudit.PromptResponse) {
	r.captured <- capturedPromptResponse{request: request, response: response}
}

func TestPromptResponseCaptureMiddlewareBypassesCaptureWhenRecordingDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &promptResponseRecorderStub{captured: make(chan capturedPromptResponse, 1), disabled: true}
	coordinator := securityaudit.NewCoordinator(nil, nil, recorder)
	handlers := &Handlers{Gateway: &GatewayHandler{securityAuditCoordinator: coordinator}}
	router := gin.New()
	router.Use(handlers.PromptResponseCaptureMiddleware())
	router.POST("/v1/test", func(c *gin.Context) {
		c.Set(securityAuditRequestContextKey, securityaudit.Request{RequestID: "req-disabled"})
		c.JSON(http.StatusOK, gin.H{"choices": []any{gin.H{"message": gin.H{"content": "must not be captured"}}}})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/test", nil))
	require.Equal(t, http.StatusOK, response.Code)
	select {
	case <-recorder.captured:
		t.Fatal("disabled prompt recording must bypass response capture")
	default:
	}
}

func TestPromptResponseCaptureMiddlewareExtractsSuccessfulResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &promptResponseRecorderStub{captured: make(chan capturedPromptResponse, 1)}
	coordinator := securityaudit.NewCoordinator(nil, nil, recorder)
	handlers := &Handlers{Gateway: &GatewayHandler{securityAuditCoordinator: coordinator}}
	router := gin.New()
	router.Use(handlers.PromptResponseCaptureMiddleware())
	router.POST("/v1/test", func(c *gin.Context) {
		c.Set(securityAuditRequestContextKey, securityaudit.Request{RequestID: "req-1", APIKeyID: 9, Protocol: "openai_chat_completions", Stage: "http"})
		c.JSON(http.StatusOK, gin.H{"choices": []any{gin.H{"message": gin.H{"content": "captured text"}}}})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/test", nil))
	require.Equal(t, http.StatusOK, response.Code)
	captured := <-recorder.captured
	require.Equal(t, "req-1", captured.request.RequestID)
	require.Equal(t, "captured text", captured.response.Text)
	require.Equal(t, 13, captured.response.Length)
}

func TestPromptResponseCaptureMiddlewareSkipsFailedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &promptResponseRecorderStub{captured: make(chan capturedPromptResponse, 1)}
	coordinator := securityaudit.NewCoordinator(nil, nil, recorder)
	handlers := &Handlers{Gateway: &GatewayHandler{securityAuditCoordinator: coordinator}}
	router := gin.New()
	router.Use(handlers.PromptResponseCaptureMiddleware())
	router.POST("/v1/test", func(c *gin.Context) {
		c.Set(securityAuditRequestContextKey, securityaudit.Request{RequestID: "req-failed", Protocol: "openai_chat_completions"})
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream failed"})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/test", nil))
	select {
	case <-recorder.captured:
		t.Fatal("failed response must not be retained as generated text")
	default:
	}
}
