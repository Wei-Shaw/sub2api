package routes

import (
	"os"
	"slices"
	"strings"
	"testing"

	adminHandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 注册位置必须挂在 accounts 分组下（前缀 /api/v1/admin + /accounts）并使用共享常量。
// 只断言"常量拼起来等于某个字符串"抓不到 admin.go 里的删除或换组，所以这里核对注册
// 源码（与 prompt_audit_route_coverage_test.go 的写法一致），并且：
//   - 只看非注释的代码行（注释掉的注册不算）；
//   - 每条注册恰好出现一次（重复注册会 panic 或造成奇怪的路由树）；
//   - 不允许残留写死前缀的字面量路径（本次就是这类漂移）。
func TestOpenDesignDeviceAuthRoutesRegisteredUnderAccounts(t *testing.T) {
	source, err := os.ReadFile("admin.go")
	require.NoError(t, err)

	var code strings.Builder
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		code.WriteString(line)
		code.WriteString("\n")
	}
	body := code.String()

	for _, want := range []string{
		`accounts.POST(adminHandler.OpenDesignDeviceAuthRoute`,
		`accounts.GET(adminHandler.OpenDesignDeviceAuthStatusRoute`,
		`accounts.DELETE(adminHandler.OpenDesignDeviceAuthStatusRoute`,
		`accounts.POST(adminHandler.OpenDesignAccountsRoute`,
	} {
		require.Equal(t, 1, strings.Count(body, want), "%q must be registered exactly once", want)
	}
	require.NotContains(t, body, `"/opendesign/`, "routes must not hardcode the fragment")
}

// 常量与分组前缀合成后的完整路径：前端 api client 直接拼这些字符串，
// 任何一侧改动都会在这里或前端 spec 里失败。
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
