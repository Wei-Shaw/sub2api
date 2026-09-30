package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		WeeklyWindowStart: &weeklyWindowStart,
	})

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		middleware.AuthSubject{},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Subscription struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

// fakePlatformUsageRepo 只服务 ListByAPIKey，其余方法不会被 /v1/usage 触及。
type fakePlatformUsageRepo struct {
	rows []service.APIKeyPlatformUsageRecord
	err  error
}

func (f *fakePlatformUsageRepo) Get(context.Context, int64, string) (*service.APIKeyPlatformUsageRecord, error) {
	return nil, nil
}

func (f *fakePlatformUsageRepo) ListByAPIKey(context.Context, int64) ([]service.APIKeyPlatformUsageRecord, error) {
	return f.rows, f.err
}

func (f *fakePlatformUsageRepo) IncrementUsage(context.Context, int64, string, float64, time.Time) error {
	return nil
}

func (f *fakePlatformUsageRepo) ResetUsage(context.Context, int64, []string) error { return nil }

func newPlatformUsageHandler(rows []service.APIKeyPlatformUsageRecord, err error) *GatewayHandler {
	apiKeyService := &service.APIKeyService{}
	apiKeyService.SetAPIKeyPlatformUsageRepo(&fakePlatformUsageRepo{rows: rows, err: err})
	return &GatewayHandler{apiKeyService: apiKeyService}
}

type platformLimitResponse struct {
	PlatformLimits []struct {
		Platform string `json:"platform"`
		Quota    *struct {
			Limit     float64 `json:"limit"`
			Used      float64 `json:"used"`
			Remaining float64 `json:"remaining"`
		} `json:"quota"`
		RateLimits []struct {
			Window    string     `json:"window"`
			Limit     float64    `json:"limit"`
			Used      float64    `json:"used"`
			Remaining float64    `json:"remaining"`
			ResetAt   *time.Time `json:"reset_at"`
		} `json:"rate_limits"`
	} `json:"platform_limits"`
}

func decodePlatformLimits(t *testing.T, recorder *httptest.ResponseRecorder) platformLimitResponse {
	t.Helper()
	var response platformLimitResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestUsageQuotaLimitedReportsPlatformLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	freshWindow := time.Now().Add(-time.Hour)
	handler := newPlatformUsageHandler([]service.APIKeyPlatformUsageRecord{
		{Platform: service.PlatformOpenAI, QuotaUsed: 3, Usage1d: 2, Window1dStart: &freshWindow},
	}, nil)

	handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
		ID:     7,
		Status: service.StatusAPIKeyActive,
		PlatformLimits: service.APIKeyPlatformLimits{
			service.PlatformOpenAI:      {Quota: 20, RateLimit1d: 5},
			service.PlatformAntigravity: {RateLimit5h: 1},
		},
	}, nil, nil, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodePlatformLimits(t, recorder)
	require.Len(t, response.PlatformLimits, 2)

	// 顺序跟随 AllowedQuotaPlatforms，openai 在 antigravity 之前。
	require.Equal(t, service.PlatformOpenAI, response.PlatformLimits[0].Platform)
	require.Equal(t, service.PlatformAntigravity, response.PlatformLimits[1].Platform)

	openai := response.PlatformLimits[0]
	require.NotNil(t, openai.Quota)
	require.Equal(t, 20.0, openai.Quota.Limit)
	require.Equal(t, 3.0, openai.Quota.Used)
	require.Equal(t, 17.0, openai.Quota.Remaining)
	require.Len(t, openai.RateLimits, 1)
	require.Equal(t, "1d", openai.RateLimits[0].Window)
	require.Equal(t, 2.0, openai.RateLimits[0].Used)
	require.Equal(t, 3.0, openai.RateLimits[0].Remaining)
	require.NotNil(t, openai.RateLimits[0].ResetAt)

	// 还没消费过的来源仍然要出现，用量按 0 展示。
	antigravity := response.PlatformLimits[1]
	require.Nil(t, antigravity.Quota)
	require.Len(t, antigravity.RateLimits, 1)
	require.Equal(t, "5h", antigravity.RateLimits[0].Window)
	require.Equal(t, 0.0, antigravity.RateLimits[0].Used)
	require.Equal(t, 1.0, antigravity.RateLimits[0].Remaining)
	require.Nil(t, antigravity.RateLimits[0].ResetAt)
}

func TestUsageQuotaLimitedDiscardsExpiredPlatformWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	expired := time.Now().Add(-48 * time.Hour)
	handler := newPlatformUsageHandler([]service.APIKeyPlatformUsageRecord{
		{Platform: service.PlatformOpenAI, Usage1d: 5, Window1dStart: &expired},
	}, nil)

	handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
		ID:             7,
		Status:         service.StatusAPIKeyActive,
		PlatformLimits: service.APIKeyPlatformLimits{service.PlatformOpenAI: {RateLimit1d: 5}},
	}, nil, nil, nil)

	response := decodePlatformLimits(t, recorder)
	require.Len(t, response.PlatformLimits, 1)
	require.Len(t, response.PlatformLimits[0].RateLimits, 1)
	// 窗口过期：用量按 0 计，也不再给出 reset_at。
	require.Equal(t, 0.0, response.PlatformLimits[0].RateLimits[0].Used)
	require.Equal(t, 5.0, response.PlatformLimits[0].RateLimits[0].Remaining)
	require.Nil(t, response.PlatformLimits[0].RateLimits[0].ResetAt)
}

func TestUsageSubscriptionReportsPlatformLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	handler := newPlatformUsageHandler(nil, nil)
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{
			ID: 7,
			Group: &service.Group{
				Name:             "Weekly plan",
				SubscriptionType: service.SubscriptionTypeSubscription,
			},
			PlatformLimits: service.APIKeyPlatformLimits{service.PlatformOpenAI: {Quota: 9}},
		},
		middleware.AuthSubject{},
		nil, nil, nil,
	)

	response := decodePlatformLimits(t, recorder)
	require.Len(t, response.PlatformLimits, 1)
	require.NotNil(t, response.PlatformLimits[0].Quota)
	require.Equal(t, 9.0, response.PlatformLimits[0].Quota.Remaining)
}

func TestUsageOmitsPlatformLimitsWhenUnconfiguredOrUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("key without platform limits", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
		handler := newPlatformUsageHandler(nil, nil)
		handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
			ID: 7, Status: service.StatusAPIKeyActive,
		}, nil, nil, nil)
		require.NotContains(t, recorder.Body.String(), "platform_limits")
	})

	t.Run("usage store failure degrades to no section", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
		handler := newPlatformUsageHandler(nil, errors.New("db down"))
		handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
			ID:             7,
			Status:         service.StatusAPIKeyActive,
			PlatformLimits: service.APIKeyPlatformLimits{service.PlatformOpenAI: {Quota: 1}},
		}, nil, nil, nil)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "platform_limits")
	})
}
