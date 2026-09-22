package admin

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMinuteRangeAllInnerCacheKeys(t *testing.T) {
	resetDashboardReadCachesForTest()
	t.Cleanup(resetDashboardReadCachesForTest)
	h := &DashboardHandler{}
	ctx := context.Background()
	for _, fraction := range []string{"123456789", "123456791"} {
		startText := "2026-09-10T00:30:00." + fraction + "Z"
		endText := "2026-09-10T00:31:00." + fraction + "Z"
		start, err := time.Parse(time.RFC3339Nano, startText)
		require.NoError(t, err)
		end, err := time.Parse(time.RFC3339Nano, endText)
		require.NoError(t, err)
		// Seed the nanosecond key; a truncated key would miss and invoke the nil service.
		trendKey := mustMarshalDashboardCacheKey(dashboardTrendCacheKey{StartTime: startText, EndTime: endText, Granularity: "hour"})
		dashboardTrendCache.Set(trendKey, []usagestats.TrendDataPoint{})
		_, hit, err := h.getUsageTrendCached(ctx, start, end, "hour", 0, 0, 0, 0, "", nil, nil, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, hit)

		modelKey := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{StartTime: startText, EndTime: endText, ModelSource: usagestats.ModelSourceRequested})
		dashboardModelStatsCache.Set(modelKey, []usagestats.ModelStat{})
		_, hit, err = h.getModelStatsCached(ctx, start, end, 0, 0, 0, 0, usagestats.ModelSourceRequested, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, hit)

		groupKey := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{StartTime: startText, EndTime: endText})
		dashboardGroupStatsCache.Set(groupKey, []usagestats.GroupStat{})
		_, hit, err = h.getGroupStatsCached(ctx, start, end, 0, 0, 0, 0, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, hit)

		entityKey := mustMarshalDashboardCacheKey(dashboardEntityTrendCacheKey{StartTime: startText, EndTime: endText, Granularity: "hour", Limit: 12})
		dashboardAPIKeysTrendCache.Set(entityKey, []usagestats.APIKeyUsageTrendPoint{})
		_, hit, err = h.getAPIKeyUsageTrendCached(ctx, start, end, "hour", 12)
		require.NoError(t, err)
		require.True(t, hit)
		dashboardUsersTrendCache.Set(entityKey, []usagestats.UserUsageTrendPoint{})
		_, hit, err = h.getUserUsageTrendCached(ctx, start, end, "hour", 12)
		require.NoError(t, err)
		require.True(t, hit)
		for _, route := range []gin.HandlerFunc{h.GetModelStats, h.GetGroupStats, h.GetAPIKeyUsageTrend, h.GetUserUsageTrend} {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/?start_date="+startText+"&end_date="+endText+"&granularity=hour&limit=12", nil)
			route(c)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var body struct{ Data map[string]any }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, startText, body.Data["start_time"])
			require.Equal(t, endText, body.Data["end_time"])
			require.Equal(t, "2026-09-10", body.Data["end_date"])
		}
	}
}

func TestMinuteRangeUsageStatsCachePrecision(t *testing.T) {
	start := time.Date(2026, 9, 10, 1, 2, 3, 123456789, time.UTC)
	end := start.Add(time.Minute)
	filters := usagestats.UsageLogFilters{StartTime: &start, EndTime: &end}
	key := usageStatsCacheKey(filters)
	start = start.Add(time.Nanosecond)
	require.NotEqual(t, key, usageStatsCacheKey(filters))
	start = start.Add(-time.Nanosecond)
	end = end.Add(time.Nanosecond)
	require.NotEqual(t, key, usageStatsCacheKey(filters))
}
