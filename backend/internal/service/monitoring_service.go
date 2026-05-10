package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type MonitoringService struct {
	db *sql.DB
}

func NewMonitoringService(db *sql.DB) *MonitoringService {
	return &MonitoringService{db: db}
}

type GroupHealth struct {
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`
	TotalAccounts  int    `json:"total_accounts"`
	ActiveAccounts int    `json:"active_accounts"`
	ErrorAccounts  int    `json:"error_accounts"`
	RateLimited    int    `json:"rate_limited"`
	Overload       int    `json:"overload"`
	Disabled       int    `json:"disabled"`
}

type ModelLatency struct {
	Model           string  `json:"model"`
	RequestCount    int     `json:"request_count"`
	SuccessCount    int     `json:"success_count"`
	ErrorCount      int     `json:"error_count"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	P50LatencyMs    float64 `json:"p50_latency_ms"`
	P95LatencyMs    float64 `json:"p95_latency_ms"`
	P99LatencyMs    float64 `json:"p99_latency_ms"`
	AvgFirstTokenMs float64 `json:"avg_first_token_ms"`
}

type GroupModelStats struct {
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	Model        string  `json:"model"`
	RequestCount int     `json:"request_count"`
	SuccessCount int     `json:"success_count"`
	ErrorCount   int     `json:"error_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	AvgTTFT      float64 `json:"avg_ttft"`
}

type ErrorAccount struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	GroupName     string `json:"group_name"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message"`
	RateLimitedAt string `json:"rate_limited_at,omitempty"`
	OverloadUntil string `json:"overload_until,omitempty"`
}

type HourlyStats struct {
	Hour    string `json:"hour"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
}

type ModelHourlyStats struct {
	GroupID int64  `json:"group_id"`
	Model   string `json:"model"`
	Hour    string `json:"hour"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
}

type MonitoringOverview struct {
	Groups           []GroupHealth      `json:"groups"`
	GroupModels      []GroupModelStats  `json:"group_models"`
	ModelLatencies   []ModelLatency     `json:"model_latencies"`
	ErrorAccounts    []ErrorAccount     `json:"error_accounts"`
	HourlyStats      []HourlyStats      `json:"hourly_stats"`
	ModelHourlyStats []ModelHourlyStats `json:"model_hourly_stats"`
	TotalRequests    int64              `json:"total_requests_today"`
	SuccessCount     int64              `json:"success_count_today"`
	ErrorCount       int64              `json:"error_count_today"`
	AvgLatencyMs     float64            `json:"avg_latency_ms_today"`
}

type MonitoringSummary struct {
	Groups        []GroupHealth  `json:"groups"`
	ErrorAccounts []ErrorAccount `json:"error_accounts"`
	HourlyStats   []HourlyStats  `json:"hourly_stats"`
	TotalRequests int64          `json:"total_requests_today"`
	SuccessCount  int64          `json:"success_count_today"`
	ErrorCount    int64          `json:"error_count_today"`
	AvgLatencyMs  float64        `json:"avg_latency_ms_today"`
}

type MonitoringGroupModels struct {
	GroupModels      []GroupModelStats  `json:"group_models"`
	ModelHourlyStats []ModelHourlyStats `json:"model_hourly_stats"`
}

type MonitoringModelLatency struct {
	ModelLatencies []ModelLatency `json:"model_latencies"`
}

func (s *MonitoringService) GetOverview(ctx context.Context) (*MonitoringOverview, error) {
	overview := &MonitoringOverview{}
	if err := s.queryTodaySummary(ctx, overview); err != nil {
		return nil, fmt.Errorf("query today summary: %w", err)
	}
	if err := s.queryGroupHealth(ctx, overview); err != nil {
		return nil, fmt.Errorf("query group health: %w", err)
	}
	if err := s.queryGroupModelStats(ctx, overview); err != nil {
		return nil, fmt.Errorf("query group model stats: %w", err)
	}
	if err := s.queryModelLatency(ctx, overview); err != nil {
		return nil, fmt.Errorf("query model latency: %w", err)
	}
	if err := s.queryErrorAccounts(ctx, overview); err != nil {
		return nil, fmt.Errorf("query error accounts: %w", err)
	}
	if err := s.queryHourlyStats(ctx, overview); err != nil {
		return nil, fmt.Errorf("query hourly stats: %w", err)
	}
	if err := s.queryModelHourlyStats(ctx, overview); err != nil {
		return nil, fmt.Errorf("query model hourly stats: %w", err)
	}
	return overview, nil
}

func (s *MonitoringService) GetSummary(ctx context.Context) (*MonitoringSummary, error) {
	tmp := &MonitoringOverview{}
	if err := s.queryTodaySummary(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query today summary: %w", err)
	}
	if err := s.queryGroupHealth(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query group health: %w", err)
	}
	if err := s.queryErrorAccounts(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query error accounts: %w", err)
	}
	if err := s.queryHourlyStats(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query hourly stats: %w", err)
	}
	return &MonitoringSummary{
		Groups:        tmp.Groups,
		ErrorAccounts: tmp.ErrorAccounts,
		HourlyStats:   tmp.HourlyStats,
		TotalRequests: tmp.TotalRequests,
		SuccessCount:  tmp.SuccessCount,
		ErrorCount:    tmp.ErrorCount,
		AvgLatencyMs:  tmp.AvgLatencyMs,
	}, nil
}

func (s *MonitoringService) GetGroupModels(ctx context.Context) (*MonitoringGroupModels, error) {
	tmp := &MonitoringOverview{}
	if err := s.queryGroupModelStats(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query group model stats: %w", err)
	}
	if err := s.queryModelHourlyStats(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query model hourly stats: %w", err)
	}
	return &MonitoringGroupModels{
		GroupModels:      tmp.GroupModels,
		ModelHourlyStats: tmp.ModelHourlyStats,
	}, nil
}

func (s *MonitoringService) GetModelLatency(ctx context.Context) (*MonitoringModelLatency, error) {
	tmp := &MonitoringOverview{}
	if err := s.queryModelLatency(ctx, tmp); err != nil {
		return nil, fmt.Errorf("query model latency: %w", err)
	}
	return &MonitoringModelLatency{
		ModelLatencies: tmp.ModelLatencies,
	}, nil
}

func (s *MonitoringService) queryGroupHealth(ctx context.Context, overview *MonitoringOverview) error {
	now := time.Now().UTC()
	query := `
		SELECT
			g.id,
			COALESCE(g.name, ''),
			COUNT(DISTINCT ag.account_id),
			COUNT(DISTINCT ag.account_id) FILTER (WHERE a.status = 'active' AND a.schedulable = true
				AND (a.rate_limited_at IS NULL OR a.rate_limit_reset_at <= $1)
				AND (a.overload_until IS NULL OR a.overload_until <= $1)
				AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= $1)),
			COUNT(DISTINCT ag.account_id) FILTER (WHERE a.status = 'error'),
			COUNT(DISTINCT ag.account_id) FILTER (WHERE a.rate_limited_at IS NOT NULL AND a.rate_limit_reset_at > $1),
			COUNT(DISTINCT ag.account_id) FILTER (WHERE a.overload_until IS NOT NULL AND a.overload_until > $1),
			COUNT(DISTINCT ag.account_id) FILTER (WHERE a.status = 'disabled' OR a.schedulable = false)
		FROM groups g
		LEFT JOIN account_groups ag ON ag.group_id = g.id
		LEFT JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
		WHERE g.deleted_at IS NULL
		GROUP BY g.id, g.name
		ORDER BY g.name`

	rows, err := s.db.QueryContext(ctx, query, now)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var g GroupHealth
		if err := rows.Scan(&g.GroupID, &g.GroupName, &g.TotalAccounts, &g.ActiveAccounts, &g.ErrorAccounts, &g.RateLimited, &g.Overload, &g.Disabled); err != nil {
			return err
		}
		overview.Groups = append(overview.Groups, g)
	}
	return rows.Err()
}

func (s *MonitoringService) queryGroupModelStats(ctx context.Context, overview *MonitoringOverview) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	query := `
		SELECT
			g.id,
			COALESCE(g.name, ''),
			COALESCE(u.requested_model, u.model),
			COUNT(*) AS cnt,
			COUNT(*) FILTER (WHERE u.output_tokens > 0 OR COALESCE(u.image_count, 0) > 0) AS success_cnt,
			COUNT(*) FILTER (WHERE u.output_tokens = 0 AND COALESCE(u.image_count, 0) = 0) AS error_cnt,
			COALESCE(AVG(u.duration_ms) FILTER (WHERE u.duration_ms IS NOT NULL AND u.duration_ms > 0), 0)::float8,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY CASE WHEN u.duration_ms IS NOT NULL AND u.duration_ms > 0 THEN u.duration_ms END), 0)::float8,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY CASE WHEN u.duration_ms IS NOT NULL AND u.duration_ms > 0 THEN u.duration_ms END), 0)::float8,
			COALESCE(AVG(u.first_token_ms) FILTER (WHERE u.first_token_ms IS NOT NULL AND u.first_token_ms > 0), 0)::float8
		FROM usage_logs u
		JOIN groups g ON u.group_id = g.id
		WHERE u.created_at >= $1
		  AND g.deleted_at IS NULL
		  AND COALESCE(u.requested_model, u.model) IS NOT NULL
		GROUP BY g.id, g.name, COALESCE(u.requested_model, u.model)
		HAVING COUNT(*) > 0
		ORDER BY g.name, cnt DESC`

	rows, err := s.db.QueryContext(ctx, query, since)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var m GroupModelStats
		if err := rows.Scan(&m.GroupID, &m.GroupName, &m.Model, &m.RequestCount, &m.SuccessCount, &m.ErrorCount, &m.AvgLatencyMs, &m.P50LatencyMs, &m.P95LatencyMs, &m.AvgTTFT); err != nil {
			return err
		}
		overview.GroupModels = append(overview.GroupModels, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	errQ := `
		SELECT
			e.group_id,
			COALESCE(e.requested_model, e.model),
			COUNT(*) AS err_cnt
		FROM ops_error_logs e
		WHERE e.created_at >= $1
		  AND e.is_count_tokens = false
		  AND e.group_id IS NOT NULL
		  AND COALESCE(e.requested_model, e.model) IS NOT NULL
		GROUP BY e.group_id, COALESCE(e.requested_model, e.model)`

	errRows, err := s.db.QueryContext(ctx, errQ, since)
	if err != nil {
		return err
	}
	defer errRows.Close()

	type groupModelKey struct {
		groupID int64
		model   string
	}
	errMap := make(map[groupModelKey]int)
	for errRows.Next() {
		var groupID int64
		var model string
		var cnt int
		if err := errRows.Scan(&groupID, &model, &cnt); err != nil {
			return err
		}
		errMap[groupModelKey{groupID, model}] = cnt
	}
	if err := errRows.Err(); err != nil {
		return err
	}

	if len(errMap) > 0 {
		existing := make(map[groupModelKey]int)
		for i, m := range overview.GroupModels {
			existing[groupModelKey{m.GroupID, m.Model}] = i
		}
		for key, cnt := range errMap {
			if idx, ok := existing[key]; ok {
				overview.GroupModels[idx].ErrorCount += cnt
				overview.GroupModels[idx].RequestCount += cnt
			} else {
				var groupName string
				s.db.QueryRowContext(ctx, `SELECT COALESCE(name, '') FROM groups WHERE id = $1`, key.groupID).Scan(&groupName)
				overview.GroupModels = append(overview.GroupModels, GroupModelStats{
					GroupID:      key.groupID,
					GroupName:    groupName,
					Model:        key.model,
					RequestCount: cnt,
					SuccessCount: 0,
					ErrorCount:   cnt,
				})
			}
		}
	}

	return nil
}

func (s *MonitoringService) queryModelLatency(ctx context.Context, overview *MonitoringOverview) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	query := `
		SELECT
			COALESCE(requested_model, model),
			COUNT(*) as cnt,
			COUNT(*) FILTER (WHERE output_tokens > 0 OR COALESCE(image_count, 0) > 0) as success_cnt,
			COUNT(*) FILTER (WHERE output_tokens = 0 AND COALESCE(image_count, 0) = 0) as error_cnt,
			COALESCE(AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL AND duration_ms > 0), 0)::float8,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN duration_ms END), 0)::float8,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN duration_ms END), 0)::float8,
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN duration_ms END), 0)::float8,
			COALESCE(AVG(first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND first_token_ms > 0), 0)::float8
		FROM usage_logs
		WHERE created_at >= $1
		  AND COALESCE(requested_model, model) IS NOT NULL
		GROUP BY COALESCE(requested_model, model)
		ORDER BY cnt DESC
		LIMIT 30`

	rows, err := s.db.QueryContext(ctx, query, since)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var m ModelLatency
		if err := rows.Scan(&m.Model, &m.RequestCount, &m.SuccessCount, &m.ErrorCount, &m.AvgLatencyMs, &m.P50LatencyMs, &m.P95LatencyMs, &m.P99LatencyMs, &m.AvgFirstTokenMs); err != nil {
			return err
		}
		overview.ModelLatencies = append(overview.ModelLatencies, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	errQ := `
		SELECT
			COALESCE(requested_model, model),
			COUNT(*) AS err_cnt
		FROM ops_error_logs
		WHERE created_at >= $1
		  AND is_count_tokens = false
		  AND COALESCE(requested_model, model) IS NOT NULL
		GROUP BY COALESCE(requested_model, model)`

	errRows, err := s.db.QueryContext(ctx, errQ, since)
	if err != nil {
		return err
	}
	defer errRows.Close()

	errMap := make(map[string]int)
	for errRows.Next() {
		var model string
		var cnt int
		if err := errRows.Scan(&model, &cnt); err != nil {
			return err
		}
		errMap[model] = cnt
	}
	if err := errRows.Err(); err != nil {
		return err
	}

	if len(errMap) > 0 {
		existing := make(map[string]int)
		for i, m := range overview.ModelLatencies {
			existing[m.Model] = i
		}
		for model, cnt := range errMap {
			if idx, ok := existing[model]; ok {
				overview.ModelLatencies[idx].ErrorCount += cnt
				overview.ModelLatencies[idx].RequestCount += cnt
			} else {
				overview.ModelLatencies = append(overview.ModelLatencies, ModelLatency{
					Model:        model,
					RequestCount: cnt,
					ErrorCount:   cnt,
				})
			}
		}
	}

	return nil
}

func (s *MonitoringService) queryErrorAccounts(ctx context.Context, overview *MonitoringOverview) error {
	query := `
		SELECT
			a.id,
			COALESCE(a.name, ''),
			COALESCE(g.name, ''),
			a.status,
			COALESCE(a.error_message, ''),
			a.rate_limited_at,
			a.overload_until
		FROM accounts a
		LEFT JOIN account_groups ag ON ag.account_id = a.id
		LEFT JOIN groups g ON g.id = ag.group_id
		WHERE a.deleted_at IS NULL
		  AND (a.status = 'error'
		       OR (a.rate_limited_at IS NOT NULL AND a.rate_limit_reset_at > NOW())
		       OR (a.overload_until IS NOT NULL AND a.overload_until > NOW()))
		ORDER BY
			CASE a.status WHEN 'error' THEN 0 ELSE 1 END,
			a.rate_limited_at DESC NULLS LAST
		LIMIT 50`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var e ErrorAccount
		var rateLimitedAt, overloadUntil sql.NullTime
		if err := rows.Scan(&e.AccountID, &e.AccountName, &e.GroupName, &e.Status, &e.ErrorMessage, &rateLimitedAt, &overloadUntil); err != nil {
			return err
		}
		if rateLimitedAt.Valid {
			e.RateLimitedAt = rateLimitedAt.Time.Format("2006-01-02 15:04:05")
		}
		if overloadUntil.Valid {
			e.OverloadUntil = overloadUntil.Time.Format("2006-01-02 15:04:05")
		}
		overview.ErrorAccounts = append(overview.ErrorAccounts, e)
	}
	return rows.Err()
}

func (s *MonitoringService) queryTodaySummary(ctx context.Context, overview *MonitoringOverview) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	query := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE output_tokens > 0 OR COALESCE(image_count, 0) > 0),
			COUNT(*) FILTER (WHERE output_tokens = 0 AND COALESCE(image_count, 0) = 0),
			COALESCE(AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL AND duration_ms > 0), 0)::float8
		FROM usage_logs
		WHERE created_at >= $1`

	if err := s.db.QueryRowContext(ctx, query, since).Scan(&overview.TotalRequests, &overview.SuccessCount, &overview.ErrorCount, &overview.AvgLatencyMs); err != nil {
		return err
	}

	var opsErrorCount int64
	errQ := `SELECT COUNT(*) FROM ops_error_logs WHERE created_at >= $1 AND is_count_tokens = false AND group_id IS NOT NULL AND COALESCE(requested_model, model) IS NOT NULL`
	if err := s.db.QueryRowContext(ctx, errQ, since).Scan(&opsErrorCount); err != nil {
		return err
	}
	overview.ErrorCount += opsErrorCount
	overview.TotalRequests += opsErrorCount

	return nil
}

func (s *MonitoringService) queryHourlyStats(ctx context.Context, overview *MonitoringOverview) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	query := `
		WITH usage_hours AS (
			SELECT
				DATE_TRUNC('hour', created_at) AS hour,
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE output_tokens > 0) AS success
			FROM usage_logs
			WHERE created_at >= $1
			GROUP BY DATE_TRUNC('hour', created_at)
		),
		error_hours AS (
			SELECT
				DATE_TRUNC('hour', created_at) AS hour,
				COUNT(*) AS total
			FROM ops_error_logs
			WHERE created_at >= $1
			  AND is_count_tokens = false
			  AND group_id IS NOT NULL
			  AND COALESCE(requested_model, model) IS NOT NULL
			GROUP BY DATE_TRUNC('hour', created_at)
		)
		SELECT
			COALESCE(u.hour, e.hour) AS hour,
			COALESCE(u.total, 0) + COALESCE(e.total, 0) AS total,
			COALESCE(u.success, 0) AS success
		FROM usage_hours u
		FULL OUTER JOIN error_hours e ON u.hour = e.hour
		ORDER BY hour`

	rows, err := s.db.QueryContext(ctx, query, since)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var h HourlyStats
		if err := rows.Scan(&h.Hour, &h.Total, &h.Success); err != nil {
			return err
		}
		overview.HourlyStats = append(overview.HourlyStats, h)
	}
	return rows.Err()
}

func (s *MonitoringService) queryModelHourlyStats(ctx context.Context, overview *MonitoringOverview) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	query := `
		WITH usage_hours AS (
			SELECT
				u.group_id,
				COALESCE(u.requested_model, u.model) AS model,
				DATE_TRUNC('hour', u.created_at) AS hour,
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE u.output_tokens > 0) AS success
			FROM usage_logs u
			WHERE u.created_at >= $1
			  AND u.group_id IS NOT NULL
			  AND COALESCE(u.requested_model, u.model) IS NOT NULL
			GROUP BY u.group_id, COALESCE(u.requested_model, u.model), DATE_TRUNC('hour', u.created_at)
		),
		error_hours AS (
			SELECT
				e.group_id,
				COALESCE(e.requested_model, e.model) AS model,
				DATE_TRUNC('hour', e.created_at) AS hour,
				COUNT(*) AS total
			FROM ops_error_logs e
			WHERE e.created_at >= $1
			  AND e.is_count_tokens = false
			  AND e.group_id IS NOT NULL
			  AND COALESCE(e.requested_model, e.model) IS NOT NULL
			GROUP BY e.group_id, COALESCE(e.requested_model, e.model), DATE_TRUNC('hour', e.created_at)
		)
		SELECT
			COALESCE(u.group_id, e.group_id) AS group_id,
			COALESCE(u.model, e.model) AS model,
			COALESCE(u.hour, e.hour) AS hour,
			COALESCE(u.total, 0) + COALESCE(e.total, 0) AS total,
			COALESCE(u.success, 0) AS success
		FROM usage_hours u
		FULL OUTER JOIN error_hours e
			ON u.group_id = e.group_id AND u.model = e.model AND u.hour = e.hour
		ORDER BY group_id, model, hour`

	rows, err := s.db.QueryContext(ctx, query, since)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var m ModelHourlyStats
		if err := rows.Scan(&m.GroupID, &m.Model, &m.Hour, &m.Total, &m.Success); err != nil {
			return err
		}
		overview.ModelHourlyStats = append(overview.ModelHourlyStats, m)
	}
	return rows.Err()
}
