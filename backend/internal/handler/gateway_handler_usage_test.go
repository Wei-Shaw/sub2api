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

type gatewayUsageModelStatsRepo struct {
	service.UsageLogRepository
	modelStats []usagestats.ModelStat
}

func (r *gatewayUsageModelStatsRepo) GetAPIKeyDashboardStats(context.Context, int64) (*usagestats.UserDashboardStats, error) {
	return &usagestats.UserDashboardStats{}, nil
}

func (r *gatewayUsageModelStatsRepo) GetUsageTrendWithFilters(context.Context, time.Time, time.Time, string, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.TrendDataPoint, error) {
	return []usagestats.TrendDataPoint{}, nil
}

func (r *gatewayUsageModelStatsRepo) GetModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	return r.modelStats, nil
}

func TestUsageModelStatsHideAccountCost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: 42, Status: service.StatusAPIKeyActive, Quota: 10})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	repo := &gatewayUsageModelStatsRepo{modelStats: []usagestats.ModelStat{{
		Model:       "claude-opus-5-5",
		Requests:    3,
		TotalTokens: 300,
		Cost:        1.5,
		ActualCost:  1.2,
		AccountCost: 0.4,
	}}}
	handler := &GatewayHandler{usageService: service.NewUsageService(repo, nil, nil, nil)}
	handler.Usage(c) // quota-limited key: no user lookup needed

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.NotContains(t, body, "account_cost")
	var response struct {
		ModelStats []map[string]any `json:"model_stats"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.ModelStats, 1)
	require.Equal(t, "claude-opus-5-5", response.ModelStats[0]["model"])
	require.InDelta(t, 1.2, response.ModelStats[0]["actual_cost"], 1e-9)
}
