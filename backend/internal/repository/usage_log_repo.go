package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbapikey "github.com/Wei-Shaw/sub2api/ent/apikey"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	dbusersub "github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const usageLogSelectColumns = "id, user_id, api_key_id, account_id, request_id, model, group_id, subscription_id, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cache_creation_5m_tokens, cache_creation_1h_tokens, input_cost, output_cost, cache_creation_cost, cache_read_cost, total_cost, actual_cost, rate_multiplier, account_rate_multiplier, billing_type, request_type, stream, openai_ws_mode, duration_ms, first_token_ms, user_agent, ip_address, image_count, image_size, media_type, service_tier, reasoning_effort, cache_ttl_overridden, created_at"

// dateFormatWhitelist 将 granularity 参数映射为 PostgreSQL TO_CHAR 格式字符串，防止外部输入直接拼入 SQL
var dateFormatWhitelist = map[string]string{
	"hour":  "YYYY-MM-DD HH24:00",
	"day":   "YYYY-MM-DD",
	"week":  "IYYY-IW",
	"month": "YYYY-MM",
REDACTED

// safeDateFormat 根据白名单获取 dateFormat，未匹配时返回默认值
func safeDateFormat(granularity string) string {
	if f, ok := dateFormatWhitelist[granularity]; ok {
		return f
REDACTED
	return "YYYY-MM-DD"
REDACTED

type usageLogRepository struct {
	client *dbent.Client
	sql    sqlExecutor
REDACTED

func NewUsageLogRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageLogRepository {
	return newUsageLogRepositoryWithSQL(client, sqlDB)
REDACTED

func newUsageLogRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *usageLogRepository {
	// 使用 scanSingleRow 替代 QueryRowContext，保证 ent.Tx 作为 sqlExecutor 可用。
	return &usageLogRepository{client: client, sql: sqlqREDACTED
REDACTED

// getPerformanceStats 获取 RPM 和 TPM（近5分钟平均值，可选按用户过滤）
func (r *usageLogRepository) getPerformanceStats(ctx context.Context, userID int64) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	query := `
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(input_tokens + output_tokens), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1`
	args := []any{fiveMinutesAgoREDACTED
	if userID > 0 {
		query += " AND user_id = $2"
		args = append(args, userID)
REDACTED

	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, args, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
REDACTED
	return requestCount / 5, tokenCount / 5, nil
REDACTED

func (r *usageLogRepository) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	if log == nil {
		return false, nil
REDACTED

	// 在事务上下文中，使用 tx 绑定的 ExecQuerier 执行原生 SQL，保证与其他更新同事务。
	// 无事务时回退到默认的 *sql.DB 执行器。
	sqlq := r.sql
	if tx := dbent.TxFromContext(ctx); tx != nil {
		sqlq = tx.Client()
REDACTED

	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
REDACTED

	requestID := strings.TrimSpace(log.RequestID)
	log.RequestID = requestID

	rateMultiplier := log.RateMultiplier
	log.SyncRequestTypeAndLegacyFields()
	requestType := int16(log.RequestType)

	query := `
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			media_type,
			service_tier,
			reasoning_effort,
			cache_ttl_overridden,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9, $10, $11,
			$12, $13,
			$14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, created_at
	`

	groupID := nullInt64(log.GroupID)
	subscriptionID := nullInt64(log.SubscriptionID)
	duration := nullInt(log.DurationMs)
	firstToken := nullInt(log.FirstTokenMs)
	userAgent := nullString(log.UserAgent)
	ipAddress := nullString(log.IPAddress)
	imageSize := nullString(log.ImageSize)
	mediaType := nullString(log.MediaType)
	serviceTier := nullString(log.ServiceTier)
	reasoningEffort := nullString(log.ReasoningEffort)

	var requestIDArg any
	if requestID != "" {
		requestIDArg = requestID
REDACTED

	args := []any{
		log.UserID,
		log.APIKeyID,
		log.AccountID,
		requestIDArg,
		log.Model,
		groupID,
		subscriptionID,
		log.InputTokens,
		log.OutputTokens,
		log.CacheCreationTokens,
		log.CacheReadTokens,
		log.CacheCreation5mTokens,
		log.CacheCreation1hTokens,
		log.InputCost,
		log.OutputCost,
		log.CacheCreationCost,
		log.CacheReadCost,
		log.TotalCost,
		log.ActualCost,
		rateMultiplier,
		log.AccountRateMultiplier,
		log.BillingType,
		requestType,
		log.Stream,
		log.OpenAIWSMode,
		duration,
		firstToken,
		userAgent,
		ipAddress,
		log.ImageCount,
		imageSize,
		mediaType,
		serviceTier,
		reasoningEffort,
		log.CacheTTLOverridden,
		createdAt,
REDACTED
	if err := scanSingleRow(ctx, sqlq, query, args, &log.ID, &log.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) && requestID != "" {
			selectQuery := "SELECT id, created_at FROM usage_logs WHERE request_id = $1 AND api_key_id = $2"
			if err := scanSingleRow(ctx, sqlq, selectQuery, []any{requestID, log.APIKeyIDREDACTED, &log.ID, &log.CreatedAt); err != nil {
				return false, err
		REDACTED
			log.RateMultiplier = rateMultiplier
			return false, nil
	REDACTED else {
			return false, err
	REDACTED
REDACTED
	log.RateMultiplier = rateMultiplier
	return true, nil
REDACTED

func (r *usageLogRepository) GetByID(ctx context.Context, id int64) (log *service.UsageLog, err error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE id = $1"
	rows, err := r.sql.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			log = nil
	REDACTED
REDACTED()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
	REDACTED
		return nil, service.ErrUsageLogNotFound
REDACTED
	log, err = scanUsageLog(rows)
	if err != nil {
		return nil, err
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED
	return log, nil
REDACTED

func (r *usageLogRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE user_id = $1", []any{userIDREDACTED, params)
REDACTED

func (r *usageLogRepository) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE api_key_id = $1", []any{apiKeyIDREDACTED, params)
REDACTED

// UserStats 用户使用统计
type UserStats struct {
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
REDACTED

func (r *usageLogRepository) GetUserStats(ctx context.Context, userID int64, startTime, endTime time.Time) (*UserStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(actual_cost), 0) as total_cost,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`

	stats := &UserStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{userID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalTokens,
		&stats.TotalCost,
		&stats.InputTokens,
		&stats.OutputTokens,
		&stats.CacheReadTokens,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// DashboardStats 仪表盘统计
type DashboardStats = usagestats.DashboardStats

func (r *usageLogRepository) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{REDACTED
	now := timezone.Now()
	todayStart := timezone.Today()

	if err := r.fillDashboardEntityStats(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED
	if err := r.fillDashboardUsageStatsAggregated(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED

	rpm, tpm, err := r.getPerformanceStats(ctx, 0)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

func (r *usageLogRepository) GetDashboardStatsWithRange(ctx context.Context, start, end time.Time) (*DashboardStats, error) {
	startUTC := start.UTC()
	endUTC := end.UTC()
	if !endUTC.After(startUTC) {
		return nil, errors.New("统计时间范围无效")
REDACTED

	stats := &DashboardStats{REDACTED
	now := timezone.Now()
	todayStart := timezone.Today()

	if err := r.fillDashboardEntityStats(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED
	if err := r.fillDashboardUsageStatsFromUsageLogs(ctx, stats, startUTC, endUTC, todayStart, now); err != nil {
		return nil, err
REDACTED

	rpm, tpm, err := r.getPerformanceStats(ctx, 0)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

func (r *usageLogRepository) fillDashboardEntityStats(ctx context.Context, stats *DashboardStats, todayUTC, now time.Time) error {
	userStatsQuery := `
		SELECT
			COUNT(*) as total_users,
			COUNT(CASE WHEN created_at >= $1 THEN 1 END) as today_new_users
		FROM users
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		userStatsQuery,
		[]any{todayUTCREDACTED,
		&stats.TotalUsers,
		&stats.TodayNewUsers,
	); err != nil {
		return err
REDACTED

	apiKeyStatsQuery := `
		SELECT
			COUNT(*) as total_api_keys,
			COUNT(CASE WHEN status = $1 THEN 1 END) as active_api_keys
		FROM api_keys
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		apiKeyStatsQuery,
		[]any{service.StatusActiveREDACTED,
		&stats.TotalAPIKeys,
		&stats.ActiveAPIKeys,
	); err != nil {
		return err
REDACTED

	accountStatsQuery := `
		SELECT
			COUNT(*) as total_accounts,
			COUNT(CASE WHEN status = $1 AND schedulable = true THEN 1 END) as normal_accounts,
			COUNT(CASE WHEN status = $2 THEN 1 END) as error_accounts,
			COUNT(CASE WHEN rate_limited_at IS NOT NULL AND rate_limit_reset_at > $3 THEN 1 END) as ratelimit_accounts,
			COUNT(CASE WHEN overload_until IS NOT NULL AND overload_until > $4 THEN 1 END) as overload_accounts
		FROM accounts
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		accountStatsQuery,
		[]any{service.StatusActive, service.StatusError, now, nowREDACTED,
		&stats.TotalAccounts,
		&stats.NormalAccounts,
		&stats.ErrorAccounts,
		&stats.RateLimitAccounts,
		&stats.OverloadAccounts,
	); err != nil {
		return err
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) fillDashboardUsageStatsAggregated(ctx context.Context, stats *DashboardStats, todayUTC, now time.Time) error {
	totalStatsQuery := `
		SELECT
			COALESCE(SUM(total_requests), 0) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(SUM(total_duration_ms), 0) as total_duration_ms
		FROM usage_dashboard_daily
	`
	var totalDurationMs int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		nil,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&totalDurationMs,
	); err != nil {
		return err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
REDACTED

	todayStatsQuery := `
		SELECT
			total_requests as today_requests,
			input_tokens as today_input_tokens,
			output_tokens as today_output_tokens,
			cache_creation_tokens as today_cache_creation_tokens,
			cache_read_tokens as today_cache_read_tokens,
			total_cost as today_cost,
			actual_cost as today_actual_cost,
			active_users as active_users
		FROM usage_dashboard_daily
		WHERE bucket_date = $1::date
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{todayUTCREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
		&stats.ActiveUsers,
	); err != nil {
		if err != sql.ErrNoRows {
			return err
	REDACTED
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	hourlyActiveQuery := `
		SELECT active_users
		FROM usage_dashboard_hourly
		WHERE bucket_start = $1
	`
	hourStart := now.In(timezone.Location()).Truncate(time.Hour)
	if err := scanSingleRow(ctx, r.sql, hourlyActiveQuery, []any{hourStartREDACTED, &stats.HourlyActiveUsers); err != nil {
		if err != sql.ErrNoRows {
			return err
	REDACTED
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) fillDashboardUsageStatsFromUsageLogs(ctx context.Context, stats *DashboardStats, startUTC, endUTC, todayUTC, now time.Time) error {
	todayEnd := todayUTC.Add(24 * time.Hour)
	combinedStatsQuery := `
		WITH scoped AS (
			SELECT
				created_at,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				total_cost,
				actual_cost,
				COALESCE(duration_ms, 0) AS duration_ms
			FROM usage_logs
			WHERE created_at >= LEAST($1::timestamptz, $3::timestamptz)
				AND created_at < GREATEST($2::timestamptz, $4::timestamptz)
		)
		SELECT
			COUNT(*) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz) AS total_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_actual_cost,
			COALESCE(SUM(duration_ms) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_duration_ms,
			COUNT(*) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz) AS today_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_actual_cost
		FROM scoped
	`
	var totalDurationMs int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		combinedStatsQuery,
		[]any{startUTC, endUTC, todayUTC, todayEndREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&totalDurationMs,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
	); err != nil {
		return err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
REDACTED

	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	hourStart := now.UTC().Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)
	activeUsersQuery := `
		WITH scoped AS (
			SELECT user_id, created_at
			FROM usage_logs
			WHERE created_at >= LEAST($1::timestamptz, $3::timestamptz)
				AND created_at < GREATEST($2::timestamptz, $4::timestamptz)
		)
		SELECT
			COUNT(DISTINCT CASE WHEN created_at >= $1::timestamptz AND created_at < $2::timestamptz THEN user_id END) AS active_users,
			COUNT(DISTINCT CASE WHEN created_at >= $3::timestamptz AND created_at < $4::timestamptz THEN user_id END) AS hourly_active_users
		FROM scoped
	`
	if err := scanSingleRow(ctx, r.sql, activeUsersQuery, []any{todayUTC, todayEnd, hourStart, hourEndREDACTED, &stats.ActiveUsers, &stats.HourlyActiveUsers); err != nil {
		return err
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE account_id = $1", []any{accountIDREDACTED, params)
REDACTED

func (r *usageLogRepository) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE user_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, userID, startTime, endTime)
	return logs, nil, err
REDACTED

// GetUserStatsAggregated returns aggregated usage statistics for a user using database-level aggregation
func (r *usageLogRepository) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{userID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetAPIKeyStatsAggregated returns aggregated usage statistics for an API key using database-level aggregation
func (r *usageLogRepository) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE api_key_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{apiKeyID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetAccountStatsAggregated 使用 SQL 聚合统计账号使用数据
//
// 性能优化说明：
// 原实现先查询所有日志记录，再在应用层循环计算统计值：
// 1. 需要传输大量数据到应用层
// 2. 应用层循环计算增加 CPU 和内存开销
//
// 新实现使用 SQL 聚合函数：
// 1. 在数据库层完成 COUNT/SUM/AVG 计算
// 2. 只返回单行聚合结果，大幅减少数据传输量
// 3. 利用数据库索引优化聚合查询性能
func (r *usageLogRepository) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetModelStatsAggregated 使用 SQL 聚合统计模型使用数据
// 性能优化：数据库层聚合计算，避免应用层循环统计
func (r *usageLogRepository) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE model = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{modelName, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetDailyStatsAggregated 使用 SQL 聚合统计用户的每日使用数据
// 性能优化：使用 GROUP BY 在数据库层按日期分组聚合，避免应用层循环分组统计
func (r *usageLogRepository) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (result []map[string]any, err error) {
	tzName := resolveUsageStatsTimezone()
	query := `
		SELECT
			-- 使用应用时区分组，避免数据库会话时区导致日边界偏移。
			TO_CHAR(created_at AT TIME ZONE $4, 'YYYY-MM-DD') as date,
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1
		ORDER BY 1
	`

	rows, err := r.sql.QueryContext(ctx, query, userID, startTime, endTime, tzName)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
	REDACTED
REDACTED()

	result = make([]map[string]any, 0)
	for rows.Next() {
		var (
			date              string
			totalRequests     int64
			totalInputTokens  int64
			totalOutputTokens int64
			totalCacheTokens  int64
			totalCost         float64
			totalActualCost   float64
			avgDurationMs     float64
		)
		if err = rows.Scan(
			&date,
			&totalRequests,
			&totalInputTokens,
			&totalOutputTokens,
			&totalCacheTokens,
			&totalCost,
			&totalActualCost,
			&avgDurationMs,
		); err != nil {
			return nil, err
	REDACTED
		result = append(result, map[string]any{
			"date":                date,
			"total_requests":      totalRequests,
			"total_input_tokens":  totalInputTokens,
			"total_output_tokens": totalOutputTokens,
			"total_cache_tokens":  totalCacheTokens,
			"total_tokens":        totalInputTokens + totalOutputTokens + totalCacheTokens,
			"total_cost":          totalCost,
			"total_actual_cost":   totalActualCost,
			"average_duration_ms": avgDurationMs,
	REDACTED)
REDACTED

	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// resolveUsageStatsTimezone 获取用于 SQL 分组的时区名称。
// 优先使用应用初始化的时区，其次尝试读取 TZ 环境变量，最后回落为 UTC。
func resolveUsageStatsTimezone() string {
	tzName := timezone.Name()
	if tzName != "" && tzName != "Local" {
		return tzName
REDACTED
	if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" {
		return envTZ
REDACTED
	return "UTC"
REDACTED

func (r *usageLogRepository) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE api_key_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, apiKeyID, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, accountID, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE model = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, modelName, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, "DELETE FROM usage_logs WHERE id = $1", id)
	return err
REDACTED

// GetAccountTodayStats 获取账号今日统计
func (r *usageLogRepository) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	today := timezone.Today()

	query := `
		SELECT
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`

	stats := &usagestats.AccountStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, todayREDACTED,
		&stats.Requests,
		&stats.Tokens,
		&stats.Cost,
		&stats.StandardCost,
		&stats.UserCost,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// GetAccountWindowStats 获取账号时间窗口内的统计
func (r *usageLogRepository) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	query := `
		SELECT
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`

	stats := &usagestats.AccountStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, startTimeREDACTED,
		&stats.Requests,
		&stats.Tokens,
		&stats.Cost,
		&stats.StandardCost,
		&stats.UserCost,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// GetAccountWindowStatsBatch 批量获取同一窗口起点下多个账号的统计数据。
// 返回 map[accountID]*AccountStats，未命中的账号会返回零值统计，便于上层直接复用。
func (r *usageLogRepository) GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error) {
	result := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
REDACTED

	query := `
		SELECT
			account_id,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2
		GROUP BY account_id
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var accountID int64
		stats := &usagestats.AccountStats{REDACTED
		if err := rows.Scan(
			&accountID,
			&stats.Requests,
			&stats.Tokens,
			&stats.Cost,
			&stats.StandardCost,
			&stats.UserCost,
		); err != nil {
			return nil, err
	REDACTED
		result[accountID] = stats
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	for _, accountID := range accountIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = &usagestats.AccountStats{REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

// GetGeminiUsageTotalsBatch 批量聚合 Gemini 账号在窗口内的 Pro/Flash 请求与用量。
// 模型分类规则与 service.geminiModelClassFromName 一致：model 包含 flash/lite 视为 flash，其余视为 pro。
func (r *usageLogRepository) GetGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, startTime, endTime time.Time) (map[int64]service.GeminiUsageTotals, error) {
	result := make(map[int64]service.GeminiUsageTotals, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
REDACTED

	query := `
		SELECT
			account_id,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 1 ELSE 0 END), 0) AS flash_requests,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE 1 END), 0) AS pro_requests,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN (input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) ELSE 0 END), 0) AS flash_tokens,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE (input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) END), 0) AS pro_tokens,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN actual_cost ELSE 0 END), 0) AS flash_cost,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE actual_cost END), 0) AS pro_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3
		GROUP BY account_id
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var accountID int64
		var totals service.GeminiUsageTotals
		if err := rows.Scan(
			&accountID,
			&totals.FlashRequests,
			&totals.ProRequests,
			&totals.FlashTokens,
			&totals.ProTokens,
			&totals.FlashCost,
			&totals.ProCost,
		); err != nil {
			return nil, err
	REDACTED
		result[accountID] = totals
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	for _, accountID := range accountIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = service.GeminiUsageTotals{REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

// TrendDataPoint represents a single point in trend data
type TrendDataPoint = usagestats.TrendDataPoint

// ModelStat represents usage statistics for a single model
type ModelStat = usagestats.ModelStat

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint = usagestats.UserUsageTrendPoint

// APIKeyUsageTrendPoint represents API key usage trend data point
type APIKeyUsageTrendPoint = usagestats.APIKeyUsageTrendPoint

// GetAPIKeyUsageTrend returns usage trend data grouped by API key and date
func (r *usageLogRepository) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) (results []APIKeyUsageTrendPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		WITH top_keys AS (
			SELECT api_key_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY api_key_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.api_key_id,
			COALESCE(k.name, '') as key_name,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens
		FROM usage_logs u
		LEFT JOIN api_keys k ON u.api_key_id = k.id
		WHERE u.api_key_id IN (SELECT api_key_id FROM top_keys)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.api_key_id, k.name
		ORDER BY date ASC, tokens DESC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]APIKeyUsageTrendPoint, 0)
	for rows.Next() {
		var row APIKeyUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.APIKeyID, &row.KeyName, &row.Requests, &row.Tokens); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetUserUsageTrend returns usage trend data grouped by user and date
func (r *usageLogRepository) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) (results []UserUsageTrendPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		WITH top_users AS (
			SELECT user_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY user_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.user_id,
			COALESCE(us.email, '') as email,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens,
			COALESCE(SUM(u.total_cost), 0) as cost,
			COALESCE(SUM(u.actual_cost), 0) as actual_cost
		FROM usage_logs u
		LEFT JOIN users us ON u.user_id = us.id
		WHERE u.user_id IN (SELECT user_id FROM top_users)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.user_id, us.email
		ORDER BY date ASC, tokens DESC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]UserUsageTrendPoint, 0)
	for rows.Next() {
		var row UserUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.UserID, &row.Email, &row.Requests, &row.Tokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats = usagestats.UserDashboardStats

// GetUserDashboardStats 获取用户专属的仪表盘统计
func (r *usageLogRepository) GetUserDashboardStats(ctx context.Context, userID int64) (*UserDashboardStats, error) {
	stats := &UserDashboardStats{REDACTED
	today := timezone.Today()

	// API Key 统计
	if err := scanSingleRow(
		ctx,
		r.sql,
		"SELECT COUNT(*) FROM api_keys WHERE user_id = $1 AND deleted_at IS NULL",
		[]any{userIDREDACTED,
		&stats.TotalAPIKeys,
	); err != nil {
		return nil, err
REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		"SELECT COUNT(*) FROM api_keys WHERE user_id = $1 AND status = $2 AND deleted_at IS NULL",
		[]any{userID, service.StatusActiveREDACTED,
		&stats.ActiveAPIKeys,
	); err != nil {
		return nil, err
REDACTED

	// 累计 Token 统计
	totalStatsQuery := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		[]any{userIDREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens

	// 今日 Token 统计
	todayStatsQuery := `
		SELECT
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{userID, todayREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
	); err != nil {
		return nil, err
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	// 性能指标：RPM 和 TPM（最近1分钟，仅统计该用户的请求）
	rpm, tpm, err := r.getPerformanceStats(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

// getPerformanceStatsByAPIKey 获取指定 API Key 的 RPM 和 TPM（近5分钟平均值）
func (r *usageLogRepository) getPerformanceStatsByAPIKey(ctx context.Context, apiKeyID int64) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	query := `
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1 AND api_key_id = $2`
	args := []any{fiveMinutesAgo, apiKeyIDREDACTED

	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, args, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
REDACTED
	return requestCount / 5, tokenCount / 5, nil
REDACTED

// GetAPIKeyDashboardStats 获取指定 API Key 的仪表盘统计（按 api_key_id 过滤）
func (r *usageLogRepository) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*UserDashboardStats, error) {
	stats := &UserDashboardStats{REDACTED
	today := timezone.Today()

	// API Key 维度不需要统计 key 数量，设为 1
	stats.TotalAPIKeys = 1
	stats.ActiveAPIKeys = 1

	// 累计 Token 统计
	totalStatsQuery := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE api_key_id = $1
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		[]any{apiKeyIDREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens

	// 今日 Token 统计
	todayStatsQuery := `
		SELECT
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		FROM usage_logs
		WHERE api_key_id = $1 AND created_at >= $2
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{apiKeyID, todayREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
	); err != nil {
		return nil, err
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	// 性能指标：RPM 和 TPM（最近5分钟，按 API Key 过滤）
	rpm, tpm, err := r.getPerformanceStatsByAPIKey(ctx, apiKeyID)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

// GetUserUsageTrendByUserID 获取指定用户的使用趋势
func (r *usageLogRepository) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY date
		ORDER BY date ASC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, userID, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetUserModelStats 获取指定用户的模型统计
func (r *usageLogRepository) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) (results []ModelStat, err error) {
	query := `
		SELECT
			model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY model
		ORDER BY total_tokens DESC
	`

	rows, err := r.sql.QueryContext(ctx, query, userID, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanModelStatsRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters = usagestats.UsageLogFilters

// ListWithFilters lists usage logs with optional filters (for admin)
func (r *usageLogRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
REDACTED
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
REDACTED
	if filters.AccountID > 0 {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)+1))
		args = append(args, filters.AccountID)
REDACTED
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
REDACTED
	if filters.Model != "" {
		conditions = append(conditions, fmt.Sprintf("model = $%d", len(args)+1))
		args = append(args, filters.Model)
REDACTED
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
REDACTED
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)+1))
		args = append(args, *filters.EndTime)
REDACTED

	whereClause := buildWhere(conditions)
	var (
		logs []service.UsageLog
		page *pagination.PaginationResult
		err  error
	)
	if shouldUseFastUsageLogTotal(filters) {
		logs, page, err = r.listUsageLogsWithFastPagination(ctx, whereClause, args, params)
REDACTED else {
		logs, page, err = r.listUsageLogsWithPagination(ctx, whereClause, args, params)
REDACTED
	if err != nil {
		return nil, nil, err
REDACTED

	if err := r.hydrateUsageLogAssociations(ctx, logs); err != nil {
		return nil, nil, err
REDACTED
	return logs, page, nil
REDACTED

func shouldUseFastUsageLogTotal(filters UsageLogFilters) bool {
	if filters.ExactTotal {
		return false
REDACTED
	// 强选择过滤下记录集通常较小，保留精确总数。
	return filters.UserID == 0 && filters.APIKeyID == 0 && filters.AccountID == 0
REDACTED

// UsageStats represents usage statistics
type UsageStats = usagestats.UsageStats

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats = usagestats.BatchUserUsageStats

func normalizePositiveInt64IDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
REDACTED
	seen := make(map[int64]struct{REDACTED, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		out = append(out, id)
REDACTED
	return out
REDACTED

// GetBatchUserUsageStats gets today and total actual_cost for multiple users within a time range.
// If startTime is zero, defaults to 30 days ago.
func (r *usageLogRepository) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*BatchUserUsageStats, error) {
	result := make(map[int64]*BatchUserUsageStats)
	normalizedUserIDs := normalizePositiveInt64IDs(userIDs)
	if len(normalizedUserIDs) == 0 {
		return result, nil
REDACTED

	// 默认最近 30 天
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
REDACTED
	if endTime.IsZero() {
		endTime = time.Now()
REDACTED

	for _, id := range normalizedUserIDs {
		result[id] = &BatchUserUsageStats{UserID: idREDACTED
REDACTED

	query := `
		SELECT
			user_id,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $2 AND created_at < $3), 0) as total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $4), 0) as today_cost
		FROM usage_logs
		WHERE user_id = ANY($1)
		  AND created_at >= LEAST($2, $4)
		GROUP BY user_id
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedUserIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
REDACTED
	for rows.Next() {
		var userID int64
		var total float64
		var todayTotal float64
		if err := rows.Scan(&userID, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
		if stats, ok := result[userID]; ok {
			stats.TotalActualCost = total
			stats.TodayActualCost = todayTotal
	REDACTED
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// BatchAPIKeyUsageStats represents usage stats for a single API key
type BatchAPIKeyUsageStats = usagestats.BatchAPIKeyUsageStats

// GetBatchAPIKeyUsageStats gets today and total actual_cost for multiple API keys within a time range.
// If startTime is zero, defaults to 30 days ago.
func (r *usageLogRepository) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*BatchAPIKeyUsageStats, error) {
	result := make(map[int64]*BatchAPIKeyUsageStats)
	normalizedAPIKeyIDs := normalizePositiveInt64IDs(apiKeyIDs)
	if len(normalizedAPIKeyIDs) == 0 {
		return result, nil
REDACTED

	// 默认最近 30 天
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
REDACTED
	if endTime.IsZero() {
		endTime = time.Now()
REDACTED

	for _, id := range normalizedAPIKeyIDs {
		result[id] = &BatchAPIKeyUsageStats{APIKeyID: idREDACTED
REDACTED

	query := `
		SELECT
			api_key_id,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $2 AND created_at < $3), 0) as total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $4), 0) as today_cost
		FROM usage_logs
		WHERE api_key_id = ANY($1)
		  AND created_at >= LEAST($2, $4)
		GROUP BY api_key_id
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedAPIKeyIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
REDACTED
	for rows.Next() {
		var apiKeyID int64
		var total float64
		var todayTotal float64
		if err := rows.Scan(&apiKeyID, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
		if stats, ok := result[apiKeyID]; ok {
			stats.TotalActualCost = total
			stats.TodayActualCost = todayTotal
	REDACTED
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// GetUsageTrendWithFilters returns usage trend data with optional filters
func (r *usageLogRepository) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) (results []TrendDataPoint, err error) {
	if shouldUsePreaggregatedTrend(granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType) {
		aggregated, aggregatedErr := r.getUsageTrendFromAggregates(ctx, startTime, endTime, granularity)
		if aggregatedErr == nil && len(aggregated) > 0 {
			return aggregated, nil
	REDACTED
REDACTED

	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, dateFormat)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	if model != "" {
		query += fmt.Sprintf(" AND model = $%d", len(args)+1)
		args = append(args, model)
REDACTED
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query += " GROUP BY date ORDER BY date ASC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func shouldUsePreaggregatedTrend(granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) bool {
	if granularity != "day" && granularity != "hour" {
		return false
REDACTED
	return userID == 0 &&
		apiKeyID == 0 &&
		accountID == 0 &&
		groupID == 0 &&
		model == "" &&
		requestType == nil &&
		stream == nil &&
		billingType == nil
REDACTED

func (r *usageLogRepository) getUsageTrendFromAggregates(ctx context.Context, startTime, endTime time.Time, granularity string) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)
	query := ""
	args := []any{startTime, endTimeREDACTED

	switch granularity {
	case "hour":
		query = fmt.Sprintf(`
			SELECT
				TO_CHAR(bucket_start, '%s') as date,
				total_requests as requests,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) as total_tokens,
				total_cost as cost,
				actual_cost
			FROM usage_dashboard_hourly
			WHERE bucket_start >= $1 AND bucket_start < $2
			ORDER BY bucket_start ASC
		`, dateFormat)
	case "day":
		query = fmt.Sprintf(`
			SELECT
				TO_CHAR(bucket_date::timestamp, '%s') as date,
				total_requests as requests,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) as total_tokens,
				total_cost as cost,
				actual_cost
			FROM usage_dashboard_daily
			WHERE bucket_date >= $1::date AND bucket_date < $2::date
			ORDER BY bucket_date ASC
		`, dateFormat)
	default:
		return nil, nil
REDACTED

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetModelStatsWithFilters returns model statistics with optional filters
func (r *usageLogRepository) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) (results []ModelStat, err error) {
	actualCostExpr := "COALESCE(SUM(actual_cost), 0) as actual_cost"
	// 当仅按 account_id 聚合时，实际费用使用账号倍率（total_cost * account_rate_multiplier）。
	if accountID > 0 && userID == 0 && apiKeyID == 0 {
		actualCostExpr = "COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost"
REDACTED

	query := fmt.Sprintf(`
		SELECT
			model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			%s
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, actualCostExpr)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query += " GROUP BY model ORDER BY total_tokens DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanModelStatsRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetGroupStatsWithFilters returns group usage statistics with optional filters
func (r *usageLogRepository) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) (results []usagestats.GroupStat, err error) {
	query := `
		SELECT
			COALESCE(ul.group_id, 0) as group_id,
			COALESCE(g.name, '') as group_name,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query += " GROUP BY ul.group_id, g.name ORDER BY total_tokens DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]usagestats.GroupStat, 0)
	for rows.Next() {
		var row usagestats.GroupStat
		if err := rows.Scan(
			&row.GroupID,
			&row.GroupName,
			&row.Requests,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetGlobalStats gets usage statistics for all users within a time range
func (r *usageLogRepository) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2
	`

	stats := &UsageStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return stats, nil
REDACTED

// GetStatsWithFilters gets usage statistics with optional filters
func (r *usageLogRepository) GetStatsWithFilters(ctx context.Context, filters UsageLogFilters) (*UsageStats, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
REDACTED
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
REDACTED
	if filters.AccountID > 0 {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)+1))
		args = append(args, filters.AccountID)
REDACTED
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
REDACTED
	if filters.Model != "" {
		conditions = append(conditions, fmt.Sprintf("model = $%d", len(args)+1))
		args = append(args, filters.Model)
REDACTED
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
REDACTED
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)+1))
		args = append(args, *filters.EndTime)
REDACTED

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as total_account_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		%s
	`, buildWhere(conditions))

	stats := &UsageStats{REDACTED
	var totalAccountCost float64
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		args,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&totalAccountCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	if filters.AccountID > 0 {
		stats.TotalAccountCost = &totalAccountCost
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return stats, nil
REDACTED

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory = usagestats.AccountUsageHistory

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary = usagestats.AccountUsageSummary

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse = usagestats.AccountUsageStatsResponse

// GetAccountUsageStats returns comprehensive usage statistics for an account over a time range
func (r *usageLogRepository) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (resp *AccountUsageStatsResponse, err error) {
	daysCount := int(endTime.Sub(startTime).Hours()/24) + 1
	if daysCount <= 0 {
		daysCount = 30
REDACTED

	query := `
		SELECT
			TO_CHAR(created_at, 'YYYY-MM-DD') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(total_cost * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY date
		ORDER BY date ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			resp = nil
	REDACTED
REDACTED()

	history := make([]AccountUsageHistory, 0)
	for rows.Next() {
		var date string
		var requests int64
		var tokens int64
		var cost float64
		var actualCost float64
		var userCost float64
		if err = rows.Scan(&date, &requests, &tokens, &cost, &actualCost, &userCost); err != nil {
			return nil, err
	REDACTED
		t, _ := time.Parse("2006-01-02", date)
		history = append(history, AccountUsageHistory{
			Date:       date,
			Label:      t.Format("01/02"),
			Requests:   requests,
			Tokens:     tokens,
			Cost:       cost,
			ActualCost: actualCost,
			UserCost:   userCost,
	REDACTED)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	var totalAccountCost, totalUserCost, totalStandardCost float64
	var totalRequests, totalTokens int64
	var highestCostDay, highestRequestDay *AccountUsageHistory

	for i := range history {
		h := &history[i]
		totalAccountCost += h.ActualCost
		totalUserCost += h.UserCost
		totalStandardCost += h.Cost
		totalRequests += h.Requests
		totalTokens += h.Tokens

		if highestCostDay == nil || h.ActualCost > highestCostDay.ActualCost {
			highestCostDay = h
	REDACTED
		if highestRequestDay == nil || h.Requests > highestRequestDay.Requests {
			highestRequestDay = h
	REDACTED
REDACTED

	actualDaysUsed := len(history)
	if actualDaysUsed == 0 {
		actualDaysUsed = 1
REDACTED

	avgQuery := "SELECT COALESCE(AVG(duration_ms), 0) as avg_duration_ms FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3"
	var avgDuration float64
	if err := scanSingleRow(ctx, r.sql, avgQuery, []any{accountID, startTime, endTimeREDACTED, &avgDuration); err != nil {
		return nil, err
REDACTED

	summary := AccountUsageSummary{
		Days:              daysCount,
		ActualDaysUsed:    actualDaysUsed,
		TotalCost:         totalAccountCost,
		TotalUserCost:     totalUserCost,
		TotalStandardCost: totalStandardCost,
		TotalRequests:     totalRequests,
		TotalTokens:       totalTokens,
		AvgDailyCost:      totalAccountCost / float64(actualDaysUsed),
		AvgDailyUserCost:  totalUserCost / float64(actualDaysUsed),
		AvgDailyRequests:  float64(totalRequests) / float64(actualDaysUsed),
		AvgDailyTokens:    float64(totalTokens) / float64(actualDaysUsed),
		AvgDurationMs:     avgDuration,
REDACTED

	todayStr := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date == todayStr {
			summary.Today = &struct {
				Date     string  `json:"date"`
				Cost     float64 `json:"cost"`
				UserCost float64 `json:"user_cost"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
		REDACTED{
				Date:     history[i].Date,
				Cost:     history[i].ActualCost,
				UserCost: history[i].UserCost,
				Requests: history[i].Requests,
				Tokens:   history[i].Tokens,
		REDACTED
			break
	REDACTED
REDACTED

	if highestCostDay != nil {
		summary.HighestCostDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
			Requests int64   `json:"requests"`
	REDACTED{
			Date:     highestCostDay.Date,
			Label:    highestCostDay.Label,
			Cost:     highestCostDay.ActualCost,
			UserCost: highestCostDay.UserCost,
			Requests: highestCostDay.Requests,
	REDACTED
REDACTED

	if highestRequestDay != nil {
		summary.HighestRequestDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Requests int64   `json:"requests"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
	REDACTED{
			Date:     highestRequestDay.Date,
			Label:    highestRequestDay.Label,
			Requests: highestRequestDay.Requests,
			Cost:     highestRequestDay.ActualCost,
			UserCost: highestRequestDay.UserCost,
	REDACTED
REDACTED

	models, err := r.GetModelStatsWithFilters(ctx, startTime, endTime, 0, 0, accountID, 0, nil, nil, nil)
	if err != nil {
		models = []ModelStat{REDACTED
REDACTED

	resp = &AccountUsageStatsResponse{
		History: history,
		Summary: summary,
		Models:  models,
REDACTED
	return resp, nil
REDACTED

func (r *usageLogRepository) listUsageLogsWithPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	countQuery := "SELECT COUNT(*) FROM usage_logs " + whereClause
	var total int64
	if err := scanSingleRow(ctx, r.sql, countQuery, args, &total); err != nil {
		return nil, nil, err
REDACTED

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{REDACTED, args...), params.Limit(), params.Offset())
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY id DESC LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, limitPos, offsetPos)
	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
REDACTED
	return logs, paginationResultFromTotal(total, params), nil
REDACTED

func (r *usageLogRepository) listUsageLogsWithFastPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	limit := params.Limit()
	offset := params.Offset()

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{REDACTED, args...), limit+1, offset)
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY id DESC LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, limitPos, offsetPos)

	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
REDACTED

	hasMore := false
	if len(logs) > limit {
		hasMore = true
		logs = logs[:limit]
REDACTED

	total := int64(offset) + int64(len(logs))
	if hasMore {
		// 只保证“还有下一页”，避免对超大表做全量 COUNT(*)。
		total = int64(offset) + int64(limit) + 1
REDACTED

	return logs, paginationResultFromTotal(total, params), nil
REDACTED

func (r *usageLogRepository) queryUsageLogs(ctx context.Context, query string, args ...any) (logs []service.UsageLog, err error) {
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			logs = nil
	REDACTED
REDACTED()

	logs = make([]service.UsageLog, 0)
	for rows.Next() {
		var log *service.UsageLog
		log, err = scanUsageLog(rows)
		if err != nil {
			return nil, err
	REDACTED
		logs = append(logs, *log)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED
	return logs, nil
REDACTED

func (r *usageLogRepository) hydrateUsageLogAssociations(ctx context.Context, logs []service.UsageLog) error {
	// 关联数据使用 Ent 批量加载，避免把复杂 SQL 继续膨胀。
	if len(logs) == 0 {
		return nil
REDACTED

	ids := collectUsageLogIDs(logs)
	users, err := r.loadUsers(ctx, ids.userIDs)
	if err != nil {
		return err
REDACTED
	apiKeys, err := r.loadAPIKeys(ctx, ids.apiKeyIDs)
	if err != nil {
		return err
REDACTED
	accounts, err := r.loadAccounts(ctx, ids.accountIDs)
	if err != nil {
		return err
REDACTED
	groups, err := r.loadGroups(ctx, ids.groupIDs)
	if err != nil {
		return err
REDACTED
	subs, err := r.loadSubscriptions(ctx, ids.subscriptionIDs)
	if err != nil {
		return err
REDACTED

	for i := range logs {
		if user, ok := users[logs[i].UserID]; ok {
			logs[i].User = user
	REDACTED
		if key, ok := apiKeys[logs[i].APIKeyID]; ok {
			logs[i].APIKey = key
	REDACTED
		if acc, ok := accounts[logs[i].AccountID]; ok {
			logs[i].Account = acc
	REDACTED
		if logs[i].GroupID != nil {
			if group, ok := groups[*logs[i].GroupID]; ok {
				logs[i].Group = group
		REDACTED
	REDACTED
		if logs[i].SubscriptionID != nil {
			if sub, ok := subs[*logs[i].SubscriptionID]; ok {
				logs[i].Subscription = sub
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

type usageLogIDs struct {
	userIDs         []int64
	apiKeyIDs       []int64
	accountIDs      []int64
	groupIDs        []int64
	subscriptionIDs []int64
REDACTED

func collectUsageLogIDs(logs []service.UsageLog) usageLogIDs {
	idSet := func() map[int64]struct{REDACTED { return make(map[int64]struct{REDACTED) REDACTED

	userIDs := idSet()
	apiKeyIDs := idSet()
	accountIDs := idSet()
	groupIDs := idSet()
	subscriptionIDs := idSet()

	for i := range logs {
		userIDs[logs[i].UserID] = struct{REDACTED{REDACTED
		apiKeyIDs[logs[i].APIKeyID] = struct{REDACTED{REDACTED
		accountIDs[logs[i].AccountID] = struct{REDACTED{REDACTED
		if logs[i].GroupID != nil {
			groupIDs[*logs[i].GroupID] = struct{REDACTED{REDACTED
	REDACTED
		if logs[i].SubscriptionID != nil {
			subscriptionIDs[*logs[i].SubscriptionID] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	return usageLogIDs{
		userIDs:         setToSlice(userIDs),
		apiKeyIDs:       setToSlice(apiKeyIDs),
		accountIDs:      setToSlice(accountIDs),
		groupIDs:        setToSlice(groupIDs),
		subscriptionIDs: setToSlice(subscriptionIDs),
REDACTED
REDACTED

func (r *usageLogRepository) loadUsers(ctx context.Context, ids []int64) (map[int64]*service.User, error) {
	out := make(map[int64]*service.User)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.User.Query().Where(dbuser.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = userEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadAPIKeys(ctx context.Context, ids []int64) (map[int64]*service.APIKey, error) {
	out := make(map[int64]*service.APIKey)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.APIKey.Query().Where(dbapikey.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = apiKeyEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadAccounts(ctx context.Context, ids []int64) (map[int64]*service.Account, error) {
	out := make(map[int64]*service.Account)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.Account.Query().Where(dbaccount.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = accountEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadGroups(ctx context.Context, ids []int64) (map[int64]*service.Group, error) {
	out := make(map[int64]*service.Group)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.Group.Query().Where(dbgroup.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = groupEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadSubscriptions(ctx context.Context, ids []int64) (map[int64]*service.UserSubscription, error) {
	out := make(map[int64]*service.UserSubscription)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.UserSubscription.Query().Where(dbusersub.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = userSubscriptionEntityToService(m)
REDACTED
	return out, nil
REDACTED

func scanUsageLog(scanner interface{ Scan(...any) error REDACTED) (*service.UsageLog, error) {
	var (
		id                    int64
		userID                int64
		apiKeyID              int64
		accountID             int64
		requestID             sql.NullString
		model                 string
		groupID               sql.NullInt64
		subscriptionID        sql.NullInt64
		inputTokens           int
		outputTokens          int
		cacheCreationTokens   int
		cacheReadTokens       int
		cacheCreation5m       int
		cacheCreation1h       int
		inputCost             float64
		outputCost            float64
		cacheCreationCost     float64
		cacheReadCost         float64
		totalCost             float64
		actualCost            float64
		rateMultiplier        float64
		accountRateMultiplier sql.NullFloat64
		billingType           int16
		requestTypeRaw        int16
		stream                bool
		openaiWSMode          bool
		durationMs            sql.NullInt64
		firstTokenMs          sql.NullInt64
		userAgent             sql.NullString
		ipAddress             sql.NullString
		imageCount            int
		imageSize             sql.NullString
		mediaType             sql.NullString
		serviceTier           sql.NullString
		reasoningEffort       sql.NullString
		cacheTTLOverridden    bool
		createdAt             time.Time
	)

	if err := scanner.Scan(
		&id,
		&userID,
		&apiKeyID,
		&accountID,
		&requestID,
		&model,
		&groupID,
		&subscriptionID,
		&inputTokens,
		&outputTokens,
		&cacheCreationTokens,
		&cacheReadTokens,
		&cacheCreation5m,
		&cacheCreation1h,
		&inputCost,
		&outputCost,
		&cacheCreationCost,
		&cacheReadCost,
		&totalCost,
		&actualCost,
		&rateMultiplier,
		&accountRateMultiplier,
		&billingType,
		&requestTypeRaw,
		&stream,
		&openaiWSMode,
		&durationMs,
		&firstTokenMs,
		&userAgent,
		&ipAddress,
		&imageCount,
		&imageSize,
		&mediaType,
		&serviceTier,
		&reasoningEffort,
		&cacheTTLOverridden,
		&createdAt,
	); err != nil {
		return nil, err
REDACTED

	log := &service.UsageLog{
		ID:                    id,
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		Model:                 model,
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheReadTokens:       cacheReadTokens,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
		InputCost:             inputCost,
		OutputCost:            outputCost,
		CacheCreationCost:     cacheCreationCost,
		CacheReadCost:         cacheReadCost,
		TotalCost:             totalCost,
		ActualCost:            actualCost,
		RateMultiplier:        rateMultiplier,
		AccountRateMultiplier: nullFloat64Ptr(accountRateMultiplier),
		BillingType:           int8(billingType),
		RequestType:           service.RequestTypeFromInt16(requestTypeRaw),
		ImageCount:            imageCount,
		CacheTTLOverridden:    cacheTTLOverridden,
		CreatedAt:             createdAt,
REDACTED
	// 先回填 legacy 字段，再基于 legacy + request_type 计算最终请求类型，保证历史数据兼容。
	log.Stream = stream
	log.OpenAIWSMode = openaiWSMode
	log.RequestType = log.EffectiveRequestType()
	log.Stream, log.OpenAIWSMode = service.ApplyLegacyRequestFields(log.RequestType, stream, openaiWSMode)

	if requestID.Valid {
		log.RequestID = requestID.String
REDACTED
	if groupID.Valid {
		value := groupID.Int64
		log.GroupID = &value
REDACTED
	if subscriptionID.Valid {
		value := subscriptionID.Int64
		log.SubscriptionID = &value
REDACTED
	if durationMs.Valid {
		value := int(durationMs.Int64)
		log.DurationMs = &value
REDACTED
	if firstTokenMs.Valid {
		value := int(firstTokenMs.Int64)
		log.FirstTokenMs = &value
REDACTED
	if userAgent.Valid {
		log.UserAgent = &userAgent.String
REDACTED
	if ipAddress.Valid {
		log.IPAddress = &ipAddress.String
REDACTED
	if imageSize.Valid {
		log.ImageSize = &imageSize.String
REDACTED
	if mediaType.Valid {
		log.MediaType = &mediaType.String
REDACTED
	if serviceTier.Valid {
		log.ServiceTier = &serviceTier.String
REDACTED
	if reasoningEffort.Valid {
		log.ReasoningEffort = &reasoningEffort.String
REDACTED

	return log, nil
REDACTED

func scanTrendRows(rows *sql.Rows) ([]TrendDataPoint, error) {
	results := make([]TrendDataPoint, 0)
	for rows.Next() {
		var row TrendDataPoint
		if err := rows.Scan(
			&row.Date,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func scanModelStatsRows(rows *sql.Rows) ([]ModelStat, error) {
	results := make([]ModelStat, 0)
	for rows.Next() {
		var row ModelStat
		if err := rows.Scan(
			&row.Model,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func buildWhere(conditions []string) string {
	if len(conditions) == 0 {
		return ""
REDACTED
	return "WHERE " + strings.Join(conditions, " AND ")
REDACTED

func appendRequestTypeOrStreamWhereCondition(conditions []string, args []any, requestType *int16, stream *bool) ([]string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
		return conditions, args
REDACTED
	if stream != nil {
		conditions = append(conditions, fmt.Sprintf("stream = $%d", len(args)+1))
		args = append(args, *stream)
REDACTED
	return conditions, args
REDACTED

func appendRequestTypeOrStreamQueryFilter(query string, args []any, requestType *int16, stream *bool) (string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		query += " AND " + condition
		args = append(args, conditionArgs...)
		return query, args
REDACTED
	if stream != nil {
		query += fmt.Sprintf(" AND stream = $%d", len(args)+1)
		args = append(args, *stream)
REDACTED
	return query, args
REDACTED

// buildRequestTypeFilterCondition 在 request_type 过滤时兼容 legacy 字段，避免历史数据漏查。
func buildRequestTypeFilterCondition(startArgIndex int, requestType int16) (string, []any) {
	normalized := service.RequestTypeFromInt16(requestType)
	requestTypeArg := int16(normalized)
	switch normalized {
	case service.RequestTypeSync:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND stream = FALSE AND openai_ws_mode = FALSE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	case service.RequestTypeStream:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND stream = TRUE AND openai_ws_mode = FALSE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	case service.RequestTypeWSV2:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND openai_ws_mode = TRUE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	default:
		return fmt.Sprintf("request_type = $%d", startArgIndex), []any{requestTypeArgREDACTED
REDACTED
REDACTED

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{REDACTED
REDACTED
	return sql.NullInt64{Int64: *v, Valid: trueREDACTED
REDACTED

func nullInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{REDACTED
REDACTED
	return sql.NullInt64{Int64: int64(*v), Valid: trueREDACTED
REDACTED

func nullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
REDACTED
	out := v.Float64
	return &out
REDACTED

func nullString(v *string) sql.NullString {
	if v == nil || *v == "" {
		return sql.NullString{REDACTED
REDACTED
	return sql.NullString{String: *v, Valid: trueREDACTED
REDACTED

func setToSlice(set map[int64]struct{REDACTED) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
REDACTED
	return out
REDACTED
