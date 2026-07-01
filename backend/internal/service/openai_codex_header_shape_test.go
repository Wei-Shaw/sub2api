package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAICodexWindowID_RebuildsOnSessionMismatch(t *testing.T) {
	require.Equal(t, "sess:2", resolveOpenAICodexWindowID("sess:2", "sess"))
	require.Equal(t, "isolated:0", resolveOpenAICodexWindowID("raw:1", "isolated"))
	require.Equal(t, "isolated:0", resolveOpenAICodexWindowID("", "isolated"))
	require.Empty(t, resolveOpenAICodexWindowID("", ""))
}

func TestApplyOpenAICodexHeaderShape_RebuildsWindowAndInstallation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("x-codex-window-id", "client-session:7")
	c.Request.Header.Set("x-codex-installation-id", "install-client")
	c.Request.Header.Set("x-codex-parent-thread-id", "parent-1")
	c.Request.Header.Set("x-client-request-id", "req-1")

	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	req.Header.Set("session_id", "isolated-session")
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	applyOpenAICodexHeaderShape(req, c, account, true)

	require.Equal(t, "isolated-session:0", req.Header.Get("x-codex-window-id"))
	require.Equal(t, "install-client", req.Header.Get("x-codex-installation-id"))
	require.Equal(t, "parent-1", req.Header.Get("x-codex-parent-thread-id"))
	require.Equal(t, "req-1", req.Header.Get("x-client-request-id"))
}
