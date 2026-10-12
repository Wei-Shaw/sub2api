package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminVideoCookieIsContentRouteOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AdminVideoPreviewCookieAuth())
	h := func(c *gin.Context) { c.String(200, c.GetHeader("Authorization")) }
	r.GET("/api/v1/admin/usage/:id/videos/:index/content", h)
	r.GET("/api/v1/admin/usage/:id/videos/:index", h)
	r.GET("/api/v1/admin/users", h)
	for _, tc := range []struct{ path, site, header, want string }{
		{"/api/v1/admin/usage/1/videos/0/content", "same-origin", "", "Bearer test-jwt"},
		{"/api/v1/admin/usage/1/videos/0/content", "cross-site", "", ""},
		{"/api/v1/admin/usage/1/videos/0/content", "same-origin", "Bearer explicit", "Bearer explicit"},
		{"/api/v1/admin/usage/1/videos/0", "same-origin", "", ""},
		{"/api/v1/admin/users", "same-origin", "", ""},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: AdminVideoPreviewCookie, Value: "test-jwt"})
		req.Header.Set("Sec-Fetch-Site", tc.site)
		req.Header.Set("Authorization", tc.header)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.want, w.Body.String(), tc.path)
	}
}
