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

func upstreamQuotaRouteHandlers() *handler.Handlers {
	account := adminhandler.NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	account.SetUpstreamBillingProbeService(service.NewUpstreamBillingProbeService(nil, nil, nil))
	return &handler.Handlers{Admin: &handler.AdminHandlers{
		Account:     account,
		OAuth:       &adminhandler.OAuthHandler{},
		OpenAIOAuth: &adminhandler.OpenAIOAuthHandler{},
	}}
}

func TestUpstreamQuotaRouteRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	registerAccountRoutes(admin, upstreamQuotaRouteHandlers(), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/7/upstream-quota/query", nil))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestUpstreamQuotaRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	registerAccountRoutes(admin, upstreamQuotaRouteHandlers(), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/not-an-id/upstream-quota/query", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUpstreamBillingRatesRouteRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	registerAccountRoutes(admin, upstreamQuotaRouteHandlers(), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/upstream-billing-rates", nil))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestUpstreamBillingRatesRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	registerAccountRoutes(admin, upstreamQuotaRouteHandlers(), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/upstream-billing-rates", nil))
	// The test handler has no AdminService; reaching the dedicated handler (rather
	// than the /:id route) is the registration assertion.
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}
