package routes

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

// intelligenceCheckRouteSettingRepoStub 是路由守卫用的最小 SettingRepository。
// 与渠道监控的 stub 不同：智力检测的总开关读取路径会走 GetAll，所以这里必须有真实实现，
// 否则守卫一被调用就会 panic。
type intelligenceCheckRouteSettingRepoStub struct {
	values map[string]string
}

func (s *intelligenceCheckRouteSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *intelligenceCheckRouteSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *intelligenceCheckRouteSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *intelligenceCheckRouteSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *intelligenceCheckRouteSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

// GetAll 必须返回真实值：智力检测的全局设置读取会走到这里。
func (s *intelligenceCheckRouteSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}

func (s *intelligenceCheckRouteSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func newIntelligenceCheckRouteSettings(enabled bool, modelID string) *service.SettingService {
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	return service.NewSettingService(&intelligenceCheckRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyIntelligenceCheckEnabled: enabledValue,
			service.SettingKeyIntelligenceCheckModelID: modelID,
		},
	}, &config.Config{})
}

// TestIntelligenceCheckEnabledGuard 覆盖用户侧作品墙的总开关守卫。
//
// 只按「开关是否打开」放行：没配模型时作品墙依然可见（历史作品照旧展示，
// 只是不会再有新的跑测），所以这里刻意不断言 modelID 必须非空——那是
// 调度器 Runnable() 的职责，不是路由守卫的。
func TestIntelligenceCheckEnabledGuard(t *testing.T) {
	tests := []struct {
		name       string
		svc        *service.SettingService
		wantStatus int
	}{
		{
			name:       "nil setting service blocks",
			svc:        nil,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "disabled blocks",
			svc:        newIntelligenceCheckRouteSettings(false, "test-model"),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "enabled allows",
			svc:        newIntelligenceCheckRouteSettings(true, "test-model"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "enabled without model still allows",
			svc:        newIntelligenceCheckRouteSettings(true, ""),
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			router := gin.New()
			router.Use(intelligenceCheckEnabledGuard(tt.svc))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusForbidden {
				require.Contains(t, rec.Body.String(), "INTELLIGENCE_CHECK_DISABLED")
			}
		})
	}
}
