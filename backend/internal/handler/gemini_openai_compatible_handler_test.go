package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiOpenAICompatibleModelsUsesOpenAIShape(t *testing.T) {
	got := geminiModelsToOpenAIModelList(gemini.FallbackModelsList())

	require.Equal(t, "list", got.Object)
	require.NotEmpty(t, got.Data)
	require.Equal(t, "model", got.Data[0].Object)
	require.Equal(t, "google", got.Data[0].OwnedBy)
	require.NotContains(t, got.Data[0].ID, "models/")
}

func TestGeminiOpenAICompatibleRejectsNonGeminiGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/openai/models", nil)
	groupID := int64(1)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		GroupID: &groupID,
		Group:   &service.Group{Platform: service.PlatformOpenAI},
	})

	ok := ensureGeminiOpenAICompatibleGroup(c)

	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "requires a Gemini group")
}

func TestGeminiOpenAICompatibleUnsupportedUsesOpenAIErrorShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/videos", nil)

	(&GatewayHandler{}).GeminiOpenAICompatibleUnsupported(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.JSONEq(t, `{"error":{"type":"invalid_request_error","message":"Unsupported endpoint for Gemini OpenAI compatibility"}}`, w.Body.String())
}

func TestGeminiOpenAICompatibleEmbeddingsValidatesModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/embeddings", strings.NewReader(`{"input":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(1)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group:   &service.Group{Platform: service.PlatformGemini},
	})
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 20, Concurrency: 1})

	(&GatewayHandler{}).GeminiOpenAICompatibleEmbeddings(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "model is required")
}

func TestGeminiOpenAICompatibleImagesRequireGroupPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/images/generations", strings.NewReader(`{"model":"gemini-2.5-flash-image","prompt":"draw"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(1)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		ID:      11,
		GroupID: &groupID,
		Group:   &service.Group{Platform: service.PlatformGemini, AllowImageGeneration: false},
	})
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 21, Concurrency: 1})

	(&GatewayHandler{}).GeminiOpenAICompatibleImagesGenerations(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), service.ImageGenerationPermissionMessage())
}
