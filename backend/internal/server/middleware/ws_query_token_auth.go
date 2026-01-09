package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InjectBearerTokenFromQueryForWebSocket copies `?token=` into the Authorization header
// for WebSocket handshake requests on a small allow-list of endpoints.
//
// Why: browsers can't set custom headers on WebSocket handshake, but our admin routes
// are protected by header-based auth. This keeps the token support scoped to WS only.
func InjectBearerTokenFromQueryForWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil || c.Request == nil {
			if c != nil {
				c.Next()
		REDACTED
			return
	REDACTED

		// Only GET websocket upgrades.
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
	REDACTED
		if !strings.EqualFold(strings.TrimSpace(c.GetHeader("Upgrade")), "websocket") {
			c.Next()
			return
	REDACTED

		// If caller already supplied auth headers, don't override.
		if strings.TrimSpace(c.GetHeader("Authorization")) != "" || strings.TrimSpace(c.GetHeader("x-api-key")) != "" {
			c.Next()
			return
	REDACTED

		// Allow-list ops websocket endpoints.
		path := strings.TrimSpace(c.Request.URL.Path)
		if !strings.HasPrefix(path, "/api/v1/admin/ops/ws/") {
			c.Next()
			return
	REDACTED

		token := strings.TrimSpace(c.Query("token"))
		if token != "" {
			c.Request.Header.Set("Authorization", "Bearer "+token)
	REDACTED

		c.Next()
REDACTED
REDACTED
