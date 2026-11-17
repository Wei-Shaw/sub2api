package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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
	tailStart    time.Time // 尾段（还没进日桶的那部分 usage_logs）的起点，不早于 expired_before
}

// readGroupUsageRollupSnapshot 读一次汇总水位并在 Go 侧判定有效性。
//
// 判定条件与原先 SQL 里的 state_values CTE 完全一致：
// 恰好一行、时区名与当前服务端配置相同、且水位不在未来。
// expired_before 之前的 usage_logs 已超出保留期：无论水位是否有效都不进入尾段，
// 其所在日之前的日桶也不读取。
func (r *usageLogRepository) readGroupUsageRollupSnapshot(ctx context.Context, timezoneName, todayDate string) (groupUsageRollupSnapshot, error) {
	var rowCount int
	var closedBefore sql.NullString
	var retainedFrom sql.NullTime
	var stateTimezone sql.NullString
	var expiredBefore sql.NullTime
	if err := scanSingleRow(ctx, r.sql, `
		SELECT
			COUNT(*),
			MAX(closed_before)::text,
			MAX(retained_from),
			MAX(timezone_name),
			MAX(expired_before)
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &rowCount, &closedBefore, &retainedFrom, &stateTimezone, &expiredBefore); err != nil {
		return groupUsageRollupSnapshot{}, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}

	tailFloor := time.Unix(0, 0).UTC()
	if rowCount == 1 && expiredBefore.Valid && expiredBefore.Time.After(tailFloor) {
		tailFloor = expiredBefore.Time.UTC()
	}
	invalid := groupUsageRollupSnapshot{
		closedBefore: "1970-01-01",
		retainedDate: "1970-01-01",
		tailStart:    tailFloor,
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
	tailStart = tailStart.UTC()
	if tailFloor.After(tailStart) {
		tailStart = tailFloor
	}
	retainedDate := service.GroupUsageDate(retainedFrom.Time)
	if expiredDate := service.GroupUsageDate(tailFloor); expiredDate > retainedDate {
		retainedDate = expiredDate
	}
	return groupUsageRollupSnapshot{
		valid:        true,
		closedBefore: closedBefore.String,
		retainedDate: retainedDate,
		tailStart:    tailStart,
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
	const query = `
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

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		todayStart,
		yesterdayStart,
		yesterdayDate,
		state.valid,
		state.retainedDate,
		state.closedBefore,
		state.tailStart,
	)
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

const (
	groupUsageRollupBuildLockSQL   = "SELECT pg_try_advisory_lock(hashtext('usage_group_daily_rollups:build'))"
	groupUsageRollupBuildUnlockSQL = "SELECT pg_advisory_unlock(hashtext('usage_group_daily_rollups:build'))"
	// groupUsageRollupBuildMaxRounds 限定一次同步内因并发变更重算脏区间的轮数；
	// 未收敛的部分保持未发布，由下一次同步继续。
	groupUsageRollupBuildMaxRounds = 3
)

var errGroupUsageRollupBuildPreempted = errors.New("分组用量汇总重建登记已被替换")

// groupUsageRollupBuildPlan 是一轮日桶重建：用 [rowsFrom, rowsBefore) 的 usage_logs
// 重算 [startDate, targetDate) 的日桶，并清理 retainedDate 之前的日桶。
type groupUsageRollupBuildPlan struct {
	startDate    string
	targetDate   string
	retainedDate string
	rowsFrom     time.Time
	rowsBefore   time.Time
	timezoneName string
}

// groupUsageRollupTxRunner 在一个事务里执行 fn。
type groupUsageRollupTxRunner func(ctx context.Context, fn func(q sqlExecutor) error) error

// SyncGroupUsageRollups 将服务端配置时区今日以前的用量发布为分组日桶。
//
// 每轮分三个事务：登记（FOR UPDATE，写入 building_until）、计算（FOR KEY SHARE，重算日桶）、
// 发布（FOR UPDATE，水位推进到 build_dirty_from 或目标日期）。usage_logs 写入与失效触发器只持
// FOR KEY SHARE，因此只与登记、发布这两个短事务串行，计算阶段不阻塞写入。
// 以 *sql.DB 构造时，整个过程持有会话级 advisory lock，同一时刻只有一个重建；
// 以事务构造时，三个阶段在调用方事务内顺序执行。
func (r *dashboardAggregationRepository) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	todayStart = service.GroupUsageTodayStart(todayStart)
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return syncGroupUsageRollupRounds(ctx, func(ctx context.Context, fn func(q sqlExecutor) error) error {
			return fn(r.sql)
		}, todayStart)
	}

	upToDate, err := groupUsageRollupsUpToDate(ctx, db, todayStart)
	if err != nil {
		return err
	}
	if upToDate {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	var locked bool
	if err := scanSingleRow(ctx, conn, groupUsageRollupBuildLockSQL, nil, &locked); err != nil {
		// 查询出错时服务端可能已授予锁，丢弃该物理连接以免锁随连接回到连接池。
		discardGroupUsageRollupBuildConn(conn)
		return fmt.Errorf("获取分组用量汇总重建锁: %w", err)
	}
	if !locked {
		return conn.Close()
	}
	defer releaseGroupUsageRollupBuildLock(conn)

	return syncGroupUsageRollupRounds(ctx, func(ctx context.Context, fn func(q sqlExecutor) error) error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}, todayStart)
}

// releaseGroupUsageRollupBuildLock 释放重建锁；释放失败时丢弃该物理连接，由服务端随会话结束释放锁，
// 避免锁随连接回到连接池。
func releaseGroupUsageRollupBuildLock(conn *sql.Conn) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var unlocked bool
	if err := scanSingleRow(unlockCtx, conn, groupUsageRollupBuildUnlockSQL, nil, &unlocked); err != nil || !unlocked {
		discardGroupUsageRollupBuildConn(conn)
		return
	}
	_ = conn.Close()
}

// discardGroupUsageRollupBuildConn 关闭底层物理连接而不是归还连接池，会话级锁随会话结束释放。
func discardGroupUsageRollupBuildConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// groupUsageRollupsUpToDate 不加锁地判断今日以前的日桶是否已全部发布。
func groupUsageRollupsUpToDate(ctx context.Context, q sqlQueryer, todayStart time.Time) (bool, error) {
	var closedBefore string
	var stateTimezoneName string
	var idle bool
	err := scanSingleRow(ctx, q, `
		SELECT closed_before::text, timezone_name, building_until IS NULL
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &closedBefore, &stateTimezoneName, &idle)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}
	return idle &&
		closedBefore == service.GroupUsageDate(todayStart) &&
		stateTimezoneName == service.GroupUsageTimezoneName(), nil
}

func syncGroupUsageRollupRounds(ctx context.Context, run groupUsageRollupTxRunner, todayStart time.Time) error {
	for range groupUsageRollupBuildMaxRounds {
		var plan *groupUsageRollupBuildPlan
		if err := run(ctx, func(q sqlExecutor) error {
			var err error
			plan, err = beginGroupUsageRollupBuild(ctx, q, todayStart)
			return err
		}); err != nil {
			return err
		}
		if plan == nil {
			return nil
		}
		if err := run(ctx, func(q sqlExecutor) error {
			return buildGroupUsageRollups(ctx, q, plan)
		}); err != nil {
			return err
		}
		var dirty bool
		if err := run(ctx, func(q sqlExecutor) error {
			var err error
			dirty, err = publishGroupUsageRollups(ctx, q, plan)
			return err
		}); err != nil {
			return err
		}
		if !dirty {
			return nil
		}
	}
	return nil
}

// beginGroupUsageRollupBuild 在 FOR UPDATE 下确定重建区间并登记 building_until。
// 日桶已发布到今日时返回 nil。
func beginGroupUsageRollupBuild(ctx context.Context, q sqlExecutor, todayStart time.Time) (*groupUsageRollupBuildPlan, error) {
	var closedBefore string
	var retainedFrom time.Time
	var stateTimezoneName string
	var expiredBefore time.Time
	var buildingUntil sql.NullString
	if err := scanSingleRow(ctx, q, `
		SELECT closed_before::text, retained_from, timezone_name, expired_before, building_until::text
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`, nil, &closedBefore, &retainedFrom, &stateTimezoneName, &expiredBefore, &buildingUntil); err != nil {
		return nil, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}

	todayDate := service.GroupUsageDate(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	timezoneChanged := stateTimezoneName != timezoneName
	if !timezoneChanged {
		closedTime, err := service.ParseGroupUsageDate(closedBefore)
		if err != nil {
			return nil, fmt.Errorf("解析分组用量汇总水位 %q: %w", closedBefore, err)
		}
		todayDateTime, err := service.ParseGroupUsageDate(todayDate)
		if err != nil {
			return nil, err
		}
		if closedTime.After(todayDateTime) {
			return nil, fmt.Errorf("分组用量汇总水位位于未来: %s", closedBefore)
		}
		if closedBefore == todayDate {
			if buildingUntil.Valid {
				if _, err := q.ExecContext(ctx, `
					UPDATE usage_group_rollup_state
					SET building_until = NULL,
						build_dirty_from = NULL,
						updated_at = NOW()
					WHERE id = 1
				`); err != nil {
					return nil, fmt.Errorf("清除分组用量汇总重建登记: %w", err)
				}
			}
			return nil, nil
		}
	}

	// 可读日桶从 retained_from 所在日开始，且不早于保留下界。
	if retainedFrom.Before(expiredBefore) {
		retainedFrom = expiredBefore
	}
	// closed_before 不晚于保留起点时，可读日桶为空，按保留起点全量重建；否则从水位续建。
	retainedDate := service.GroupUsageDate(retainedFrom)
	startDate := closedBefore
	if timezoneChanged || closedBefore <= retainedDate {
		var earliest sql.NullTime
		if err := scanSingleRow(ctx, q, `
			SELECT MIN(created_at)
			FROM usage_logs
			WHERE created_at >= $1
		`, []any{expiredBefore.UTC()}, &earliest); err != nil {
			return nil, fmt.Errorf("读取最早用量记录: %w", err)
		}
		retainedFrom = todayStart
		if earliest.Valid {
			retainedFrom = earliest.Time.UTC()
		}
		retainedDate = service.GroupUsageDate(retainedFrom)
		startDate = retainedDate
	}

	if _, err := q.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = $1::date,
			retained_from = $2,
			timezone_name = $3,
			building_until = $4::date,
			build_dirty_from = NULL,
			updated_at = NOW()
		WHERE id = 1
	`, startDate, retainedFrom.UTC(), timezoneName, todayDate); err != nil {
		return nil, fmt.Errorf("登记分组用量汇总重建: %w", err)
	}

	startTime, err := service.ParseGroupUsageDate(startDate)
	if err != nil {
		return nil, err
	}
	rowsFrom := startTime.UTC()
	if expiredBefore.After(rowsFrom) {
		rowsFrom = expiredBefore.UTC()
	}
	return &groupUsageRollupBuildPlan{
		startDate:    startDate,
		targetDate:   todayDate,
		retainedDate: retainedDate,
		rowsFrom:     rowsFrom,
		rowsBefore:   todayStart.UTC(),
		timezoneName: timezoneName,
	}, nil
}

// buildGroupUsageRollups 重算 [startDate, targetDate) 的日桶，只写入本地日期不早于 startDate 的桶。
// 状态行只持 FOR KEY SHARE，与 usage_logs 写入和失效触发器兼容；登记与发布的 FOR UPDATE 须等待本事务结束。
func buildGroupUsageRollups(ctx context.Context, q sqlExecutor, plan *groupUsageRollupBuildPlan) error {
	var buildingUntil sql.NullString
	var stateTimezoneName string
	if err := scanSingleRow(ctx, q, `
		SELECT building_until::text, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR KEY SHARE
	`, nil, &buildingUntil, &stateTimezoneName); err != nil {
		return fmt.Errorf("读取分组用量汇总重建登记: %w", err)
	}
	if !buildingUntil.Valid || buildingUntil.String != plan.targetDate || stateTimezoneName != plan.timezoneName {
		return errGroupUsageRollupBuildPreempted
	}

	if _, err := q.ExecContext(ctx, `
		DELETE FROM usage_group_daily_rollups
		WHERE bucket_date < $1::date
			OR bucket_date >= $2::date
	`, plan.retainedDate, plan.startDate); err != nil {
		return fmt.Errorf("清理分组用量日桶: %w", err)
	}

	if _, err := q.ExecContext(ctx, `
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
			AND (created_at AT TIME ZONE $3::text)::date >= $4::date
		GROUP BY 1, 2
		ON CONFLICT (bucket_date, group_id)
		DO UPDATE SET
			actual_cost = EXCLUDED.actual_cost,
			computed_at = EXCLUDED.computed_at
	`, plan.rowsFrom, plan.rowsBefore, plan.timezoneName, plan.startDate); err != nil {
		return fmt.Errorf("重建分组用量日桶: %w", err)
	}
	return nil
}

// publishGroupUsageRollups 在 FOR UPDATE 下发布本轮结果并清除登记，返回是否存在需要重算的脏区间。
//
// 登记之后 [startDate, targetDate) 内的源记录变更都记入了 build_dirty_from，[startDate, build_dirty_from)
// 的日桶与计算快照一致；水位低于 startDate 说明有未记脏的失效，此时发布不越过当前水位。
func publishGroupUsageRollups(ctx context.Context, q sqlExecutor, plan *groupUsageRollupBuildPlan) (bool, error) {
	var closedBefore string
	var buildingUntil sql.NullString
	var dirtyFrom sql.NullString
	if err := scanSingleRow(ctx, q, `
		SELECT closed_before::text, building_until::text, build_dirty_from::text
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`, nil, &closedBefore, &buildingUntil, &dirtyFrom); err != nil {
		return false, fmt.Errorf("读取分组用量汇总重建登记: %w", err)
	}
	if !buildingUntil.Valid || buildingUntil.String != plan.targetDate {
		return false, errGroupUsageRollupBuildPreempted
	}

	publishBefore := plan.targetDate
	dirty := false
	if dirtyFrom.Valid && dirtyFrom.String < publishBefore {
		publishBefore = dirtyFrom.String
		dirty = true
	}
	if closedBefore < plan.startDate && closedBefore < publishBefore {
		publishBefore = closedBefore
		dirty = true
	}

	if _, err := q.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = $1::date,
			building_until = NULL,
			build_dirty_from = NULL,
			updated_at = NOW()
		WHERE id = 1
	`, publishBefore); err != nil {
		return false, fmt.Errorf("发布分组用量汇总水位: %w", err)
	}
	return dirty, nil
}

// expireGroupUsageRollupsBefore 把保留下界推进到 cutoff：早于 cutoff 的 usage_logs 不属于任何可读日桶，
// 删除它们不触发失效。cutoff 须为服务端配置时区的自然日零点，使边界日桶不包含被删除的记录。
func expireGroupUsageRollupsBefore(ctx context.Context, q sqlExecutor, cutoff time.Time) error {
	if _, err := q.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET expired_before = GREATEST(expired_before, $1),
			retained_from = GREATEST(retained_from, $1),
			updated_at = NOW()
		WHERE id = 1
	`, cutoff.UTC()); err != nil {
		return fmt.Errorf("推进分组用量汇总保留下界: %w", err)
	}
	return nil
}

// invalidateGroupUsageRollupsAt 使 affectedAt 所在自然日及之后的日桶失效；早于保留下界的时刻不生效。
func invalidateGroupUsageRollupsAt(ctx context.Context, q sqlExecutor, affectedAt time.Time) error {
	_, err := q.ExecContext(ctx,
		"SELECT invalidate_group_usage_rollups_at($1, $2)",
		affectedAt.UTC(), service.GroupUsageTimezoneName(),
	)
	return err
}
