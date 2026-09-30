//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAISessionInputsKeepsRoutingAndCacheIdentitySeparate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("session_id", "stable-session")
	body := []byte(`{"model":"logical-model","prompt_cache_key":"separate-cache-key"}`)

	gateway := &service.OpenAIGatewayService{}
	sessionHash, promptCacheKey := resolveOpenAISessionInputs(gateway, c, body)

	require.Equal(t, service.DeriveSessionHashFromSeed("stable-session"), sessionHash)
	require.Equal(t, "separate-cache-key", promptCacheKey)
}

func TestResolveOpenAISessionInputsPreservesSessionFallbackWithoutBodyCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("session_id", "stable-session")

	gateway := &service.OpenAIGatewayService{}
	sessionHash, promptCacheKey := resolveOpenAISessionInputs(gateway, c, []byte(`{"model":"logical-model"}`))

	require.Equal(t, service.DeriveSessionHashFromSeed("stable-session"), sessionHash)
	require.Equal(t, "stable-session", promptCacheKey)
}
