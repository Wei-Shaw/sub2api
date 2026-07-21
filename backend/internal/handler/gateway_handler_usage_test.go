package handler

import (
	"context"
	"encoding/json"
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

func TestUsageQuotaLimitedIncludesSubscriptionMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	groupID := int64(7)
	dailyStart := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	weeklyStart := dailyStart.Add(-2 * 24 * time.Hour)
	monthlyStart := dailyStart.Add(-10 * 24 * time.Hour)
	expiresAt := dailyStart.Add(45 * 24 * time.Hour)
	dailyLimit, weeklyLimit, monthlyLimit := 10.0, 50.0, 200.0
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		UserID:             11,
		GroupID:            groupID,
		ExpiresAt:          expiresAt,
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
		DailyUsageUSD:      2,
		WeeklyUsageUSD:     5,
		MonthlyUsageUSD:    20,
	})

	handler := &GatewayHandler{}
	handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
		Quota:     100,
		QuotaUsed: 25,
		Status:    service.StatusAPIKeyActive,
		GroupID:   &groupID,
		Group: &service.Group{
			ID:               groupID,
			Name:             "Pro Monthly",
			SubscriptionType: service.SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
			WeeklyLimitUSD:   &weeklyLimit,
			MonthlyLimitUSD:  &monthlyLimit,
		},
	}, nil, nil, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Mode     string `json:"mode"`
		PlanName string `json:"planName"`
		Quota    struct {
			Remaining float64 `json:"remaining"`
		} `json:"quota"`
		Subscription struct {
			DailyResetAt   *time.Time `json:"daily_reset_at"`
			WeeklyResetAt  *time.Time `json:"weekly_reset_at"`
			MonthlyResetAt *time.Time `json:"monthly_reset_at"`
			ExpiresAt      time.Time  `json:"expires_at"`
			Unlimited      bool       `json:"unlimited"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "quota_limited", response.Mode)
	require.Equal(t, "Pro Monthly", response.PlanName)
	require.Equal(t, 75.0, response.Quota.Remaining)
	require.Equal(t, dailyStart.Add(24*time.Hour), *response.Subscription.DailyResetAt)
	require.Equal(t, weeklyStart.Add(7*24*time.Hour), *response.Subscription.WeeklyResetAt)
	require.Equal(t, monthlyStart.Add(30*24*time.Hour), *response.Subscription.MonthlyResetAt)
	require.Equal(t, expiresAt, response.Subscription.ExpiresAt)
	require.False(t, response.Subscription.Unlimited)
	require.NotContains(t, recorder.Body.String(), `"group_id"`)
	require.NotContains(t, recorder.Body.String(), `"subscription_id"`)
	require.NotContains(t, recorder.Body.String(), `"user_id"`)
}
