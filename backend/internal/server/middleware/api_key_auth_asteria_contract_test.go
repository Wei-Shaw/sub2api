//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// asteria-control 依赖这个事实：把 status 写成 "inactive"（HTTP 层 oneof 允许、
// 但不是 service 层域常量）后，数据面鉴权中间件靠"非 active 且非 expired/quota_exhausted
// → 无条件拦截"这条兜底分支返回 401 API_KEY_DISABLED。
// 控制面的"停用"就是写 inactive；这条分支一旦收窄成只认 disabled，被停用的 key 会继续放行。
func TestAsteriaContract_InactiveStatusIsRejectedAsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, status := range []string{
		"inactive",                   // asteria-control 实际写入的值
		service.StatusAPIKeyDisabled, // 域常量，作对照：两者必须同样被拦
	} {
		t.Run(status, func(t *testing.T) {
			user := &service.User{ID: 11, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
			group := &service.Group{ID: 8, Platform: service.PlatformAnthropic, Status: service.StatusActive}
			apiKey := &service.APIKey{
				ID: 106, UserID: user.ID, Key: "asteria-" + status, Status: status,
				User: user, Group: group, GroupID: &group.ID,
			}
			apiKeyRepo := &stubApiKeyRepo{getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
				if key != apiKey.Key {
					return nil, service.ErrAPIKeyNotFound
				}
				clone := *apiKey
				userClone := *user
				clone.User = &userClone
				return &clone, nil
			}}

			for _, mode := range []string{config.RunModeSimple, config.RunModeStandard} {
				cfg := &config.Config{RunMode: mode}
				router := newAuthTestRouter(service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg), nil, cfg)
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				req.Header.Set("x-api-key", apiKey.Key)
				router.ServeHTTP(w, req)

				require.Equal(t, http.StatusUnauthorized, w.Code, "run mode %s", mode)
				requireAPIKeyAuthError(t, w, "API_KEY_DISABLED", "API key is disabled")
			}
		})
	}
}
