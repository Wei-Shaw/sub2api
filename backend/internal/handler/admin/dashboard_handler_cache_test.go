package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCacheProbe struct {
	service.UsageLogRepository
	trendCalls      atomic.Int32
	usersTrendCalls atomic.Int32
	keyTrendMetrics []string
	keyTrendLimit   int
}

func (r *dashboardUsageRepoCacheProbe) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	r.trendCalls.Add(1)
	return []usagestats.TrendDataPoint{{
		Date:        "2026-03-11",
		Requests:    1,
		TotalTokens: 2,
		Cost:        3,
		ActualCost:  4,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	limit int,
	metric string,
) ([]usagestats.UserUsageTrendPoint, error) {
	r.usersTrendCalls.Add(1)
	return []usagestats.UserUsageTrendPoint{{
		Date:       "2026-03-11",
		UserID:     1,
		Email:      "cache@test.dev",
		Requests:   2,
		Tokens:     20,
		Cost:       2,
		ActualCost: 1,
	}}, nil
}

func resetDashboardReadCachesForTest() {
	dashboardTrendCache = newSnapshotCache(30 * time.Second)
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
}

func TestDashboardHandler_GetUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.trendCalls.Load())
}

func TestDashboardHandler_GetUserUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.usersTrendCalls.Load())

	for _, tc := range []struct {
		metric, cache string
		calls         int32
	}{
		{"actual_cost", "miss", 2},
		{"actual_cost", "hit", 2},
		{"tokens", "hit", 2},
	} {
		req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8&metric="+tc.metric, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
		require.Equal(t, tc.calls, repo.usersTrendCalls.Load())
	}
}

func (r *dashboardUsageRepoCacheProbe) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, metric string) ([]usagestats.APIKeyUsageTrendPoint, error) {
	r.keyTrendMetrics = append(r.keyTrendMetrics, metric)
	r.keyTrendLimit = limit
	return []usagestats.APIKeyUsageTrendPoint{{APIKeyID: 7, KeyName: "demo", Tokens: 10, ActualCost: 2.5}}, nil
}

func TestDashboardHandler_GetAPIKeyUsageTrend_MetricCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()
	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	handler := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
	router := gin.New()
	router.GET("/admin/dashboard/api-keys-trend", handler.GetAPIKeyUsageTrend)

	for _, tc := range []struct {
		metric, cache string
		calls         int
	}{
		{"", "miss", 1},
		{"tokens", "hit", 1},
		{"actual_cost", "miss", 2},
		{"actual_cost", "hit", 2},
		{"invalid", "hit", 2},
	} {
		query := ""
		if tc.metric != "" {
			query = "&metric=" + tc.metric
		}
		req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/api-keys-trend?start_date=2026-03-01&end_date=2026-03-07"+query, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
		require.Len(t, repo.keyTrendMetrics, tc.calls)
		require.Equal(t, 5, repo.keyTrendLimit)
		var body struct {
			Data struct {
				Trend []usagestats.APIKeyUsageTrendPoint `json:"trend"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Data.Trend, 1)
		require.Equal(t, 2.5, body.Data.Trend[0].ActualCost)
	}
	require.Equal(t, []string{"tokens", "actual_cost"}, repo.keyTrendMetrics)
}
