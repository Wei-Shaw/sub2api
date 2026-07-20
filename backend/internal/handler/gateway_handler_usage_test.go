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

func TestUsageUnrestrictedIncludesSubscriptionWindowStartsAndResets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	dailyWindowStart := time.Date(2026, time.July, 18, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	monthlyWindowStart := time.Date(2026, time.July, 1, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		DailyWindowStart:   &dailyWindowStart,
		WeeklyWindowStart:  &weeklyWindowStart,
		MonthlyWindowStart: &monthlyWindowStart,
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
			DailyWindowStart   *time.Time `json:"daily_window_start"`
			WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
			MonthlyWindowStart *time.Time `json:"monthly_window_start"`
			DailyResetAt       *time.Time `json:"daily_reset_at"`
			WeeklyResetAt      *time.Time `json:"weekly_reset_at"`
			MonthlyResetAt     *time.Time `json:"monthly_reset_at"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Subscription.DailyWindowStart)
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.NotNil(t, response.Subscription.MonthlyWindowStart)
	require.NotNil(t, response.Subscription.DailyResetAt)
	require.NotNil(t, response.Subscription.WeeklyResetAt)
	require.NotNil(t, response.Subscription.MonthlyResetAt)
	require.True(t, dailyWindowStart.Equal(*response.Subscription.DailyWindowStart))
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
	require.True(t, monthlyWindowStart.Equal(*response.Subscription.MonthlyWindowStart))
	require.True(t, dailyWindowStart.Add(24*time.Hour).Equal(*response.Subscription.DailyResetAt))
	require.True(t, weeklyWindowStart.Add(7*24*time.Hour).Equal(*response.Subscription.WeeklyResetAt))
	require.True(t, monthlyWindowStart.Add(30*24*time.Hour).Equal(*response.Subscription.MonthlyResetAt))
}
