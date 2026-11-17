//go:build unit

package routes

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type scopedPaymentRateSettings struct{ service.SettingRepository }

func (*scopedPaymentRateSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyPanelRateLimitSettings {
		return `{"enabled":true,"user_rpm":100,"heavy_rpm":1,"exempt_admin":true}`, nil
	}
	return "", service.ErrSettingNotFound
}

func TestScopedAdminPaymentWritesUseHeavyRateLimitWithAdminExemption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	settings := service.NewSettingService(&scopedPaymentRateSettings{}, nil)
	settings.SetScopedAdminKeyRepository(&scopedRouteKeys{})
	key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "支付取消", Enabled: true, Policy: adminaccess.Policy{GlobalPermissions: []string{"payment.cancel"}}}, 9)
	require.NoError(t, err)
	auth := middleware.NewAdminAuthMiddleware(nil, service.NewUserService(&scopedRouteUsers{}, nil, nil, nil), settings, nil)
	r := gin.New()
	RegisterPaymentRoutes(r.Group("/api/v1"), &handler.PaymentHandler{}, &handler.PaymentWebhookHandler{}, &adminhandler.PaymentHandler{}, middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }), auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), nil, middleware.NewPanelRateLimiter(client, settings), client)
	// 无效订单 ID 在实际支付处理器返回 400，证明首个请求已经通过认证和授权。
	// 第二次应在解析订单之前触发严格档限流，不依赖真实支付数据库。
	for _, want := range []int{400, 429} {
		req := httptest.NewRequest("POST", "/api/v1/admin/payment/orders/invalid/cancel", nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, want, w.Code, w.Body.String())
		if want == 429 {
			require.NotEmpty(t, w.Header().Get("Retry-After"))
		}
	}
}
