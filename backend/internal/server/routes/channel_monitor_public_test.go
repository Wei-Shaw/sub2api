//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestRegisterUserRoutesExposesChannelMonitorPublicly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")
	settingService := service.NewSettingService(disabledMonitorSettingsRepo{}, nil)
	handlers := &handler.Handlers{
		ChannelMonitor: handler.NewChannelMonitorUserHandler(nil, settingService),
	}

	RegisterUserRoutes(v1, handlers, func(c *gin.Context) {
		t.Fatalf("channel monitor public routes must not invoke jwt auth")
	}, nil)

	publicPaths := []string{
		"/api/v1/channel-monitors",
		"/api/v1/channel-monitors/1/status",
		"/api/v1/public/channel-monitors",
		"/api/v1/public/channel-monitors/1/status",
	}

	for _, path := range publicPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusUnauthorized {
				t.Fatalf("GET %s should be public, got %d", path, w.Code)
			}
		})
	}
}

type disabledMonitorSettingsRepo struct{}

func (disabledMonitorSettingsRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (disabledMonitorSettingsRepo) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (disabledMonitorSettingsRepo) Set(context.Context, string, string) error { return nil }

func (disabledMonitorSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if key == service.SettingKeyChannelMonitorEnabled {
			out[key] = "false"
		}
	}
	return out, nil
}

func (disabledMonitorSettingsRepo) SetMultiple(context.Context, map[string]string) error { return nil }

func (disabledMonitorSettingsRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{service.SettingKeyChannelMonitorEnabled: "false"}, nil
}

func (disabledMonitorSettingsRepo) Delete(context.Context, string) error { return nil }
