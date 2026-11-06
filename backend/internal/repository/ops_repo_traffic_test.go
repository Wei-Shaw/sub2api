package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func trafficInt64(v int64) *int64 { return &v }

func TestOpsTrafficSummaryUnknownZeroAndPairedCoverage(t *testing.T) {
	tests := []struct {
		name string
		in   opsTrafficLinkAggregate
		want *float64
	}{
		{name: "unknown"},
		{name: "measured empty", in: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(0), ActualBytes: trafficInt64(0),
			OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: 1,
			PairedOriginalBytes: trafficInt64(0), PairedActualBytes: trafficInt64(0),
		}},
		{name: "mixed independent coverage", in: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(1100), ActualBytes: trafficInt64(10050),
			OriginalSampleCount: 2, ActualSampleCount: 2, PairedSampleCount: 1,
			PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(50),
		}, want: float64TrafficPtr(50)},
		{name: "compression overhead is negative", in: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(125),
			OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: 1,
			PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(125),
		}, want: float64TrafficPtr(-25)},
		{name: "unpaired totals cannot make a ratio", in: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(50),
			OriginalSampleCount: 1, ActualSampleCount: 1,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.summary()
			require.Equal(t, tc.in.OriginalBytes, got.OriginalBytes)
			require.Equal(t, tc.in.ActualBytes, got.ActualBytes)
			require.Equal(t, tc.in.PairedSampleCount, got.PairedSampleCount)
			require.Equal(t, tc.want, got.CompressionPercent)
		})
	}
}

func float64TrafficPtr(v float64) *float64 { return &v }

func TestOpsTrafficDirectionSummaryUsesPairedSamplesOnly(t *testing.T) {
	tests := []struct {
		name string
		in   opsTrafficDirectionAggregate
		want *float64
	}{
		{name: "unknown"},
		{name: "measured empty", in: opsTrafficDirectionAggregate{
			OriginalBytes: trafficInt64(0), ActualBytes: trafficInt64(0),
			OriginalSampleCount: 1, ActualSampleCount: 1,
			PairedSampleCount: trafficInt64(1), PairedOriginalBytes: trafficInt64(0), PairedActualBytes: trafficInt64(0),
		}},
		{name: "paired samples only", in: opsTrafficDirectionAggregate{
			// The independently covered totals would claim 90% here.
			OriginalBytes: trafficInt64(1000), ActualBytes: trafficInt64(100),
			OriginalSampleCount: 2, ActualSampleCount: 2,
			PairedSampleCount: trafficInt64(1), PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(50),
		}, want: float64TrafficPtr(50)},
		{name: "encoding overhead is negative", in: opsTrafficDirectionAggregate{
			OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(125),
			OriginalSampleCount: 1, ActualSampleCount: 1,
			PairedSampleCount: trafficInt64(1), PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(125),
		}, want: float64TrafficPtr(-25)},
		{name: "no paired sample", in: opsTrafficDirectionAggregate{
			OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(50),
			OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: trafficInt64(0),
		}},
		{name: "bucket aggregated before directional pairing", in: opsTrafficDirectionAggregate{
			OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(50),
			OriginalSampleCount: 1, ActualSampleCount: 1,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.summary()
			require.Equal(t, tc.in.OriginalBytes, got.OriginalBytes)
			require.Equal(t, tc.in.ActualBytes, got.ActualBytes)
			require.Equal(t, tc.want, got.CompressionPercent)
		})
	}
}

func TestOpsTrafficDirectionMergeKeepsMixedLegacyWindowsUnknown(t *testing.T) {
	paired := &opsTrafficDirectionAggregate{
		OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(50),
		OriginalSampleCount: 1, ActualSampleCount: 1,
		PairedSampleCount: trafficInt64(1), PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(50),
	}
	legacy := &opsTrafficDirectionAggregate{
		OriginalBytes: trafficInt64(300), ActualBytes: trafficInt64(150),
		OriginalSampleCount: 1, ActualSampleCount: 1,
	}
	var merged *opsTrafficDirectionAggregate
	addTrafficDirection(&merged, paired)
	require.InDelta(t, 50, *merged.summary().CompressionPercent, 1e-9)

	addTrafficDirection(&merged, legacy)
	got := merged.summary()
	require.EqualValues(t, 400, *got.OriginalBytes)
	require.EqualValues(t, 200, *got.ActualBytes)
	require.Nil(t, got.CompressionPercent, "one legacy bucket must not become a full-window paired ratio")
	// Merging must not mutate the source buckets.
	require.Nil(t, legacy.PairedSampleCount)
	require.EqualValues(t, 100, *paired.PairedOriginalBytes)
}

func TestOpsTrafficMergePreservesNullableSumsAndPairedDenominator(t *testing.T) {
	head := opsTrafficAggregate{RequestCount: 2, Upstream: opsTrafficLinkAggregate{
		OriginalBytes: trafficInt64(100), ActualBytes: trafficInt64(50),
		OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: 1,
		PairedOriginalBytes: trafficInt64(100), PairedActualBytes: trafficInt64(50),
	}}
	stable := opsTrafficAggregate{RequestCount: 3, Upstream: opsTrafficLinkAggregate{
		OriginalBytes: trafficInt64(1000), ActualBytes: trafficInt64(700),
		OriginalSampleCount: 2, ActualSampleCount: 1, PairedSampleCount: 1,
		PairedOriginalBytes: trafficInt64(900), PairedActualBytes: trafficInt64(700),
	}}
	tail := opsTrafficAggregate{RequestCount: 1, Downstream: opsTrafficLinkAggregate{
		OriginalBytes: trafficInt64(0), ActualBytes: trafficInt64(0),
		OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: 1,
		PairedOriginalBytes: trafficInt64(0), PairedActualBytes: trafficInt64(0),
	}}
	merged := aggregateHourlyRows([]opsHourlyMetricsRow{{traffic: &stable}}).traffic
	merged.add(head)
	merged.add(tail)
	got := merged.summary()
	require.EqualValues(t, 6, got.RequestCount)
	require.Equal(t, "http_requests", got.Source)
	require.EqualValues(t, 1100, *got.Upstream.OriginalBytes)
	require.EqualValues(t, 750, *got.Upstream.ActualBytes)
	require.InDelta(t, 25, *got.Upstream.CompressionPercent, 1e-9)
	require.EqualValues(t, 3, got.Upstream.OriginalSampleCount)
	require.EqualValues(t, 2, got.Upstream.PairedSampleCount)
	require.EqualValues(t, 0, *got.Downstream.ActualBytes)
	require.Nil(t, got.Downstream.CompressionPercent)
	// Merging must not mutate pointers in the source bucket.
	require.EqualValues(t, 1000, *stable.Upstream.OriginalBytes)
	unknown := opsTrafficAggregate{RequestCount: 5}
	unknown.add(opsTrafficAggregate{RequestCount: 1})
	require.Nil(t, unknown.summary().Upstream.ActualBytes)
}

func TestOpsTrafficRawQueryFiltersAndUnknownPayloads(t *testing.T) {
	start := time.Date(2021, 1, 1, 1, 0, 0, 0, time.FixedZone("offset", 3600))
	end := start.Add(time.Hour)
	groupID := int64(42)
	for _, tc := range []struct {
		name, platform, suffix, payload string
		group                           *int64
		args                            []driver.Value
	}{
		{"empty", "", "", `{"request_count":0}`, nil, nil},
		{"unknown historical", " OpenAI ", " AND te.platform = $3", `{"request_count":3}`, nil, []driver.Value{"openai"}},
		{"group", "", " AND te.group_id = $3", `{"request_count":1,"upstream":{"actual_bytes":0,"actual_sample_count":1}}`, &groupID, []driver.Value{groupID}},
		{"both", "OpenAI", " AND te.group_id = $3 AND te.platform = $4", `{"request_count":1,"upstream":{"original_bytes":100,"actual_bytes":50,"original_sample_count":1,"actual_sample_count":1,"paired_sample_count":1,"paired_original_bytes":100,"paired_actual_bytes":50}}`, &groupID, []driver.Value{groupID, "openai"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			query := `WITH traffic_base AS (
  SELECT ` + opsTrafficExtractSQL("te") + `
  FROM ops_traffic_events te
  WHERE te.created_at >= $1 AND te.created_at < $2` + tc.suffix + `
)
SELECT ` + opsTrafficAggregateSQL() + ` FROM traffic_base`
			args := append([]driver.Value{start.UTC(), end.UTC()}, tc.args...)
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"traffic"}).AddRow(tc.payload))
			repo := &opsRepository{db: db}
			got, err := repo.queryHTTPTraffic(context.Background(), &service.OpsDashboardFilter{Platform: tc.platform, GroupID: tc.group}, start, end)
			require.NoError(t, err)
			var want opsTrafficAggregate
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &want))
			require.Equal(t, want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpsTrafficHourlyUsesIndependentLedgerAndSharedAggregate(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	// Same raw sufficient-statistic expression, independently grouped ledger and
	// full join: failed requests with neither usage nor error rows survive.
	pattern := `(?s)WITH usage_base AS .*traffic_base AS \(.*FROM ops_traffic_events te.*traffic_agg AS \(.*` +
		regexp.QuoteMeta(opsTrafficAggregateSQL()) + `.*HAVING GROUPING\(group_id\) = 1 OR group_id IS NOT NULL.*FULL OUTER JOIN traffic_agg t.*traffic = EXCLUDED.traffic`
	mock.ExpectExec(pattern).WithArgs(start, start.Add(time.Hour)).WillReturnResult(sqlmock.NewResult(0, 1))
	repo := &opsRepository{db: db}
	require.NoError(t, repo.UpsertHourlyMetrics(context.Background(), start, start.Add(time.Hour)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func trafficHourlyRows(start time.Time, payload driver.Value) *sqlmock.Rows {
	columns := make([]string, 23)
	values := make([]driver.Value, len(columns))
	for i := range columns {
		columns[i] = fmt.Sprint(i)
		values[i] = int64(0)
	}
	values[0], values[22] = start, payload
	return sqlmock.NewRows(columns).AddRow(values...)
}

func expectTrafficNumericQueries(mock sqlmock.Sqlmock, sizes ...int) {
	for _, size := range sizes {
		columns := make([]string, size)
		values := make([]driver.Value, size)
		for i := range columns {
			columns[i] = fmt.Sprint(i)
			values[i] = int64(0)
		}
		mock.ExpectQuery("SELECT|WITH").WillReturnRows(sqlmock.NewRows(columns).AddRow(values...))
	}
}

func TestOpsTrafficQueryModesLegacyPreagg(t *testing.T) {
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, mode := range []service.OpsQueryMode{service.OpsQueryModeRaw, service.OpsQueryModeAuto, service.OpsQueryModePreagg} {
		t.Run(string(mode), func(t *testing.T) {
			db, mock := newSQLMock(t)
			if mode != service.OpsQueryModeRaw {
				mock.ExpectQuery("FROM ops_metrics_hourly").WithArgs(start, start.Add(time.Hour)).WillReturnRows(trafficHourlyRows(start, nil))
			}
			if mode != service.OpsQueryModePreagg {
				expectTrafficNumericQueries(mock, 2, 13, 6, 2, 6, 2)
				mock.ExpectQuery("WITH traffic_base AS").WithArgs(start, start.Add(time.Hour)).WillReturnRows(sqlmock.NewRows([]string{"traffic"}).AddRow(`{"request_count":2}`))
				expectTrafficNumericQueries(mock, 5)
			}
			repo := &opsRepository{db: db}
			got, err := repo.GetDashboardOverview(context.Background(), &service.OpsDashboardFilter{StartTime: start, EndTime: start.Add(time.Hour), QueryMode: mode})
			if mode == service.OpsQueryModePreagg {
				require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, 2, got.Traffic.RequestCount)
				require.Nil(t, got.Traffic.Upstream.OriginalBytes)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpsTrafficPreaggUsesHourlyWithoutRawFullWindowTraffic(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(42)
	payload := `{"request_count":3,"upstream":{"original_bytes":100,"actual_bytes":125,"original_sample_count":1,"actual_sample_count":1,"paired_sample_count":1,"paired_original_bytes":100,"paired_actual_bytes":125}}`
	mock.ExpectQuery(`FROM ops_metrics_hourly WHERE bucket_start >= \$1 AND bucket_start < \$2 AND group_id = \$3 AND platform = \$4`).
		WithArgs(start, start.Add(time.Hour), groupID, "openai").WillReturnRows(trafficHourlyRows(start, payload))
	// Only existing realtime/peak/output-rate queries remain; no raw traffic scan.
	expectTrafficNumericQueries(mock, 2, 6, 2, 5)
	repo := &opsRepository{db: db}
	got, err := repo.GetDashboardOverview(context.Background(), &service.OpsDashboardFilter{
		StartTime: start, EndTime: start.Add(time.Hour), QueryMode: service.OpsQueryModePreagg,
		Platform: " OpenAI ", GroupID: &groupID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, got.Traffic.RequestCount)
	require.Equal(t, -25.0, *got.Traffic.Upstream.CompressionPercent)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficMissingStableBucketDoesNotLoseTrafficOnlyRequests(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM ops_metrics_hourly").WithArgs(start, start.Add(2*time.Hour)).WillReturnRows(trafficHourlyRows(start, `{"request_count":1}`))
	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM ops_traffic_events te").WithArgs(start.Add(time.Hour), start.Add(2*time.Hour)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	repo := &opsRepository{db: db}
	got, err := repo.GetDashboardOverview(context.Background(), &service.OpsDashboardFilter{StartTime: start, EndTime: start.Add(2 * time.Hour), QueryMode: service.OpsQueryModePreagg})
	require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficSparseGapsAreBatchedWithFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(42)
	gaps := []opsTimeRange{{start: start, end: start.Add(time.Hour)}, {start: start.Add(2 * time.Hour), end: start.Add(3 * time.Hour)}}
	query := `SELECT EXISTS(SELECT 1 FROM ops_traffic_events te WHERE te.created_at >= $1 AND te.created_at < $2 AND te.group_id = $3 AND te.platform = $4 AND ((te.created_at >= $5 AND te.created_at < $6) OR (te.created_at >= $7 AND te.created_at < $8)) LIMIT 1)`
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(start, gaps[1].end, groupID, "openai", gaps[0].start, gaps[0].end, gaps[1].start, gaps[1].end).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	repo := &opsRepository{db: db}
	exists, err := repo.rawOpsDataExists(context.Background(), &service.OpsDashboardFilter{Platform: " OpenAI ", GroupID: &groupID}, start, gaps[1].end, gaps...)
	require.NoError(t, err)
	require.True(t, exists)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficQueryErrorIsNotReportedAsZero(t *testing.T) {
	db, mock := newSQLMock(t)
	wantErr := errors.New("ledger unavailable")
	mock.ExpectQuery("WITH traffic_base AS").WillReturnError(wantErr)
	repo := &opsRepository{db: db}
	_, err := repo.queryHTTPTraffic(context.Background(), nil, time.Now().Add(-time.Hour), time.Now())
	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
