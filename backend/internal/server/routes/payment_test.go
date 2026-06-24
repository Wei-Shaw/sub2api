//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterPaymentRoutesUsageViewerCannotAccessAdminPayment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")
	adminAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUsageViewer)
		c.Next()
	}

	RegisterPaymentRoutes(
		v1,
		handler.NewPaymentHandler(nil, nil, nil),
		handler.NewPaymentWebhookHandler(nil, nil),
		adminhandler.NewPaymentHandler(nil, nil),
		func(c *gin.Context) { c.Next() },
		middleware.AdminAuthMiddleware(adminAuth),
		nil,
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/dashboard", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "Admin access required")
}
