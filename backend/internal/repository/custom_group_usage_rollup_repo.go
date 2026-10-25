package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// groupUsageRollupSnapshot 是一次汇总水位的快照。
// closedBefore / retainedFrom 在水位无效时退化为 1970-01-01，
// 与此前 SQL 里 CASE WHEN valid 的语义一一对应。
type groupUsageRollupSnapshot struct {
	valid        bool
	closedBefore string    // date，历史日桶的右开边界
	retainedDate string    // retained_from 在服务端时区内的日期，历史日桶的左闭边界
	tailStart    time.Time // 尾段（还没进日桶的那部分 usage_logs）的起点
}

// readGroupUsageRollupSnapshot 读一次汇总水位并在 Go 侧判定有效性。
//
// 判定条件与原先 SQL 里的 state_values CTE 完全一致：
// 恰好一行、时区名与当前服务端配置相同、且水位不在未来。
func (r *usageLogRepository) readGroupUsageRollupSnapshot(ctx context.Context, timezoneName, todayDate string) (groupUsageRollupSnapshot, error) {
	if IsMySQL() {
		return r.readGroupUsageRollupSnapshotMySQL(ctx, todayDate)
	}
	epoch := time.Unix(0, 0).UTC()
	invalid := groupUsageRollupSnapshot{
		closedBefore: "1970-01-01",
		retainedDate: "1970-01-01",
		tailStart:    epoch,
	}

	var rowCount int
	var closedBefore sql.NullString
	var retainedFrom sql.NullTime
	var stateTimezone sql.NullString
	if err := scanSingleRow(ctx, r.sql, `
		SELECT
			COUNT(*),
			MAX(closed_before)::text,
			MAX(retained_from),
			MAX(timezone_name)
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &rowCount, &closedBefore, &retainedFrom, &stateTimezone); err != nil {
		return groupUsageRollupSnapshot{}, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}
	if rowCount != 1 || !closedBefore.Valid || !retainedFrom.Valid ||
		!stateTimezone.Valid || stateTimezone.String != timezoneName ||
		closedBefore.String > todayDate {
		return invalid, nil
	}

	tailStart, err := service.ParseGroupUsageDate(closedBefore.String)
	if err != nil {
		// 水位值解析不出来，按无效处理：宁可多扫一次，也不要算错钱。
		return invalid, nil //nolint:nilerr // 与上面的 valid 判定同语义，均降级为全量重算
	}
	return groupUsageRollupSnapshot{
		valid:        true,
		closedBefore: closedBefore.String,
		retainedDate: service.GroupUsageDate(retainedFrom.Time),
		tailStart:    tailStart.UTC(),
	}, nil
}

// readGroupUsageRollupSnapshotMySQL 是 readGroupUsageRollupSnapshot 的 MySQL 实现：
//   - MAX(closed_before)::text → DATE_FORMAT(MAX(closed_before), '%Y-%m-%d')（DATE 列）；
//   - 时区一致性判定改为与 @@session.time_zone 比较：MySQL 失效触发器与同步作业
//     （migrations/mysql/222/223）写入 timezone_name = @@session.time_zone（固定偏移），
//     而 PG 版比较 IANA 时区名（current_setting('TimeZone')）。
//
// REVIEW: MySQL DSN 会话时区由 internal/config 固定为应用时区偏移；若运行期有人为修改
// 数据库会话时区，快照判定会失效并退化为全量重算（安全但低效）。PG/MySQL 水位状态不互导。
func (r *usageLogRepository) readGroupUsageRollupSnapshotMySQL(ctx context.Context, todayDate string) (groupUsageRollupSnapshot, error) {
	epoch := time.Unix(0, 0).UTC()
	invalid := groupUsageRollupSnapshot{
		closedBefore: "1970-01-01",
		retainedDate: "1970-01-01",
		tailStart:    epoch,
	}

	var rowCount int
	var closedBefore sql.NullString
	var retainedFrom sql.NullTime
	var stateTimezone sql.NullString
	var sessionTimezone string
	if err := scanSingleRow(ctx, r.sql, `
		SELECT
			COUNT(*),
			DATE_FORMAT(MAX(closed_before), '%Y-%m-%d'),
			MAX(retained_from),
			MAX(timezone_name),
			@@session.time_zone
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &rowCount, &closedBefore, &retainedFrom, &stateTimezone, &sessionTimezone); err != nil {
		return groupUsageRollupSnapshot{}, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}
	if rowCount != 1 || !closedBefore.Valid || !retainedFrom.Valid ||
		!stateTimezone.Valid || stateTimezone.String != sessionTimezone ||
		closedBefore.String > todayDate {
		return invalid, nil
	}

	tailStart, err := service.ParseGroupUsageDate(closedBefore.String)
	if err != nil {
		// 水位值解析不出来，按无效处理：宁可多扫一次，也不要算错钱。
		return invalid, nil //nolint:nilerr // 与上面的 valid 判定同语义，均降级为全量重算
	}
	return groupUsageRollupSnapshot{
		valid:        true,
		closedBefore: closedBefore.String,
		retainedDate: service.GroupUsageDate(retainedFrom.Time),
		tailStart:    tailStart.UTC(),
	}, nil
}

func (r *usageLogRepository) getAllGroupUsageSummaryFromRollups(ctx context.Context, todayStart time.Time) (results []usagestats.GroupUsageSummary, err error) {
	todayStart = service.GroupUsageTodayStart(todayStart)
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	todayDate := service.GroupUsageDate(todayStart)
	yesterdayDate := service.GroupUsageDate(yesterdayStart)

	state, err := r.readGroupUsageRollupSnapshot(ctx, timezoneName, todayDate)
	if err != nil {
		return nil, err
	}

	// 尾段起点必须以**查询参数**的形式给进来。
	//
	// 此前它是在同一条 SQL 里由 state CTE 算出、再 CROSS JOIN 给 tail 用的，
	// 于是 created_at 的下界对 planner 来说是个运行期才知道的值：既进不了
	// index cond，也估不准选择性，只能退化成 usage_logs 全表扫。
	// 生产实测（1721 万行 / PostgreSQL 17）：
	//   Seq Scan on usage_logs … rows=17212870, Rows Removed by Join Filter: 17178082
	//   Execution Time: 48720 ms
	// 换成参数之后走 idx_usage_logs_created_at：
	//   Index Scan … Index Cond: (created_at >= $7)
	//   Execution Time: 34 ms
	var query string
	var queryArgs []any
	if IsMySQL() {
		// MySQL 版：FILTER 聚合 → SUM(CASE WHEN ... ELSE 0 END)；$n::date → ?（DATE 列
		// 与字符串日期比较）；WHERE $4::boolean → WHERE ?；`groups` 为 MySQL 8 保留字
		// 加反引号。? 出现顺序与 PG $n 编号不同，参数按语句文本顺序重排。
		query = `
		WITH historical AS (
			SELECT
				rollup.group_id,
				COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(CASE WHEN rollup.bucket_date = ? THEN rollup.actual_cost ELSE 0 END), 0) AS yesterday_cost
			FROM usage_group_daily_rollups rollup
			WHERE ?
				AND rollup.bucket_date >= ?
				AND rollup.bucket_date < ?
			GROUP BY rollup.group_id
		),
		tail AS (
			SELECT
				ul.group_id,
				COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(CASE WHEN ul.created_at >= ? THEN ul.actual_cost ELSE 0 END), 0) AS today_cost,
				COALESCE(SUM(CASE WHEN ul.created_at >= ?
						AND ul.created_at < ?
					THEN ul.actual_cost ELSE 0 END), 0) AS yesterday_cost
			FROM usage_logs ul
			WHERE ul.created_at >= ?
			GROUP BY ul.group_id
		)
		SELECT
			g.id AS group_id,
			COALESCE(historical.actual_cost, 0) + COALESCE(tail.actual_cost, 0) AS total_cost,
			COALESCE(tail.today_cost, 0) AS today_cost,
			COALESCE(historical.yesterday_cost, 0) + COALESCE(tail.yesterday_cost, 0) AS yesterday_cost
		FROM ` + "`groups`" + ` g
		LEFT JOIN historical ON historical.group_id = g.id
		LEFT JOIN tail ON tail.group_id = g.id
		ORDER BY g.id
	`
		queryArgs = []any{
			yesterdayDate,
			state.valid,
			state.retainedDate,
			state.closedBefore,
			todayStart,
			yesterdayStart,
			todayStart,
			state.tailStart,
		}
	} else {
		query = `
		WITH historical AS (
			SELECT
				rollup.group_id,
				COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(rollup.actual_cost) FILTER (
					WHERE rollup.bucket_date = $3::date
				), 0) AS yesterday_cost
			FROM usage_group_daily_rollups rollup
			WHERE $4::boolean
				AND rollup.bucket_date >= $5::date
				AND rollup.bucket_date < $6::date
			GROUP BY rollup.group_id
		),
		tail AS (
			SELECT
				ul.group_id,
				COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(ul.actual_cost) FILTER (WHERE ul.created_at >= $1), 0) AS today_cost,
				COALESCE(SUM(ul.actual_cost) FILTER (
					WHERE ul.created_at >= $2
						AND ul.created_at < $1
				), 0) AS yesterday_cost
			FROM usage_logs ul
			WHERE ul.created_at >= $7
			GROUP BY ul.group_id
		)
		SELECT
			g.id AS group_id,
			COALESCE(historical.actual_cost, 0) + COALESCE(tail.actual_cost, 0) AS total_cost,
			COALESCE(tail.today_cost, 0) AS today_cost,
			COALESCE(historical.yesterday_cost, 0) + COALESCE(tail.yesterday_cost, 0) AS yesterday_cost
		FROM groups g
		LEFT JOIN historical ON historical.group_id = g.id
		LEFT JOIN tail ON tail.group_id = g.id
		ORDER BY g.id
	`
		queryArgs = []any{
			todayStart,
			yesterdayStart,
			yesterdayDate,
			state.valid,
			state.retainedDate,
			state.closedBefore,
			state.tailStart,
		}
	}

	rows, err := r.sql.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.GroupUsageSummary, 0)
	for rows.Next() {
		var row usagestats.GroupUsageSummary
		if err := rows.Scan(&row.GroupID, &row.TotalCost, &row.TodayCost, &row.YesterdayCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// SyncGroupUsageRollups 将服务端配置时区今日以前的用量发布为分组日桶。
func (r *dashboardAggregationRepository) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	todayStart = service.GroupUsageTodayStart(todayStart)
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.syncGroupUsageRollupsInTx(ctx, todayStart); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.syncGroupUsageRollupsInTx(ctx, todayStart)
}

func (r *dashboardAggregationRepository) syncGroupUsageRollupsInTx(ctx context.Context, todayStart time.Time) error {
	var closedBefore string
	var previousRetainedFrom time.Time
	var stateTimezoneName string
	timezoneName := service.GroupUsageTimezoneName()
	if IsMySQL() {
		// MySQL 失效触发器（migrations/mysql/222/223）写 timezone_name = @@session.time_zone，
		// 读取与写回保持同一取值，保证时区一致性判定与触发器对齐。
		if err := scanSingleRow(ctx, r.sql, `
			SELECT DATE_FORMAT(closed_before, '%Y-%m-%d'), retained_from, timezone_name, @@session.time_zone
			FROM usage_group_rollup_state
			WHERE id = 1
			FOR UPDATE
		`, nil, &closedBefore, &previousRetainedFrom, &stateTimezoneName, &timezoneName); err != nil {
			return fmt.Errorf("读取分组用量汇总水位: %w", err)
		}
	} else {
		if err := scanSingleRow(ctx, r.sql, `
			SELECT closed_before::text, retained_from, timezone_name
			FROM usage_group_rollup_state
			WHERE id = 1
			FOR UPDATE
		`, nil, &closedBefore, &previousRetainedFrom, &stateTimezoneName); err != nil {
			return fmt.Errorf("读取分组用量汇总水位: %w", err)
		}
	}

	todayDate := service.GroupUsageDate(todayStart)
	timezoneChanged := stateTimezoneName != timezoneName
	var closedTime time.Time
	if !timezoneChanged {
		var err error
		closedTime, err = service.ParseGroupUsageDate(closedBefore)
		if err != nil {
			return fmt.Errorf("解析分组用量汇总水位 %q: %w", closedBefore, err)
		}
		todayDateTime, err := service.ParseGroupUsageDate(todayDate)
		if err != nil {
			return err
		}
		if closedTime.After(todayDateTime) {
			return fmt.Errorf("分组用量汇总水位位于未来: %s", closedBefore)
		}
		if closedBefore == todayDate {
			return nil
		}
	}

	var earliest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, "SELECT MIN(created_at) FROM usage_logs", nil, &earliest); err != nil {
		return fmt.Errorf("读取最早用量记录: %w", err)
	}
	retainedFrom := todayStart
	if earliest.Valid {
		retainedFrom = earliest.Time.UTC()
	}
	retainedDate := service.GroupUsageDate(retainedFrom)
	retainedDateTime, err := service.ParseGroupUsageDate(retainedDate)
	if err != nil {
		return err
	}
	rebuildStartDate := retainedDate
	if !timezoneChanged && closedTime.After(retainedDateTime) {
		rebuildStartDate = closedBefore
	}
	rebuildStart, err := service.ParseGroupUsageDate(rebuildStartDate)
	if err != nil {
		return err
	}

	if IsMySQL() {
		if _, err := r.sql.ExecContext(ctx, `
			DELETE FROM usage_group_daily_rollups
			WHERE bucket_date < ?
				OR (bucket_date >= ? AND bucket_date < ?)
				OR bucket_date >= ?
		`, retainedDate, rebuildStartDate, todayDate, todayDate); err != nil {
			return fmt.Errorf("清理分组用量日桶: %w", err)
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			DELETE FROM usage_group_daily_rollups
			WHERE bucket_date < $1::date
				OR (bucket_date >= $2::date AND bucket_date < $3::date)
				OR bucket_date >= $3::date
		`, retainedDate, rebuildStartDate, todayDate); err != nil {
			return fmt.Errorf("清理分组用量日桶: %w", err)
		}
	}

	if IsMySQL() {
		// (created_at AT TIME ZONE $3::text)::date → DATE(created_at)：MySQL DATETIME 存的是
		// 会话时区墙钟（internal/config 保证 DSN Loc == 会话 time_zone == 服务端时区），
		// DATE() 即服务端时区自然日（与 migrations/mysql/223 的 DATE(created_at) 约定一致）；
		// ON CONFLICT (bucket_date, group_id) DO UPDATE → ODKU；NOW() → NOW(3)。
		if _, err := r.sql.ExecContext(ctx, `
			INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
			SELECT
				DATE(created_at) AS bucket_date,
				group_id,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				NOW(3)
			FROM usage_logs
			WHERE group_id IS NOT NULL
				AND created_at >= ?
				AND created_at < ?
			GROUP BY DATE(created_at), group_id
			ON DUPLICATE KEY UPDATE
				actual_cost = VALUES(actual_cost),
				computed_at = VALUES(computed_at)
		`, rebuildStart.UTC(), todayStart.UTC()); err != nil {
			return fmt.Errorf("重建分组用量日桶: %w", err)
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
			SELECT
				(created_at AT TIME ZONE $3::text)::date AS bucket_date,
				group_id,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				NOW()
			FROM usage_logs
			WHERE group_id IS NOT NULL
				AND created_at >= $1
				AND created_at < $2
			GROUP BY 1, 2
			ON CONFLICT (bucket_date, group_id)
			DO UPDATE SET
				actual_cost = EXCLUDED.actual_cost,
				computed_at = EXCLUDED.computed_at
		`, rebuildStart.UTC(), todayStart.UTC(), timezoneName); err != nil {
			return fmt.Errorf("重建分组用量日桶: %w", err)
		}
	}

	if IsMySQL() {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE usage_group_rollup_state
			SET closed_before = ?,
				retained_from = ?,
				timezone_name = ?,
				updated_at = NOW(3)
			WHERE id = 1
		`, todayDate, retainedFrom, timezoneName); err != nil {
			return fmt.Errorf("更新分组用量汇总水位: %w", err)
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE usage_group_rollup_state
			SET closed_before = $1::date,
				retained_from = $2,
				timezone_name = $3,
				updated_at = NOW()
			WHERE id = 1
		`, todayDate, retainedFrom, timezoneName); err != nil {
			return fmt.Errorf("更新分组用量汇总水位: %w", err)
		}
	}
	return nil
}

func lockGroupUsageRollupState(ctx context.Context, tx *sql.Tx) error {
	var id int16
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`).Scan(&id); err != nil {
		return fmt.Errorf("锁定分组用量汇总水位: %w", err)
	}
	return nil
}

func invalidateGroupUsageRollupsAt(ctx context.Context, tx *sql.Tx, affectedAt time.Time) error {
	if IsMySQL() {
		return invalidateGroupUsageRollupsAtMySQL(ctx, tx, affectedAt)
	}
	timezoneName := service.GroupUsageTimezoneName()
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = LEAST(
			closed_before,
			($1::timestamptz AT TIME ZONE $2::text)::date
		),
			updated_at = NOW()
		WHERE id = 1
	`, affectedAt.UTC(), timezoneName)
	return err
}

// invalidateGroupUsageRollupsAtMySQL 是 invalidateGroupUsageRollupsAt 的 MySQL 实现。
// PG 版在 SQL 内把 affectedAt（绝对时刻）按服务端时区取日期后 LEAST 回退水位；
// MySQL DATETIME 存的是会话时区墙钟（internal/config MySQLConfig 保证 DSN Loc ==
// 会话 time_zone == 服务端时区），affectedAt 由数据库扫描而来已是该墙钟，
// 故在应用侧按服务端时区取日期（与 usage_logs 的 MySQL 失效触发器
// DATE(CONVERT_TZ(created_at, @@session.time_zone, ...)) 语义一致）。
// closed_before 为 DATE NOT NULL，MySQL LEAST 无 NULL 退化问题。
// REVIEW: 依赖 DSN Loc == 会话 time_zone == 服务端时区；若 affectedAt 来自
// 非数据库扫描的其他时钟源且时区标记不一致，日期换算会有偏差。
func invalidateGroupUsageRollupsAtMySQL(ctx context.Context, tx *sql.Tx, affectedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = LEAST(closed_before, DATE(?)),
			updated_at = NOW(3)
		WHERE id = 1
	`, service.GroupUsageDate(affectedAt))
	return err
}
