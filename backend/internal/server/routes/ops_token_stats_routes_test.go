package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsTokenStatsRoutesExposeOnlyCurrentEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Ops: adminhandler.NewOpsHandler(nil)}}
	registerOpsRoutes(router.Group("/api/v1/admin"), handlers)

	registeredGET := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			registeredGET[route.Path] = true
		}
	}

	require.True(t, registeredGET["/api/v1/admin/ops/dashboard/token-stats"])
	require.False(t, registeredGET["/api/v1/admin/ops/dashboard/openai-token-stats"])
}
