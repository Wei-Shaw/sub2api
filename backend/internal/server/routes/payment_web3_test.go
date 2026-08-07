package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositConfigRouteUsesPaymentAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	passThrough := func(c *gin.Context) { c.Next() }

	RegisterPaymentRoutes(
		router.Group("/api/v1"),
		handler.NewPaymentHandler(nil, nil),
		handler.NewWeb3DepositHandler(&config.Config{}, nil, nil),
		handler.NewPaymentWebhookHandler(nil, nil),
		admin.NewPaymentHandler(nil, nil),
		jwtAuth,
		middleware.AdminAuthMiddleware(passThrough),
		middleware.AuditLogMiddleware(passThrough),
		nil,
		nil,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/config", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/address", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/address", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
