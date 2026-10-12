//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminVideoCookieStillEnforcesAdminRoleAndRevocation(t *testing.T) {
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "video-test-only-secret", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 1, Email: "video-test@example.com", Status: service.StatusActive, Role: service.RoleAdmin, TokenVersion: 1, TokenVersionResolved: true}
	repo := &stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) { return user, nil }}
	r := gin.New()
	r.Use(AdminVideoPreviewCookieAuth(), gin.HandlerFunc(NewAdminAuthMiddleware(auth, service.NewUserService(repo, nil, nil, nil), nil, nil)))
	r.GET("/api/v1/admin/usage/:id/videos/:index/content", func(c *gin.Context) { c.Status(200) })
	token, err := auth.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	for _, tc := range []struct {
		role, cookie    string
		version, status int
	}{
		{service.RoleAdmin, "", 1, 401},
		{service.RoleAdmin, "invalid-token", 1, 401},
		{service.RoleAdmin, token, 1, 200},
		{service.RoleUser, token, 1, 403},
		{service.RoleAdmin, token, 2, 401},
	} {
		user.Role, user.TokenVersion = tc.role, int64(tc.version)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/12/videos/0/content", nil)
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: AdminVideoPreviewCookie, Value: tc.cookie})
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
	}
}
