package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newRequiredStepUpAdminRouteTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin-API-Key") == "1" {
			c.Set("auth_method", service.AuditAuthMethodAdminAPIKey)
		} else {
			c.Set("auth_method", service.AuditAuthMethodJWT)
		}
		c.Next()
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) {
		if !servermiddleware.IsStepUpRequired(c) {
			servermiddleware.AbortWithError(c, http.StatusInternalServerError, "REQUIRED_STEP_UP_MARKER_MISSING", "required step-up marker missing")
			return
		}
		if c.GetString("auth_method") == service.AuditAuthMethodAdminAPIKey {
			servermiddleware.AbortWithError(c, http.StatusForbidden, "STEP_UP_ADMIN_API_KEY_FORBIDDEN", "admin API key forbidden")
			return
		}
		servermiddleware.AbortWithError(c, http.StatusTeapot, "REQUIRED_STEP_UP_REACHED", "required step-up reached")
	})

	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)
	return router
}

func TestAdminHighRiskRoutesRequireUnconditionalStepUp(t *testing.T) {
	router := newRequiredStepUpAdminRouteTestRouter(t)
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/admin/accounts"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/42/duplicate"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/import/codex-session"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/sync/crs"},
		{method: http.MethodPut, path: "/api/v1/admin/accounts/42"},
		{method: http.MethodPut, path: "/api/v1/admin/accounts/42/ollama-cloud-usage/session"},
		{method: http.MethodDelete, path: "/api/v1/admin/accounts/42/ollama-cloud-usage/session"},
		{method: http.MethodDelete, path: "/api/v1/admin/accounts/42"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/42/apply-oauth-credentials"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/batch"},
		{method: http.MethodGet, path: "/api/v1/admin/accounts/data"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/data"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/batch-update-credentials"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/bulk-update"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/batch-delete"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/42/shadow"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/exchange-code"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/exchange-setup-token-code"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/cookie-auth"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/setup-token-cookie-auth"},
		{method: http.MethodPost, path: "/api/v1/admin/openai/exchange-code"},
		{method: http.MethodPost, path: "/api/v1/admin/openai/refresh-token"},
		{method: http.MethodPost, path: "/api/v1/admin/openai/accounts/42/refresh"},
		{method: http.MethodPost, path: "/api/v1/admin/openai/create-from-oauth"},
		{method: http.MethodPost, path: "/api/v1/admin/openai/create-from-codex-pat"},
		{method: http.MethodPost, path: "/api/v1/admin/gemini/oauth/exchange-code"},
		{method: http.MethodPost, path: "/api/v1/admin/antigravity/oauth/exchange-code"},
		{method: http.MethodPost, path: "/api/v1/admin/antigravity/oauth/refresh-token"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/exchange-code"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/refresh-token"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/sso-token"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/password"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/create-from-oauth"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/sso-to-oauth"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/oauth/reconcile"},
		{method: http.MethodPost, path: "/api/v1/admin/grok/accounts/42/refresh"},
		{method: http.MethodGet, path: "/api/v1/admin/users"},
		{method: http.MethodGet, path: "/api/v1/admin/users/42"},
		{method: http.MethodPost, path: "/api/v1/admin/users/42/auth-identities"},
		{method: http.MethodPost, path: "/api/v1/admin/users"},
		{method: http.MethodPut, path: "/api/v1/admin/users/42"},
		{method: http.MethodDelete, path: "/api/v1/admin/users/42"},
		{method: http.MethodGet, path: "/api/v1/admin/users/42/api-keys"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies/all"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies/data"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies/data"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies/42"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies"},
		{method: http.MethodPut, path: "/api/v1/admin/proxies/42"},
		{method: http.MethodDelete, path: "/api/v1/admin/proxies/42"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies/42/test"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies/42/quality-check"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies/42/stats"},
		{method: http.MethodGet, path: "/api/v1/admin/proxies/42/accounts"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies/batch-delete"},
		{method: http.MethodPost, path: "/api/v1/admin/proxies/batch"},
		{method: http.MethodGet, path: "/api/v1/admin/data-management/agent/health"},
		{method: http.MethodPut, path: "/api/v1/admin/data-management/config"},
		{method: http.MethodPost, path: "/api/v1/admin/data-management/s3/profiles"},
		{method: http.MethodPost, path: "/api/v1/admin/data-management/backups"},
		{method: http.MethodGet, path: "/api/v1/admin/backups/s3-config"},
		{method: http.MethodPut, path: "/api/v1/admin/backups/s3-config"},
		{method: http.MethodGet, path: "/api/v1/admin/backups"},
		{method: http.MethodPost, path: "/api/v1/admin/backups"},
		{method: http.MethodGet, path: "/api/v1/admin/backups/42/download-url"},
		{method: http.MethodPost, path: "/api/v1/admin/backups/42/restore"},
		{method: http.MethodPost, path: "/api/v1/admin/system/update"},
		{method: http.MethodPost, path: "/api/v1/admin/system/rollback"},
		{method: http.MethodPost, path: "/api/v1/admin/system/restart"},
		{method: http.MethodPost, path: "/api/v1/admin/settings/admin-api-key/regenerate"},
		{method: http.MethodDelete, path: "/api/v1/admin/settings/admin-api-key"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path+" requires marker", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusTeapot, recorder.Code)
			require.Contains(t, recorder.Body.String(), "REQUIRED_STEP_UP_REACHED")
		})

		t.Run(tc.method+" "+tc.path+" rejects admin api key", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Admin-API-Key", "1")

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Contains(t, recorder.Body.String(), "STEP_UP_ADMIN_API_KEY_FORBIDDEN")
		})
	}
}
