package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpstreamQuotaRoutesRequireAuthentication(t *testing.T) {
	router := gin.New()
	h := &handler.Handlers{UpstreamQuota: handler.NewUpstreamQuotaHandler(nil, nil, nil, nil), Admin: &handler.AdminHandlers{}}
	reject := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }
	next := func(c *gin.Context) { c.Next() }
	RegisterUserRoutes(router.Group("/api/v1"), h, middleware.JWTAuthMiddleware(reject), middleware.AuditLogMiddleware(next), nil, nil)
	RegisterAdminRoutes(router.Group("/api/v1"), h, middleware.AdminAuthMiddleware(reject), middleware.AuditLogMiddleware(next), middleware.StepUpAuthMiddleware(next), nil, nil)
	for _, path := range []string{"/api/v1/upstream-quota", "/api/v1/upstream-quota/config", "/api/v1/admin/upstream-quota/settings"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/admin/upstream-quota/settings", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("PUT settings: %d", w.Code)
	}
}
