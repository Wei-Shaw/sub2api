package repository

import (
	"context"
	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/pkg/timezone"
	"sub2api/internal/pkg/usagestats"
	"time"

	"gorm.io/gorm"
)

type UsageLogRepository struct {
	db *gorm.DB
REDACTED

func NewUsageLogRepository(db *gorm.DB) *UsageLogRepository {
	return &UsageLogRepository{db: dbREDACTED
REDACTED

func (r *UsageLogRepository) Create(ctx context.Context, log *model.UsageLog) error {
	return r.db.WithContext(ctx).Create(log).Error
REDACTED

func (r *UsageLogRepository) GetByID(ctx context.Context, id int64) (*model.UsageLog, error) {
	var log model.UsageLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err != nil {
		return nil, err
REDACTED
	return &log, nil
REDACTED

func (r *UsageLogRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	var total int64

	db := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).Where("user_id = ?", userID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return logs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *UsageLogRepository) ListByApiKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	var total int64

	db := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).Where("api_key_id = ?", apiKeyID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return logs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
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

func (r *UsageLogRepository) GetUserStats(ctx context.Context, userID int64, startTime, endTime time.Time) (*UserStats, error) {
	var stats UserStats
	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(actual_cost), 0) as total_cost,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens
		`).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startTime, endTime).
		Scan(&stats).Error
	return &stats, err
REDACTED

// DashboardStats 仪表盘统计
type DashboardStats struct {
	// 用户统计
	TotalUsers    int64 `json:"total_users"`
	TodayNewUsers int64 `json:"today_new_users"` // 今日新增用户数
	ActiveUsers   int64 `json:"active_users"`    // 今日有请求的用户数

	// API Key 统计
	TotalApiKeys  int64 `json:"total_api_keys"`
	ActiveApiKeys int64 `json:"active_api_keys"` // 状态为 active 的 API Key 数

	// 账户统计
	TotalAccounts     int64 `json:"total_accounts"`
	NormalAccounts    int64 `json:"normal_accounts"`    // 正常账户数 (schedulable=true, status=active)
	ErrorAccounts     int64 `json:"error_accounts"`     // 异常账户数 (status=error)
	RateLimitAccounts int64 `json:"ratelimit_accounts"` // 限流账户数
	OverloadAccounts  int64 `json:"overload_accounts"`  // 过载账户数

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`        // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"` // 累计实际扣除

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`        // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"` // 今日实际扣除

	// 系统运行统计
	AverageDurationMs float64 `json:"average_duration_ms"` // 平均响应时间
REDACTED

func (r *UsageLogRepository) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
	var stats DashboardStats
	today := timezone.Today()

	// 总用户数
	r.db.WithContext(ctx).Model(&model.User{REDACTED).Count(&stats.TotalUsers)

	// 今日新增用户数
	r.db.WithContext(ctx).Model(&model.User{REDACTED).
		Where("created_at >= ?", today).
		Count(&stats.TodayNewUsers)

	// 今日活跃用户数 (今日有请求的用户)
	r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Distinct("user_id").
		Where("created_at >= ?", today).
		Count(&stats.ActiveUsers)

	// 总 API Key 数
	r.db.WithContext(ctx).Model(&model.ApiKey{REDACTED).Count(&stats.TotalApiKeys)

	// 活跃 API Key 数
	r.db.WithContext(ctx).Model(&model.ApiKey{REDACTED).
		Where("status = ?", model.StatusActive).
		Count(&stats.ActiveApiKeys)

	// 总账户数
	r.db.WithContext(ctx).Model(&model.Account{REDACTED).Count(&stats.TotalAccounts)

	// 正常账户数 (schedulable=true, status=active)
	r.db.WithContext(ctx).Model(&model.Account{REDACTED).
		Where("status = ? AND schedulable = ?", model.StatusActive, true).
		Count(&stats.NormalAccounts)

	// 异常账户数 (status=error)
	r.db.WithContext(ctx).Model(&model.Account{REDACTED).
		Where("status = ?", model.StatusError).
		Count(&stats.ErrorAccounts)

	// 限流账户数
	r.db.WithContext(ctx).Model(&model.Account{REDACTED).
		Where("rate_limited_at IS NOT NULL AND rate_limit_reset_at > ?", time.Now()).
		Count(&stats.RateLimitAccounts)

	// 过载账户数
	r.db.WithContext(ctx).Model(&model.Account{REDACTED).
		Where("overload_until IS NOT NULL AND overload_until > ?", time.Now()).
		Count(&stats.OverloadAccounts)

	// 累计 Token 统计
	var totalStats struct {
		TotalRequests            int64   `gorm:"column:total_requests"`
		TotalInputTokens         int64   `gorm:"column:total_input_tokens"`
		TotalOutputTokens        int64   `gorm:"column:total_output_tokens"`
		TotalCacheCreationTokens int64   `gorm:"column:total_cache_creation_tokens"`
		TotalCacheReadTokens     int64   `gorm:"column:total_cache_read_tokens"`
		TotalCost                float64 `gorm:"column:total_cost"`
		TotalActualCost          float64 `gorm:"column:total_actual_cost"`
		AverageDurationMs        float64 `gorm:"column:avg_duration_ms"`
REDACTED
	r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		`).
		Scan(&totalStats)

	stats.TotalRequests = totalStats.TotalRequests
	stats.TotalInputTokens = totalStats.TotalInputTokens
	stats.TotalOutputTokens = totalStats.TotalOutputTokens
	stats.TotalCacheCreationTokens = totalStats.TotalCacheCreationTokens
	stats.TotalCacheReadTokens = totalStats.TotalCacheReadTokens
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	stats.TotalCost = totalStats.TotalCost
	stats.TotalActualCost = totalStats.TotalActualCost
	stats.AverageDurationMs = totalStats.AverageDurationMs

	// 今日 Token 统计
	var todayStats struct {
		TodayRequests            int64   `gorm:"column:today_requests"`
		TodayInputTokens         int64   `gorm:"column:today_input_tokens"`
		TodayOutputTokens        int64   `gorm:"column:today_output_tokens"`
		TodayCacheCreationTokens int64   `gorm:"column:today_cache_creation_tokens"`
		TodayCacheReadTokens     int64   `gorm:"column:today_cache_read_tokens"`
		TodayCost                float64 `gorm:"column:today_cost"`
		TodayActualCost          float64 `gorm:"column:today_actual_cost"`
REDACTED
	r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		`).
		Where("created_at >= ?", today).
		Scan(&todayStats)

	stats.TodayRequests = todayStats.TodayRequests
	stats.TodayInputTokens = todayStats.TodayInputTokens
	stats.TodayOutputTokens = todayStats.TodayOutputTokens
	stats.TodayCacheCreationTokens = todayStats.TodayCacheCreationTokens
	stats.TodayCacheReadTokens = todayStats.TodayCacheReadTokens
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens
	stats.TodayCost = todayStats.TodayCost
	stats.TodayActualCost = todayStats.TodayActualCost

	return &stats, nil
REDACTED

func (r *UsageLogRepository) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	var total int64

	db := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).Where("account_id = ?", accountID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return logs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *UsageLogRepository) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return logs, nil, err
REDACTED

func (r *UsageLogRepository) ListByApiKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	err := r.db.WithContext(ctx).
		Where("api_key_id = ? AND created_at >= ? AND created_at < ?", apiKeyID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return logs, nil, err
REDACTED

func (r *UsageLogRepository) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return logs, nil, err
REDACTED

func (r *UsageLogRepository) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	err := r.db.WithContext(ctx).
		Where("model = ? AND created_at >= ? AND created_at < ?", modelName, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return logs, nil, err
REDACTED

func (r *UsageLogRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.UsageLog{REDACTED, id).Error
REDACTED

// GetAccountTodayStats 获取账号今日统计
func (r *UsageLogRepository) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	today := timezone.Today()

	var stats struct {
		Requests int64   `gorm:"column:requests"`
		Tokens   int64   `gorm:"column:tokens"`
		Cost     float64 `gorm:"column:cost"`
REDACTED

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(actual_cost), 0) as cost
		`).
		Where("account_id = ? AND created_at >= ?", accountID, today).
		Scan(&stats).Error

	if err != nil {
		return nil, err
REDACTED

	return &usagestats.AccountStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
REDACTED, nil
REDACTED

// GetAccountWindowStats 获取账号时间窗口内的统计
func (r *UsageLogRepository) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	var stats struct {
		Requests int64   `gorm:"column:requests"`
		Tokens   int64   `gorm:"column:tokens"`
		Cost     float64 `gorm:"column:cost"`
REDACTED

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(actual_cost), 0) as cost
		`).
		Where("account_id = ? AND created_at >= ?", accountID, startTime).
		Scan(&stats).Error

	if err != nil {
		return nil, err
REDACTED

	return &usagestats.AccountStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
REDACTED, nil
REDACTED

// TrendDataPoint represents a single point in trend data
type TrendDataPoint struct {
	Date         string  `json:"date"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheTokens  int64   `json:"cache_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`        // 标准计费
	ActualCost   float64 `json:"actual_cost"` // 实际扣除
REDACTED

// ModelStat represents usage statistics for a single model
type ModelStat struct {
	Model        string  `json:"model"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`        // 标准计费
	ActualCost   float64 `json:"actual_cost"` // 实际扣除
REDACTED

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint struct {
	Date       string  `json:"date"`
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费
	ActualCost float64 `json:"actual_cost"` // 实际扣除
REDACTED

// GetUsageTrend returns usage trend data grouped by date
// granularity: "day" or "hour"
func (r *UsageLogRepository) GetUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string) ([]TrendDataPoint, error) {
	var results []TrendDataPoint

	// Choose date format based on granularity
	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
REDACTED else {
		dateFormat = "YYYY-MM-DD"
REDACTED

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			TO_CHAR(created_at, ?) as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as cache_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`, dateFormat).
		Where("created_at >= ? AND created_at < ?", startTime, endTime).
		Group("date").
		Order("date ASC").
		Scan(&results).Error

	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetModelStats returns usage statistics grouped by model
func (r *UsageLogRepository) GetModelStats(ctx context.Context, startTime, endTime time.Time) ([]ModelStat, error) {
	var results []ModelStat

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`).
		Where("created_at >= ? AND created_at < ?", startTime, endTime).
		Group("model").
		Order("total_tokens DESC").
		Scan(&results).Error

	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// ApiKeyUsageTrendPoint represents API key usage trend data point
type ApiKeyUsageTrendPoint struct {
	Date     string `json:"date"`
	ApiKeyID int64  `json:"api_key_id"`
	KeyName  string `json:"key_name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
REDACTED

// GetApiKeyUsageTrend returns usage trend data grouped by API key and date
func (r *UsageLogRepository) GetApiKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]ApiKeyUsageTrendPoint, error) {
	var results []ApiKeyUsageTrendPoint

	// Choose date format based on granularity
	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
REDACTED else {
		dateFormat = "YYYY-MM-DD"
REDACTED

	// Use raw SQL for complex subquery
	query := `
		WITH top_keys AS (
			SELECT api_key_id
			FROM usage_logs
			WHERE created_at >= ? AND created_at < ?
			GROUP BY api_key_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT ?
		)
		SELECT
			TO_CHAR(u.created_at, '` + dateFormat + `') as date,
			u.api_key_id,
			COALESCE(k.name, '') as key_name,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens
		FROM usage_logs u
		LEFT JOIN api_keys k ON u.api_key_id = k.id
		WHERE u.api_key_id IN (SELECT api_key_id FROM top_keys)
		  AND u.created_at >= ? AND u.created_at < ?
		GROUP BY date, u.api_key_id, k.name
		ORDER BY date ASC, tokens DESC
	`

	err := r.db.WithContext(ctx).Raw(query, startTime, endTime, limit, startTime, endTime).Scan(&results).Error
	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetUserUsageTrend returns usage trend data grouped by user and date
func (r *UsageLogRepository) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]UserUsageTrendPoint, error) {
	var results []UserUsageTrendPoint

	// Choose date format based on granularity
	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
REDACTED else {
		dateFormat = "YYYY-MM-DD"
REDACTED

	// Use raw SQL for complex subquery
	query := `
		WITH top_users AS (
			SELECT user_id
			FROM usage_logs
			WHERE created_at >= ? AND created_at < ?
			GROUP BY user_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT ?
		)
		SELECT
			TO_CHAR(u.created_at, '` + dateFormat + `') as date,
			u.user_id,
			COALESCE(us.email, '') as email,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens,
			COALESCE(SUM(u.total_cost), 0) as cost,
			COALESCE(SUM(u.actual_cost), 0) as actual_cost
		FROM usage_logs u
		LEFT JOIN users us ON u.user_id = us.id
		WHERE u.user_id IN (SELECT user_id FROM top_users)
		  AND u.created_at >= ? AND u.created_at < ?
		GROUP BY date, u.user_id, us.email
		ORDER BY date ASC, tokens DESC
	`

	err := r.db.WithContext(ctx).Raw(query, startTime, endTime, limit, startTime, endTime).Scan(&results).Error
	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats struct {
	// API Key 统计
	TotalApiKeys  int64 `json:"total_api_keys"`
	ActiveApiKeys int64 `json:"active_api_keys"`

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`        // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"` // 累计实际扣除

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`        // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"` // 今日实际扣除

	// 性能统计
	AverageDurationMs float64 `json:"average_duration_ms"`
REDACTED

// GetUserDashboardStats 获取用户专属的仪表盘统计
func (r *UsageLogRepository) GetUserDashboardStats(ctx context.Context, userID int64) (*UserDashboardStats, error) {
	var stats UserDashboardStats
	today := timezone.Today()

	// API Key 统计
	r.db.WithContext(ctx).Model(&model.ApiKey{REDACTED).
		Where("user_id = ?", userID).
		Count(&stats.TotalApiKeys)

	r.db.WithContext(ctx).Model(&model.ApiKey{REDACTED).
		Where("user_id = ? AND status = ?", userID, model.StatusActive).
		Count(&stats.ActiveApiKeys)

	// 累计 Token 统计
	var totalStats struct {
		TotalRequests            int64   `gorm:"column:total_requests"`
		TotalInputTokens         int64   `gorm:"column:total_input_tokens"`
		TotalOutputTokens        int64   `gorm:"column:total_output_tokens"`
		TotalCacheCreationTokens int64   `gorm:"column:total_cache_creation_tokens"`
		TotalCacheReadTokens     int64   `gorm:"column:total_cache_read_tokens"`
		TotalCost                float64 `gorm:"column:total_cost"`
		TotalActualCost          float64 `gorm:"column:total_actual_cost"`
		AverageDurationMs        float64 `gorm:"column:avg_duration_ms"`
REDACTED
	r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		`).
		Where("user_id = ?", userID).
		Scan(&totalStats)

	stats.TotalRequests = totalStats.TotalRequests
	stats.TotalInputTokens = totalStats.TotalInputTokens
	stats.TotalOutputTokens = totalStats.TotalOutputTokens
	stats.TotalCacheCreationTokens = totalStats.TotalCacheCreationTokens
	stats.TotalCacheReadTokens = totalStats.TotalCacheReadTokens
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	stats.TotalCost = totalStats.TotalCost
	stats.TotalActualCost = totalStats.TotalActualCost
	stats.AverageDurationMs = totalStats.AverageDurationMs

	// 今日 Token 统计
	var todayStats struct {
		TodayRequests            int64   `gorm:"column:today_requests"`
		TodayInputTokens         int64   `gorm:"column:today_input_tokens"`
		TodayOutputTokens        int64   `gorm:"column:today_output_tokens"`
		TodayCacheCreationTokens int64   `gorm:"column:today_cache_creation_tokens"`
		TodayCacheReadTokens     int64   `gorm:"column:today_cache_read_tokens"`
		TodayCost                float64 `gorm:"column:today_cost"`
		TodayActualCost          float64 `gorm:"column:today_actual_cost"`
REDACTED
	r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		`).
		Where("user_id = ? AND created_at >= ?", userID, today).
		Scan(&todayStats)

	stats.TodayRequests = todayStats.TodayRequests
	stats.TodayInputTokens = todayStats.TodayInputTokens
	stats.TodayOutputTokens = todayStats.TodayOutputTokens
	stats.TodayCacheCreationTokens = todayStats.TodayCacheCreationTokens
	stats.TodayCacheReadTokens = todayStats.TodayCacheReadTokens
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens
	stats.TodayCost = todayStats.TodayCost
	stats.TodayActualCost = todayStats.TodayActualCost

	return &stats, nil
REDACTED

// GetUserUsageTrendByUserID 获取指定用户的使用趋势
func (r *UsageLogRepository) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]TrendDataPoint, error) {
	var results []TrendDataPoint

	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
REDACTED else {
		dateFormat = "YYYY-MM-DD"
REDACTED

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			TO_CHAR(created_at, ?) as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as cache_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`, dateFormat).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startTime, endTime).
		Group("date").
		Order("date ASC").
		Scan(&results).Error

	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetUserModelStats 获取指定用户的模型统计
func (r *UsageLogRepository) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]ModelStat, error) {
	var results []ModelStat

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startTime, endTime).
		Group("model").
		Order("total_tokens DESC").
		Scan(&results).Error

	if err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters struct {
	UserID    int64
	ApiKeyID  int64
	StartTime *time.Time
	EndTime   *time.Time
REDACTED

// ListWithFilters lists usage logs with optional filters (for admin)
func (r *UsageLogRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UsageLogFilters) ([]model.UsageLog, *pagination.PaginationResult, error) {
	var logs []model.UsageLog
	var total int64

	db := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED)

	// Apply filters
	if filters.UserID > 0 {
		db = db.Where("user_id = ?", filters.UserID)
REDACTED
	if filters.ApiKeyID > 0 {
		db = db.Where("api_key_id = ?", filters.ApiKeyID)
REDACTED
	if filters.StartTime != nil {
		db = db.Where("created_at >= ?", *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		db = db.Where("created_at <= ?", *filters.EndTime)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	// Preload user and api_key for display
	if err := db.Preload("User").Preload("ApiKey").
		Offset(params.Offset()).Limit(params.Limit()).
		Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return logs, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

// UsageStats represents usage statistics
type UsageStats struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCacheTokens  int64   `json:"total_cache_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	AverageDurationMs float64 `json:"average_duration_ms"`
REDACTED

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats struct {
	UserID          int64   `json:"user_id"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
REDACTED

// GetBatchUserUsageStats gets today and total actual_cost for multiple users
func (r *UsageLogRepository) GetBatchUserUsageStats(ctx context.Context, userIDs []int64) (map[int64]*BatchUserUsageStats, error) {
	if len(userIDs) == 0 {
		return make(map[int64]*BatchUserUsageStats), nil
REDACTED

	today := timezone.Today()
	result := make(map[int64]*BatchUserUsageStats)

	// Initialize result map
	for _, id := range userIDs {
		result[id] = &BatchUserUsageStats{UserID: idREDACTED
REDACTED

	// Get total actual_cost per user
	var totalStats []struct {
		UserID    int64   `gorm:"column:user_id"`
		TotalCost float64 `gorm:"column:total_cost"`
REDACTED
	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select("user_id, COALESCE(SUM(actual_cost), 0) as total_cost").
		Where("user_id IN ?", userIDs).
		Group("user_id").
		Scan(&totalStats).Error
	if err != nil {
		return nil, err
REDACTED

	for _, stat := range totalStats {
		if s, ok := result[stat.UserID]; ok {
			s.TotalActualCost = stat.TotalCost
	REDACTED
REDACTED

	// Get today actual_cost per user
	var todayStats []struct {
		UserID    int64   `gorm:"column:user_id"`
		TodayCost float64 `gorm:"column:today_cost"`
REDACTED
	err = r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select("user_id, COALESCE(SUM(actual_cost), 0) as today_cost").
		Where("user_id IN ? AND created_at >= ?", userIDs, today).
		Group("user_id").
		Scan(&todayStats).Error
	if err != nil {
		return nil, err
REDACTED

	for _, stat := range todayStats {
		if s, ok := result[stat.UserID]; ok {
			s.TodayActualCost = stat.TodayCost
	REDACTED
REDACTED

	return result, nil
REDACTED

// BatchApiKeyUsageStats represents usage stats for a single API key
type BatchApiKeyUsageStats struct {
	ApiKeyID        int64   `json:"api_key_id"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
REDACTED

// GetBatchApiKeyUsageStats gets today and total actual_cost for multiple API keys
func (r *UsageLogRepository) GetBatchApiKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*BatchApiKeyUsageStats, error) {
	if len(apiKeyIDs) == 0 {
		return make(map[int64]*BatchApiKeyUsageStats), nil
REDACTED

	today := timezone.Today()
	result := make(map[int64]*BatchApiKeyUsageStats)

	// Initialize result map
	for _, id := range apiKeyIDs {
		result[id] = &BatchApiKeyUsageStats{ApiKeyID: idREDACTED
REDACTED

	// Get total actual_cost per api key
	var totalStats []struct {
		ApiKeyID  int64   `gorm:"column:api_key_id"`
		TotalCost float64 `gorm:"column:total_cost"`
REDACTED
	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select("api_key_id, COALESCE(SUM(actual_cost), 0) as total_cost").
		Where("api_key_id IN ?", apiKeyIDs).
		Group("api_key_id").
		Scan(&totalStats).Error
	if err != nil {
		return nil, err
REDACTED

	for _, stat := range totalStats {
		if s, ok := result[stat.ApiKeyID]; ok {
			s.TotalActualCost = stat.TotalCost
	REDACTED
REDACTED

	// Get today actual_cost per api key
	var todayStats []struct {
		ApiKeyID  int64   `gorm:"column:api_key_id"`
		TodayCost float64 `gorm:"column:today_cost"`
REDACTED
	err = r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select("api_key_id, COALESCE(SUM(actual_cost), 0) as today_cost").
		Where("api_key_id IN ? AND created_at >= ?", apiKeyIDs, today).
		Group("api_key_id").
		Scan(&todayStats).Error
	if err != nil {
		return nil, err
REDACTED

	for _, stat := range todayStats {
		if s, ok := result[stat.ApiKeyID]; ok {
			s.TodayActualCost = stat.TodayCost
	REDACTED
REDACTED

	return result, nil
REDACTED

// GetGlobalStats gets usage statistics for all users within a time range
func (r *UsageLogRepository) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*UsageStats, error) {
	var stats struct {
		TotalRequests     int64   `gorm:"column:total_requests"`
		TotalInputTokens  int64   `gorm:"column:total_input_tokens"`
		TotalOutputTokens int64   `gorm:"column:total_output_tokens"`
		TotalCacheTokens  int64   `gorm:"column:total_cache_tokens"`
		TotalCost         float64 `gorm:"column:total_cost"`
		TotalActualCost   float64 `gorm:"column:total_actual_cost"`
		AverageDurationMs float64 `gorm:"column:avg_duration_ms"`
REDACTED

	err := r.db.WithContext(ctx).Model(&model.UsageLog{REDACTED).
		Select(`
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		`).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Scan(&stats).Error

	if err != nil {
		return nil, err
REDACTED

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
REDACTED, nil
REDACTED
