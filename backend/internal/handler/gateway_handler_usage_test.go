package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type apiKeyTodayUsageRepo struct {
	service.UsageLogRepository
	todayStart time.Time
}

func (r *apiKeyTodayUsageRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64, todayStart time.Time) (*usagestats.UserDashboardStats, error) {
	r.todayStart = todayStart
	return &usagestats.UserDashboardStats{}, nil
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

func TestParseUsageDateRangeUsesRequestedTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage?start_date=2026-03-08&end_date=2026-03-08&timezone=America%2FNew_York", nil)

	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	startTime, endTime := (&GatewayHandler{}).parseUsageDateRange(c)
	require.Equal(t, time.Date(2026, time.March, 8, 0, 0, 0, 0, location), startTime)
	require.Equal(t, time.Date(2026, time.March, 9, 0, 0, 0, 0, location), endTime)
	require.Equal(t, 23*time.Hour, endTime.Sub(startTime))
}

func TestParseUsageDateRangeUsesFirstValidInstantAfterMidnightGap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage?start_date=2026-09-06&end_date=2026-09-06&timezone=America%2FSantiago", nil)

	startTime, endTime := (&GatewayHandler{}).parseUsageDateRange(c)
	require.Equal(t, "2026-09-06 01:00 -03:00", startTime.Format("2006-01-02 15:04 -07:00"))
	require.Equal(t, "2026-09-07 00:00 -03:00", endTime.Format("2006-01-02 15:04 -07:00"))
}

func TestAPIKeyTodayStartUsesRequestedTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage?timezone=America%2FSantiago", nil)

	location, err := time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	now := time.Date(2026, time.September, 6, 12, 30, 0, 0, location)

	start := apiKeyTodayStart(c, now)
	require.Equal(t, "2026-09-06 01:00 -03:00", start.Format("2006-01-02 15:04 -07:00"))

	repo := &apiKeyTodayUsageRepo{}
	handler := &GatewayHandler{usageService: service.NewUsageService(repo, nil, nil, nil)}
	require.NotNil(t, handler.buildUsageData(context.Background(), 7, start))
	require.Equal(t, start, repo.todayStart)
}
