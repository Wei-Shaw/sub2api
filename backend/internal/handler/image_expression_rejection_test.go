package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageExpressionRejectedBeforeTaskCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"model":"gpt-image-1","prompt":"draw"}`, `{"prompt":"draw"}`} {
		t.Run(body, func(t *testing.T) {
			store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
			tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
			h := &AsyncImageHandler{tasks: tasks, execute: func(_ string, _ *gin.Context) { t.Error("unsupported expression request was executed") }}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7, Group: &service.Group{Platform: service.PlatformOpenAI, AllowImageGeneration: true, ImagePriceExpressions: map[string]string{"gpt-image-1": "fixed(0.04)*image_count", "gpt-image-2": "fixed(0.04)*image_count"}}})
				c.Next()
			})
			router.POST("/v1/images/generations/async", h.Submit)
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "Image price expressions require")
			store.mu.RLock()
			defer store.mu.RUnlock()
			require.Empty(t, store.tasks, "reject before saving or scheduling a task")
		})
	}
}

func TestImageExpressionPreflightRunsWithoutBalanceReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformGrok} {
		for _, subscription := range []bool{false, true} {
			t.Run(platform+fmt.Sprint(subscription), func(t *testing.T) {
				h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
				cfg := &config.Config{}
				cfg.Billing.InflightReservation.Enabled = subscription
				h.billingCacheService = service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(h.billingCacheService.Stop)
				model := "gpt-image-1"
				if platform == service.PlatformGrok {
					model = "grok-imagine-image-2.0"
				}
				group := &service.Group{Platform: platform, RateMultiplier: 1, AllowImageGeneration: true, ImagePriceExpressions: map[string]string{model: "-1"}}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"draw"}`, model)))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7, User: &service.User{ID: 7}, Group: group})
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7, Concurrency: 1})
				if subscription {
					group.SubscriptionType = service.SubscriptionTypeSubscription
					c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{})
				}
				if platform == service.PlatformGrok {
					h.GrokImages(c)
				} else {
					h.Images(c)
				}
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Contains(t, recorder.Body.String(), "invalid image price expression")
			})
		}
	}
}

func TestImageExpressionWebSocketTurnUsesRefreshedPricingGroup(t *testing.T) {
	conn := &service.APIKey{Group: &service.Group{}}
	refreshed := &service.APIKey{Group: &service.Group{ImagePriceExpressions: map[string]string{"public-image": "0.04*image_count"}}}
	var turns openAIWSTurnBillingAPIKeys
	turns.setModel(2, "public-image")
	turns.set(2, refreshed)
	require.ErrorContains(t, turns.rejectImageExpression(2, conn, "original-model"), service.ImageExpressionUnsupportedMessage)
	turns.setModel(3, "another-model")
	turns.set(3, refreshed)
	require.NoError(t, turns.rejectImageExpression(3, conn, "original-model"))
	// First native turn uses the authenticated connection model as fallback.
	require.ErrorContains(t, turns.rejectImageExpression(1, refreshed, "public-image"), service.ImageExpressionUnsupportedMessage)
}
