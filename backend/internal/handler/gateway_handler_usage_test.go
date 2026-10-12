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

func usageRangeContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, recorder
}

func TestParseUsageDateRangeAcceptsRFC3339FromTo(t *testing.T) {
	c, _ := usageRangeContext("/v1/usage?from=2026-10-12T12:30:00Z&to=2026-10-12T13:30:00Z")

	start, end, ok := (&GatewayHandler{}).parseUsageDateRange(c)

	require.True(t, ok)
	require.True(t, start.Equal(time.Date(2026, time.October, 12, 12, 30, 0, 0, time.UTC)))
	require.True(t, end.Equal(time.Date(2026, time.October, 12, 13, 30, 0, 0, time.UTC)))
}

func TestParseUsageDateRangeFromToOverrideDates(t *testing.T) {
	c, _ := usageRangeContext("/v1/usage?start_date=2026-10-01&end_date=2026-10-31&from=2026-10-12T15:30:00%2B03:00&to=2026-10-12T13:30:00Z")

	start, end, ok := (&GatewayHandler{}).parseUsageDateRange(c)

	require.True(t, ok)
	require.True(t, start.Equal(time.Date(2026, time.October, 12, 12, 30, 0, 0, time.UTC)))
	require.True(t, end.Equal(time.Date(2026, time.October, 12, 13, 30, 0, 0, time.UTC)))
}

func TestParseUsageDateRangeRejectsBadFromTo(t *testing.T) {
	for target, message := range map[string]string{
		"/v1/usage?from=2026-10-12":                                   "Invalid from, expect RFC3339",
		"/v1/usage?from=2026-10-12T12:30:00Z&to=12:30":                "Invalid to, expect RFC3339",
		"/v1/usage?from=2026-10-12T13:30:00Z&to=2026-10-12T12:30:00Z": "Invalid range, to must be after from",
		"/v1/usage?from=2026-10-12T12:30:00Z&to=2026-10-12T12:30:00Z": "Invalid range, to must be after from",
	} {
		c, recorder := usageRangeContext(target)

		_, _, ok := (&GatewayHandler{}).parseUsageDateRange(c)

		require.False(t, ok, target)
		require.Equal(t, http.StatusBadRequest, recorder.Code, target)
		require.Contains(t, recorder.Body.String(), message, target)
	}
}

func TestUsageRejectsInvalidFromWithBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage?from=yesterday", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})

	(&GatewayHandler{}).Usage(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Invalid from, expect RFC3339")
}

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
