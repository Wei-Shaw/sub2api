//go:build unit

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatAudioValidationHandlerSchedulerBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settingsService := service.NewSettingService(&contentModerationHandlerSettingRepo{}, &config.Config{})
	settings, err := settingsService.GetAllSettings(context.Background())
	require.NoError(t, err)
	originalSchedulerEnabled := settings.OpenAIAdvancedSchedulerEnabled
	t.Cleanup(func() {
		settings.OpenAIAdvancedSchedulerEnabled = originalSchedulerEnabled
		require.NoError(t, settingsService.UpdateSettings(context.Background(), settings))
	})
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformGrok} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", platform, stream), func(t *testing.T) {
				settings.OpenAIAdvancedSchedulerEnabled = true
				require.NoError(t, settingsService.UpdateSettings(context.Background(), settings))
				handler, repo, upstream, _, cleanup := newGrokCredentialFailoverHandler(t, "first_402")
				defer cleanup()
				for index := range repo.accounts {
					repo.accounts[index].Platform = platform
				}
				recorder := httptest.NewRecorder()
				cache := &concurrencyCacheMock{
					acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return !stream || recorder.Flushed, nil },
					acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
				}
				handler.concurrencyHelper = NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatComment, time.Millisecond)
				groupID := int64(901)
				apiKey := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: platform, Status: service.StatusActive}}
				router := gin.New()
				router.Use(func(ginContext *gin.Context) {
					ginContext.Set(string(middleware.ContextKeyAPIKey), apiKey)
					ginContext.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 903, Concurrency: 1})
				})
				router.POST("/v1/chat/completions", handler.ChatCompletions)
				model := "gpt-5"
				if platform == service.PlatformGrok {
					model = "grok-4.5"
				}
				body := fmt.Sprintf(`{"model":%q,"stream":%t,"prompt_cache_key":"audio-boundary-session","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]}]}`, model, stream)
				if platform == service.PlatformGrok {
					body = strings.Replace(body, `"type":"input_audio"`, `"type":"input_audio","type":"text","text":"hidden"`, 1)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)).WithContext(ctx)
				router.ServeHTTP(recorder, request)
				require.Empty(t, upstream.hits)
				require.Contains(t, recorder.Body.String(), "input_audio")
				metrics := handler.gatewayService.SnapshotOpenAIAccountSchedulerMetrics()
				require.Positive(t, metrics.SelectTotal)
				require.Zero(t, metrics.RuntimeStatsAccountCount, "local validation must not update scheduler EWMA")
				if stream {
					require.True(t, recorder.Flushed)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.True(t, strings.HasPrefix(recorder.Body.String(), ":\n\n"))
					require.Contains(t, recorder.Body.String(), "data: {\"error\":")
					require.True(t, strings.HasSuffix(recorder.Body.String(), "\n\n"))
				} else {
					require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
					require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
				}
			})
		}
	}
}
