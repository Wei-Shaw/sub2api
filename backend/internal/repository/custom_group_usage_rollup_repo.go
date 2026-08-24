package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) getAllGroupUsageSummaryFromRollups(ctx context.Context, todayStart time.Time) (results []usagestats.GroupUsageSummary, err error) {
	todayStart = service.GroupUsageTodayStart(todayStart)
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	todayDate := service.GroupUsageDate(todayStart)
	yesterdayDate := service.GroupUsageDate(yesterdayStart)

	// 日桶是不可变的历史沉淀：即便原始日志已被保留期清理，
	// closed_before 以前的桶依旧全部计入累计用量，不随清理缩水。
	const query = `
		WITH state_values AS (
			SELECT
				COUNT(*) = 1
					AND MAX(timezone_name) = $3
					AND MAX(closed_before) <= $4::date AS valid,
				MAX(closed_before) AS closed_before
			FROM usage_group_rollup_state
			WHERE id = 1
		),
		state AS (
			SELECT
				CASE WHEN valid THEN closed_before ELSE DATE '1970-01-01' END AS closed_before,
				CASE
					WHEN valid THEN closed_before::timestamp AT TIME ZONE $3::text
					ELSE TIMESTAMPTZ '1970-01-01 00:00:00+00'
				END AS tail_start,
				valid
			FROM state_values
		),
		historical AS (
			SELECT
				rollup.group_id,
				COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(rollup.actual_cost) FILTER (
					WHERE rollup.bucket_date = $5::date
				), 0) AS yesterday_cost
			FROM usage_group_daily_rollups rollup
			CROSS JOIN state
			WHERE state.valid
				AND rollup.bucket_date < state.closed_before
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
			CROSS JOIN state
			WHERE ul.created_at >= state.tail_start
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
		timezoneName,
		todayDate,
		yesterdayDate,
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

// groupUsageSyncMaxDaysPerRun 限制单次同步推进的天数。
// 回填/时区切换可能横跨很长区间，分块本身不会阻塞写入，但一轮跑太久会挤占
// 定时器与 leader 锁；余量走下个周期，水位已持久化因而可以续跑。
const groupUsageSyncMaxDaysPerRun = 400

// SyncGroupUsageRollups 将服务端配置时区今日以前的用量发布为分组日桶。
//
// 发布必须持有 usage_group_rollup_state 的 FOR UPDATE：重建 SELECT 要在拿到锁之后
// 执行，否则并发的未提交写入既不会被本次重建看见，又会在提交时读到旧水位而不触发
// 失效，那条用量就丢了。而该锁与 INSERT 触发器的 FOR KEY SHARE 冲突，持锁期间
// usage_logs 写入全部阻塞——所以这里按自然日分块，一天一个短事务，
// 把锁持有时间从「整个重建区间」压到「单日重建」，写入在块之间得以通过。
func (r *dashboardAggregationRepository) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	todayStart = service.GroupUsageTodayStart(todayStart)
	db, ok := r.sql.(*sql.DB)
	if !ok {
		// 退化路径（已在外层事务中）：无法分块提交，只能一次性完成。
		_, err := r.syncGroupUsageRollupDayInTx(ctx, todayStart)
		return err
	}

	for range groupUsageSyncMaxDaysPerRun {
		// 每块提交后状态行锁即释放，积压的 usage_logs 写入得以通过。
		done, err := syncGroupUsageRollupDay(ctx, db, todayStart)
		if err != nil {
			// 超时/取消属于正常中断：已完成的天数都已提交，下个周期从断点续跑。
			if ctx.Err() != nil {
				logger.LegacyPrintf(
					"repository.group_usage_rollup",
					"[GroupUsageRollup] 同步中断，余量留待下个周期: %v",
					err,
				)
				return nil
			}
			return err
		}
		if done {
			return nil
		}
	}
	logger.LegacyPrintf(
		"repository.group_usage_rollup",
		"[GroupUsageRollup] 单轮已推进 %d 天仍未追平，余量留待下个周期",
		groupUsageSyncMaxDaysPerRun,
	)
	return nil
}

func syncGroupUsageRollupDay(ctx context.Context, db *sql.DB, todayStart time.Time) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	txRepo := newDashboardAggregationRepositoryWithSQL(tx)
	done, err := txRepo.syncGroupUsageRollupDayInTx(ctx, todayStart)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return done, nil
}

// syncGroupUsageRollupDayInTx 发布水位所指的那一天，并把水位前推一天。
// 返回 done=true 表示已追平今日，无需继续分块。
func (r *dashboardAggregationRepository) syncGroupUsageRollupDayInTx(ctx context.Context, todayStart time.Time) (bool, error) {
	var closedBefore string
	var previousRetainedFrom time.Time
	var stateTimezoneName string
	if err := scanSingleRow(ctx, r.sql, `
		SELECT closed_before::text, retained_from, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`, nil, &closedBefore, &previousRetainedFrom, &stateTimezoneName); err != nil {
		return false, fmt.Errorf("读取分组用量汇总水位: %w", err)
	}

	todayDate := service.GroupUsageDate(todayStart)
	todayDateTime, err := service.ParseGroupUsageDate(todayDate)
	if err != nil {
		return false, err
	}
	timezoneName := service.GroupUsageTimezoneName()
	timezoneChanged := stateTimezoneName != timezoneName
	var closedTime time.Time
	if !timezoneChanged {
		closedTime, err = service.ParseGroupUsageDate(closedBefore)
		if err != nil {
			return false, fmt.Errorf("解析分组用量汇总水位 %q: %w", closedBefore, err)
		}
		if closedTime.After(todayDateTime) {
			return false, fmt.Errorf("分组用量汇总水位位于未来: %s", closedBefore)
		}
		if closedBefore == todayDate {
			return true, nil
		}
	}

	var earliest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, "SELECT MIN(created_at) FROM usage_logs", nil, &earliest); err != nil {
		return false, fmt.Errorf("读取最早用量记录: %w", err)
	}
	retainedFrom := todayStart
	if earliest.Valid {
		retainedFrom = earliest.Time.UTC()
	}
	retainedDate := service.GroupUsageDate(retainedFrom)
	retainedDateTime, err := service.ParseGroupUsageDate(retainedDate)
	if err != nil {
		return false, err
	}
	// 重建下界不能早于 retainedDate：更早的原始日志已被清理，
	// 那段日桶是不可重算的历史沉淀，只能原样保留。
	rebuildStartDate := retainedDate
	rebuildStart := retainedDateTime
	if !timezoneChanged && closedTime.After(retainedDateTime) {
		rebuildStartDate = closedBefore
		rebuildStart = closedTime
	}
	if timezoneChanged && previousRetainedFrom.Before(retainedFrom) {
		logger.LegacyPrintf(
			"repository.group_usage_rollup",
			"[GroupUsageRollup] 时区切换为 %s，%s 以前的日桶因原始日志已归档无法重建，保留旧时区口径",
			timezoneName,
			retainedDate,
		)
	}
	if !rebuildStart.Before(todayDateTime) {
		// 归档屏障已越过今日：没有可发布的历史日，仅把状态对齐到今日。
		return true, r.publishGroupUsageWatermark(ctx, todayDate, todayDate, retainedFrom, timezoneName)
	}

	// 本块只重建 [rebuildStart, nextClosed) 这一个自然日。
	nextClosed := rebuildStart.AddDate(0, 0, 1)
	if nextClosed.After(todayDateTime) {
		nextClosed = todayDateTime
	}
	nextClosedDate := service.GroupUsageDate(nextClosed)

	if err := r.rebuildUsageDailyRollupsForDay(ctx, rebuildStartDate, nextClosedDate, rebuildStart, nextClosed, timezoneName); err != nil {
		return false, err
	}

	done := nextClosedDate == todayDate
	// 追平今日时顺带清掉今日及以后的残桶：当天用量走原始日志尾段，
	// 留着这些桶会与尾段重复计入。
	trimFrom := ""
	if done {
		trimFrom = todayDate
	}
	if err := r.publishGroupUsageWatermark(ctx, nextClosedDate, trimFrom, retainedFrom, timezoneName); err != nil {
		return false, err
	}
	return done, nil
}

// rebuildUsageDailyRollupsForDay 重建一个自然日的用量日桶。
//
// group 与 api_key 是同一批原始日志的两个维度，因此用一次 GROUPING SETS 扫描同时产出，
// 避免为 api_key 再扫一遍 usage_logs（大规模部署下是千万行级别）。两张表在同一事务内更新，
// 共用 usage_group_rollup_state 的水位与归档屏障。
func (r *dashboardAggregationRepository) rebuildUsageDailyRollupsForDay(
	ctx context.Context,
	rebuildStartDate string,
	nextClosedDate string,
	rebuildStart time.Time,
	nextClosed time.Time,
	timezoneName string,
) error {
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_group_daily_rollups
		WHERE bucket_date >= $1::date AND bucket_date < $2::date
	`, rebuildStartDate, nextClosedDate); err != nil {
		return fmt.Errorf("清理分组用量日桶: %w", err)
	}
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_apikey_daily_rollups
		WHERE bucket_date >= $1::date AND bucket_date < $2::date
	`, rebuildStartDate, nextClosedDate); err != nil {
		return fmt.Errorf("清理 API Key 用量日桶: %w", err)
	}

	// 一次扫描出两个维度：GROUPING SETS 的两组分别落到两张表。
	if _, err := r.sql.ExecContext(ctx, `
		WITH agg AS (
			SELECT
				(created_at AT TIME ZONE $3::text)::date AS bucket_date,
				-- GROUPING()=1 表示该列在本分组集里被汇总掉了，用它区分
				-- "被 rollup 置空" 与 "该列本身就是 NULL"。
				GROUPING(group_id) AS group_rolled_up,
				GROUPING(api_key_id) AS apikey_rolled_up,
				group_id,
				api_key_id,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COUNT(*) AS request_count
			FROM usage_logs
			WHERE created_at >= $1
				AND created_at < $2
			GROUP BY GROUPING SETS (
				((created_at AT TIME ZONE $3::text)::date, group_id),
				((created_at AT TIME ZONE $3::text)::date, api_key_id)
			)
		),
		group_rows AS (
			INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
			SELECT bucket_date, group_id, actual_cost, NOW()
			FROM agg
			WHERE group_rolled_up = 0 AND group_id IS NOT NULL
			ON CONFLICT (bucket_date, group_id)
			DO UPDATE SET
				actual_cost = EXCLUDED.actual_cost,
				computed_at = EXCLUDED.computed_at
			RETURNING 1
		)
		INSERT INTO usage_apikey_daily_rollups (bucket_date, api_key_id, actual_cost, request_count, computed_at)
		SELECT bucket_date, api_key_id, actual_cost, request_count, NOW()
		FROM agg
		WHERE apikey_rolled_up = 0 AND api_key_id IS NOT NULL
		ON CONFLICT (bucket_date, api_key_id)
		DO UPDATE SET
			actual_cost = EXCLUDED.actual_cost,
			request_count = EXCLUDED.request_count,
			computed_at = EXCLUDED.computed_at
	`, rebuildStart.UTC(), nextClosed.UTC(), timezoneName); err != nil {
		return fmt.Errorf("重建用量日桶: %w", err)
	}

	return nil
}

// publishGroupUsageWatermark 推进发布水位。trimFrom 非空时一并删除该日期及以后的日桶。
// 归档屏障只增不减：迟到的历史写入不得把它拉回，否则会解除已归档日桶的保护。
func (r *dashboardAggregationRepository) publishGroupUsageWatermark(
	ctx context.Context,
	closedBefore string,
	trimFrom string,
	retainedFrom time.Time,
	timezoneName string,
) error {
	if trimFrom != "" {
		if _, err := r.sql.ExecContext(ctx, `
			DELETE FROM usage_group_daily_rollups
			WHERE bucket_date >= $1::date
		`, trimFrom); err != nil {
			return fmt.Errorf("清理今日及以后的分组用量日桶: %w", err)
		}
		// api_key 日桶与 group 日桶共用水位，今日的量同样走原始日志尾段。
		if _, err := r.sql.ExecContext(ctx, `
			DELETE FROM usage_apikey_daily_rollups
			WHERE bucket_date >= $1::date
		`, trimFrom); err != nil {
			return fmt.Errorf("清理今日及以后的 API Key 用量日桶: %w", err)
		}
	}
	if _, err := r.sql.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = $1::date,
			retained_from = GREATEST(retained_from, $2::timestamptz),
			timezone_name = $3,
			updated_at = NOW()
		WHERE id = 1
	`, closedBefore, retainedFrom, timezoneName); err != nil {
		return fmt.Errorf("更新分组用量汇总水位: %w", err)
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

// invalidateGroupUsageRollupsAt 把发布水位回退到受影响日期，使该日之后的日桶在下次同步时重建。
// 回退不会越过归档屏障：retained_from 之前的原始日志已被清理，那段日桶无法重算。
func invalidateGroupUsageRollupsAt(ctx context.Context, tx *sql.Tx, affectedAt time.Time) error {
	timezoneName := service.GroupUsageTimezoneName()
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = LEAST(
			closed_before,
			GREATEST(
				($1::timestamptz AT TIME ZONE $2::text)::date,
				(retained_from AT TIME ZONE $2::text)::date
			)
		),
			updated_at = NOW()
		WHERE id = 1
	`, affectedAt.UTC(), timezoneName)
	return err
}

// advanceGroupUsageRetention 推进归档屏障，声明 cutoff 之前的原始日志已被清理。
// 归档删除必须先调用它再删数据（能用事务时放进同一事务），失效触发器据此跳过水位回退，
// 从而让已发布的历史日桶保持不变、不被重算。屏障单调递增，可重复调用。
func advanceGroupUsageRetention(ctx context.Context, exec sqlExecutor, cutoff time.Time) error {
	// 月分区按 UTC 边界切分，该边界可能落在配置时区的一天中间。屏障必须
	// 上取整到下一个本地日边界，冻结包含已删除行的整个日桶。
	localDayStart := timezone.StartOfDay(cutoff)
	archiveBoundary := localDayStart
	if !cutoff.Equal(localDayStart) {
		archiveBoundary = localDayStart.AddDate(0, 0, 1)
	}
	if _, err := exec.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET retained_from = GREATEST(retained_from, $1::timestamptz),
			updated_at = NOW()
		WHERE id = 1
	`, archiveBoundary.UTC()); err != nil {
		return fmt.Errorf("推进分组用量归档屏障: %w", err)
	}
	return nil
}
