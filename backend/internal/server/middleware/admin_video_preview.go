package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

const AdminVideoPreviewCookie = "buaga_video_preview"

// Only the media content route accepts the short-lived, path-scoped cookie.
// Normal admin JWT verification (including role/revocation) still runs afterward.
func AdminVideoPreviewCookieAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet &&
			c.FullPath() == "/api/v1/admin/usage/:id/videos/:index/content" &&
			c.GetHeader("Authorization") == "" && c.GetHeader("x-api-key") == "" &&
			c.GetHeader("Sec-Fetch-Site") != "cross-site" {
			if token, err := c.Cookie(AdminVideoPreviewCookie); err == nil && token != "" {
				c.Request.Header.Set("Authorization", "Bearer "+token)
			}
		}
		c.Next()
	}
}
