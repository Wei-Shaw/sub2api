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

// channelMonitorRouteSettingRepoStub is a minimal SettingRepository for route guards.
type channelMonitorRouteSettingRepoStub struct {
	values map[string]string
}

func (s *channelMonitorRouteSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *channelMonitorRouteSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *channelMonitorRouteSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *channelMonitorRouteSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *channelMonitorRouteSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *channelMonitorRouteSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *channelMonitorRouteSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func newChannelMonitorRouteSettings(enabled bool) *service.SettingService {
	value := "false"
	if enabled {
		value = "true"
	}
	return service.NewSettingService(&channelMonitorRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyChannelMonitorEnabled: value,
		},
	}, &config.Config{})
}

func newChannelMonitorModeSettings(enabled bool, mode string) *service.SettingService {
	enabledVal := "false"
	if enabled {
		enabledVal = "true"
	}
	return service.NewSettingService(&channelMonitorRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyChannelMonitorEnabled: enabledVal,
			service.SettingKeyChannelMonitorMode:    mode,
		},
	}, &config.Config{})
}

func TestChannelMonitorAdminFeatureGuard(t *testing.T) {
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
			svc:        newChannelMonitorRouteSettings(false),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "enabled allows",
			svc:        newChannelMonitorRouteSettings(true),
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			router := gin.New()
			router.Use(channelMonitorAdminFeatureGuard(tt.svc))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusForbidden {
				require.Contains(t, rec.Body.String(), "CHANNEL_MONITOR_DISABLED")
			}
		})
	}
}

func TestChannelMonitorModeV2Guard(t *testing.T) {
	tests := []struct {
		name       string
		svc        *service.SettingService
		wantStatus int
		wantCode   string
	}{
		{
			name:       "nil blocks as disabled",
			svc:        nil,
			wantStatus: http.StatusForbidden,
			wantCode:   "CHANNEL_MONITOR_DISABLED",
		},
		{
			name:       "feature off blocks",
			svc:        newChannelMonitorModeSettings(false, service.ChannelMonitorModeV2),
			wantStatus: http.StatusForbidden,
			wantCode:   "CHANNEL_MONITOR_DISABLED",
		},
		{
			name:       "mode v1 blocks with mode mismatch",
			svc:        newChannelMonitorModeSettings(true, service.ChannelMonitorModeV1),
			wantStatus: http.StatusForbidden,
			wantCode:   "CHANNEL_MONITOR_MODE_MISMATCH",
		},
		{
			name:       "mode v2 allows",
			svc:        newChannelMonitorModeSettings(true, service.ChannelMonitorModeV2),
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(channelMonitorModeV2Guard(tt.svc))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			router.ServeHTTP(rec, req)
			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode != "" {
				require.Contains(t, rec.Body.String(), tt.wantCode)
			}
		})
	}
}
