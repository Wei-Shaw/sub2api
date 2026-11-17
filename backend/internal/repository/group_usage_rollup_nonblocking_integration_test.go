//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 计算阶段持有状态行 FOR KEY SHARE 期间，usage_logs 的当天写入、迟到写入与窗口内删除都不等待；
// 迟到写入与删除记入 build_dirty_from，同步重算脏区间后发布到今日，日桶与明细逐日一致。
func TestGroupUsageRollupSyncDoesNotBlockUsageWritesDuringBuild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	execGroupUsageRollupSQL(t, ctx, db, `
		INSERT INTO groups (id) VALUES (10), (20);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-08-10 09:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-11 09:00:00+08'),
			(3, 1, 20, 4, TIMESTAMPTZ '2026-08-12 09:00:00+08'),
			(4, 1, 10, 8, TIMESTAMPTZ '2026-08-13 09:00:00+08');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost)
		VALUES (DATE '2026-08-10', 10, 1);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-11',
			retained_from = TIMESTAMPTZ '2026-08-10 09:00:00+08',
			timezone_name = 'Asia/Shanghai'
		WHERE id = 1;
	`)

	// 让计算阶段停在清理日桶这一步：此时它已持有状态行的 FOR KEY SHARE。
	blocker, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	_, err = blocker.ExecContext(ctx, "LOCK TABLE usage_group_daily_rollups IN SHARE MODE")
	require.NoError(t, err)

	repo := newDashboardAggregationRepositoryWithSQL(db)
	syncDone := make(chan error, 1)
	go func() { syncDone <- repo.SyncGroupUsageRollups(ctx, todayStart) }()
	waitForBlockedGroupUsageRollupStatement(t, ctx, "DELETE FROM usage_group_daily_rollups")

	writeCtx, writeCancel := context.WithTimeout(ctx, 3*time.Second)
	defer writeCancel()
	execGroupUsageRollupSQL(t, writeCtx, db, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (5, 1, 20, 16, TIMESTAMPTZ '2026-08-12 18:00:00+08')
	`)
	execGroupUsageRollupSQL(t, writeCtx, db, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (6, 1, 10, 32, TIMESTAMPTZ '2026-08-14 09:00:00+08')
	`)
	execGroupUsageRollupSQL(t, writeCtx, db, `DELETE FROM usage_logs WHERE id = 2`)

	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-08-11", state.closedBefore)
	require.Equal(t, "2026-08-14", state.buildingUntil.String)
	require.Equal(t, "2026-08-11", state.buildDirtyFrom.String)

	require.NoError(t, blocker.Rollback())
	select {
	case err := <-syncDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待分组日桶同步完成超时")
	}

	state = readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-08-14", state.closedBefore)
	require.False(t, state.buildingUntil.Valid)
	require.False(t, state.buildDirtyFrom.Valid)
	requireGroupUsageRollupsMatchUsageLogs(t, ctx, db, "Asia/Shanghai", state.retainedFrom, todayStart)

	summary := groupUsageSummaryByGroup(t, ctx, db, todayStart)
	require.InDelta(t, 41, summary[10][0], 1e-9)
	require.InDelta(t, 32, summary[10][1], 1e-9)
	require.InDelta(t, 8, summary[10][2], 1e-9)
	require.InDelta(t, 20, summary[20][0], 1e-9)
	require.InDelta(t, 0, summary[20][1], 1e-9)
	require.InDelta(t, 0, summary[20][2], 1e-9)
}

// 保留期清理按整日推进保留下界后删除：发布水位不动，过期记录的写入不触发失效，
// 窗口内的删除照常使日桶失效。
func TestGroupUsageRollupRetentionCleanupKeepsPublishedWatermark(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)

	execGroupUsageRollupSQL(t, ctx, db, `
		INSERT INTO groups (id) VALUES (10), (20);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-08-01 10:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-02 05:00:00+08'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-08-02 20:00:00+08'),
			(4, 1, 20, 8, TIMESTAMPTZ '2026-08-05 12:00:00+08'),
			(5, 1, 10, 16, TIMESTAMPTZ '2026-08-10 08:00:00+08');
	`)

	repo := newDashboardAggregationRepositoryWithSQL(db)
	repo.clock = func() time.Time { return todayStart.Add(10 * time.Hour) }
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))
	require.Equal(t, "2026-08-10", readGroupUsageRollupTestState(t, ctx, db).closedBefore)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	require.NoError(t, repo.CleanupUsageLogs(ctx, time.Date(2026, 8, 2, 12, 0, 0, 0, shanghai)))

	alignedCutoff := time.Date(2026, 8, 2, 0, 0, 0, 0, shanghai)
	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-08-10", state.closedBefore)
	require.True(t, state.expiredBefore.Equal(alignedCutoff), "expired_before=%s", state.expiredBefore)
	require.True(t, state.retainedFrom.Equal(alignedCutoff), "retained_from=%s", state.retainedFrom)
	require.False(t, state.buildingUntil.Valid)

	var remaining []int64
	rows, err := db.QueryContext(ctx, "SELECT id FROM usage_logs ORDER BY id")
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		remaining = append(remaining, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []int64{2, 3, 4, 5}, remaining)

	requireGroupUsageRollupsMatchUsageLogs(t, ctx, db, "Asia/Shanghai", alignedCutoff, todayStart)
	summary := groupUsageSummaryByGroup(t, ctx, db, todayStart)
	require.InDelta(t, 22, summary[10][0], 1e-9)
	require.InDelta(t, 16, summary[10][1], 1e-9)
	require.InDelta(t, 8, summary[20][0], 1e-9)

	execGroupUsageRollupSQL(t, ctx, db, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (6, 1, 10, 100, TIMESTAMPTZ '2026-08-01 12:00:00+08')
	`)
	require.Equal(t, "2026-08-10", readGroupUsageRollupTestState(t, ctx, db).closedBefore)
	summary = groupUsageSummaryByGroup(t, ctx, db, todayStart)
	require.InDelta(t, 22, summary[10][0], 1e-9)

	execGroupUsageRollupSQL(t, ctx, db, `DELETE FROM usage_logs WHERE id = 4`)
	require.Equal(t, "2026-08-05", readGroupUsageRollupTestState(t, ctx, db).closedBefore)
	summary = groupUsageSummaryByGroup(t, ctx, db, todayStart)
	require.InDelta(t, 0, summary[20][0], 1e-9)
	require.InDelta(t, 22, summary[10][0], 1e-9)
}

func TestGroupUsageRollupTriggersIgnoreRowsBeforeExpiredFloor(t *testing.T) {
	for _, partitioned := range []bool{false, true} {
		name := "ordinary"
		if partitioned {
			name = "partitioned"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			schema := createGroupUsageRollupTriggerTestSchema(t, ctx, partitioned)
			tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
			defer func() { _ = tx.Rollback() }()
			require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, tx, "Asia/Shanghai"))

			_, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
					(1, 1, 10, 1, TIMESTAMPTZ '2020-01-15 08:00:00+08'),
					(2, 1, 10, 2, TIMESTAMPTZ '2020-02-05 08:00:00+08');
				UPDATE usage_group_rollup_state
				SET closed_before = DATE '2020-03-01',
					expired_before = TIMESTAMPTZ '2020-02-01 00:00:00+08'
				WHERE id = 1;
				DELETE FROM usage_logs WHERE id = 1;
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
				VALUES (3, 1, 10, 4, TIMESTAMPTZ '2020-01-20 08:00:00+08');
				UPDATE usage_logs SET actual_cost = 5 WHERE id = 3;
			`)
			require.NoError(t, err)
			require.Equal(t, "2020-03-01", readGroupUsageRollupTestStateTx(t, ctx, tx).closedBefore)

			// 把过期记录改到保留窗口内，按新日期失效。
			_, err = tx.ExecContext(ctx, `
				UPDATE usage_logs SET created_at = TIMESTAMPTZ '2020-02-20 08:00:00+08' WHERE id = 3
			`)
			require.NoError(t, err)
			require.Equal(t, "2020-02-20", readGroupUsageRollupTestStateTx(t, ctx, tx).closedBefore)

			_, err = tx.ExecContext(ctx, `DELETE FROM usage_logs WHERE id = 2`)
			require.NoError(t, err)
			require.Equal(t, "2020-02-05", readGroupUsageRollupTestStateTx(t, ctx, tx).closedBefore)
		})
	}
}

func TestGroupUsageRollupTriggersMarkDirtyDuringBuild(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, tx, "Asia/Shanghai"))

	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2020-01-05',
			building_until = DATE '2020-01-10'
		WHERE id = 1;
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1, TIMESTAMPTZ '2020-01-12 08:00:00+08');
	`)
	require.NoError(t, err)
	state := readGroupUsageRollupTestStateTx(t, ctx, tx)
	require.Equal(t, "2020-01-05", state.closedBefore)
	require.False(t, state.buildDirtyFrom.Valid)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (2, 1, 10, 1, TIMESTAMPTZ '2020-01-07 08:00:00+08')
	`)
	require.NoError(t, err)
	state = readGroupUsageRollupTestStateTx(t, ctx, tx)
	require.Equal(t, "2020-01-05", state.closedBefore)
	require.Equal(t, "2020-01-07", state.buildDirtyFrom.String)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (3, 1, 10, 1, TIMESTAMPTZ '2020-01-03 08:00:00+08')
	`)
	require.NoError(t, err)
	state = readGroupUsageRollupTestStateTx(t, ctx, tx)
	require.Equal(t, "2020-01-03", state.closedBefore)
	require.Equal(t, "2020-01-03", state.buildDirtyFrom.String)

	_, err = tx.ExecContext(ctx, `DELETE FROM usage_logs WHERE id = 1`)
	require.NoError(t, err)
	state = readGroupUsageRollupTestStateTx(t, ctx, tx)
	require.Equal(t, "2020-01-03", state.buildDirtyFrom.String)
}

// 计算阶段对状态行只持 FOR KEY SHARE：迟到写入与窗口内删除都不等待它。
func TestGroupUsageRollupBuildKeyShareDoesNotBlockLateWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1, TIMESTAMPTZ '2020-01-08 08:00:00+08');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2020-01-05',
			building_until = DATE '2020-01-10'
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	buildTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = buildTx.Rollback() }()
	var stateID int16
	require.NoError(t, buildTx.QueryRowContext(ctx, `
		SELECT id FROM usage_group_rollup_state WHERE id = 1 FOR KEY SHARE
	`).Scan(&stateID))

	for _, statement := range []string{
		`INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		 VALUES (2, 1, 10, 1, TIMESTAMPTZ '2020-01-07 08:00:00+08')`,
		`DELETE FROM usage_logs WHERE id = 1`,
	} {
		writeTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
		require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, writeTx, "Asia/Shanghai"))
		var writePID int
		require.NoError(t, writeTx.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&writePID))
		result := make(chan error, 1)
		go func(statement string) {
			_, execErr := writeTx.ExecContext(ctx, statement)
			result <- execErr
		}(statement)
		blocked, err := waitForGroupUsageRollupStateLock(ctx, writePID, result)
		require.NoError(t, err)
		require.False(t, blocked, "计算阶段不得阻塞: %s", statement)
		require.NoError(t, writeTx.Commit())
	}

	require.NoError(t, buildTx.Commit())
	var dirtyFrom string
	require.NoError(t, integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT build_dirty_from::text FROM %s.usage_group_rollup_state WHERE id = 1",
		pq.QuoteIdentifier(schema),
	)).Scan(&dirtyFrom))
	require.Equal(t, "2020-01-07", dirtyFrom)
}

func TestGroupUsageRollupSyncSkipsWhileAnotherBuildHoldsLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	holder, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	var locked bool
	require.NoError(t, holder.QueryRowContext(ctx, groupUsageRollupBuildLockSQL).Scan(&locked))
	require.True(t, locked)
	defer func() { _, _ = holder.ExecContext(context.Background(), groupUsageRollupBuildUnlockSQL) }()

	repo := newDashboardAggregationRepositoryWithSQL(db)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))
	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "1970-01-01", state.closedBefore)
	require.False(t, state.buildingUntil.Valid)
}

// 上一次重建在登记之后中断：下一次同步覆盖遗留登记并发布到今日。
func TestGroupUsageRollupSyncRecoversFromInterruptedBuild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	execGroupUsageRollupSQL(t, ctx, db, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-08-11 09:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-12 09:00:00+08'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-08-13 09:00:00+08');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost) VALUES
			(DATE '2026-08-11', 10, 1),
			(DATE '2026-08-12', 10, 999);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-12',
			retained_from = TIMESTAMPTZ '2026-08-11 09:00:00+08',
			timezone_name = 'Asia/Shanghai',
			building_until = DATE '2026-08-14',
			build_dirty_from = DATE '2026-08-13'
		WHERE id = 1;
	`)

	repo := newDashboardAggregationRepositoryWithSQL(db)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-08-14", state.closedBefore)
	require.False(t, state.buildingUntil.Valid)
	require.False(t, state.buildDirtyFrom.Valid)
	requireGroupUsageRollupsMatchUsageLogs(t, ctx, db, "Asia/Shanghai", state.retainedFrom, todayStart)
}

type groupUsageRollupTestState struct {
	closedBefore   string
	retainedFrom   time.Time
	expiredBefore  time.Time
	buildingUntil  sql.NullString
	buildDirtyFrom sql.NullString
}

const groupUsageRollupTestStateQuery = `
	SELECT closed_before::text, retained_from, expired_before, building_until::text, build_dirty_from::text
	FROM usage_group_rollup_state
	WHERE id = 1
`

func readGroupUsageRollupTestState(t *testing.T, ctx context.Context, q sqlQueryer) groupUsageRollupTestState {
	t.Helper()
	var state groupUsageRollupTestState
	require.NoError(t, scanSingleRow(ctx, q, groupUsageRollupTestStateQuery, nil,
		&state.closedBefore, &state.retainedFrom, &state.expiredBefore, &state.buildingUntil, &state.buildDirtyFrom))
	return state
}

func readGroupUsageRollupTestStateTx(t *testing.T, ctx context.Context, tx *sql.Tx) groupUsageRollupTestState {
	t.Helper()
	return readGroupUsageRollupTestState(t, ctx, tx)
}

// openGroupUsageRollupSchemaDB 打开一个 search_path 指向 schema 的连接池，供仓储以 *sql.DB 方式运行。
func openGroupUsageRollupSchemaDB(t *testing.T, schema, timezoneName string) *sql.DB {
	t.Helper()
	dsn, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("search_path", schema)
	query.Set("TimeZone", timezoneName)
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(context.Background()))
	return db
}

func execGroupUsageRollupSQL(t *testing.T, ctx context.Context, db *sql.DB, query string) {
	t.Helper()
	_, err := db.ExecContext(ctx, query)
	require.NoError(t, err)
}

// waitForBlockedGroupUsageRollupStatement 等待某个后端在执行包含 fragment 的语句时进入锁等待。
func waitForBlockedGroupUsageRollupStatement(t *testing.T, ctx context.Context, fragment string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE wait_event_type = 'Lock'
					AND position($1 in query) > 0
			)
		`, fragment).Scan(&blocked))
		if blocked {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("等待语句进入锁等待超时: %s", fragment)
		}
	}
}

// requireGroupUsageRollupsMatchUsageLogs 断言 [retainedFrom 所在日, todayStart) 的日桶与 usage_logs 明细逐日一致。
func requireGroupUsageRollupsMatchUsageLogs(t *testing.T, ctx context.Context, db *sql.DB, timezoneName string, retainedFrom, todayStart time.Time) {
	t.Helper()
	expected := collectGroupUsageRollupBuckets(t, ctx, db, `
		SELECT (created_at AT TIME ZONE $3::text)::date::text, group_id, SUM(actual_cost)::float8
		FROM usage_logs
		WHERE group_id IS NOT NULL
			AND created_at >= $1
			AND created_at < $2
		GROUP BY 1, 2
	`, retainedFrom, todayStart, timezoneName)
	actual := collectGroupUsageRollupBuckets(t, ctx, db, `
		SELECT bucket_date::text, group_id, actual_cost::float8
		FROM usage_group_daily_rollups
		WHERE bucket_date >= ($1::timestamptz AT TIME ZONE $3::text)::date
			AND bucket_date < ($2::timestamptz AT TIME ZONE $3::text)::date
	`, retainedFrom, todayStart, timezoneName)
	require.Equal(t, expected, actual)
}

func collectGroupUsageRollupBuckets(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) map[string]float64 {
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	buckets := make(map[string]float64)
	for rows.Next() {
		var bucketDate string
		var groupID int64
		var cost float64
		require.NoError(t, rows.Scan(&bucketDate, &groupID, &cost))
		buckets[fmt.Sprintf("%s/%d", bucketDate, groupID)] = cost
	}
	require.NoError(t, rows.Err())
	return buckets
}

// groupUsageSummaryByGroup 返回 group_id → [total, today, yesterday]。
func groupUsageSummaryByGroup(t *testing.T, ctx context.Context, db *sql.DB, todayStart time.Time) map[int64][3]float64 {
	t.Helper()
	result, err := newUsageLogRepositoryWithSQL(nil, db).GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	summary := make(map[int64][3]float64, len(result))
	for _, row := range result {
		summary[row.GroupID] = [3]float64{row.TotalCost, row.TodayCost, row.YesterdayCost}
	}
	return summary
}

// 重建进行中的大批量删除：脏日期已覆盖的行不写状态行，状态行在事务内只更新一次。
func TestGroupUsageRollupBulkDeleteDuringBuildWritesStateOnce(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, tx, "Asia/Shanghai"))

	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		SELECT n, 1, 10, 1, TIMESTAMPTZ '2020-01-15 08:00:00+08' + n * INTERVAL '1 second'
		FROM generate_series(1, 2000) AS n;
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2020-01-10',
			building_until = DATE '2020-02-01'
		WHERE id = 1;
	`)
	require.NoError(t, err)

	var updatesBefore int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT n_tup_upd FROM pg_stat_xact_user_tables
		WHERE schemaname = current_schema() AND relname = 'usage_group_rollup_state'
	`).Scan(&updatesBefore))

	_, err = tx.ExecContext(ctx, `DELETE FROM usage_logs`)
	require.NoError(t, err)

	var updatesAfter int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT n_tup_upd FROM pg_stat_xact_user_tables
		WHERE schemaname = current_schema() AND relname = 'usage_group_rollup_state'
	`).Scan(&updatesAfter))
	require.Equal(t, int64(1), updatesAfter-updatesBefore)

	state := readGroupUsageRollupTestStateTx(t, ctx, tx)
	require.Equal(t, "2020-01-10", state.closedBefore)
	require.Equal(t, "2020-01-15", state.buildDirtyFrom.String)
}

// 应用侧失效的起点早于保留下界时，从保留下界所在日开始失效。
func TestGroupUsageRollupInvalidateAtStartsFromExpiredFloor(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2020-03-01',
			expired_before = TIMESTAMPTZ '2020-02-01 00:00:00+08'
		WHERE id = 1;
		SELECT invalidate_group_usage_rollups_at(TIMESTAMPTZ '2020-01-10 08:00:00+08', 'Asia/Shanghai');
	`)
	require.NoError(t, err)
	require.Equal(t, "2020-02-01", readGroupUsageRollupTestStateTx(t, ctx, tx).closedBefore)

	_, err = tx.ExecContext(ctx, `
		SELECT invalidate_group_usage_rollups_at(TIMESTAMPTZ '2020-02-10 08:00:00+08', 'Asia/Shanghai')
	`)
	require.NoError(t, err)
	require.Equal(t, "2020-02-01", readGroupUsageRollupTestStateTx(t, ctx, tx).closedBefore)
}

// 夏令时在零点跳变的日期（America/Santiago 2026-09-06 没有 00:00）：从该日续建时不覆盖前一天的日桶。
func TestGroupUsageRollupSyncKeepsPreviousDayBucketAcrossMidnightDSTGap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "America/Santiago")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	db := openGroupUsageRollupSchemaDB(t, schema, "America/Santiago")
	santiago, err := time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	todayStart := time.Date(2026, 9, 8, 0, 0, 0, 0, santiago)

	execGroupUsageRollupSQL(t, ctx, db, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-09-06 02:30:00+00'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-09-06 03:30:00+00'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-09-06 05:00:00+00'),
			(4, 1, 10, 8, TIMESTAMPTZ '2026-09-07 13:00:00+00');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost)
		VALUES (DATE '2026-09-05', 10, 3);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-09-06',
			retained_from = TIMESTAMPTZ '2026-09-06 02:30:00+00',
			timezone_name = 'America/Santiago'
		WHERE id = 1;
	`)

	repo := newDashboardAggregationRepositoryWithSQL(db)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-09-08", state.closedBefore)
	requireGroupUsageRollupsMatchUsageLogs(t, ctx, db, "America/Santiago", state.retainedFrom, todayStart)
	var previousDay float64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT actual_cost::float8 FROM usage_group_daily_rollups
		WHERE bucket_date = DATE '2026-09-05' AND group_id = 10
	`).Scan(&previousDay))
	require.InDelta(t, 3, previousDay, 1e-9)
}

// 有长读者持有 usage_logs 的 ACCESS SHARE 时，删除分区在锁等待超时后跳过；
// 过期记录由按行删除清理，清理整体成功，分组日汇总保持一致。
func TestGroupUsageRollupRetentionCleanupSkipsPartitionDropOnLockTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, true)
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)

	execGroupUsageRollupSQL(t, ctx, db, `
		CREATE TABLE usage_logs_202605 PARTITION OF usage_logs
			FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-05-10 10:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-07-20 10:00:00+08'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-08-05 10:00:00+08');
	`)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	repo.clock = func() time.Time { return todayStart.Add(10 * time.Hour) }
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	reader, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = reader.Rollback() }()
	var count int
	require.NoError(t, reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs").Scan(&count))
	require.Equal(t, 3, count)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	require.NoError(t, repo.CleanupUsageLogs(ctx, time.Date(2026, 7, 1, 0, 0, 0, 0, shanghai)))
	require.NoError(t, reader.Rollback())

	var partitionExists bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass('usage_logs_202605') IS NOT NULL").Scan(&partitionExists))
	require.True(t, partitionExists)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs").Scan(&count))
	require.Equal(t, 2, count)

	state := readGroupUsageRollupTestState(t, ctx, db)
	require.Equal(t, "2026-08-10", state.closedBefore)
	requireGroupUsageRollupsMatchUsageLogs(t, ctx, db, "Asia/Shanghai", state.retainedFrom, todayStart)
	summary := groupUsageSummaryByGroup(t, ctx, db, todayStart)
	require.InDelta(t, 6, summary[10][0], 1e-9)
}
