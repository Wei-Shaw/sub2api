//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Real PostgreSQL evaluates the shared JSONB/SUM/FILTER expressions rather than
// returning mock totals. No usage/error rows are created: traffic-only failures
// must survive the hourly FULL JOIN and use the ledger population independently.
func TestOpsTrafficLedgerRawPreaggParity(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2019, 3, 17, 0, 30, 0, 0, time.UTC)
	end := start.Add(4 * time.Hour)
	groupID, otherGroupID := int64(987654321), int64(987654322)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_traffic_events WHERE created_at >= $1 AND created_at < $2", start.Add(-time.Hour), end.Add(time.Hour))
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_metrics_hourly WHERE bucket_start >= $1 AND bucket_start < $2", start.Truncate(time.Hour), end.Add(time.Hour))
	})
	insert := func(at time.Time, platform any, group any, payload string) {
		t.Helper()
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO ops_traffic_events (event_id, request_id, platform, group_id, created_at, traffic)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, uuid.NewString(), "traffic-test", platform, group, at, payload)
		require.NoError(t, err)
	}
	insert(start, "openai", groupID, `{"upstream":{"request":{"original_bytes":100,"actual_bytes":50,"compressed":true}},"downstream":{"request":{"original_bytes":0,"actual_bytes":0},"response":{"original_bytes":200,"actual_bytes":210,"compressed":true}}}`)
	insert(start.Add(time.Hour), "openai", groupID, `{"upstream":{"request":{"original_bytes":1000,"actual_bytes":null},"response":{"original_bytes":null,"actual_bytes":700}}}`)
	insert(start.Add(2*time.Hour), "openai", groupID, `{"upstream":{"response":{"original_bytes":0,"actual_bytes":25,"compressed":true}}}`)
	insert(start.Add(2*time.Hour+time.Minute), "openai", groupID, `{}`)
	insert(end.Add(-time.Minute), "openai", groupID, `{"upstream":{"request":{"original_bytes":300,"actual_bytes":100,"compressed":true}}}`)
	// Ungrouped / unresolved routing and other dimensions are independent.
	insert(start.Add(time.Hour), "anthropic", nil, `{"upstream":{"request":{"original_bytes":20,"actual_bytes":10}}}`)
	insert(start.Add(2*time.Hour), nil, nil, `{}`)
	insert(start.Add(time.Hour), "openai", otherGroupID, `{"downstream":{"request":{"original_bytes":0,"actual_bytes":0}}}`)
	// Half-open time bounds must remain consistent in raw head/tail and preagg.
	insert(start.Add(-time.Second), "openai", groupID, `{"upstream":{"request":{"original_bytes":9999,"actual_bytes":9999}}}`)
	insert(end, "openai", groupID, `{"upstream":{"request":{"original_bytes":9999,"actual_bytes":9999}}}`)

	repo, ok := NewOpsRepository(integrationDB).(*opsRepository)
	require.True(t, ok)
	filter := &service.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: " OpenAI ", GroupID: &groupID, QueryMode: service.OpsQueryModeRaw}
	raw, err := repo.GetDashboardOverview(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 0, raw.SuccessCount)
	require.EqualValues(t, 0, raw.ErrorCountTotal)
	require.EqualValues(t, 5, raw.Traffic.RequestCount)
	require.Equal(t, "http_requests", raw.Traffic.Source)
	require.EqualValues(t, 1400, *raw.Traffic.Upstream.OriginalBytes)
	require.EqualValues(t, 875, *raw.Traffic.Upstream.ActualBytes)
	require.EqualValues(t, 4, raw.Traffic.Upstream.OriginalSampleCount)
	require.EqualValues(t, 4, raw.Traffic.Upstream.ActualSampleCount)
	require.EqualValues(t, 3, raw.Traffic.Upstream.PairedSampleCount)
	require.InDelta(t, 56.25, *raw.Traffic.Upstream.CompressionPercent, 1e-9)
	require.InDelta(t, -5, *raw.Traffic.Downstream.CompressionPercent, 1e-9)
	require.Equal(t, &service.OpsTrafficDirection{
		OriginalBytes: trafficInt64(1400), ActualBytes: trafficInt64(150),
		OriginalSampleCount: 3, ActualSampleCount: 2,
		CompressionPercent: float64TrafficPtr(62.5),
	}, raw.Traffic.Upstream.Request)
	require.Equal(t, &service.OpsTrafficDirection{
		OriginalBytes: trafficInt64(0), ActualBytes: trafficInt64(725),
		OriginalSampleCount: 1, ActualSampleCount: 2,
	}, raw.Traffic.Upstream.Response)
	require.Equal(t, &service.OpsTrafficDirection{
		OriginalBytes: trafficInt64(0), ActualBytes: trafficInt64(0),
		OriginalSampleCount: 1, ActualSampleCount: 1,
	}, raw.Traffic.Downstream.Request)
	downstreamResponse := raw.Traffic.Downstream.Response
	require.NotNil(t, downstreamResponse)
	require.Equal(t, trafficInt64(200), downstreamResponse.OriginalBytes)
	require.Equal(t, trafficInt64(210), downstreamResponse.ActualBytes)
	require.EqualValues(t, 1, downstreamResponse.OriginalSampleCount)
	require.EqualValues(t, 1, downstreamResponse.ActualSampleCount)
	require.NotNil(t, downstreamResponse.CompressionPercent)
	require.InDelta(t, -5, *downstreamResponse.CompressionPercent, 1e-9)

	// Auto falls back when only traffic ledger data exists in missing buckets.
	filter.QueryMode = service.OpsQueryModeAuto
	auto, err := repo.GetDashboardOverview(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, raw.Traffic, auto.Traffic)
	filter.QueryMode = service.OpsQueryModePreagg
	_, err = repo.GetDashboardOverview(ctx, filter)
	require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)

	require.NoError(t, repo.UpsertHourlyMetrics(ctx, start.Truncate(time.Hour), end.Add(time.Hour).Truncate(time.Hour)))
	// Re-running must be idempotent, not double-count traffic-only dimensions.
	require.NoError(t, repo.UpsertHourlyMetrics(ctx, start.Truncate(time.Hour), end.Add(time.Hour).Truncate(time.Hour)))
	for _, scope := range []struct {
		name     string
		platform string
		group    *int64
		count    int64
	}{
		{"both dimensions", "openai", &groupID, 5},
		{"group", "", &groupID, 5},
		{"platform", "openai", nil, 6},
		{"ungrouped platform", "anthropic", nil, 1},
		{"unresolved platform", "unknown", nil, 1},
		{"global", "", nil, 8},
		{"empty platform", "gemini", nil, 0},
	} {
		t.Run(scope.name, func(t *testing.T) {
			f := &service.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: scope.platform, GroupID: scope.group, QueryMode: service.OpsQueryModeRaw}
			want, err := repo.GetDashboardOverview(ctx, f)
			require.NoError(t, err)
			require.Equal(t, scope.count, want.Traffic.RequestCount)
			for _, mode := range []service.OpsQueryMode{service.OpsQueryModePreagg, service.OpsQueryModeAuto} {
				f.QueryMode = mode
				got, err := repo.GetDashboardOverview(ctx, f)
				require.NoError(t, err)
				require.Equal(t, want.Traffic, got.Traffic)
			}
		})
	}

	// Previously persisted hourly totals have no directional fields. Keep their
	// valid totals and paired compression rate, but never present a split from
	// only the new raw head/tail as if it covered the entire selected window.
	_, err = integrationDB.ExecContext(ctx, `UPDATE ops_metrics_hourly SET traffic =
traffic #- '{upstream,request}' #- '{upstream,response}' #- '{downstream,request}' #- '{downstream,response}'
WHERE bucket_start = $1 AND group_id = $2`, start.Truncate(time.Hour).Add(time.Hour), groupID)
	require.NoError(t, err)
	legacyExpected := *raw.Traffic
	legacyExpected.Upstream.Request, legacyExpected.Upstream.Response = nil, nil
	legacyExpected.Downstream.Request, legacyExpected.Downstream.Response = nil, nil
	for _, mode := range []service.OpsQueryMode{service.OpsQueryModePreagg, service.OpsQueryModeAuto} {
		filter.QueryMode = mode
		legacy, err := repo.GetDashboardOverview(ctx, filter)
		require.NoError(t, err)
		require.Equal(t, &legacyExpected, legacy.Traffic)
	}
	// The normal hourly refresh supplies the new split without a DDL migration.
	require.NoError(t, repo.UpsertHourlyMetrics(ctx, start.Truncate(time.Hour), end.Add(time.Hour).Truncate(time.Hour)))
	filter.QueryMode = service.OpsQueryModePreagg
	refreshed, err := repo.GetDashboardOverview(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, raw.Traffic, refreshed.Traffic)

	// A historical preaggregation row without traffic is not a zero sample.
	_, err = integrationDB.ExecContext(ctx, `UPDATE ops_metrics_hourly SET traffic = NULL WHERE bucket_start = $1 AND group_id = $2`, start.Truncate(time.Hour).Add(time.Hour), groupID)
	require.NoError(t, err)
	filter.QueryMode = service.OpsQueryModePreagg
	_, err = repo.GetDashboardOverview(ctx, filter)
	require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)
	filter.QueryMode = service.OpsQueryModeAuto
	auto, err = repo.GetDashboardOverview(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, raw.Traffic, auto.Traffic)

	// Stored aggregate byte keys preserve JSON null vs measured zero.
	var stored []byte
	err = integrationDB.QueryRowContext(ctx, `SELECT traffic FROM ops_metrics_hourly WHERE bucket_start = $1 AND group_id = $2`, start.Truncate(time.Hour).Add(time.Hour), otherGroupID).Scan(&stored)
	require.NoError(t, err)
	var aggregate opsTrafficAggregate
	require.NoError(t, json.Unmarshal(stored, &aggregate))
	require.Nil(t, aggregate.Upstream.OriginalBytes)
	require.EqualValues(t, 0, *aggregate.Downstream.OriginalBytes)
	require.Nil(t, aggregate.summary().Downstream.CompressionPercent)
}
