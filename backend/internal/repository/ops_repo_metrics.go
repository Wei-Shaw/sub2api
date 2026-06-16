package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) InsertSystemMetrics(ctx context.Context, input *service.OpsInsertSystemMetricsInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}

	window := input.WindowMinutes
	if window <= 0 {
		window = 1
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	q := `
INSERT INTO ops_system_metrics (
  created_at,
  window_minutes,
  platform,
  group_id,

  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,

  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,

  token_consumed,
  account_switch_count,
  qps,
  tps,

  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,

  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,

  cpu_usage_percent,
  memory_used_mb,
  memory_total_mb,
  memory_usage_percent,

  db_ok,
  redis_ok,

  redis_conn_total,
  redis_conn_idle,

  db_conn_active,
  db_conn_idle,
  db_conn_waiting,

  goroutine_count,
  concurrency_queue_depth
) VALUES (
  $1,$2,$3,$4,
  $5,$6,$7,$8,
  $9,$10,$11,
  $12,$13,$14,$15,
  $16,$17,$18,$19,$20,$21,
  $22,$23,$24,$25,$26,$27,
  $28,$29,$30,$31,
  $32,$33,
  $34,$35,
  $36,$37,$38,
  $39,$40
)`

	_, err := r.db.ExecContext(
		ctx,
		q,
		createdAt,
		window,
		opsNullString(input.Platform),
		opsNullInt64(input.GroupID),

		input.SuccessCount,
		input.ErrorCountTotal,
		input.BusinessLimitedCount,
		input.ErrorCountSLA,

		input.UpstreamErrorCountExcl429529,
		input.Upstream429Count,
		input.Upstream529Count,

		input.TokenConsumed,
		input.AccountSwitchCount,
		opsNullFloat64(input.QPS),
		opsNullFloat64(input.TPS),

		opsNullInt(input.DurationP50Ms),
		opsNullInt(input.DurationP90Ms),
		opsNullInt(input.DurationP95Ms),
		opsNullInt(input.DurationP99Ms),
		opsNullFloat64(input.DurationAvgMs),
		opsNullInt(input.DurationMaxMs),

		opsNullInt(input.TTFTP50Ms),
		opsNullInt(input.TTFTP90Ms),
		opsNullInt(input.TTFTP95Ms),
		opsNullInt(input.TTFTP99Ms),
		opsNullFloat64(input.TTFTAvgMs),
		opsNullInt(input.TTFTMaxMs),

		opsNullFloat64(input.CPUUsagePercent),
		opsNullInt(input.MemoryUsedMB),
		opsNullInt(input.MemoryTotalMB),
		opsNullFloat64(input.MemoryUsagePercent),

		opsNullBool(input.DBOK),
		opsNullBool(input.RedisOK),

		opsNullInt(input.RedisConnTotal),
		opsNullInt(input.RedisConnIdle),

		opsNullInt(input.DBConnActive),
		opsNullInt(input.DBConnIdle),
		opsNullInt(input.DBConnWaiting),

		opsNullInt(input.GoroutineCount),
		opsNullInt(input.ConcurrencyQueueDepth),
	)
	return err
}

func (r *opsRepository) GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*service.OpsSystemMetricsSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}

	q := `
SELECT
  id,
  created_at,
  window_minutes,

  cpu_usage_percent,
  memory_used_mb,
  memory_total_mb,
  memory_usage_percent,

  db_ok,
  redis_ok,

  redis_conn_total,
  redis_conn_idle,

  db_conn_active,
  db_conn_idle,
  db_conn_waiting,

  goroutine_count,
  concurrency_queue_depth,
  account_switch_count
FROM ops_system_metrics
WHERE window_minutes = $1
  AND platform IS NULL
  AND group_id IS NULL
ORDER BY created_at DESC
LIMIT 1`

	var out service.OpsSystemMetricsSnapshot
	var cpu sql.NullFloat64
	var memUsed sql.NullInt64
	var memTotal sql.NullInt64
	var memPct sql.NullFloat64
	var dbOK sql.NullBool
	var redisOK sql.NullBool
	var redisTotal sql.NullInt64
	var redisIdle sql.NullInt64
	var dbActive sql.NullInt64
	var dbIdle sql.NullInt64
	var dbWaiting sql.NullInt64
	var goroutines sql.NullInt64
	var queueDepth sql.NullInt64
	var accountSwitchCount sql.NullInt64

	if err := r.db.QueryRowContext(ctx, q, windowMinutes).Scan(
		&out.ID,
		&out.CreatedAt,
		&out.WindowMinutes,
		&cpu,
		&memUsed,
		&memTotal,
		&memPct,
		&dbOK,
		&redisOK,
		&redisTotal,
		&redisIdle,
		&dbActive,
		&dbIdle,
		&dbWaiting,
		&goroutines,
		&queueDepth,
		&accountSwitchCount,
	); err != nil {
		return nil, err
	}

	if cpu.Valid {
		v := cpu.Float64
		out.CPUUsagePercent = &v
	}
	if memUsed.Valid {
		v := memUsed.Int64
		out.MemoryUsedMB = &v
	}
	if memTotal.Valid {
		v := memTotal.Int64
		out.MemoryTotalMB = &v
	}
	if memPct.Valid {
		v := memPct.Float64
		out.MemoryUsagePercent = &v
	}
	if dbOK.Valid {
		v := dbOK.Bool
		out.DBOK = &v
	}
	if redisOK.Valid {
		v := redisOK.Bool
		out.RedisOK = &v
	}
	if redisTotal.Valid {
		v := int(redisTotal.Int64)
		out.RedisConnTotal = &v
	}
	if redisIdle.Valid {
		v := int(redisIdle.Int64)
		out.RedisConnIdle = &v
	}
	if dbActive.Valid {
		v := int(dbActive.Int64)
		out.DBConnActive = &v
	}
	if dbIdle.Valid {
		v := int(dbIdle.Int64)
		out.DBConnIdle = &v
	}
	if dbWaiting.Valid {
		v := int(dbWaiting.Int64)
		out.DBConnWaiting = &v
	}
	if goroutines.Valid {
		v := int(goroutines.Int64)
		out.GoroutineCount = &v
	}
	if queueDepth.Valid {
		v := int(queueDepth.Int64)
		out.ConcurrencyQueueDepth = &v
	}
	if accountSwitchCount.Valid {
		v := accountSwitchCount.Int64
		out.AccountSwitchCount = &v
	}

	return &out, nil
}

func (r *opsRepository) UpsertJobHeartbeat(ctx context.Context, input *service.OpsUpsertJobHeartbeatInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}
	if input.JobName == "" {
		return fmt.Errorf("job_name required")
	}

	q := `
INSERT INTO ops_job_heartbeats (
  job_name,
  last_run_at,
  last_success_at,
  last_error_at,
  last_error,
  last_duration_ms,
  last_result,
  updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,NOW()
)
ON CONFLICT (job_name) DO UPDATE SET
  last_run_at = COALESCE(EXCLUDED.last_run_at, ops_job_heartbeats.last_run_at),
  last_success_at = COALESCE(EXCLUDED.last_success_at, ops_job_heartbeats.last_success_at),
  last_error_at = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN NULL
    ELSE COALESCE(EXCLUDED.last_error_at, ops_job_heartbeats.last_error_at)
  END,
  last_error = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN NULL
    ELSE COALESCE(EXCLUDED.last_error, ops_job_heartbeats.last_error)
  END,
  last_duration_ms = COALESCE(EXCLUDED.last_duration_ms, ops_job_heartbeats.last_duration_ms),
  last_result = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN COALESCE(EXCLUDED.last_result, ops_job_heartbeats.last_result)
    ELSE ops_job_heartbeats.last_result
  END,
  updated_at = NOW()`

	_, err := r.db.ExecContext(
		ctx,
		q,
		input.JobName,
		opsNullTime(input.LastRunAt),
		opsNullTime(input.LastSuccessAt),
		opsNullTime(input.LastErrorAt),
		opsNullString(input.LastError),
		opsNullInt(input.LastDurationMs),
		opsNullString(input.LastResult),
	)
	return err
}

func (r *opsRepository) ListJobHeartbeats(ctx context.Context) ([]*service.OpsJobHeartbeat, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	q := `
SELECT
  job_name,
  last_run_at,
  last_success_at,
  last_error_at,
  last_error,
  last_duration_ms,
  last_result,
  updated_at
FROM ops_job_heartbeats
ORDER BY job_name ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsJobHeartbeat, 0, 8)
	for rows.Next() {
		var item service.OpsJobHeartbeat
		var lastRun sql.NullTime
		var lastSuccess sql.NullTime
		var lastErrorAt sql.NullTime
		var lastError sql.NullString
		var lastDuration sql.NullInt64

		var lastResult sql.NullString

		if err := rows.Scan(
			&item.JobName,
			&lastRun,
			&lastSuccess,
			&lastErrorAt,
			&lastError,
			&lastDuration,
			&lastResult,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if lastRun.Valid {
			v := lastRun.Time
			item.LastRunAt = &v
		}
		if lastSuccess.Valid {
			v := lastSuccess.Time
			item.LastSuccessAt = &v
		}
		if lastErrorAt.Valid {
			v := lastErrorAt.Time
			item.LastErrorAt = &v
		}
		if lastError.Valid {
			v := lastError.String
			item.LastError = &v
		}
		if lastDuration.Valid {
			v := lastDuration.Int64
			item.LastDurationMs = &v
		}
		if lastResult.Valid {
			v := lastResult.String
			item.LastResult = &v
		}

		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func opsNullBool(v *bool) any {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

func opsNullFloat64(v *float64) any {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

func opsNullTime(v *time.Time) any {
	if v == nil || v.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *v, Valid: true}
}
