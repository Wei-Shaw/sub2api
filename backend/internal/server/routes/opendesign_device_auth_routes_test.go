package routes

import (
	"slices"
	"testing"

	adminHandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// OpenDesign 设备授权的四条路由是全站契约的一部分：前端 api client 直接拼这些
// 路径，前缀写错就会整体 404。这里用生产同款分组逐字固定完整路径。
func TestOpenDesignDeviceAuthRoutePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	v1 := router.Group("/api/v1")
	admin := v1.Group("/admin")
	accounts := admin.Group("/accounts")
	accounts.POST(adminHandler.OpenDesignDeviceAuthRoute, func(*gin.Context) {})
	accounts.GET(adminHandler.OpenDesignDeviceAuthStatusRoute, func(*gin.Context) {})
	accounts.DELETE(adminHandler.OpenDesignDeviceAuthStatusRoute, func(*gin.Context) {})
	accounts.POST(adminHandler.OpenDesignAccountsRoute, func(*gin.Context) {})

	var registered []string
	for _, route := range router.Routes() {
		registered = append(registered, route.Method+" "+route.Path)
	}

	for _, want := range []string{
		"POST /api/v1/admin/accounts/opendesign/device-auth",
		"GET /api/v1/admin/accounts/opendesign/device-auth/:sessionId",
		"DELETE /api/v1/admin/accounts/opendesign/device-auth/:sessionId",
		"POST /api/v1/admin/accounts/opendesign/accounts",
	} {
		require.True(t, slices.Contains(registered, want), "missing route %q in %v", want, registered)
	}
}
