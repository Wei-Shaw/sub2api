package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 智谱重置卡的查询与用卡接口注册在管理面 /admin/cn-providers 下。
func TestCNProviderResetQuotaRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pass := func(c *gin.Context) { c.Next() }
	router := gin.New()
	RegisterAdminRoutes(
		router.Group("/api/v1"),
		&handler.Handlers{Admin: &handler.AdminHandlers{}},
		servermiddleware.AdminAuthMiddleware(pass),
		servermiddleware.AuditLogMiddleware(pass),
		servermiddleware.StepUpAuthMiddleware(pass),
		nil,
		nil,
	)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/v1/admin/cn-providers/accounts/:id/reset-quota",
		"POST /api/v1/admin/cn-providers/accounts/:id/reset-quota",
	} {
		require.Contains(t, registered, route)
	}
}
