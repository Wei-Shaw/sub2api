package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexAccountTicketProxyCredentialsOmittedFromAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditCaptureRepository{}
	svc := service.NewAuditLogService(repo, nil)
	svc.Start()
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAuditLogMiddleware(svc)))
	r.PUT("/api/v1/admin/accounts/:id/codex-ticket", func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, c.ShouldBindJSON(&body))
		require.Contains(t, body["proxy_url"], "canary-secret")
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/4/codex-ticket", strings.NewReader(`{"enabled":true,"proxy_url":"socks5h://canary-user:canary-secret@proxy.example:1080"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	svc.Stop()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.logs, 1)
	require.Equal(t, "<credential-bearing body omitted>", repo.logs[0].RequestBody)
	require.NotContains(t, repo.logs[0].RequestBody, "canary")
}

func TestCodexTicketGlobalPoolCredentialsRedactedFromSettingsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditCaptureRepository{}
	svc := service.NewAuditLogService(repo, nil)
	svc.Start()
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAuditLogMiddleware(svc)))
	r.PUT("/api/v1/admin/settings", func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, c.ShouldBindJSON(&body))
		require.Contains(t, body["openai_codex_ticket_harvest_proxy_url"], "canary-secret")
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(`{"openai_codex_ticket_enabled":true,"openai_codex_ticket_harvest_proxy_url":"socks5h://canary-user:canary-secret@pool.example:1080"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	svc.Stop()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.logs, 1)
	require.NotContains(t, repo.logs[0].RequestBody, "canary")
	require.NotContains(t, repo.logs[0].RequestBody, "pool.example")
	require.Contains(t, repo.logs[0].RequestBody, `"openai_codex_ticket_enabled":true`)
}
