//go:build unit

package admin

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 回归分组平台枚举:kimi/zhipu/deepseek 必须能通过 Create/Update 的 binding 校验
// （历史 bug:调度/路由链路已支持 CN 平台分组,但 oneof 白名单漏加三平台,导致
// 平台分组无法创建、CN 账号"无可用分组"）;非法值仍须被拒。
func bindGroupPlatformJSON(t *testing.T, target any, body string) error {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c.ShouldBindJSON(target)
}

func TestGroupPlatformBinding_AllowedPlatforms(t *testing.T) {
	allowed := []string{
		"anthropic", "openai", "gemini", "antigravity", "kiro", "grok",
		"adobe", "kimi", "zhipu", "deepseek", "minimax", "composite",
	}
	for _, platform := range allowed {
		t.Run("create_"+platform, func(t *testing.T) {
			var req CreateGroupRequest
			body := fmt.Sprintf(`{"name":"g","platform":%q}`, platform)
			require.NoError(t, bindGroupPlatformJSON(t, &req, body),
				"platform %q 应通过 CreateGroupRequest 校验", platform)
			require.Equal(t, platform, req.Platform)
		})
		t.Run("update_"+platform, func(t *testing.T) {
			var req UpdateGroupRequest
			body := fmt.Sprintf(`{"platform":%q}`, platform)
			require.NoError(t, bindGroupPlatformJSON(t, &req, body),
				"platform %q 应通过 UpdateGroupRequest 校验", platform)
			require.Equal(t, platform, req.Platform)
		})
	}
}

func TestGroupPlatformBinding_RejectsInvalidPlatforms(t *testing.T) {
	invalid := []string{
		"moonshot", // 厂商别名,不是平台标识
		"Kimi",     // 大小写敏感
		"openai ",  // 尾随空格
		"glm",
		"bogus",
	}
	for _, platform := range invalid {
		t.Run("create_"+platform, func(t *testing.T) {
			var req CreateGroupRequest
			body := fmt.Sprintf(`{"name":"g","platform":%q}`, platform)
			require.Error(t, bindGroupPlatformJSON(t, &req, body),
				"platform %q 应被 CreateGroupRequest 拒绝", platform)
		})
		t.Run("update_"+platform, func(t *testing.T) {
			var req UpdateGroupRequest
			body := fmt.Sprintf(`{"platform":%q}`, platform)
			require.Error(t, bindGroupPlatformJSON(t, &req, body),
				"platform %q 应被 UpdateGroupRequest 拒绝", platform)
		})
	}
}

func TestCompositeRouteTargetPlatform_AllowsCNProviders(t *testing.T) {
	for _, platform := range []string{"kimi", "zhipu", "deepseek", "minimax", "adobe"} {
		var req CompositeRouteRequest
		body := fmt.Sprintf(`{"public_model":"m","target_platform":%q}`, platform)
		require.NoError(t, bindGroupPlatformJSON(t, &req, body))
		require.Equal(t, platform, req.TargetPlatform)
	}
}

// 上面的白名单是手写的，加平台时同样会漏。这条把 binding tag 直接钉到
// model.AllPlatforms()——那份清单自己有守卫测试，于是新增平台只要漏了 oneof tag，
// 这里就会失败，而不是等到管理端建分组时报
// "Field validation for 'Platform' failed on the 'oneof' tag"。
func TestGroupPlatformBinding_CoversEveryConcretePlatform(t *testing.T) {
	for _, platform := range model.AllPlatforms() {
		t.Run(platform, func(t *testing.T) {
			var createReq CreateGroupRequest
			require.NoError(t, bindGroupPlatformJSON(t, &createReq,
				fmt.Sprintf(`{"name":"g","platform":%q}`, platform)),
				"platform %q 已在 model.AllPlatforms() 里，却过不了 CreateGroupRequest 的 oneof 白名单", platform)

			var updateReq UpdateGroupRequest
			require.NoError(t, bindGroupPlatformJSON(t, &updateReq,
				fmt.Sprintf(`{"platform":%q}`, platform)),
				"platform %q 过不了 UpdateGroupRequest 的 oneof 白名单", platform)
		})
	}
}
