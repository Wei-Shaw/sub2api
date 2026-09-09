package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIWSSessionHeadersPrefersCodexHyphenHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "codex-session")
	c.Request.Header.Set("session_id", "legacy-session")

	resolution := resolveOpenAIWSSessionHeaders(c, "prompt-cache")

	require.Equal(t, "codex-session", resolution.SessionID)
	require.Equal(t, "header_session-id", resolution.SessionSource)
}

func TestResolveOpenAIWSSessionHeadersFallsBackToLegacyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session_id", "legacy-session")

	resolution := resolveOpenAIWSSessionHeaders(c, "prompt-cache")

	require.Equal(t, "legacy-session", resolution.SessionID)
	require.Equal(t, "header_session_id", resolution.SessionSource)
}
