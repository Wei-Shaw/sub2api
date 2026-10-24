//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaOwnedReadsAfterBalanceExhausted(t *testing.T) {
	for _, endpoint := range []service.GrokMediaEndpoint{
		service.GrokMediaEndpointVideoStatus, service.GrokMediaEndpointVideoContent, service.SeedanceEndpointStatus,
	} {
		t.Run(string(endpoint), func(t *testing.T) {
			platform := service.PlatformGrok
			if endpoint.IsSeedance() {
				platform = service.PlatformOpenAI
			}
			var h *OpenAIGatewayHandler
			var slots *grokMediaSlotsCache
			var upstream *grokMediaSlotUpstream
			if platform == service.PlatformOpenAI {
				h, slots, _, upstream = newGrokMediaSlotHandler(t, false, false, platform)
			} else {
				h, slots, _, upstream = newGrokMediaSlotHandler(t, false, false)
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, FailClosedOnUnpriced: true}
			cache := newHandlerInflightCache(-1)
			billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h.billingCacheService = billing
			taskID := "task"
			groupID := int64(24)
			if endpoint.IsSeedance() {
				taskID = service.SeedanceTaskKey(taskID)
				require.NoError(t, h.gatewayService.BindGrokMediaVideoRequestAccount(context.Background(), &groupID, taskID, 10, 20, 1))
			}
			upstream.call = func(req *http.Request, accountID int64) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, int64(1), accountID, "reads stay on the accepting account")
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: http.NoBody}, nil
			}
			for _, other := range []string{"", "user", "key", "group", "task"} {
				c, w := grokMediaSlotContext(context.Background(), false)
				key, ok := middleware.GetAPIKeyFromContext(c)
				require.True(t, ok)
				key.Status, key.User.Balance = service.StatusAPIKeyQuotaExhausted, -1
				key.Group.Platform = platform
				if endpoint.IsSeedance() {
					c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task", nil)
					c.Params = gin.Params{{Key: "task_id", Value: "task"}}
				} else if endpoint == service.GrokMediaEndpointVideoContent {
					c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task/content", nil)
				}
				switch other {
				case "user":
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11, Concurrency: 5})
				case "key":
					key.ID++
				case "group":
					otherGroup := int64(25)
					key.GroupID = &otherGroup
				case "task":
					c.Params[0].Value = "other-task"
				}
				before := upstream.calls
				if endpoint.IsSeedance() {
					h.SeedanceTasks(c)
				} else if endpoint == service.GrokMediaEndpointVideoContent {
					h.GrokVideoContent(c)
				} else {
					h.GrokVideoStatus(c)
				}
				if other == "" {
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					require.Greater(t, upstream.calls, before, "real forwarding must run")
				} else {
					require.Equal(t, http.StatusNotFound, w.Code, other+": "+w.Body.String())
					require.Equal(t, before, upstream.calls)
				}
				slots.assertReleased(t)
			}
			// The same low balance still refuses a new job and a destructive request.
			c, w := grokMediaSlotContext(context.Background(), true)
			h.GrokVideoGeneration(c)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			c, w = grokMediaSlotContext(context.Background(), false)
			c.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task", strings.NewReader(""))
			before := upstream.calls
			h.handleGrokMedia(c, service.SeedanceEndpointDelete, taskID)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			require.Equal(t, before, upstream.calls)
			slots.assertReleased(t)
		})
	}
}
