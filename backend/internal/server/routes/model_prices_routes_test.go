package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterModelPriceRoutesUsesOptionalJWTForAnonymousRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	optionalCalls := 0
	optional := middleware.OptionalJWTAuthMiddleware(func(c *gin.Context) {
		optionalCalls++
		c.Header("X-Test-Optional-JWT", "used")
		c.AbortWithStatus(http.StatusNoContent)
	})

	RegisterModelPriceRoutes(v1, &handler.Handlers{}, optional, nil, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/model-prices", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "used", recorder.Header().Get("X-Test-Optional-JWT"))
	require.Equal(t, 1, optionalCalls)
}
