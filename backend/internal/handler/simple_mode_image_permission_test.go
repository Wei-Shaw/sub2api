package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayHandler_ImagePermissionRunMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := []struct {
		name     string
		platform string
		body     string
		handle   func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{"images", service.PlatformOpenAI, `{"model":"gpt-image-2","prompt":"draw"}`, (*OpenAIGatewayHandler).Images},
		{"responses", service.PlatformOpenAI, `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}]}`, (*OpenAIGatewayHandler).Responses},
		{"Grok images", service.PlatformGrok, `{"model":"grok-imagine-image","prompt":"draw"}`, (*OpenAIGatewayHandler).GrokImages},
		{"Grok video shared flag", service.PlatformGrok, `{"model":"grok-imagine-video","prompt":"draw"}`, (*OpenAIGatewayHandler).GrokVideoGeneration},
	}
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		for _, route := range routes {
			t.Run(mode+"/"+route.name, func(t *testing.T) {
				cfg := &config.Config{RunMode: mode}
				cfg.Gateway.ImageConcurrency = config.ImageConcurrencyConfig{
					Enabled: true, MaxConcurrentRequests: 1, OverflowMode: config.ImageConcurrencyOverflowModeReject,
				}
				h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
				h.cfg = cfg
				h.imageLimiter = &imageConcurrencyLimiter{}
				release, acquired := h.imageLimiter.TryAcquire(true, 1)
				require.True(t, acquired)
				defer release()
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(route.body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					ID: 1, UserID: 2, Group: &service.Group{ID: 3, Platform: route.platform, AllowImageGeneration: false}, User: &service.User{ID: 2},
				})
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2, Concurrency: 1})
				route.handle(h, c)
				if mode == config.RunModeStandard {
					require.Equal(t, http.StatusForbidden, rec.Code)
					require.Equal(t, "permission_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
					return
				}
				// Passing the permission gate must not bypass image concurrency.
				require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "Image generation concurrency limit exceeded")
			})
		}
	}
}

func TestAsyncImageHandler_ImagePermissionRunMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		for _, platform := range []string{service.PlatformOpenAI, service.PlatformGrok} {
			t.Run(mode+"/"+platform, func(t *testing.T) {
				store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
				h := NewAsyncImageHandler(service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute), &OpenAIGatewayHandler{cfg: &config.Config{RunMode: mode}})
				completed := make(chan struct{})
				h.execute = func(_ string, c *gin.Context) {
					defer close(completed)
					c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/image.png"}}})
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"draw"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, UserID: 2, Group: &service.Group{Platform: platform}})
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2, Concurrency: 1})
				h.Submit(c)
				if mode == config.RunModeStandard {
					require.Equal(t, http.StatusForbidden, rec.Code)
					require.Equal(t, "permission_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
					require.Empty(t, store.tasks)
					return
				}
				require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
				select {
				case <-completed:
				case <-time.After(time.Second):
					t.Fatal("accepted task did not execute")
				}
			})
		}
	}
}

func TestOpenAIResponsesWebSocket_ImagePermissionRunMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		t.Run(mode, func(t *testing.T) {
			h := newOpenAIHandlerForPreviousResponseIDValidation(t, &concurrencyCacheMock{
				acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return false, nil },
			})
			h.cfg = &config.Config{RunMode: mode}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					ID: 1, Group: &service.Group{Platform: service.PlatformOpenAI}, User: &service.User{ID: 2},
				})
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2, Concurrency: 1})
			})
			router.GET("/v1/responses", h.ResponsesWebSocket)
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","tools":[{"type":"image_generation"}],"input":"draw"}`)))
			_, _, err = client.Read(ctx)
			var closeErr coderws.CloseError
			require.ErrorAs(t, err, &closeErr)
			if mode == config.RunModeStandard {
				require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
				require.Equal(t, service.ImageGenerationPermissionMessage(), closeErr.Reason)
			} else {
				// The first-frame bypass still enforces user concurrency.
				require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
				require.Contains(t, closeErr.Reason, "too many concurrent requests")
			}
		})
	}
}
