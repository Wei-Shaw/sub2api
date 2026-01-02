package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultWindowMinutes = 1

	MaxErrorLogsLimit     = 500
	DefaultErrorLogsLimit = 200

	MaxRecentSystemMetricsLimit     = 500
	DefaultRecentSystemMetricsLimit = 60

	MaxMetricsLimit     = 5000
	DefaultMetricsLimit = 300
)

type OpsRepository struct {
	sql sqlExecutor
	rdb *redis.Client
REDACTED

func NewOpsRepository(_ *dbent.Client, sqlDB *sql.DB, rdb *redis.Client) service.OpsRepository {
	return &OpsRepository{sql: sqlDB, rdb: rdbREDACTED
REDACTED

func (r *OpsRepository) CreateErrorLog(ctx context.Context, log *service.OpsErrorLog) error {
	if log == nil {
		return nil
REDACTED

	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
REDACTED

	query := `
		INSERT INTO ops_error_logs (
			request_id,
			user_id,
			api_key_id,
			account_id,
			group_id,
			client_ip,
			error_phase,
			error_type,
			severity,
			status_code,
			platform,
			model,
			request_path,
			stream,
			error_message,
			duration_ms,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17
		)
		RETURNING id, created_at
	`

	requestID := nullString(log.RequestID)
	clientIP := nullString(log.ClientIP)
	platform := nullString(log.Platform)
	model := nullString(log.Model)
	requestPath := nullString(log.RequestPath)
	message := nullString(log.Message)
	latency := nullInt(log.LatencyMs)

	args := []any{
		requestID,
		nullInt64(log.UserID),
		nullInt64(log.APIKeyID),
		nullInt64(log.AccountID),
		nullInt64(log.GroupID),
		clientIP,
		log.Phase,
		log.Type,
		log.Severity,
		log.StatusCode,
		platform,
		model,
		requestPath,
		log.Stream,
		message,
		latency,
		createdAt,
REDACTED

	if err := scanSingleRow(ctx, r.sql, query, args, &log.ID, &log.CreatedAt); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (r *OpsRepository) ListErrorLogsLegacy(ctx context.Context, filters service.OpsErrorLogFilters) ([]service.OpsErrorLog, error) {
	conditions := make([]string, 0)
	args := make([]any, 0)

	addCondition := func(condition string, values ...any) {
		conditions = append(conditions, condition)
		args = append(args, values...)
REDACTED

	if filters.StartTime != nil {
		addCondition(fmt.Sprintf("created_at >= $%d", len(args)+1), *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		addCondition(fmt.Sprintf("created_at <= $%d", len(args)+1), *filters.EndTime)
REDACTED
	if filters.Platform != "" {
		addCondition(fmt.Sprintf("platform = $%d", len(args)+1), filters.Platform)
REDACTED
	if filters.Phase != "" {
		addCondition(fmt.Sprintf("error_phase = $%d", len(args)+1), filters.Phase)
REDACTED
	if filters.Severity != "" {
		addCondition(fmt.Sprintf("severity = $%d", len(args)+1), filters.Severity)
REDACTED
	if filters.Query != "" {
		like := "%" + strings.ToLower(filters.Query) + "%"
		startIdx := len(args) + 1
		addCondition(
			fmt.Sprintf("(LOWER(request_id) LIKE $%d OR LOWER(model) LIKE $%d OR LOWER(error_message) LIKE $%d OR LOWER(error_type) LIKE $%d)",
				startIdx, startIdx+1, startIdx+2, startIdx+3,
			),
			like, like, like, like,
		)
REDACTED

	limit := filters.Limit
	if limit <= 0 || limit > MaxErrorLogsLimit {
		limit = DefaultErrorLogsLimit
REDACTED

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
REDACTED

	query := fmt.Sprintf(`
		SELECT
			id,
			created_at,
			user_id,
			api_key_id,
			account_id,
			group_id,
			client_ip,
			error_phase,
			error_type,
			severity,
			status_code,
			platform,
			model,
			request_path,
			stream,
			duration_ms,
			request_id,
			error_message
		FROM ops_error_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d
	`, where, len(args)+1)

	args = append(args, limit)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]service.OpsErrorLog, 0)
	for rows.Next() {
		logEntry, err := scanOpsErrorLog(rows)
		if err != nil {
			return nil, err
	REDACTED
		results = append(results, *logEntry)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) GetLatestSystemMetric(ctx context.Context) (*service.OpsMetrics, error) {
	query := `
		SELECT
			window_minutes,
			request_count,
			success_count,
			error_count,
			success_rate,
			error_rate,
			p95_latency_ms,
			p99_latency_ms,
			http2_errors,
			active_alerts,
			cpu_usage_percent,
			memory_used_mb,
			memory_total_mb,
			memory_usage_percent,
			heap_alloc_mb,
			gc_pause_ms,
			concurrency_queue_depth,
			created_at AS updated_at
		FROM ops_system_metrics
		WHERE window_minutes = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT 1
	`

	var windowMinutes sql.NullInt64
	var requestCount, successCount, errorCount sql.NullInt64
	var successRate, errorRate sql.NullFloat64
	var p95Latency, p99Latency, http2Errors, activeAlerts sql.NullInt64
	var cpuUsage, memoryUsage, gcPause sql.NullFloat64
	var memoryUsed, memoryTotal, heapAlloc, queueDepth sql.NullInt64
	var createdAt time.Time
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{DefaultWindowMinutesREDACTED,
		&windowMinutes,
		&requestCount,
		&successCount,
		&errorCount,
		&successRate,
		&errorRate,
		&p95Latency,
		&p99Latency,
		&http2Errors,
		&activeAlerts,
		&cpuUsage,
		&memoryUsed,
		&memoryTotal,
		&memoryUsage,
		&heapAlloc,
		&gcPause,
		&queueDepth,
		&createdAt,
	); err != nil {
		return nil, err
REDACTED

	metric := &service.OpsMetrics{
		UpdatedAt: createdAt,
REDACTED
	if windowMinutes.Valid {
		metric.WindowMinutes = int(windowMinutes.Int64)
REDACTED
	if requestCount.Valid {
		metric.RequestCount = requestCount.Int64
REDACTED
	if successCount.Valid {
		metric.SuccessCount = successCount.Int64
REDACTED
	if errorCount.Valid {
		metric.ErrorCount = errorCount.Int64
REDACTED
	if successRate.Valid {
		metric.SuccessRate = successRate.Float64
REDACTED
	if errorRate.Valid {
		metric.ErrorRate = errorRate.Float64
REDACTED
	if p95Latency.Valid {
		metric.P95LatencyMs = int(p95Latency.Int64)
REDACTED
	if p99Latency.Valid {
		metric.P99LatencyMs = int(p99Latency.Int64)
REDACTED
	if http2Errors.Valid {
		metric.HTTP2Errors = int(http2Errors.Int64)
REDACTED
	if activeAlerts.Valid {
		metric.ActiveAlerts = int(activeAlerts.Int64)
REDACTED
	if cpuUsage.Valid {
		metric.CPUUsagePercent = cpuUsage.Float64
REDACTED
	if memoryUsed.Valid {
		metric.MemoryUsedMB = memoryUsed.Int64
REDACTED
	if memoryTotal.Valid {
		metric.MemoryTotalMB = memoryTotal.Int64
REDACTED
	if memoryUsage.Valid {
		metric.MemoryUsagePercent = memoryUsage.Float64
REDACTED
	if heapAlloc.Valid {
		metric.HeapAllocMB = heapAlloc.Int64
REDACTED
	if gcPause.Valid {
		metric.GCPauseMs = gcPause.Float64
REDACTED
	if queueDepth.Valid {
		metric.ConcurrencyQueueDepth = int(queueDepth.Int64)
REDACTED
	return metric, nil
REDACTED

func (r *OpsRepository) CreateSystemMetric(ctx context.Context, metric *service.OpsMetrics) error {
	if metric == nil {
		return nil
REDACTED
	createdAt := metric.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
REDACTED
	windowMinutes := metric.WindowMinutes
	if windowMinutes <= 0 {
		windowMinutes = DefaultWindowMinutes
REDACTED

	query := `
		INSERT INTO ops_system_metrics (
			window_minutes,
			request_count,
			success_count,
			error_count,
			success_rate,
			error_rate,
			p95_latency_ms,
			p99_latency_ms,
			http2_errors,
			active_alerts,
			cpu_usage_percent,
			memory_used_mb,
			memory_total_mb,
			memory_usage_percent,
			heap_alloc_mb,
			gc_pause_ms,
			concurrency_queue_depth,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18
		)
	`
	_, err := r.sql.ExecContext(ctx, query,
		windowMinutes,
		metric.RequestCount,
		metric.SuccessCount,
		metric.ErrorCount,
		metric.SuccessRate,
		metric.ErrorRate,
		metric.P95LatencyMs,
		metric.P99LatencyMs,
		metric.HTTP2Errors,
		metric.ActiveAlerts,
		metric.CPUUsagePercent,
		metric.MemoryUsedMB,
		metric.MemoryTotalMB,
		metric.MemoryUsagePercent,
		metric.HeapAllocMB,
		metric.GCPauseMs,
		metric.ConcurrencyQueueDepth,
		createdAt,
	)
	return err
REDACTED

func (r *OpsRepository) ListRecentSystemMetrics(ctx context.Context, windowMinutes, limit int) ([]service.OpsMetrics, error) {
	if windowMinutes <= 0 {
		windowMinutes = DefaultWindowMinutes
REDACTED
	if limit <= 0 || limit > MaxRecentSystemMetricsLimit {
		limit = DefaultRecentSystemMetricsLimit
REDACTED

	query := `
		SELECT
			window_minutes,
			request_count,
			success_count,
			error_count,
			success_rate,
			error_rate,
			p95_latency_ms,
			p99_latency_ms,
			http2_errors,
			active_alerts,
			cpu_usage_percent,
			memory_used_mb,
			memory_total_mb,
			memory_usage_percent,
			heap_alloc_mb,
			gc_pause_ms,
			concurrency_queue_depth,
			created_at AS updated_at
		FROM ops_system_metrics
		WHERE window_minutes = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`

	rows, err := r.sql.QueryContext(ctx, query, windowMinutes, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]service.OpsMetrics, 0)
	for rows.Next() {
		metric, err := scanOpsSystemMetric(rows)
		if err != nil {
			return nil, err
	REDACTED
		results = append(results, *metric)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) ListSystemMetricsRange(ctx context.Context, windowMinutes int, startTime, endTime time.Time, limit int) ([]service.OpsMetrics, error) {
	if windowMinutes <= 0 {
		windowMinutes = DefaultWindowMinutes
REDACTED
	if limit <= 0 || limit > MaxMetricsLimit {
		limit = DefaultMetricsLimit
REDACTED
	if endTime.IsZero() {
		endTime = time.Now()
REDACTED
	if startTime.IsZero() {
		startTime = endTime.Add(-time.Duration(limit) * time.Minute)
REDACTED
	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
REDACTED

	query := `
		SELECT
			window_minutes,
			request_count,
			success_count,
			error_count,
			success_rate,
			error_rate,
			p95_latency_ms,
			p99_latency_ms,
			http2_errors,
			active_alerts,
			cpu_usage_percent,
			memory_used_mb,
			memory_total_mb,
			memory_usage_percent,
			heap_alloc_mb,
			gc_pause_ms,
			concurrency_queue_depth,
			created_at
		FROM ops_system_metrics
		WHERE window_minutes = $1
		  AND created_at >= $2
		  AND created_at <= $3
		ORDER BY created_at ASC
		LIMIT $4
	`

	rows, err := r.sql.QueryContext(ctx, query, windowMinutes, startTime, endTime, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]service.OpsMetrics, 0)
	for rows.Next() {
		metric, err := scanOpsSystemMetric(rows)
		if err != nil {
			return nil, err
	REDACTED
		results = append(results, *metric)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) ListAlertRules(ctx context.Context) ([]service.OpsAlertRule, error) {
	query := `
		SELECT
			id,
			name,
			description,
			enabled,
			metric_type,
			operator,
			threshold,
			window_minutes,
			sustained_minutes,
			severity,
			notify_email,
			notify_webhook,
			webhook_url,
			cooldown_minutes,
			dimension_filters,
			notify_channels,
			notify_config,
			created_at,
			updated_at
		FROM ops_alert_rules
		ORDER BY id ASC
	`

	rows, err := r.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	rules := make([]service.OpsAlertRule, 0)
	for rows.Next() {
		var rule service.OpsAlertRule
		var description sql.NullString
		var webhookURL sql.NullString
		var dimensionFilters, notifyChannels, notifyConfig []byte
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&description,
			&rule.Enabled,
			&rule.MetricType,
			&rule.Operator,
			&rule.Threshold,
			&rule.WindowMinutes,
			&rule.SustainedMinutes,
			&rule.Severity,
			&rule.NotifyEmail,
			&rule.NotifyWebhook,
			&webhookURL,
			&rule.CooldownMinutes,
			&dimensionFilters,
			&notifyChannels,
			&notifyConfig,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
	REDACTED
		if description.Valid {
			rule.Description = description.String
	REDACTED
		if webhookURL.Valid {
			rule.WebhookURL = webhookURL.String
	REDACTED
		if len(dimensionFilters) > 0 {
			_ = json.Unmarshal(dimensionFilters, &rule.DimensionFilters)
	REDACTED
		if len(notifyChannels) > 0 {
			_ = json.Unmarshal(notifyChannels, &rule.NotifyChannels)
	REDACTED
		if len(notifyConfig) > 0 {
			_ = json.Unmarshal(notifyConfig, &rule.NotifyConfig)
	REDACTED
		rules = append(rules, rule)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return rules, nil
REDACTED

func (r *OpsRepository) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	return r.getAlertEvent(ctx, `WHERE rule_id = $1 AND status = $2`, []any{ruleID, service.OpsAlertStatusFiringREDACTED)
REDACTED

func (r *OpsRepository) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	return r.getAlertEvent(ctx, `WHERE rule_id = $1`, []any{ruleIDREDACTED)
REDACTED

func (r *OpsRepository) CreateAlertEvent(ctx context.Context, event *service.OpsAlertEvent) error {
	if event == nil {
		return nil
REDACTED
	if event.FiredAt.IsZero() {
		event.FiredAt = time.Now()
REDACTED
	if event.CreatedAt.IsZero() {
		event.CreatedAt = event.FiredAt
REDACTED
	if event.Status == "" {
		event.Status = service.OpsAlertStatusFiring
REDACTED

	query := `
		INSERT INTO ops_alert_events (
			rule_id,
			severity,
			status,
			title,
			description,
			metric_value,
			threshold_value,
			fired_at,
			resolved_at,
			email_sent,
			webhook_sent,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)
		RETURNING id, created_at
	`

	var resolvedAt sql.NullTime
	if event.ResolvedAt != nil {
		resolvedAt = sql.NullTime{Time: *event.ResolvedAt, Valid: trueREDACTED
REDACTED

	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{
			event.RuleID,
			event.Severity,
			event.Status,
			event.Title,
			event.Description,
			event.MetricValue,
			event.ThresholdValue,
			event.FiredAt,
			resolvedAt,
			event.EmailSent,
			event.WebhookSent,
			event.CreatedAt,
	REDACTED,
		&event.ID,
		&event.CreatedAt,
	); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (r *OpsRepository) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	var resolved sql.NullTime
	if resolvedAt != nil {
		resolved = sql.NullTime{Time: *resolvedAt, Valid: trueREDACTED
REDACTED
	_, err := r.sql.ExecContext(ctx, `
		UPDATE ops_alert_events
		SET status = $2, resolved_at = $3
		WHERE id = $1
	`, eventID, status, resolved)
	return err
REDACTED

func (r *OpsRepository) UpdateAlertEventNotifications(ctx context.Context, eventID int64, emailSent, webhookSent bool) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE ops_alert_events
		SET email_sent = $2, webhook_sent = $3
		WHERE id = $1
	`, eventID, emailSent, webhookSent)
	return err
REDACTED

func (r *OpsRepository) CountActiveAlerts(ctx context.Context) (int, error) {
	var count int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		`SELECT COUNT(*) FROM ops_alert_events WHERE status = $1`,
		[]any{service.OpsAlertStatusFiringREDACTED,
		&count,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
	REDACTED
		return 0, err
REDACTED
	return int(count), nil
REDACTED

func (r *OpsRepository) GetWindowStats(ctx context.Context, startTime, endTime time.Time) (*service.OpsWindowStats, error) {
	query := `
		WITH
		usage_agg AS (
			SELECT
				COUNT(*) AS success_count,
				percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)
					FILTER (WHERE duration_ms IS NOT NULL) AS p95,
				percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)
					FILTER (WHERE duration_ms IS NOT NULL) AS p99
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
		),
		error_agg AS (
			SELECT
				COUNT(*) AS error_count,
				COUNT(*) FILTER (
					WHERE
						error_type = 'network_error'
						OR error_message ILIKE '%http2%'
						OR error_message ILIKE '%http/2%'
				) AS http2_errors
			FROM ops_error_logs
			WHERE created_at >= $1 AND created_at < $2
		)
		SELECT
			usage_agg.success_count,
			error_agg.error_count,
			usage_agg.p95,
			usage_agg.p99,
			error_agg.http2_errors
		FROM usage_agg
		CROSS JOIN error_agg
	`

	var stats service.OpsWindowStats
	var p95Latency, p99Latency sql.NullFloat64
	var http2Errors int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{startTime, endTimeREDACTED,
		&stats.SuccessCount,
		&stats.ErrorCount,
		&p95Latency,
		&p99Latency,
		&http2Errors,
	); err != nil {
		return nil, err
REDACTED

	stats.HTTP2Errors = int(http2Errors)
	if p95Latency.Valid {
		stats.P95LatencyMs = int(math.Round(p95Latency.Float64))
REDACTED
	if p99Latency.Valid {
		stats.P99LatencyMs = int(math.Round(p99Latency.Float64))
REDACTED

	return &stats, nil
REDACTED

func (r *OpsRepository) GetOverviewStats(ctx context.Context, startTime, endTime time.Time) (*service.OverviewStats, error) {
	query := `
		WITH
		usage_stats AS (
			SELECT
				COUNT(*) AS request_count,
				COUNT(*) FILTER (WHERE duration_ms IS NOT NULL) AS success_count,
				percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS p50,
				percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS p95,
				percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS p99,
				percentile_cont(0.999) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS p999,
				AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS avg_latency,
				MAX(duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS max_latency
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
		),
		error_stats AS (
			SELECT
				COUNT(*) AS error_count,
				COUNT(*) FILTER (WHERE status_code >= 400 AND status_code < 500) AS error_4xx,
				COUNT(*) FILTER (WHERE status_code >= 500) AS error_5xx,
				COUNT(*) FILTER (
					WHERE
						error_type IN ('timeout', 'timeout_error')
						OR error_message ILIKE '%timeout%'
						OR error_message ILIKE '%deadline exceeded%'
				) AS timeout_count
			FROM ops_error_logs
			WHERE created_at >= $1 AND created_at < $2
		),
		top_error AS (
			SELECT
				COALESCE(status_code::text, 'unknown') AS error_code,
				error_message,
				COUNT(*) AS error_count
			FROM ops_error_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY status_code, error_message
			ORDER BY error_count DESC
			LIMIT 1
		),
		latest_metrics AS (
			SELECT
				cpu_usage_percent,
				memory_usage_percent,
				memory_used_mb,
				memory_total_mb,
				concurrency_queue_depth
			FROM ops_system_metrics
			ORDER BY created_at DESC
			LIMIT 1
		)
		SELECT
			COALESCE(usage_stats.request_count, 0) + COALESCE(error_stats.error_count, 0) AS request_count,
			COALESCE(usage_stats.success_count, 0),
			COALESCE(error_stats.error_count, 0),
			COALESCE(error_stats.error_4xx, 0),
			COALESCE(error_stats.error_5xx, 0),
			COALESCE(error_stats.timeout_count, 0),
			COALESCE(usage_stats.p50, 0),
			COALESCE(usage_stats.p95, 0),
			COALESCE(usage_stats.p99, 0),
			COALESCE(usage_stats.p999, 0),
			COALESCE(usage_stats.avg_latency, 0),
			COALESCE(usage_stats.max_latency, 0),
			COALESCE(top_error.error_code, ''),
			COALESCE(top_error.error_message, ''),
			COALESCE(top_error.error_count, 0),
			COALESCE(latest_metrics.cpu_usage_percent, 0),
			COALESCE(latest_metrics.memory_usage_percent, 0),
			COALESCE(latest_metrics.memory_used_mb, 0),
			COALESCE(latest_metrics.memory_total_mb, 0),
			COALESCE(latest_metrics.concurrency_queue_depth, 0)
		FROM usage_stats
		CROSS JOIN error_stats
		LEFT JOIN top_error ON true
		LEFT JOIN latest_metrics ON true
	`

	var stats service.OverviewStats
	var p50, p95, p99, p999, avgLatency, maxLatency sql.NullFloat64

	err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{startTime, endTimeREDACTED,
		&stats.RequestCount,
		&stats.SuccessCount,
		&stats.ErrorCount,
		&stats.Error4xxCount,
		&stats.Error5xxCount,
		&stats.TimeoutCount,
		&p50,
		&p95,
		&p99,
		&p999,
		&avgLatency,
		&maxLatency,
		&stats.TopErrorCode,
		&stats.TopErrorMsg,
		&stats.TopErrorCount,
		&stats.CPUUsage,
		&stats.MemoryUsage,
		&stats.MemoryUsedMB,
		&stats.MemoryTotalMB,
		&stats.ConcurrencyQueueDepth,
	)
	if err != nil {
		return nil, err
REDACTED

	if p50.Valid {
		stats.LatencyP50 = int(p50.Float64)
REDACTED
	if p95.Valid {
		stats.LatencyP95 = int(p95.Float64)
REDACTED
	if p99.Valid {
		stats.LatencyP99 = int(p99.Float64)
REDACTED
	if p999.Valid {
		stats.LatencyP999 = int(p999.Float64)
REDACTED
	if avgLatency.Valid {
		stats.LatencyAvg = int(avgLatency.Float64)
REDACTED
	if maxLatency.Valid {
		stats.LatencyMax = int(maxLatency.Float64)
REDACTED

	return &stats, nil
REDACTED

func (r *OpsRepository) GetProviderStats(ctx context.Context, startTime, endTime time.Time) ([]*service.ProviderStats, error) {
	if startTime.IsZero() || endTime.IsZero() {
		return nil, nil
REDACTED
	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
REDACTED

	query := `
		WITH combined AS (
			SELECT
				COALESCE(g.platform, a.platform, '') AS platform,
				u.duration_ms AS duration_ms,
				1 AS is_success,
				0 AS is_error,
				NULL::INT AS status_code,
				NULL::TEXT AS error_type,
				NULL::TEXT AS error_message
			FROM usage_logs u
			LEFT JOIN groups g ON g.id = u.group_id
			LEFT JOIN accounts a ON a.id = u.account_id
			WHERE u.created_at >= $1 AND u.created_at < $2

			UNION ALL

			SELECT
				COALESCE(NULLIF(o.platform, ''), g.platform, a.platform, '') AS platform,
				o.duration_ms AS duration_ms,
				0 AS is_success,
				1 AS is_error,
				o.status_code AS status_code,
				o.error_type AS error_type,
				o.error_message AS error_message
			FROM ops_error_logs o
			LEFT JOIN groups g ON g.id = o.group_id
			LEFT JOIN accounts a ON a.id = o.account_id
			WHERE o.created_at >= $1 AND o.created_at < $2
		)
		SELECT
			platform,
			COUNT(*) AS request_count,
			COALESCE(SUM(is_success), 0) AS success_count,
			COALESCE(SUM(is_error), 0) AS error_count,
			COALESCE(AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL), 0) AS avg_latency_ms,
			percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL) AS p99_latency_ms,
			COUNT(*) FILTER (WHERE is_error = 1 AND status_code >= 400 AND status_code < 500) AS error_4xx,
			COUNT(*) FILTER (WHERE is_error = 1 AND status_code >= 500 AND status_code < 600) AS error_5xx,
			COUNT(*) FILTER (
				WHERE
					is_error = 1
					AND (
						status_code = 504
						OR error_type ILIKE '%timeout%'
						OR error_message ILIKE '%timeout%'
					)
			) AS timeout_count
		FROM combined
		WHERE platform <> ''
		GROUP BY platform
		ORDER BY request_count DESC, platform ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]*service.ProviderStats, 0)
	for rows.Next() {
		var item service.ProviderStats
		var avgLatency sql.NullFloat64
		var p99Latency sql.NullFloat64
		if err := rows.Scan(
			&item.Platform,
			&item.RequestCount,
			&item.SuccessCount,
			&item.ErrorCount,
			&avgLatency,
			&p99Latency,
			&item.Error4xxCount,
			&item.Error5xxCount,
			&item.TimeoutCount,
		); err != nil {
			return nil, err
	REDACTED

		if avgLatency.Valid {
			item.AvgLatencyMs = int(math.Round(avgLatency.Float64))
	REDACTED
		if p99Latency.Valid {
			item.P99LatencyMs = int(math.Round(p99Latency.Float64))
	REDACTED

		results = append(results, &item)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) GetLatencyHistogram(ctx context.Context, startTime, endTime time.Time) ([]*service.LatencyHistogramItem, error) {
	query := `
		WITH buckets AS (
			SELECT
				CASE
					WHEN duration_ms < 200 THEN '<200ms'
					WHEN duration_ms < 500 THEN '200-500ms'
					WHEN duration_ms < 1000 THEN '500-1000ms'
					WHEN duration_ms < 3000 THEN '1000-3000ms'
					ELSE '>3000ms'
				END AS range_name,
				CASE
					WHEN duration_ms < 200 THEN 1
					WHEN duration_ms < 500 THEN 2
					WHEN duration_ms < 1000 THEN 3
					WHEN duration_ms < 3000 THEN 4
					ELSE 5
				END AS range_order,
				COUNT(*) AS count
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2 AND duration_ms IS NOT NULL
			GROUP BY 1, 2
		),
		total AS (
			SELECT SUM(count) AS total_count FROM buckets
		)
		SELECT
			b.range_name,
			b.count,
			ROUND((b.count::numeric / t.total_count) * 100, 2) AS percentage
		FROM buckets b
		CROSS JOIN total t
		ORDER BY b.range_order ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]*service.LatencyHistogramItem, 0)
	for rows.Next() {
		var item service.LatencyHistogramItem
		if err := rows.Scan(&item.Range, &item.Count, &item.Percentage); err != nil {
			return nil, err
	REDACTED
		results = append(results, &item)
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) GetErrorDistribution(ctx context.Context, startTime, endTime time.Time) ([]*service.ErrorDistributionItem, error) {
	query := `
		WITH errors AS (
			SELECT
				COALESCE(status_code::text, 'unknown') AS code,
				COALESCE(error_message, 'Unknown error') AS message,
				COUNT(*) AS count
			FROM ops_error_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY 1, 2
		),
		total AS (
			SELECT SUM(count) AS total_count FROM errors
		)
		SELECT
			e.code,
			e.message,
			e.count,
			ROUND((e.count::numeric / t.total_count) * 100, 2) AS percentage
		FROM errors e
		CROSS JOIN total t
		ORDER BY e.count DESC
		LIMIT 20
	`

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	results := make([]*service.ErrorDistributionItem, 0)
	for rows.Next() {
		var item service.ErrorDistributionItem
		if err := rows.Scan(&item.Code, &item.Message, &item.Count, &item.Percentage); err != nil {
			return nil, err
	REDACTED
		results = append(results, &item)
REDACTED
	return results, nil
REDACTED

func (r *OpsRepository) getAlertEvent(ctx context.Context, whereClause string, args []any) (*service.OpsAlertEvent, error) {
	query := fmt.Sprintf(`
		SELECT
			id,
			rule_id,
			severity,
			status,
			title,
			description,
			metric_value,
			threshold_value,
			fired_at,
			resolved_at,
			email_sent,
			webhook_sent,
			created_at
		FROM ops_alert_events
		%s
		ORDER BY fired_at DESC
		LIMIT 1
	`, whereClause)

	var event service.OpsAlertEvent
	var resolvedAt sql.NullTime
	var metricValue sql.NullFloat64
	var thresholdValue sql.NullFloat64
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		args,
		&event.ID,
		&event.RuleID,
		&event.Severity,
		&event.Status,
		&event.Title,
		&event.Description,
		&metricValue,
		&thresholdValue,
		&event.FiredAt,
		&resolvedAt,
		&event.EmailSent,
		&event.WebhookSent,
		&event.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED

	if metricValue.Valid {
		event.MetricValue = metricValue.Float64
REDACTED
	if thresholdValue.Valid {
		event.ThresholdValue = thresholdValue.Float64
REDACTED
	if resolvedAt.Valid {
		event.ResolvedAt = &resolvedAt.Time
REDACTED
	return &event, nil
REDACTED

func scanOpsSystemMetric(rows *sql.Rows) (*service.OpsMetrics, error) {
	var metric service.OpsMetrics
	var windowMinutes sql.NullInt64
	var requestCount, successCount, errorCount sql.NullInt64
	var successRate, errorRate sql.NullFloat64
	var p95Latency, p99Latency, http2Errors, activeAlerts sql.NullInt64
	var cpuUsage, memoryUsage, gcPause sql.NullFloat64
	var memoryUsed, memoryTotal, heapAlloc, queueDepth sql.NullInt64

	if err := rows.Scan(
		&windowMinutes,
		&requestCount,
		&successCount,
		&errorCount,
		&successRate,
		&errorRate,
		&p95Latency,
		&p99Latency,
		&http2Errors,
		&activeAlerts,
		&cpuUsage,
		&memoryUsed,
		&memoryTotal,
		&memoryUsage,
		&heapAlloc,
		&gcPause,
		&queueDepth,
		&metric.UpdatedAt,
	); err != nil {
		return nil, err
REDACTED

	if windowMinutes.Valid {
		metric.WindowMinutes = int(windowMinutes.Int64)
REDACTED
	if requestCount.Valid {
		metric.RequestCount = requestCount.Int64
REDACTED
	if successCount.Valid {
		metric.SuccessCount = successCount.Int64
REDACTED
	if errorCount.Valid {
		metric.ErrorCount = errorCount.Int64
REDACTED
	if successRate.Valid {
		metric.SuccessRate = successRate.Float64
REDACTED
	if errorRate.Valid {
		metric.ErrorRate = errorRate.Float64
REDACTED
	if p95Latency.Valid {
		metric.P95LatencyMs = int(p95Latency.Int64)
REDACTED
	if p99Latency.Valid {
		metric.P99LatencyMs = int(p99Latency.Int64)
REDACTED
	if http2Errors.Valid {
		metric.HTTP2Errors = int(http2Errors.Int64)
REDACTED
	if activeAlerts.Valid {
		metric.ActiveAlerts = int(activeAlerts.Int64)
REDACTED
	if cpuUsage.Valid {
		metric.CPUUsagePercent = cpuUsage.Float64
REDACTED
	if memoryUsed.Valid {
		metric.MemoryUsedMB = memoryUsed.Int64
REDACTED
	if memoryTotal.Valid {
		metric.MemoryTotalMB = memoryTotal.Int64
REDACTED
	if memoryUsage.Valid {
		metric.MemoryUsagePercent = memoryUsage.Float64
REDACTED
	if heapAlloc.Valid {
		metric.HeapAllocMB = heapAlloc.Int64
REDACTED
	if gcPause.Valid {
		metric.GCPauseMs = gcPause.Float64
REDACTED
	if queueDepth.Valid {
		metric.ConcurrencyQueueDepth = int(queueDepth.Int64)
REDACTED

	return &metric, nil
REDACTED

func scanOpsErrorLog(rows *sql.Rows) (*service.OpsErrorLog, error) {
	var entry service.OpsErrorLog
	var userID, apiKeyID, accountID, groupID sql.NullInt64
	var clientIP sql.NullString
	var statusCode sql.NullInt64
	var platform sql.NullString
	var model sql.NullString
	var requestPath sql.NullString
	var stream sql.NullBool
	var latency sql.NullInt64
	var requestID sql.NullString
	var message sql.NullString

	if err := rows.Scan(
		&entry.ID,
		&entry.CreatedAt,
		&userID,
		&apiKeyID,
		&accountID,
		&groupID,
		&clientIP,
		&entry.Phase,
		&entry.Type,
		&entry.Severity,
		&statusCode,
		&platform,
		&model,
		&requestPath,
		&stream,
		&latency,
		&requestID,
		&message,
	); err != nil {
		return nil, err
REDACTED

	if userID.Valid {
		v := userID.Int64
		entry.UserID = &v
REDACTED
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		entry.APIKeyID = &v
REDACTED
	if accountID.Valid {
		v := accountID.Int64
		entry.AccountID = &v
REDACTED
	if groupID.Valid {
		v := groupID.Int64
		entry.GroupID = &v
REDACTED
	if clientIP.Valid {
		entry.ClientIP = clientIP.String
REDACTED
	if statusCode.Valid {
		entry.StatusCode = int(statusCode.Int64)
REDACTED
	if platform.Valid {
		entry.Platform = platform.String
REDACTED
	if model.Valid {
		entry.Model = model.String
REDACTED
	if requestPath.Valid {
		entry.RequestPath = requestPath.String
REDACTED
	if stream.Valid {
		entry.Stream = stream.Bool
REDACTED
	if latency.Valid {
		value := int(latency.Int64)
		entry.LatencyMs = &value
REDACTED
	if requestID.Valid {
		entry.RequestID = requestID.String
REDACTED
	if message.Valid {
		entry.Message = message.String
REDACTED

	return &entry, nil
REDACTED

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{REDACTED
REDACTED
	return sql.NullString{String: value, Valid: trueREDACTED
REDACTED
