//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestGroupUsageRollupTriggerInvalidatesCascadedHistoricalDelete(t *testing.T) {
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

			_, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
				VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2020-01-02 08:00:00+08');
				UPDATE usage_group_rollup_state
				SET closed_before = CURRENT_DATE
				WHERE id = 1;
				DELETE FROM users WHERE id = 1;
			`)
			require.NoError(t, err)

			var closedBefore string
			err = tx.QueryRowContext(ctx, `
				SELECT closed_before::text
				FROM usage_group_rollup_state
				WHERE id = 1
			`).Scan(&closedBefore)
			require.NoError(t, err)
			require.Equal(t, "2020-01-02", closedBefore)
		})
	}
}

// 保留期清理先推进归档屏障再删源数据，触发器据此跳过水位回退，
// 已发布的历史日桶原样保留、不被重算。
func TestGroupUsageRollupTriggerSkipsInvalidationBelowRetentionBarrier(t *testing.T) {
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

			_, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
				VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2026-05-01 08:00:00+08');
				INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
				VALUES (DATE '2026-05-01', 10, 1.25, NOW());
				UPDATE usage_group_rollup_state
				SET closed_before = DATE '2026-08-14',
					retained_from = TIMESTAMPTZ '2026-06-01 00:00:00+08'
				WHERE id = 1;
				DELETE FROM usage_logs WHERE id = 1;
			`)
			require.NoError(t, err)

			var closedBefore string
			require.NoError(t, tx.QueryRowContext(ctx, `
				SELECT closed_before::text
				FROM usage_group_rollup_state
				WHERE id = 1
			`).Scan(&closedBefore))
			require.Equal(t, "2026-08-14", closedBefore, "归档区间的删除不得回退发布水位")

			var rollupCost float64
			require.NoError(t, tx.QueryRowContext(ctx, `
				SELECT actual_cost
				FROM usage_group_daily_rollups
				WHERE bucket_date = DATE '2026-05-01' AND group_id = 10
			`).Scan(&rollupCost))
			require.InDelta(t, 1.25, rollupCost, 0.0000001, "归档日桶必须原样保留")
		})
	}
}

// 屏障之后的删除仍是数据修正，必须回退水位以便重建。
func TestGroupUsageRollupTriggerInvalidatesAboveRetentionBarrier(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2026-07-03 08:00:00+08');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-14',
			retained_from = TIMESTAMPTZ '2026-06-01 00:00:00+08'
		WHERE id = 1;
		DELETE FROM usage_logs WHERE id = 1;
	`)
	require.NoError(t, err)

	var closedBefore string
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT closed_before::text
		FROM usage_group_rollup_state
		WHERE id = 1
	`).Scan(&closedBefore))
	require.Equal(t, "2026-07-03", closedBefore)
}

// CleanupUsageLogs 的完整归档路径：删掉保留期以外的原始日志后，
// 发布水位不动、历史日桶一行不少、分组累计用量不变——即「不重新算」。
func TestCleanupUsageLogsArchivesWithoutRecomputingRollups(t *testing.T) {
	ctx := context.Background()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 7, TIMESTAMPTZ '2026-05-20 12:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-12 12:00:00+08'),
			(3, 1, 10, 3, TIMESTAMPTZ '2026-08-13 12:00:00+08'),
			(4, 1, 10, 4, TIMESTAMPTZ '2026-08-14 12:00:00+08');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at) VALUES
			(DATE '2026-05-20', 10, 7, NOW()),
			(DATE '2026-08-12', 10, 2, NOW()),
			(DATE '2026-08-13', 10, 3, NOW());
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-14',
			retained_from = TIMESTAMPTZ '2026-05-20 12:00:00+08'
		WHERE id = 1;
	`)
	require.NoError(t, err)

	usageRepo := newUsageLogRepositoryWithSQL(nil, tx)
	before, err := usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.InDelta(t, 16, before[0].TotalCost, 0.0000001)

	aggRepo := newDashboardAggregationRepositoryWithSQL(tx)
	require.NoError(t, aggRepo.CleanupUsageLogs(ctx, cutoff))

	var remaining int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs").Scan(&remaining))
	require.Equal(t, 3, remaining, "cutoff 以前的原始日志应被归档")

	var closedBefore string
	var retainedFrom time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT closed_before::text, retained_from
		FROM usage_group_rollup_state
		WHERE id = 1
	`).Scan(&closedBefore, &retainedFrom))
	require.Equal(t, "2026-08-14", closedBefore, "归档不得回退发布水位")
	require.True(t, retainedFrom.Equal(cutoff), "归档屏障应推进到 cutoff")

	var buckets int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_group_daily_rollups").Scan(&buckets))
	require.Equal(t, 3, buckets, "历史日桶必须原样保留")

	after, err := usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.InDelta(t, before[0].TotalCost, after[0].TotalCost, 0.0000001, "累计用量不得因归档缩水")
	require.InDelta(t, 4, after[0].TodayCost, 0.0000001)
	require.InDelta(t, 3, after[0].YesterdayCost, 0.0000001)
}

// 多天回填必须按天分块提交：每块结束就释放 usage_group_rollup_state 的行锁，
// usage_logs 的写入才不会被整段重建挡住。这里用真实并发写入验证。
func TestSyncGroupUsageRollupsChunksByDayAndReleasesLockBetweenDays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-08-10 12:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-11 12:00:00+08'),
			(3, 1, 10, 3, TIMESTAMPTZ '2026-08-12 12:00:00+08'),
			(4, 1, 10, 4, TIMESTAMPTZ '2026-08-13 12:00:00+08'),
			(5, 1, 10, 5, TIMESTAMPTZ '2026-08-14 12:00:00+08');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-10',
			retained_from = TIMESTAMPTZ '2026-08-10 12:00:00+08'
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	// 分块跑在真实连接池上，才能观察到块之间锁被释放。
	scopedDB := newGroupUsageRollupSchemaDB(t, schema)
	repo := newDashboardAggregationRepositoryWithSQL(scopedDB)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	var closedBefore string
	require.NoError(t, scopedDB.QueryRowContext(ctx, `
		SELECT closed_before::text FROM usage_group_rollup_state WHERE id = 1
	`).Scan(&closedBefore))
	require.Equal(t, "2026-08-14", closedBefore, "水位应追平今日")

	// 08-10 ~ 08-13 四天各一个桶；今日（08-14）走原始日志尾段，不应留桶。
	rows, err := scopedDB.QueryContext(ctx, `
		SELECT bucket_date::text, actual_cost
		FROM usage_group_daily_rollups
		WHERE group_id = 10
		ORDER BY bucket_date
	`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	buckets := map[string]float64{}
	for rows.Next() {
		var date string
		var cost float64
		require.NoError(t, rows.Scan(&date, &cost))
		buckets[date] = cost
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[string]float64{
		"2026-08-10": 1, "2026-08-11": 2, "2026-08-12": 3, "2026-08-13": 4,
	}, buckets)

	usageRepo := newUsageLogRepositoryWithSQL(nil, scopedDB)
	result, err := usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.InDelta(t, 15, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 5, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 4, result[0].YesterdayCost, 0.0000001)
}

// 分块把锁持有压到单日：即便有写入正持有状态行的 KEY SHARE，
// 同步也只会卡在当前这一天，而非整段区间。
func TestSyncGroupUsageRollupsResumesFromPersistedWatermark(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 1, TIMESTAMPTZ '2026-08-10 12:00:00+08'),
			(2, 1, 10, 2, TIMESTAMPTZ '2026-08-11 12:00:00+08'),
			(3, 1, 10, 3, TIMESTAMPTZ '2026-08-12 12:00:00+08');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-10',
			retained_from = TIMESTAMPTZ '2026-08-10 12:00:00+08'
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	scopedDB := newGroupUsageRollupSchemaDB(t, schema)
	repo := newDashboardAggregationRepositoryWithSQL(scopedDB)

	// 先只追到 08-12，模拟上一轮被打断在中途。
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)))
	var midWatermark string
	require.NoError(t, scopedDB.QueryRowContext(ctx, `
		SELECT closed_before::text FROM usage_group_rollup_state WHERE id = 1
	`).Scan(&midWatermark))
	require.Equal(t, "2026-08-12", midWatermark, "中断点应已持久化")

	// 下一轮从持久化水位续跑，不重复已发布的日子。
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)))
	var finalWatermark string
	require.NoError(t, scopedDB.QueryRowContext(ctx, `
		SELECT closed_before::text FROM usage_group_rollup_state WHERE id = 1
	`).Scan(&finalWatermark))
	require.Equal(t, "2026-08-13", finalWatermark)

	var total float64
	require.NoError(t, scopedDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(actual_cost), 0) FROM usage_group_daily_rollups WHERE group_id = 10
	`).Scan(&total))
	require.InDelta(t, 6, total, 0.0000001, "续跑不得重复或漏算")
}

// 保留期清理后累计用量不缩水：原始日志已不在，日桶依旧全额计入。
func TestGroupUsageSummaryKeepsArchivedRollupsInTotal(t *testing.T) {
	ctx := context.Background()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	// 只有 08-13 起的原始日志还在，08-12 及更早的已被归档，仅剩日桶。
	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(2, 1, 10, 3, TIMESTAMPTZ '2026-08-13 12:00:00+08'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-08-14 12:00:00+08');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at) VALUES
			(DATE '2026-05-20', 10, 11, NOW()),
			(DATE '2026-08-12', 10, 2, NOW()),
			(DATE '2026-08-13', 10, 3, NOW());
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-14',
			retained_from = TIMESTAMPTZ '2026-08-13 00:00:00+08'
		WHERE id = 1;
	`)
	require.NoError(t, err)

	repo := newUsageLogRepositoryWithSQL(nil, tx)
	result, err := repo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.InDelta(t, 20, result[0].TotalCost, 0.0000001, "归档日桶必须计入累计用量")
	require.InDelta(t, 4, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 3, result[0].YesterdayCost, 0.0000001)
}

func TestGroupUsageRollupTriggerSerializesLateHistoricalInsertWithPublish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2020-01-02'
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	syncTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = syncTx.Rollback() }()
	var stateID int16
	require.NoError(t, syncTx.QueryRowContext(ctx, `
		SELECT id
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`).Scan(&stateID))

	lateTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = lateTx.Rollback() }()
	var lateBackendPID int
	require.NoError(t, lateTx.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&lateBackendPID))

	insertResult := make(chan error, 1)
	go func() {
		_, insertErr := lateTx.ExecContext(ctx, `
			INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
			VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2020-01-02 09:00:00+08')
		`)
		insertResult <- insertErr
	}()

	blocked, err := waitForGroupUsageRollupStateLock(ctx, lateBackendPID, insertResult)
	if err != nil || !blocked {
		_ = syncTx.Rollback()
		_ = lateTx.Rollback()
		require.NoError(t, err)
		require.True(t, blocked, "迟到写入必须等待正在发布水位的事务")
	}

	_, err = syncTx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = CURRENT_DATE
		WHERE id = 1
	`)
	require.NoError(t, err)
	require.NoError(t, syncTx.Commit())

	select {
	case err = <-insertResult:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待迟到写入完成超时")
	}
	require.NoError(t, lateTx.Commit())

	var closedBefore string
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT closed_before::text FROM %s.usage_group_rollup_state WHERE id = 1",
		pq.QuoteIdentifier(schema),
	)).Scan(&closedBefore)
	require.NoError(t, err)
	require.Equal(t, "2020-01-02", closedBefore)
}

func TestGroupUsageRollupTriggerSerializesInsertTransactionAcrossMidnight(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = CURRENT_DATE
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	syncTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = syncTx.Rollback() }()
	var stateID int16
	require.NoError(t, syncTx.QueryRowContext(ctx, `
		SELECT id
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`).Scan(&stateID))

	insertTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = insertTx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, insertTx, "Asia/Shanghai"))
	var insertBackendPID int
	require.NoError(t, insertTx.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&insertBackendPID))

	insertResult := make(chan error, 1)
	go func() {
		_, insertErr := insertTx.ExecContext(ctx, `
			INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
			VALUES (1, 1, 10, 1.25, CURRENT_TIMESTAMP)
		`)
		insertResult <- insertErr
	}()

	blocked, err := waitForGroupUsageRollupStateLock(ctx, insertBackendPID, insertResult)
	if err != nil || !blocked {
		_ = syncTx.Rollback()
		_ = insertTx.Rollback()
		require.NoError(t, err)
		require.True(t, blocked, "跨越零点的在途写入必须与水位发布串行化")
	}

	_, err = syncTx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = CURRENT_DATE + 1
		WHERE id = 1
	`)
	require.NoError(t, err)
	require.NoError(t, syncTx.Commit())

	select {
	case err = <-insertResult:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待跨零点写入完成超时")
	}
	require.NoError(t, insertTx.Commit())

	var currentDate string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT CURRENT_DATE::text
	`).Scan(&currentDate))
	var closedBefore string
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT closed_before::text FROM %s.usage_group_rollup_state WHERE id = 1",
		pq.QuoteIdentifier(schema),
	)).Scan(&closedBefore)
	require.NoError(t, err)
	require.Equal(t, currentDate, closedBefore)
}

func TestGroupUsageRollupTriggerKeepsWatermarkForTodayInsert(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)

	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerTimeZone(ctx, tx, "Asia/Shanghai"))
	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = CURRENT_DATE
		WHERE id = 1;
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1.25, CURRENT_TIMESTAMP);
	`)
	require.NoError(t, err)

	var unchanged bool
	err = tx.QueryRowContext(ctx, `
		SELECT closed_before = CURRENT_DATE
		FROM usage_group_rollup_state
		WHERE id = 1
	`).Scan(&unchanged)
	require.NoError(t, err)
	require.True(t, unchanged)
}

func TestGroupUsageRollupTriggerUsesSessionTimezoneAcrossDST(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)

	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()
	_, err := tx.ExecContext(ctx, `
		SET LOCAL TIME ZONE 'America/New_York';
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-03-09',
			timezone_name = 'America/New_York'
		WHERE id = 1;
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2026-03-08 04:30:00+00');
	`)
	require.NoError(t, err)

	var closedBefore string
	err = tx.QueryRowContext(ctx, `
		SELECT closed_before::text
		FROM usage_group_rollup_state
		WHERE id = 1
	`).Scan(&closedBefore)
	require.NoError(t, err)
	require.Equal(t, "2026-03-07", closedBefore)
}

func TestGroupUsageSummaryIncludesYesterdayAcrossWatermark(t *testing.T) {
	ctx := context.Background()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		closedBefore     string
		includeYesterday bool
	}{
		{name: "closed_rollup", closedBefore: "2026-08-14", includeYesterday: true},
		{name: "raw_tail", closedBefore: "2026-08-13", includeYesterday: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
			tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
			defer func() { _ = tx.Rollback() }()

			_, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
					(1, 1, 10, 2, TIMESTAMPTZ '2026-08-12 12:00:00+08'),
					(2, 1, 10, 3, TIMESTAMPTZ '2026-08-13 12:00:00+08'),
					(3, 1, 10, 4, TIMESTAMPTZ '2026-08-14 12:00:00+08');
				INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
				VALUES (DATE '2026-08-12', 10, 2, NOW());
			`)
			require.NoError(t, err)
			if tt.includeYesterday {
				_, err = tx.ExecContext(ctx, `
					INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
					VALUES (DATE '2026-08-13', 10, 3, NOW())
				`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE usage_group_rollup_state
				SET closed_before = $1::date,
					retained_from = TIMESTAMPTZ '2026-08-12 00:00:00+08'
				WHERE id = 1
			`, tt.closedBefore)
			require.NoError(t, err)

			repo := newUsageLogRepositoryWithSQL(nil, tx)
			result, err := repo.GetAllGroupUsageSummary(ctx, todayStart)
			require.NoError(t, err)
			require.Len(t, result, 1)
			require.InDelta(t, 9, result[0].TotalCost, 0.0000001)
			require.InDelta(t, 4, result[0].TodayCost, 0.0000001)
			require.InDelta(t, 3, result[0].YesterdayCost, 0.0000001)
		})
	}
}

func TestGroupUsageRollupSyncRebuildsAfterTimezoneChange(t *testing.T) {
	ctx := context.Background()
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		SET LOCAL TIME ZONE 'UTC';
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 3, TIMESTAMPTZ '2026-03-08 05:30:00+00'),
			(2, 1, 10, 5, TIMESTAMPTZ '2026-03-09 04:30:00+00');
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
		VALUES (DATE '2026-03-08', 10, 99, NOW());
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-03-09',
			retained_from = TIMESTAMPTZ '2026-03-08 05:30:00+00',
			timezone_name = 'Asia/Shanghai'
		WHERE id = 1;
	`)
	require.NoError(t, err)

	repo := newDashboardAggregationRepositoryWithSQL(tx)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	var stateTimezone string
	var closedBefore string
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT timezone_name, closed_before::text
		FROM usage_group_rollup_state
		WHERE id = 1
	`).Scan(&stateTimezone, &closedBefore))
	require.Equal(t, "America/New_York", stateTimezone)
	require.Equal(t, "2026-03-09", closedBefore)

	var rollupCost float64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT actual_cost
		FROM usage_group_daily_rollups
		WHERE bucket_date = DATE '2026-03-08' AND group_id = 10
	`).Scan(&rollupCost))
	require.InDelta(t, 3, rollupCost, 0.0000001)

	usageRepo := newUsageLogRepositoryWithSQL(nil, tx)
	result, err := usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.InDelta(t, 8, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 5, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 3, result[0].YesterdayCost, 0.0000001)
}

func TestGroupUsageSummaryUsesConfiguredDSTBoundaries(t *testing.T) {
	ctx := context.Background()
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		SET LOCAL TIME ZONE 'UTC';
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 100, TIMESTAMPTZ '2026-03-08 04:30:00+00'),
			(2, 1, 10, 3, TIMESTAMPTZ '2026-03-08 05:30:00+00'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-03-09 03:30:00+00'),
			(4, 1, 10, 5, TIMESTAMPTZ '2026-03-09 04:30:00+00');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '1970-01-01',
			retained_from = TIMESTAMPTZ '1970-01-01 00:00:00+00',
			timezone_name = 'America/New_York'
		WHERE id = 1;
	`)
	require.NoError(t, err)

	repo := newUsageLogRepositoryWithSQL(nil, tx)
	result, err := repo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.InDelta(t, 112, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 5, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 7, result[0].YesterdayCost, 0.0000001)
}

// 一次分块日结要同时产出 group 与 api_key 两个维度的日桶，且两者都必须与明细对账。
// 这条用例同时验证 GROUPING SETS + data-modifying CTE 的组合在真实 PostgreSQL 上成立。
func TestSyncUsageRollupsProducesGroupAndAPIKeyBucketsInOnePass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	// 2026-08-12 16:00 UTC = 2026-08-13 00:00 +08，故"今日"为 08-13，只结算 08-12。
	todayStart := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10), (20);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, api_key_id, actual_cost, created_at) VALUES
			(1, 1, 10,  100, 1.5, TIMESTAMPTZ '2026-08-12 09:00:00+08'),
			(2, 1, 10,  101, 2.5, TIMESTAMPTZ '2026-08-12 10:00:00+08'),
			(3, 1, 20,  100, 4.0, TIMESTAMPTZ '2026-08-12 11:00:00+08'),
			-- group_id 为 NULL 的行仍要计入 api_key 维度
			(4, 1, NULL, 101, 8.0, TIMESTAMPTZ '2026-08-12 12:00:00+08'),
			-- api_key_id 为 NULL 的行仍要计入 group 维度
			(5, 1, 10,  NULL, 0.5, TIMESTAMPTZ '2026-08-12 13:00:00+08'),
			(6, 1, 10,  100, 7.0, TIMESTAMPTZ '2026-08-13 09:00:00+08');
		UPDATE usage_group_rollup_state
		SET closed_before = DATE '2026-08-12',
			retained_from = TIMESTAMPTZ '2026-08-12 00:00:00+08'
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	scopedDB := newGroupUsageRollupSchemaDB(t, schema)
	repo := newDashboardAggregationRepositoryWithSQL(scopedDB)
	require.NoError(t, repo.SyncGroupUsageRollups(ctx, todayStart))

	// group 日桶：08-12 的 group 10 = 1.5+2.5+0.5，group 20 = 4.0；group_id 为 NULL 的不入表。
	groupBuckets := map[int64]float64{}
	rows, err := scopedDB.QueryContext(ctx, `
		SELECT group_id, actual_cost FROM usage_group_daily_rollups WHERE bucket_date = DATE '2026-08-12'
	`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		var cost float64
		require.NoError(t, rows.Scan(&id, &cost))
		groupBuckets[id] = cost
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, groupBuckets, 2)
	require.InDelta(t, 4.5, groupBuckets[10], 0.0000001)
	require.InDelta(t, 4.0, groupBuckets[20], 0.0000001)

	// api_key 日桶：key 100 = 1.5+4.0，key 101 = 2.5+8.0；api_key_id 为 NULL 的不入表。
	type keyBucket struct {
		cost     float64
		requests int64
	}
	keyBuckets := map[int64]keyBucket{}
	rows, err = scopedDB.QueryContext(ctx, `
		SELECT api_key_id, actual_cost, request_count
		FROM usage_apikey_daily_rollups WHERE bucket_date = DATE '2026-08-12'
	`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		var b keyBucket
		require.NoError(t, rows.Scan(&id, &b.cost, &b.requests))
		keyBuckets[id] = b
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, keyBuckets, 2)
	require.InDelta(t, 5.5, keyBuckets[100].cost, 0.0000001)
	require.Equal(t, int64(2), keyBuckets[100].requests)
	require.InDelta(t, 10.5, keyBuckets[101].cost, 0.0000001)
	require.Equal(t, int64(2), keyBuckets[101].requests)

	// 今日（08-13）不应留桶，当天用量走原始日志尾段。
	var todayBuckets int
	require.NoError(t, scopedDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM usage_apikey_daily_rollups WHERE bucket_date >= DATE '2026-08-13'
	`).Scan(&todayBuckets))
	require.Equal(t, 0, todayBuckets, "今日桶会与尾段重复计入")
}

// GetBatchAPIKeyUsageStats 必须等于「历史日桶 + 当日明细」，不能因为改走日桶而少算或重复计。
//
// 这个用例只能用相对日期：查询里的"今天"取自 timezone.Today()（真实墙钟），
// 写死日期会让尾段永远落空，从而把 bug 测成通过。
func TestGetBatchAPIKeyUsageStatsMatchesRawAggregate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	today := appTimezone.Today()

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
	`)
	require.NoError(t, err)
	_, err = seedTx.ExecContext(ctx, `
		INSERT INTO usage_logs (id, user_id, group_id, api_key_id, actual_cost, created_at) VALUES
			(1, 1, 10, 100, 3.0, $1),
			(2, 1, 10, 100, 5.0, $2),
			(3, 1, 10, 100, 7.0, $3),
			(4, 1, 10, 101, 2.0, $2)
	`,
		today.AddDate(0, 0, -3).Add(9*time.Hour),
		today.AddDate(0, 0, -2).Add(9*time.Hour),
		today, // 当天 00:00，既落在尾段又压到 >= 边界上
	)
	require.NoError(t, err)
	_, err = seedTx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = ($1::timestamptz AT TIME ZONE 'Asia/Shanghai')::date,
			retained_from = $1
		WHERE id = 1
	`, today.AddDate(0, 0, -3).Add(9*time.Hour))
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	scopedDB := newGroupUsageRollupSchemaDB(t, schema)
	require.NoError(t, newDashboardAggregationRepositoryWithSQL(scopedDB).SyncGroupUsageRollups(ctx, today))

	// 水位追平到今天后，D-3 与 D-2 已入桶，今天仍只在明细里。
	var closedBefore time.Time
	require.NoError(t, scopedDB.QueryRowContext(ctx,
		`SELECT closed_before FROM usage_group_rollup_state WHERE id = 1`).Scan(&closedBefore))
	require.Equal(t, today.Format("2006-01-02"), closedBefore.Format("2006-01-02"))

	usageRepo := newUsageLogRepositoryWithSQL(nil, scopedDB)
	stats, err := usageRepo.GetBatchAPIKeyUsageStats(
		ctx, []int64{100, 101}, today.AddDate(0, 0, -5), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Len(t, stats, 2)

	// key 100 = D-3 日桶 3.0 + D-2 日桶 5.0 + 今日尾段 7.0。
	require.InDelta(t, 15.0, stats[100].TotalActualCost, 0.0000001)
	require.InDelta(t, 7.0, stats[100].TodayActualCost, 0.0000001)
	require.InDelta(t, 2.0, stats[101].TotalActualCost, 0.0000001)
	require.InDelta(t, 0.0, stats[101].TodayActualCost, 0.0000001)

	// 与直接扫明细的口径对账：改造前后总额必须完全一致。
	rawTotals := map[int64]float64{}
	rows, err := scopedDB.QueryContext(ctx, `
		SELECT api_key_id, COALESCE(SUM(actual_cost), 0)
		FROM usage_logs WHERE api_key_id = ANY($1) AND created_at >= $2 AND created_at < $3
		GROUP BY api_key_id
	`, pq.Array([]int64{100, 101}), today.AddDate(0, 0, -5), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		var cost float64
		require.NoError(t, rows.Scan(&id, &cost))
		rawTotals[id] = cost
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for id, raw := range rawTotals {
		require.InDelta(t, raw, stats[id].TotalActualCost, 0.0000001, "api_key %d 日桶口径偏离明细", id)
	}
}

// 水位缺失时不能返回 0：宁可退化成全量扫明细（慢但正确）。
func TestGetBatchAPIKeyUsageStatsFallsBackWhenWatermarkMissing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
	today := appTimezone.Today()

	schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
	seedTx := beginGroupUsageRollupTriggerTestTx(t, ctx, schema)
	_, err := seedTx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
	`)
	require.NoError(t, err)
	_, err = seedTx.ExecContext(ctx, `
		INSERT INTO usage_logs (id, user_id, group_id, api_key_id, actual_cost, created_at) VALUES
			(1, 1, 10, 100, 3.0, $1),
			(2, 1, 10, 100, 7.0, $2)
	`, today.AddDate(0, 0, -3).Add(9*time.Hour), today)
	require.NoError(t, err)
	_, err = seedTx.ExecContext(ctx, `DELETE FROM usage_group_rollup_state WHERE id = 1`)
	require.NoError(t, err)
	require.NoError(t, seedTx.Commit())

	scopedDB := newGroupUsageRollupSchemaDB(t, schema)
	usageRepo := newUsageLogRepositoryWithSQL(nil, scopedDB)
	stats, err := usageRepo.GetBatchAPIKeyUsageStats(
		ctx, []int64{100}, today.AddDate(0, 0, -5), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.InDelta(t, 10.0, stats[100].TotalActualCost, 0.0000001)
	require.InDelta(t, 7.0, stats[100].TodayActualCost, 0.0000001)
}

func createGroupUsageRollupTriggerTestSchema(t *testing.T, ctx context.Context, partitioned bool) string {
	t.Helper()

	schema := fmt.Sprintf("group_usage_rollup_trigger_%d", time.Now().UnixNano())
	quotedSchema := pq.QuoteIdentifier(schema)
	_, err := integrationDB.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
	})

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerSearchPath(ctx, tx, quotedSchema))

	usageLogsDDL := `
		CREATE TABLE usage_logs (
			id BIGINT PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
			api_key_id BIGINT,
			actual_cost NUMERIC(20, 10) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		);
	`
	if partitioned {
		usageLogsDDL = `
			CREATE TABLE usage_logs (
				id BIGINT NOT NULL,
				user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
				api_key_id BIGINT,
				actual_cost NUMERIC(20, 10) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL
			) PARTITION BY RANGE (created_at);
			CREATE TABLE usage_logs_default PARTITION OF usage_logs DEFAULT;
		`
	}

	_, err = tx.ExecContext(ctx, `
		CREATE TABLE users (id BIGINT PRIMARY KEY);
		CREATE TABLE groups (id BIGINT PRIMARY KEY);
	`+usageLogsDDL)
	require.NoError(t, err)

	for _, migrationName := range []string{
		"222_group_usage_daily_rollups.sql",
		"223_group_usage_rollup_timezone.sql",
		"227_group_usage_rollup_archival.sql",
		"229_usage_apikey_daily_rollups.sql",
	} {
		migrationSQL, readErr := migrations.FS.ReadFile(migrationName)
		require.NoError(t, readErr)
		for range 2 {
			_, err = tx.ExecContext(ctx, string(migrationSQL))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tx.Commit())

	return schema
}

// newGroupUsageRollupSchemaDB 返回 search_path 固定在测试 schema 的独立连接池。
// 分块同步每天开一个新事务，必须有真实的 *sql.DB 才能观察到跨事务行为。
func newGroupUsageRollupSchemaDB(t *testing.T, schema string) *sql.DB {
	t.Helper()

	separator := "?"
	if strings.Contains(integrationDSN, "?") {
		separator = "&"
	}
	scopedDSN := integrationDSN + separator + "options=" + url.QueryEscape("-c search_path="+schema)
	db, err := sql.Open("postgres", scopedDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(context.Background()))
	return db
}

func beginGroupUsageRollupTriggerTestTx(t *testing.T, ctx context.Context, schema string) *sql.Tx {
	t.Helper()

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, setGroupUsageRollupTriggerSearchPath(ctx, tx, pq.QuoteIdentifier(schema)))
	return tx
}

func setGroupUsageRollupTriggerSearchPath(ctx context.Context, tx *sql.Tx, quotedSchema string) error {
	_, err := tx.ExecContext(ctx, "SET LOCAL search_path TO "+quotedSchema)
	return err
}

func setGroupUsageRollupTriggerTimeZone(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx, "SET LOCAL TIME ZONE "+pq.QuoteLiteral(name))
	return err
}

func waitForGroupUsageRollupStateLock(
	ctx context.Context,
	backendPID int,
	insertResult <-chan error,
) (bool, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-insertResult:
			if err != nil {
				return false, err
			}
			return false, nil
		case <-ticker.C:
			var waitEventType sql.NullString
			err := integrationDB.QueryRowContext(ctx, `
				SELECT wait_event_type
				FROM pg_stat_activity
				WHERE pid = $1
			`, backendPID).Scan(&waitEventType)
			if err != nil {
				return false, err
			}
			if waitEventType.Valid && waitEventType.String == "Lock" {
				return true, nil
			}
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}
