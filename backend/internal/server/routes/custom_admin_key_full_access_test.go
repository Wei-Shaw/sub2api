//go:build unit

package routes

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fullAccessRouteService struct{ scopedRouteAccounts }

func (*fullAccessRouteService) GetProxy(context.Context, int64) (*service.Proxy, error) {
	return &service.Proxy{ID: 9, Username: "proxy-user", Password: "proxy-secret", Status: "active"}, nil
}
func (*fullAccessRouteService) GetGroup(context.Context, int64) (*service.Group, error) {
	return &service.Group{ID: 9, Name: "完整分组", Platform: "openai", RateMultiplier: 12.5}, nil
}

func TestAdminFullAccessRegisteredRoutesPreserveFullHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := &scopedRouteKeys{}
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(keys)
	key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "完整管理", Enabled: true, ConfirmFullAccess: true, Policy: adminaccess.Policy{FullAccess: true}}, 9)
	require.NoError(t, err)
	svc := &fullAccessRouteService{scopedRouteAccounts{t: t}}
	account := adminhandler.NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	auth := middleware.NewAdminAuthMiddleware(nil, service.NewUserService(&scopedRouteUsers{}, nil, nil, nil), settings, nil)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: account, Group: adminhandler.NewGroupHandler(svc, nil, nil), Proxy: adminhandler.NewProxyHandler(svc)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(403) }), nil, nil)
	for _, tc := range []struct{ path, contains string }{
		{"/accounts/3", `"group_ids":[2]`},
		{"/groups/9", `"rate_multiplier":12.5`},
		{"/proxies/9", `"password":"proxy-secret"`},
	} {
		req := httptest.NewRequest("GET", "/api/v1/admin"+tc.path, nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, tc.path+": "+w.Body.String())
		require.Contains(t, w.Body.String(), tc.contains)
	}
	// 密钥全权模式仍不跨越真人会话边界。
	for _, path := range []string{"/api/v1/admin/settings/admin-api-keys", "/api/v1/admin/accounts/data"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 403, w.Code)
	}
}

func TestAdminFullAccessCreationRequiresHTTPConfirmation(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"name":"兼容","enabled":true,"policy":{"full_access":true}}`, 400},
		{`{"name":"兼容","enabled":true,"policy":{"full_access":true},"confirm_full_access":"true"}`, 400},
		{`{"name":"兼容","enabled":true,"policy":{"full_access":true},"confirm_full_access":true}`, 200},
	} {
		keys := &scopedRouteKeys{}
		settings := service.NewSettingService(nil, nil)
		settings.SetScopedAdminKeyRepository(keys)
		r := gin.New()
		auth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set("auth_method", service.AuditAuthMethodJWT)
			c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})
			c.Next()
		})
		RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
		req := httptest.NewRequest("POST", "/api/v1/admin/settings/admin-api-keys", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, w.Body.String())
		if tc.status == 400 {
			require.Nil(t, keys.item)
		} else {
			require.True(t, keys.item.Policy.FullAccess)
			require.False(t, keys.item.ConfirmFullAccess)
			require.NotContains(t, w.Body.String(), "confirm_full_access")
		}
	}
}
