package admin

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMinuteRangeSnapshotLocalMetadataCache(t *testing.T) {
	resetDashboardReadCachesForTest()
	t.Cleanup(resetDashboardReadCachesForTest)
	h := &DashboardHandler{}
	for _, tc := range []struct{ start, end, date string }{
		{"2026-09-09T23:30:00Z", "2026-09-10T00:30:00Z", "2026-09-10"},
		{"2026-09-09T19:30:00-04:00", "2026-09-09T20:30:00-04:00", "2026-09-09"},
	} {
		for i := 0; i < 2; i++ {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/?start_date="+url.QueryEscape(tc.start)+"&end_date="+url.QueryEscape(tc.end)+"&include_stats=false&include_trend=false&include_model_stats=false", nil)
			h.GetSnapshotV2(c)
			require.Equal(t, 200, rec.Code)
			var body struct{ Data map[string]any }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.start, body.Data["start_time"])
			require.Equal(t, tc.end, body.Data["end_time"])
			require.Equal(t, tc.date, body.Data["end_date"])
		}
	}
}

func TestMinuteRangeDashboardMetadataAndPrecision(t *testing.T) {
	resetDashboardReadCachesForTest()
	t.Cleanup(resetDashboardReadCachesForTest)
	repo := &dashboardUsageRepoCacheProbe{}
	h := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
	router := gin.New()
	router.GET("/trend", h.GetUsageTrend)
	router.GET("/snapshot", h.GetSnapshotV2)
	for _, route := range []string{"/trend", "/snapshot"} {
		for _, fraction := range []string{"123456789", "123456791", "123456789"} {
			start := "2026-09-10T06:30:00." + fraction + "Z"
			end := "2026-09-10T06:31:00." + fraction + "Z"
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", route+"?start_date="+start+"&end_date="+end+"&include_stats=false&include_model_stats=false", nil))
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var body struct{ Data map[string]any }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, start, body.Data["start_time"])
			require.Equal(t, end, body.Data["end_time"])
			require.Equal(t, "2026-09-10", body.Data["end_date"])
		}
	}
	require.EqualValues(t, 2, repo.trendCalls.Load(), "inner cache must distinguish nanoseconds and share identical ranges")
}

func TestMinuteRangeAdminValidation(t *testing.T) {
	h := &DashboardHandler{}
	routes := []gin.HandlerFunc{h.GetUsageTrend, h.GetModelStats, h.GetGroupStats, h.GetAPIKeyUsageTrend, h.GetUserUsageTrend, h.GetUserSpendingRanking, h.GetUserBreakdown, h.GetSnapshotV2}
	for _, route := range routes {
		for _, query := range []string{"start_date=bad", "end_date=bad", "start_date=2026-09-10&end_date=2026-09-08", "start_date=2026-09-10T00:00:00Z&end_date=2026-09-10T00:00:00Z"} {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/?"+query, nil)
			route(c)
			require.Equal(t, 400, rec.Code, query)
		}
	}
}

func TestMinuteRangeAdminStatsMetadata(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/usage/stats?nocache=true&start_date=2026-09-10T01:02:03.123456789Z&end_date=2026-09-10T01:03:03.123456789Z", nil))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var body struct{ Data map[string]any }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, repo.statsFilters.StartTime.Format(time.RFC3339Nano), body.Data["start_time"])
	require.Equal(t, repo.statsFilters.EndTime.Format(time.RFC3339Nano), body.Data["end_time"])
	for _, query := range []string{"start_date=bad", "end_date=2026-09-10", "start_date=2026-09-10&end_date=2026-09-08"} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/usage/stats?"+query, nil))
		require.Equal(t, 400, rec.Code)
	}
}
