//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsSLAMetricsExcludeClientFailures(t *testing.T) {
	ctx := context.Background()
	group := mustCreateGroup(t, testEntClient(t), &service.Group{Name: "sla-client-errors", Platform: service.PlatformOpenAI})
	start := time.Date(2020, 4, 5, 0, 30, 0, 0, time.UTC)
	end := start.Add(3 * time.Hour)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_error_logs WHERE group_id = $1", group.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_metrics_hourly WHERE bucket_start >= $1 AND bucket_start < $2", start.Truncate(time.Hour), end)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID)
	})

	samples := []struct {
		status   int
		owner    string
		business bool
	}{
		{499, "client", false},
		{499, "platform", false}, // Disconnects are excluded even without client ownership.
		{400, "client", false},
		{502, "client", false},
		{502, "provider", false},
		{500, "platform", false},
		{429, "provider", false},
		{429, "client", true},
		{503, "", false}, // Legacy rows without an owner still count as SLA errors.
	}
	repo, ok := NewOpsRepository(integrationDB).(*opsRepository)
	require.True(t, ok)
	var entries []*service.OpsInsertErrorLogInput
	// Exercise a partial-hour raw head, a complete aggregated hour, and a raw tail.
	for _, offset := range []time.Duration{5 * time.Minute, 90 * time.Minute, 175 * time.Minute} {
		for _, sample := range samples {
			entries = append(entries, &service.OpsInsertErrorLogInput{
				RequestID:         fmt.Sprintf("sla-%d-%d", group.ID, len(entries)),
				GroupID:           &group.ID,
				Platform:          service.PlatformOpenAI,
				ErrorPhase:        "upstream",
				ErrorType:         "api_error",
				Severity:          "P2",
				ErrorOwner:        sample.owner,
				StatusCode:        sample.status,
				IsBusinessLimited: sample.business,
				CreatedAt:         start.Add(offset),
			})
		}
		entries = append(entries, &service.OpsInsertErrorLogInput{
			RequestID: fmt.Sprintf("sla-%d-probe-%d", group.ID, len(entries)),
			GroupID:   &group.ID, Platform: service.PlatformOpenAI,
			ErrorPhase: "upstream", ErrorType: "api_error", ErrorOwner: "platform", Severity: "P2",
			StatusCode: 500, IsCountTokens: true, CreatedAt: start.Add(offset),
		})
	}
	inserted, err := repo.BatchInsertErrorLogs(ctx, entries)
	require.NoError(t, err)
	require.EqualValues(t, len(entries), inserted)
	filter := &service.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: service.PlatformOpenAI, GroupID: &group.ID}

	assertOverview := func(t *testing.T, mode service.OpsQueryMode) {
		t.Helper()
		filter.QueryMode = mode
		overview, err := repo.GetDashboardOverview(ctx, filter)
		require.NoError(t, err)
		require.EqualValues(t, 27, overview.ErrorCountTotal)
		require.EqualValues(t, 3, overview.BusinessLimitedCount)
		require.EqualValues(t, 12, overview.ErrorCountSLA)
		require.EqualValues(t, 27, overview.RequestCountTotal)
		require.EqualValues(t, 12, overview.RequestCountSLA)
		require.EqualValues(t, 3, overview.UpstreamErrorCountExcl429529)
		require.EqualValues(t, 3, overview.Upstream429Count)
	}
	t.Run("raw", func(t *testing.T) { assertOverview(t, service.OpsQueryModeRaw) })
	t.Run("auto_without_rollup", func(t *testing.T) { assertOverview(t, service.OpsQueryModeAuto) })
	require.NoError(t, repo.UpsertHourlyMetrics(ctx, start.Truncate(time.Hour), end))
	t.Run("preaggregated", func(t *testing.T) { assertOverview(t, service.OpsQueryModePreagg) })
	t.Run("auto_with_rollup", func(t *testing.T) { assertOverview(t, service.OpsQueryModeAuto) })

	t.Run("trend", func(t *testing.T) {
		trend, err := repo.GetErrorTrend(ctx, filter, 3600)
		require.NoError(t, err)
		var totals service.OpsErrorTrendPoint
		for _, point := range trend.Points {
			totals.ErrorCountTotal += point.ErrorCountTotal
			totals.BusinessLimitedCount += point.BusinessLimitedCount
			totals.ErrorCountSLA += point.ErrorCountSLA
			totals.UpstreamErrorCountExcl429529 += point.UpstreamErrorCountExcl429529
			totals.Upstream429Count += point.Upstream429Count
		}
		require.EqualValues(t, 27, totals.ErrorCountTotal)
		require.EqualValues(t, 3, totals.BusinessLimitedCount)
		require.EqualValues(t, 12, totals.ErrorCountSLA)
		require.EqualValues(t, 3, totals.UpstreamErrorCountExcl429529)
		require.EqualValues(t, 3, totals.Upstream429Count)
	})
	t.Run("distribution", func(t *testing.T) {
		distribution, err := repo.GetErrorDistribution(ctx, filter)
		require.NoError(t, err)
		require.EqualValues(t, 27, distribution.Total)
		byStatus := make(map[int]*service.OpsErrorDistributionItem)
		for _, item := range distribution.Items {
			byStatus[item.StatusCode] = item
		}
		for status, expected := range map[int][3]int64{
			499: {6, 0, 0}, 400: {3, 0, 0}, 502: {6, 3, 0},
			500: {3, 3, 0}, 429: {6, 3, 3}, 503: {3, 3, 0},
		} {
			require.Contains(t, byStatus, status)
			require.Equal(t, expected[0], byStatus[status].Total)
			require.Equal(t, expected[1], byStatus[status].SLA)
			require.Equal(t, expected[2], byStatus[status].BusinessLimited)
		}
	})
}
