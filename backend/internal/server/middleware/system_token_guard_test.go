//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSystemTokenRouteGuard_JWTPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth_method", "jwt"); c.Next() })
	r.Use(SystemTokenRouteGuard())
	r.PUT("/api/v1/user", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/user", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSystemTokenRouteGuard_BlocksSensitiveRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth_method", service.AuditAuthMethodSystemToken); c.Next() })
	r.Use(SystemTokenRouteGuard())
	r.PUT("/api/v1/user/password", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/user/password", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSystemTokenRouteGuard_AllowsAllowlistedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth_method", service.AuditAuthMethodSystemToken); c.Next() })
	r.Use(SystemTokenRouteGuard())
	r.GET("/api/v1/user/profile", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSystemTokenRouteGuard_AllowsKeyCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth_method", service.AuditAuthMethodSystemToken); c.Next() })
	r.Use(SystemTokenRouteGuard())
	r.POST("/api/v1/keys", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
