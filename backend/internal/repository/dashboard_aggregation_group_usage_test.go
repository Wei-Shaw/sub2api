//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDashboardAggregationRepositorySyncGroupUsageRollupsNoopsAtCurrentDate(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("2026-08-14", time.Unix(0, 0).UTC(), "Asia/Shanghai"))
	mock.ExpectCommit()

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsRebuildsWhenTimezoneChanges(t *testing.T) {
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")

	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	// 只差一天，单块即可追平；多块分批另见 SplitsRebuildIntoDailyTransactions。
	todayStart := time.Date(2026, 3, 2, 5, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("2026-03-02", time.Unix(0, 0).UTC(), "Asia/Shanghai"))
	mock.ExpectQuery(`SELECT MIN\(created_at\) FROM usage_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(retainedFrom))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs("2026-03-01", "2026-03-02").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
		WithArgs(retainedFrom, todayStart, "America/New_York").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs("2026-03-02").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs("2026-03-02", retainedFrom, "America/New_York").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsPublishesWatermarkLast(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)
	rebuildStart := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("2026-08-13", time.Unix(0, 0).UTC(), "Asia/Shanghai"))
	mock.ExpectQuery(`SELECT MIN\(created_at\) FROM usage_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(retainedFrom))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs("2026-08-13", "2026-08-14").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
		WithArgs(rebuildStart, todayStart, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 2))
	// 追平今日时清掉今日残桶，避免与原始日志尾段重复计入。
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs("2026-08-14").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs("2026-08-14", retainedFrom, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

// 重建跨多天时必须拆成每天一个事务：状态行的 FOR UPDATE 与 usage_logs 的
// INSERT 触发器冲突，块之间提交才能让积压的写入通过。
func TestDashboardAggregationRepositorySyncGroupUsageRollupsSplitsRebuildIntoDailyTransactions(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)

	// 水位停在 08-11，需要发布 08-11/08-12/08-13 三天。
	for _, day := range []struct {
		closedBefore string
		nextClosed   string
		rebuildStart time.Time
		last         bool
	}{
		{closedBefore: "2026-08-11", nextClosed: "2026-08-12", rebuildStart: time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC)},
		{closedBefore: "2026-08-12", nextClosed: "2026-08-13", rebuildStart: time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)},
		{closedBefore: "2026-08-13", nextClosed: "2026-08-14", rebuildStart: time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC), last: true},
	} {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
				AddRow(day.closedBefore, retainedFrom, "Asia/Shanghai"))
		mock.ExpectQuery(`SELECT MIN\(created_at\) FROM usage_logs`).
			WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(retainedFrom))
		mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
			WithArgs(day.closedBefore, day.nextClosed).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
			WithArgs(day.rebuildStart, day.rebuildStart.Add(24*time.Hour), "Asia/Shanghai").
			WillReturnResult(sqlmock.NewResult(0, 1))
		if day.last {
			mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
				WithArgs("2026-08-14").
				WillReturnResult(sqlmock.NewResult(0, 0))
		}
		mock.ExpectExec(`UPDATE usage_group_rollup_state`).
			WithArgs(day.nextClosed, retainedFrom, "Asia/Shanghai").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsRejectsFutureWatermark(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("2026-08-15", time.Unix(0, 0).UTC(), "Asia/Shanghai"))
	mock.ExpectRollback()

	err := repo.SyncGroupUsageRollups(context.Background(), todayStart)
	require.ErrorContains(t, err, "未来")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryRecomputeRangeInvalidatesGroupRollupsBeforeDashboardRebuild(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	start := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	// 失效标记走独立短事务并先行提交，仪表盘重建的慢查询不再被状态行锁笼罩。
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(start, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM usage_dashboard_hourly`).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err := repo.RecomputeRange(context.Background(), start, end)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

// RecomputeRange 拆成三段独立提交：失效标记、仪表盘重建、分组日桶分块重建。
// 关键在于慢查询都不在持有 usage_group_rollup_state 行锁的事务里。
func TestDashboardAggregationRepositoryRecomputeRangeSplitsTransactionsToKeepWritesUnblocked(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	// 起点选在昨天，分组日桶单块即可追平。
	start := time.Date(2026, 8, 13, 3, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	fixedNow := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	repo.clock = func() time.Time { return fixedNow }
	todayStart := service.GroupUsageTodayStart(fixedNow)
	startDate := service.GroupUsageDate(start)
	rebuildStart, err := service.ParseGroupUsageDate(startDate)
	require.NoError(t, err)

	// 第一段：失效标记，短事务。
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(start, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 第二段：仪表盘重建，独立事务且不碰状态行。
	mock.ExpectBegin()
	for _, query := range []string{
		`DELETE FROM usage_dashboard_hourly WHERE`,
		`DELETE FROM usage_dashboard_hourly_users WHERE`,
		`DELETE FROM usage_dashboard_daily WHERE`,
		`DELETE FROM usage_dashboard_daily_users WHERE`,
		`INSERT INTO usage_dashboard_hourly_users`,
		`INSERT INTO usage_dashboard_daily_users`,
		`INSERT INTO usage_dashboard_hourly`,
		`INSERT INTO usage_dashboard_daily`,
	} {
		mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	// 第三段：分组日桶按天分块。
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow(startDate, time.Unix(0, 0).UTC(), "Asia/Shanghai"))
	mock.ExpectQuery(`SELECT MIN\(created_at\) FROM usage_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(start))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs(startDate, service.GroupUsageDate(todayStart)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
		WithArgs(rebuildStart.UTC(), todayStart.UTC(), "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs(service.GroupUsageDate(todayStart)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(service.GroupUsageDate(todayStart), start, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.RecomputeRange(context.Background(), start, end))
	require.NoError(t, mock.ExpectationsWereMet())
}

// 归档只推进屏障、删源数据：不回退发布水位，也不触发任何日桶同步/重算。
func TestDashboardAggregationRepositoryCleanupUsageLogsNonPartitionedAdvancesRetentionWithoutSync(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`(?s)UPDATE usage_group_rollup_state.*retained_from = GREATEST`).
		WithArgs(cutoff).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM usage_logs`).
		WithArgs(cutoff, usageLogsCleanupBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	require.NoError(t, repo.CleanupUsageLogs(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}

// 分区归档把屏障推进到分区之后（DROP TABLE 不触发行级失效触发器），同样不做同步。
func TestDashboardAggregationRepositoryCleanupUsageLogsPartitionedSortsAndAdvancesRetentionWithoutSync(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	mayStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	julyStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT c.relname`).
		WillReturnRows(sqlmock.NewRows([]string{"relname"}).
			AddRow("usage_logs_202606").
			AddRow("usage_logs_invalid").
			AddRow("usage_logs_202604").
			AddRow("usage_logs_202607"))

	for _, partition := range []struct {
		name         string
		retainedFrom time.Time
	}{
		{name: "usage_logs_202604", retainedFrom: mayStart},
		{name: "usage_logs_202606", retainedFrom: julyStart},
	} {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectExec(`(?s)UPDATE usage_group_rollup_state.*retained_from = GREATEST`).
			WithArgs(partition.retainedFrom).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DROP TABLE IF EXISTS "` + partition.name + `"`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
	}

	require.NoError(t, repo.CleanupUsageLogs(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsNonPartitionedFailureRollsBack(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`(?s)UPDATE usage_group_rollup_state.*retained_from = GREATEST`).
		WithArgs(cutoff).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err := repo.CleanupUsageLogs(context.Background(), cutoff)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsPartitionFailureRollsBackAndStops(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	mayStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	dropErr := errors.New("drop partition failed")

	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT c.relname`).
		WillReturnRows(sqlmock.NewRows([]string{"relname"}).
			AddRow("usage_logs_202606").
			AddRow("usage_logs_202604"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`(?s)UPDATE usage_group_rollup_state.*retained_from = GREATEST`).
		WithArgs(mayStart).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DROP TABLE IF EXISTS "usage_logs_202604"`).
		WillReturnError(dropErr)
	mock.ExpectRollback()

	err := repo.CleanupUsageLogs(context.Background(), cutoff)
	require.ErrorIs(t, err, dropErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func setGroupUsageRollupTestTimezone(t *testing.T) {
	t.Helper()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
}
