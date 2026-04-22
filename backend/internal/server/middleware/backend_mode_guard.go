package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BackendModeUserGuard blocks non-admin users from accessing user routes when backend mode is enabled.
// Must be placed AFTER JWT auth middleware so that the user role is available in context.
func BackendModeUserGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil || !settingService.IsBackendModeEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		role, _ := GetUserRoleFromContext(c)
		if role == "admin" {
			c.Next()
			return
		}
		response.Forbidden(c, "Backend mode is active. User self-service is disabled.")
		c.Abort()
	}
}

func backendModeAllowsAuthPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	for _, suffix := range []string{"/auth/login", "/auth/login/2fa", "/auth/logout", "/auth/refresh"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	for _, suffix := range []string{
		"/auth/oauth/linuxdo/callback",
		"/auth/oauth/wechat/callback",
		"/auth/oauth/wechat/payment/callback",
		"/auth/oauth/oidc/callback",
		"/auth/oauth/linuxdo/complete-registration",
		"/auth/oauth/wechat/complete-registration",
		"/auth/oauth/oidc/complete-registration",
		"/auth/oauth/linuxdo/create-account",
		"/auth/oauth/wechat/create-account",
		"/auth/oauth/oidc/create-account",
		"/auth/oauth/linuxdo/bind-login",
		"/auth/oauth/wechat/bind-login",
		"/auth/oauth/oidc/bind-login",
	} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return strings.Contains(path, "/auth/oauth/pending/")
}

// BackendModeAuthGuard selectively blocks auth endpoints when backend mode is enabled.
// Allows the minimal auth surface admins still need in backend mode, including
// OAuth callbacks and pending continuations. Handler-level backend mode checks
// still enforce admin-only login and forbid self-service registration.
func BackendModeAuthGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil || !settingService.IsBackendModeEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		if backendModeAllowsAuthPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		response.Forbidden(c, "Backend mode is active. Registration and self-service auth flows are disabled.")
		c.Abort()
	}
}
