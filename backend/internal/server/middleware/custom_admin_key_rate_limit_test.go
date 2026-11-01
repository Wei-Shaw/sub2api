//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminKeyPanelRateLimitAuthenticatedRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := newPanelRateLimitTestService(t, `{"enabled":true,"user_rpm":1,"heavy_rpm":1,"exempt_admin":true,"public_ip_rpm":0}`)
	settings.SetScopedAdminKeyRepository(&adminAuthKeyRepo{})
	key, item, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{
		Name: "读取用户", Enabled: true, Policy: adminaccess.Policy{GlobalPermissions: []string{"users.read"}},
	}, 9)
	require.NoError(t, err)
	users := service.NewUserService(&stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) {
		return &service.User{ID: 9, Role: service.RoleAdmin, Status: service.StatusActive}, nil
	}}, nil, nil, nil)
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{limiter: allower, settingService: settings}
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAdminAuthMiddleware(nil, users, settings, nil)), p.Global())
	r.GET("/api/v1/admin/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusNoContent, request().Code)
	w := request()
	require.Equal(t, http.StatusTooManyRequests, w.Code, "受限 Key 不应继承管理员限流豁免")
	require.Equal(t, "60", w.Header().Get("Retry-After"))
	require.Equal(t, int64(2), allower.counts["panel:global:admin-key:"+item.ID])
}

func TestScopedAdminKeyPanelRateLimitWritesAndIndependentKeys(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			allower := &fakePanelAllower{}
			p := &PanelRateLimiter{limiter: allower, settingService: newPanelRateLimitTestService(t,
				`{"enabled":true,"user_rpm":10,"heavy_rpm":1,"exempt_admin":true}`)}
			newRouter := func(keyID string) *gin.Engine {
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set(string(ContextKeyUser), AuthSubject{UserID: 9})
					c.Set(string(ContextKeyUserRole), service.RoleAdmin)
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{KeyID: keyID}))
					c.Next()
				}, p.Global(), p.Heavy())
				r.Handle(method, "/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
				return r
			}
			request := func(r *gin.Engine) int {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(method, "/test", nil))
				return w.Code
			}
			a, b := newRouter("key-a"), newRouter("key-b")
			require.Equal(t, http.StatusNoContent, request(a), "Global 与 Heavy 叠加时严格档只计数一次")
			require.Equal(t, http.StatusTooManyRequests, request(a))
			require.Equal(t, http.StatusNoContent, request(b), "同一创建者的另一把 Key 配额独立")
			require.Equal(t, int64(2), allower.counts["panel:heavy:admin-key:key-a"])
			require.Equal(t, int64(1), allower.counts["panel:heavy:admin-key:key-b"])
			require.NotContains(t, allower.counts, "panel:global:user:9")
		})
	}
}

func TestScopedAdminKeyPanelRateLimitAuthenticatedSensitiveWrite(t *testing.T) {
	for _, tc := range []struct {
		name      string
		settings  string
		redisErr  error
		wantCodes []int
	}{
		{"严格档", `{"enabled":true,"user_rpm":10,"heavy_rpm":1,"exempt_admin":true}`, nil, []int{204, 429}},
		{"只开严格档", `{"enabled":true,"user_rpm":0,"heavy_rpm":1,"exempt_admin":true}`, nil, []int{204, 429}},
		{"严格档关闭", `{"enabled":true,"user_rpm":10,"heavy_rpm":0,"exempt_admin":true}`, nil, []int{204, 204}},
		{"总开关关闭", `{"enabled":false,"user_rpm":1,"heavy_rpm":1,"exempt_admin":true}`, nil, []int{204, 204}},
		{"Redis故障放行", `{"enabled":true,"user_rpm":1,"heavy_rpm":1,"exempt_admin":true}`, errors.New("redis down"), []int{204, 204}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := newPanelRateLimitTestService(t, tc.settings)
			settings.SetScopedAdminKeyRepository(&adminAuthKeyRepo{})
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{
				Name: "调整余额", Enabled: true, Policy: adminaccess.Policy{GlobalPermissions: []string{"users.balance.write"}},
			}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) {
				return &service.User{ID: 9, Role: service.RoleAdmin, Status: service.StatusActive}, nil
			}}, nil, nil, nil)
			p := &PanelRateLimiter{limiter: &fakePanelAllower{err: tc.redisErr}, settingService: settings}
			r := gin.New()
			r.Use(gin.HandlerFunc(NewAdminAuthMiddleware(nil, users, settings, nil)), p.Global())
			r.POST("/api/v1/admin/users/:id/balance", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for _, want := range tc.wantCodes {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/12/balance", nil)
				req.Header.Set("x-api-key", key)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, want, w.Code)
			}
		})
	}
}

func TestScopedAdminKeyPanelRateLimitSafeMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			allower := &fakePanelAllower{}
			p := &PanelRateLimiter{limiter: allower, settingService: newPanelRateLimitTestService(t,
				`{"enabled":true,"user_rpm":2,"heavy_rpm":1,"exempt_admin":true}`)}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUser), AuthSubject{UserID: 9})
				c.Set(string(ContextKeyUserRole), service.RoleAdmin)
				c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{KeyID: "read-key"}))
				c.Next()
			}, p.Global())
			r.Handle(method, "/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for _, want := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(method, "/test", nil))
				require.Equal(t, want, w.Code)
			}
			require.NotContains(t, allower.counts, "panel:heavy:admin-key:read-key")
		})
	}
}

func TestScopedAdminKeyPanelRateLimitPreservesUnrestrictedAdminWrites(t *testing.T) {
	for _, exempt := range []string{"true", "false"} {
		t.Run(exempt, func(t *testing.T) {
			allower := &fakePanelAllower{}
			p := &PanelRateLimiter{limiter: allower, settingService: newPanelRateLimitTestService(t,
				`{"enabled":true,"user_rpm":10,"heavy_rpm":1,"exempt_admin":`+exempt+`}`)}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				// JWT 与旧全权密钥都只有管理员用户身份，没有受限 Principal。
				c.Set(string(ContextKeyUser), AuthSubject{UserID: 9})
				c.Set(string(ContextKeyUserRole), service.RoleAdmin)
				c.Next()
			}, p.Global())
			r.POST("/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test", nil))
				require.Equal(t, http.StatusNoContent, w.Code)
			}
			require.NotContains(t, allower.counts, "panel:heavy:user:9")
			if exempt == "true" {
				require.Empty(t, allower.counts)
			} else {
				require.Equal(t, int64(2), allower.counts["panel:global:user:9"])
			}
		})
	}
}
