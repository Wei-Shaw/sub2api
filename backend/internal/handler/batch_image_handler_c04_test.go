//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type c04BillingEligibilityStub struct {
	err          error
	calls        int
	user         *service.User
	apiKey       *service.APIKey
	group        *service.Group
	subscription *service.UserSubscription
	platform     string
}

func (s *c04BillingEligibilityStub) CheckBillingEligibility(_ context.Context, user *service.User, apiKey *service.APIKey, group *service.Group, subscription *service.UserSubscription, platform string) error {
	s.calls++
	s.user, s.apiKey, s.group, s.subscription, s.platform = user, apiKey, group, subscription, platform
	return s.err
}

func c04BatchImageRouter(h *BatchImageHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		user := &service.User{ID: 7, Username: "batch-user", Email: "batch@example.test"}
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 9, UserID: 7, User: user, Name: "batch-key", GroupID: &groupID,
			Group: &service.Group{ID: groupID, Name: "batch-group", Platform: service.PlatformGemini, AllowBatchImageGeneration: true},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7, Concurrency: 2})
		c.Next()
	})
	router.POST("/v1/images/batches", h.Submit)
	return router
}

func c04SubmitBatch(router *gin.Engine) *httptest.ResponseRecorder {
	body := `{"model":"gemini-image","items":[{"custom_id":"one","prompt":"a cat"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/images/batches", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestBatchImageSubmitRejectsWhenBillingIneligible 钉住 C-04：批量生图提交此前从不调用
// CheckBillingEligibility，RPM、API Key 5h/1d/7d 窗口和 user×platform 配额对它都不生效。
func TestBatchImageSubmitRejectsWhenBillingIneligible(t *testing.T) {
	billing := &c04BillingEligibilityStub{err: service.ErrUserRPMExceeded}
	h := &BatchImageHandler{billing: billing}

	var recorder *httptest.ResponseRecorder
	require.NotPanics(t, func() { recorder = c04SubmitBatch(c04BatchImageRouter(h)) }, "nil service would panic if Submit were reached")
	require.Equal(t, 1, billing.calls)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.NotEmpty(t, recorder.Header().Get("Retry-After"))
	require.Contains(t, recorder.Body.String(), "rate_limit_exceeded")
}

// TestBatchImageSubmitChecksBalanceModeEligibilityForGeminiPlatform 批量生图始终从余额冻结扣款，
// 所以按余额模式（不带订阅）检查，并按 Gemini 平台计 user×platform 配额。
func TestBatchImageSubmitChecksBalanceModeEligibilityForGeminiPlatform(t *testing.T) {
	billing := &c04BillingEligibilityStub{}
	h := &BatchImageHandler{billing: billing}

	recorder := c04SubmitBatch(c04BatchImageRouter(h))
	require.Equal(t, 1, billing.calls)
	require.NotNil(t, billing.apiKey)
	require.Equal(t, int64(9), billing.apiKey.ID)
	require.NotNil(t, billing.user)
	require.Equal(t, int64(7), billing.user.ID)
	require.NotNil(t, billing.group)
	require.Equal(t, int64(3), billing.group.ID)
	require.Nil(t, billing.subscription)
	require.Equal(t, service.PlatformGemini, billing.platform)
	// 通过检查后才进入服务层（此处为 nil service → 批量生图未启用）。
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
