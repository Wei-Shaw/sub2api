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

type billingAdminAuthUsers struct {
	service.UserRepository
	users map[int64]*service.User
}

func (r *billingAdminAuthUsers) GetByID(_ context.Context, id int64) (*service.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	copy := *u
	return &copy, nil
}
func (r *billingAdminAuthUsers) GetFirstAdmin(context.Context) (*service.User, error) {
	copy := *r.users[1]
	return &copy, nil
}
func (*billingAdminAuthUsers) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

type billingAdminAuthSettings struct{ service.SettingRepository }

func (*billingAdminAuthSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyAdminAPIKey {
		return "admin-synthetic-billing-key", nil
	}
	return "false", nil
}

func TestUpstreamBillingAdminAuthAcceptsOnlyAdminCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "synthetic-billing-jwt-secret-for-tests", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	users := &billingAdminAuthUsers{users: map[int64]*service.User{
		1: {ID: 1, Email: "admin@example.test", Role: service.RoleAdmin, Status: service.StatusActive, TokenVersion: 2, TokenVersionResolved: true},
		2: {ID: 2, Email: "user@example.test", Role: service.RoleUser, Status: service.StatusActive, TokenVersion: 2, TokenVersionResolved: true},
	}}
	userService := service.NewUserService(users, nil, nil, nil)
	settings := service.NewSettingService(&billingAdminAuthSettings{}, cfg)
	adminToken, err := auth.GenerateToken(context.Background(), users.users[1])
	require.NoError(t, err)
	userToken, err := auth.GenerateToken(context.Background(), users.users[2])
	require.NoError(t, err)
	router := gin.New()
	admin := router.Group("/api/v1/admin", gin.HandlerFunc(NewAdminAuthMiddleware(auth, userService, settings, nil)))
	for _, path := range []string{"/upstream-billing/bills", "/upstream-billing/bills/:id"} {
		admin.GET(path, func(c *gin.Context) {
			subject, ok := GetAuthSubjectFromContext(c)
			role, _ := GetUserRoleFromContext(c)
			require.True(t, ok)
			require.EqualValues(t, 1, subject.UserID)
			require.Equal(t, service.RoleAdmin, role)
			c.Status(http.StatusOK)
		})
	}
	for _, tc := range []struct {
		name, header, value string
		status              int
	}{
		{"admin API key", "x-api-key", "admin-synthetic-billing-key", http.StatusOK},
		{"admin JWT", "Authorization", "Bearer " + adminToken, http.StatusOK},
		{"ordinary JWT", "Authorization", "Bearer " + userToken, http.StatusForbidden},
		{"virtual key header", "x-api-key", "sk-synthetic-virtual-model-key", http.StatusUnauthorized},
		{"virtual key bearer", "Authorization", "Bearer sk-synthetic-virtual-model-key", http.StatusUnauthorized},
		{"missing credential", "", "", http.StatusUnauthorized},
	} {
		for _, path := range []string{"/api/v1/admin/upstream-billing/bills?month=2026-09", "/api/v1/admin/upstream-billing/bills/1?month=2026-09"} {
			t.Run(tc.name+path, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if tc.header != "" {
					request.Header.Set(tc.header, tc.value)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, request)
				require.Equal(t, tc.status, w.Code)
			})
		}
	}
}
