package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
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

type usageSubscriptionResponse struct {
	Remaining    float64 `json:"remaining"`
	Subscription struct {
		DailyUsageUSD     float64    `json:"daily_usage_usd"`
		WeeklyUsageUSD    float64    `json:"weekly_usage_usd"`
		MonthlyUsageUSD   float64    `json:"monthly_usage_usd"`
		DailyResetAt      *time.Time `json:"daily_reset_at"`
		WeeklyResetAt     *time.Time `json:"weekly_reset_at"`
		MonthlyResetAt    *time.Time `json:"monthly_reset_at"`
		WeeklyWindowStart *time.Time `json:"weekly_window_start"`
	} `json:"subscription"`
}

func usageLimitedGroup(daily, weekly, monthly float64) *service.Group {
	return &service.Group{
		Name:             "Subscription plan",
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &daily,
		WeeklyLimitUSD:   &weekly,
		MonthlyLimitUSD:  &monthly,
	}
}

func serveUsageUnrestrictedSubscription(t *testing.T, group *service.Group, subscription *service.UserSubscription) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	c.Set(string(middleware.ContextKeySubscription), subscription)

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: group},
		middleware.AuthSubject{},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.Bytes()
}

func decodeUsageSubscription(t *testing.T, body []byte) usageSubscriptionResponse {
	t.Helper()
	var response usageSubscriptionResponse
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func requireSameInstant(t *testing.T, expected time.Time, actual *time.Time, name string) {
	t.Helper()
	require.NotNil(t, actual, name)
	require.True(t, expected.Equal(*actual), "%s: expected %s, got %s", name, expected, *actual)
}

func TestUsageUnrestrictedReportsCurrentWindowResetTimes(t *testing.T) {
	now := time.Now()
	weeklyWindowStart := now.Add(-2 * 24 * time.Hour)
	monthlyWindowStart := now.Add(-10 * 24 * time.Hour)
	dailyWindowStart := timezone.StartOfDay(now)

	body := serveUsageUnrestrictedSubscription(t, usageLimitedGroup(10, 50, 100), &service.UserSubscription{
		StartsAt:           now.Add(-20 * 24 * time.Hour),
		ExpiresAt:          now.Add(60 * 24 * time.Hour),
		DailyWindowStart:   &dailyWindowStart,
		WeeklyWindowStart:  &weeklyWindowStart,
		MonthlyWindowStart: &monthlyWindowStart,
		DailyUsageUSD:      4,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
	})

	response := decodeUsageSubscription(t, body)
	requireSameInstant(t, dailyWindowStart.AddDate(0, 0, 1), response.Subscription.DailyResetAt, "daily_reset_at")
	requireSameInstant(t, weeklyWindowStart.Add(7*24*time.Hour), response.Subscription.WeeklyResetAt, "weekly_reset_at")
	requireSameInstant(t, monthlyWindowStart.Add(30*24*time.Hour), response.Subscription.MonthlyResetAt, "monthly_reset_at")
	require.Equal(t, 4.0, response.Subscription.DailyUsageUSD)
	require.Equal(t, 20.0, response.Subscription.WeeklyUsageUSD)
	require.Equal(t, 30.0, response.Subscription.MonthlyUsageUSD)
	require.Equal(t, 6.0, response.Remaining)
}

// A window whose reset point already passed with no charge since is still
// stale in storage; /v1/usage must report what billing would enforce instead.
func TestUsageUnrestrictedAppliesPendingLazyWindowResets(t *testing.T) {
	now := time.Now()
	staleDailyStart := timezone.StartOfDay(now).AddDate(0, 0, -2)
	staleWeeklyStart := now.Add(-9 * 24 * time.Hour)
	staleMonthlyStart := now.Add(-31 * 24 * time.Hour)

	body := serveUsageUnrestrictedSubscription(t, usageLimitedGroup(10, 50, 100), &service.UserSubscription{
		StartsAt:           now.Add(-40 * 24 * time.Hour),
		ExpiresAt:          now.Add(60 * 24 * time.Hour),
		DailyWindowStart:   &staleDailyStart,
		WeeklyWindowStart:  &staleWeeklyStart,
		MonthlyWindowStart: &staleMonthlyStart,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     45,
		MonthlyUsageUSD:    99,
	})

	response := decodeUsageSubscription(t, body)
	require.Zero(t, response.Subscription.DailyUsageUSD)
	require.Zero(t, response.Subscription.WeeklyUsageUSD)
	require.Zero(t, response.Subscription.MonthlyUsageUSD)
	require.Equal(t, 10.0, response.Remaining)
	requireSameInstant(t, timezone.StartOfDay(now).AddDate(0, 0, 1), response.Subscription.DailyResetAt, "daily_reset_at")
	requireSameInstant(t, staleWeeklyStart.Add(14*24*time.Hour), response.Subscription.WeeklyResetAt, "weekly_reset_at")
	requireSameInstant(t, staleMonthlyStart.Add(60*24*time.Hour), response.Subscription.MonthlyResetAt, "monthly_reset_at")
	requireSameInstant(t, staleWeeklyStart.Add(7*24*time.Hour), response.Subscription.WeeklyWindowStart, "weekly_window_start")
}

func TestUsageUnrestrictedCapsResetTimesAtExpiry(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(3 * 24 * time.Hour)
	weeklyWindowStart := now.Add(-24 * time.Hour)
	monthlyWindowStart := now.Add(-24 * time.Hour)
	dailyWindowStart := timezone.StartOfDay(now)

	body := serveUsageUnrestrictedSubscription(t, usageLimitedGroup(10, 50, 100), &service.UserSubscription{
		StartsAt:           now.Add(-24 * time.Hour),
		ExpiresAt:          expiresAt,
		DailyWindowStart:   &dailyWindowStart,
		WeeklyWindowStart:  &weeklyWindowStart,
		MonthlyWindowStart: &monthlyWindowStart,
	})

	response := decodeUsageSubscription(t, body)
	requireSameInstant(t, dailyWindowStart.AddDate(0, 0, 1), response.Subscription.DailyResetAt, "daily_reset_at")
	requireSameInstant(t, expiresAt, response.Subscription.WeeklyResetAt, "weekly_reset_at")
	requireSameInstant(t, expiresAt, response.Subscription.MonthlyResetAt, "monthly_reset_at")
}

func TestUsageUnrestrictedOneTimeDailyQuotaResetsAtExpiry(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	expiresAt := startsAt.Add(24 * time.Hour)
	dailyWindowStart := timezone.StartOfDay(startsAt)

	body := serveUsageUnrestrictedSubscription(t, usageLimitedGroup(10, 50, 100), &service.UserSubscription{
		StartsAt:         startsAt,
		ExpiresAt:        expiresAt,
		DailyWindowStart: &dailyWindowStart,
	})

	response := decodeUsageSubscription(t, body)
	requireSameInstant(t, expiresAt, response.Subscription.DailyResetAt, "daily_reset_at")
}

func TestUsageUnrestrictedReturnsNullResetTimesForInactiveOrUnlimitedWindows(t *testing.T) {
	now := time.Now()
	activeStart := now.Add(-time.Hour)
	cases := []struct {
		name         string
		group        *service.Group
		subscription *service.UserSubscription
	}{
		{
			name:  "windows not activated",
			group: usageLimitedGroup(10, 50, 100),
			subscription: &service.UserSubscription{
				StartsAt:  now.Add(-time.Hour),
				ExpiresAt: now.Add(30 * 24 * time.Hour),
			},
		},
		{
			name: "no limits configured",
			group: &service.Group{
				Name:             "Unlimited plan",
				SubscriptionType: service.SubscriptionTypeSubscription,
			},
			subscription: &service.UserSubscription{
				StartsAt:           now.Add(-time.Hour),
				ExpiresAt:          now.Add(30 * 24 * time.Hour),
				DailyWindowStart:   &activeStart,
				WeeklyWindowStart:  &activeStart,
				MonthlyWindowStart: &activeStart,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := serveUsageUnrestrictedSubscription(t, tc.group, tc.subscription)

			var response struct {
				Subscription map[string]json.RawMessage `json:"subscription"`
			}
			require.NoError(t, json.Unmarshal(body, &response))
			for _, key := range []string{"daily_reset_at", "weekly_reset_at", "monthly_reset_at"} {
				raw, ok := response.Subscription[key]
				require.True(t, ok, "missing %s", key)
				require.JSONEq(t, "null", string(raw), key)
			}
		})
	}
}
