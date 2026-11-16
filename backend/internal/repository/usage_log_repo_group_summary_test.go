//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetAllGroupUsageSummaryUsesRollupTail(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)

	// 水位有效：closed_before = 2026-03-07（≤ today），时区与配置一致。
	mock.ExpectQuery(`(?s)COUNT\(\*\).*usage_group_rollup_state.*WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
			AddRow(1, "2026-03-07", time.Date(2026, 2, 1, 5, 0, 0, 0, time.UTC), "America/New_York", time.Unix(0, 0).UTC()))

	// 尾段起点必须以参数形式进 WHERE —— 这正是把 usage_logs 全表扫换回
	// idx_usage_logs_created_at 索引扫的那一步。
	mock.ExpectQuery(`(?s)usage_group_daily_rollups.*FROM usage_logs ul\s+WHERE ul\.created_at >= \$7`).
		WithArgs(
			todayStart,
			yesterdayStart,
			"2026-03-08", // yesterdayDate
			true,         // 水位有效
			"2026-02-01", // retained_from 在配置时区内的日期
			"2026-03-07", // closed_before
			time.Date(2026, 3, 7, 5, 0, 0, 0, time.UTC), // tail_start = closed_before 在配置时区的零点
		).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}).
			AddRow(int64(7), 12.5, 1.25, 2.5))

	result, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
	require.NoError(t, err)
	require.Equal(t, int64(7), result[0].GroupID)
	require.InDelta(t, 12.5, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 1.25, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 2.5, result[0].YesterdayCost, 0.0000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 水位无效（时区变了 / 水位在未来 / 行缺失）时必须退化成"从 epoch 起全量重算"，
// 与改动前 SQL 里 CASE WHEN valid 的 ELSE 分支逐字对应：
// 历史日桶整段不参与，尾段起点回到 1970-01-01。
func TestUsageLogRepositoryGetAllGroupUsageSummaryFallsBackWhenWatermarkInvalid(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		rows *sqlmock.Rows
	}{
		{
			name: "水位行缺失",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(0, nil, nil, nil, nil),
		},
		{
			name: "时区与当前配置不一致",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(1, "2026-03-07", time.Date(2026, 2, 1, 5, 0, 0, 0, time.UTC), "Asia/Shanghai", time.Unix(0, 0).UTC()),
		},
		{
			name: "水位位于未来",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(1, "2026-03-10", time.Date(2026, 2, 1, 5, 0, 0, 0, time.UTC), "America/New_York", time.Unix(0, 0).UTC()),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			useGroupUsageRepositoryTestTimezone(t, "America/New_York")

			mock.ExpectQuery(`(?s)COUNT\(\*\).*usage_group_rollup_state.*WHERE id = 1`).
				WillReturnRows(tc.rows)
			mock.ExpectQuery(`(?s)usage_group_daily_rollups.*FROM usage_logs ul\s+WHERE ul\.created_at >= \$7`).
				WithArgs(todayStart, yesterdayStart, "2026-03-08", false, "1970-01-01", "1970-01-01", epoch).
				WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}))

			_, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// 尾段不早于保留下界：水位落后于保留下界时（例如清理已推进下界而重建尚未完成），
// 尚未删除的过期记录也不计入。
func TestUsageLogRepositoryGetAllGroupUsageSummaryTailStartsAtExpiredBefore(t *testing.T) {
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)
	expiredBefore := time.Date(2026, 2, 10, 5, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		valid     bool
		retained  string
		closed    string
		tailStart time.Time
	}{
		{
			name: "水位有效但早于保留下界",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(1, "2026-02-01", expiredBefore, "America/New_York", expiredBefore),
			valid:     true,
			retained:  "2026-02-10",
			closed:    "2026-02-01",
			tailStart: expiredBefore,
		},
		{
			name: "retained_from 早于保留下界",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(1, "2026-03-07", time.Date(2026, 1, 20, 5, 0, 0, 0, time.UTC), "America/New_York", expiredBefore),
			valid:     true,
			retained:  "2026-02-10",
			closed:    "2026-03-07",
			tailStart: time.Date(2026, 3, 7, 5, 0, 0, 0, time.UTC),
		},
		{
			name: "水位无效",
			rows: sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name", "expired_before"}).
				AddRow(1, "2026-03-07", expiredBefore, "Asia/Shanghai", expiredBefore),
			valid:     false,
			retained:  "1970-01-01",
			closed:    "1970-01-01",
			tailStart: expiredBefore,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			useGroupUsageRepositoryTestTimezone(t, "America/New_York")

			mock.ExpectQuery(`(?s)COUNT\(\*\).*MAX\(expired_before\).*usage_group_rollup_state.*WHERE id = 1`).
				WillReturnRows(tc.rows)
			mock.ExpectQuery(`(?s)usage_group_daily_rollups.*FROM usage_logs ul\s+WHERE ul\.created_at >= \$7`).
				WithArgs(todayStart, yesterdayStart, "2026-03-08", tc.valid, tc.retained, tc.closed, tc.tailStart).
				WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}))

			_, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
