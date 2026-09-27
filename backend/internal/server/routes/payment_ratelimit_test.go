//go:build unit

package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// newPaymentRoutesTestRouter mounts the payment routes with zero-value handlers and the given panel rate limit settings JSON.
func newPaymentRoutesTestRouter(t *testing.T, panelRateLimitSettings string) (*gin.Engine, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// Reuses the route-guard SettingRepository stub from channel_monitor_feature_gate_test.go.
	settings := service.NewSettingService(&channelMonitorRouteSettingRepoStub{values: map[string]string{
		service.SettingKeyPanelRateLimitSettings: panelRateLimitSettings,
		// Admin user 1 has acknowledged compliance, so admin routes get past AdminComplianceGuard.
		"admin_compliance_acknowledgement:1": `{"version":"` + service.AdminComplianceVersion + `"}`,
	}}, &config.Config{})
	passThrough := func(c *gin.Context) { c.Next() }
	adminAuth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
		c.Next()
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Zero-value handlers panic on their nil dependencies; only the limiter's verdict matters here.
	router.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	RegisterPaymentRoutes(
		router.Group("/api/v1"),
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&admin.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(passThrough),
		servermiddleware.AdminAuthMiddleware(adminAuth),
		servermiddleware.AuditLogMiddleware(passThrough),
		servermiddleware.StepUpAuthMiddleware(fakePaymentStepUp),
		settings,
		servermiddleware.NewPanelRateLimiter(rdb, settings),
	)
	return router, mr
}

func TestPaymentUnauthenticatedRoutesArePerIPRateLimited(t *testing.T) {
	router, mr := newPaymentRoutesTestRouter(t, `{"enabled":true,"public_ip_rpm":2}`)

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/payment/webhook/sepay"},
		{http.MethodPost, "/api/v1/payment/webhook/nowpayments"},
		{http.MethodPost, "/api/v1/payment/public/orders/verify"},
		{http.MethodPost, "/api/v1/payment/public/orders/resolve"},
		{http.MethodGet, "/api/v1/payment/checkout"},
	}
	for _, rt := range routes {
		mr.FlushAll()
		for i := 1; i <= 3; i++ {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			req.RemoteAddr = "203.0.113.10:12345"
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if i <= 2 {
				require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "%s %s request %d", rt.method, rt.path, i)
			} else {
				require.Equal(t, http.StatusTooManyRequests, rec.Code, "%s %s request %d", rt.method, rt.path, i)
			}
		}
	}
}
