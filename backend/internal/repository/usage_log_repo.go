package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"gorm.io/gorm"
)

type usageLogRepository struct {
	db *gorm.DB
}

func NewUsageLogRepository(db *gorm.DB) service.UsageLogRepository {
	return &usageLogRepository{db: db}
}

// getPerformanceStats 获取 RPM 和 TPM（近5分钟平均值，可选按用户过滤）
func (r *usageLogRepository) getPerformanceStats(ctx context.Context, userID int64) (rpm, tpm int64) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	var perfStats struct {
		RequestCount int64 `gorm:"column:request_count"`
		TokenCount   int64 `gorm:"column:token_count"`
	}

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select(`
			COUNT(*) as request_count,
			COALESCE(SUM(input_tokens + output_tokens), 0) as token_count
		`).
		Where("created_at >= ?", fiveMinutesAgo)

	if userID > 0 {
		db = db.Where("user_id = ?", userID)
	}

	db.Scan(&perfStats)
	// 返回5分钟平均值
	return perfStats.RequestCount / 5, perfStats.TokenCount / 5
}

func (r *usageLogRepository) Create(ctx context.Context, log *service.UsageLog) error {
	m := usageLogModelFromService(log)
	err := r.db.WithContext(ctx).Create(m).Error
	if err == nil {
		applyUsageLogModelToService(log, m)
	}
	return err
}

func (r *usageLogRepository) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	var log usageLogModel
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUsageLogNotFound, nil)
	}
	return usageLogModelToService(&log), nil
}

func (r *usageLogRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	var total int64

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).Where("user_id = ?", userID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
	}

	return usageLogModelsToService(logs), paginationResultFromTotal(total, params), nil
}

func (r *usageLogRepository) ListByApiKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	var total int64

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).Where("api_key_id = ?", apiKeyID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
	}

	return usageLogModelsToService(logs), paginationResultFromTotal(total, params), nil
}

// UserStats 用户使用统计
type UserStats struct {
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
}

func (r *usageLogRepository) GetUserStats(ctx context.Context, userID int64, startTime, endTime time.Time) (*UserStats, error) {
	var stats UserStats
	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
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
}

// DashboardStats 仪表盘统计
type DashboardStats = usagestats.DashboardStats

func (r *usageLogRepository) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
	var stats DashboardStats
	today := timezone.Today()

	// 总用户数
	r.db.WithContext(ctx).Model(&userModel{}).Count(&stats.TotalUsers)

	// 今日新增用户数
	r.db.WithContext(ctx).Model(&userModel{}).
		Where("created_at >= ?", today).
		Count(&stats.TodayNewUsers)

	// 今日活跃用户数 (今日有请求的用户)
	r.db.WithContext(ctx).Model(&usageLogModel{}).
		Distinct("user_id").
		Where("created_at >= ?", today).
		Count(&stats.ActiveUsers)

	// 总 API Key 数
	r.db.WithContext(ctx).Model(&apiKeyModel{}).Count(&stats.TotalApiKeys)

	// 活跃 API Key 数
	r.db.WithContext(ctx).Model(&apiKeyModel{}).
		Where("status = ?", service.StatusActive).
		Count(&stats.ActiveApiKeys)

	// 总账户数
	r.db.WithContext(ctx).Model(&accountModel{}).Count(&stats.TotalAccounts)

	// 正常账户数 (schedulable=true, status=active)
	r.db.WithContext(ctx).Model(&accountModel{}).
		Where("status = ? AND schedulable = ?", service.StatusActive, true).
		Count(&stats.NormalAccounts)

	// 异常账户数 (status=error)
	r.db.WithContext(ctx).Model(&accountModel{}).
		Where("status = ?", service.StatusError).
		Count(&stats.ErrorAccounts)

	// 限流账户数
	r.db.WithContext(ctx).Model(&accountModel{}).
		Where("rate_limited_at IS NOT NULL AND rate_limit_reset_at > ?", time.Now()).
		Count(&stats.RateLimitAccounts)

	// 过载账户数
	r.db.WithContext(ctx).Model(&accountModel{}).
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
	}
	r.db.WithContext(ctx).Model(&usageLogModel{}).
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
	}
	r.db.WithContext(ctx).Model(&usageLogModel{}).
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

	// 性能指标：RPM 和 TPM（最近1分钟，全局）
	stats.Rpm, stats.Tpm = r.getPerformanceStats(ctx, 0)

	return &stats, nil
}

func (r *usageLogRepository) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	var total int64

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).Where("account_id = ?", accountID)

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
	}

	return usageLogModelsToService(logs), paginationResultFromTotal(total, params), nil
}

func (r *usageLogRepository) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return usageLogModelsToService(logs), nil, err
}

func (r *usageLogRepository) ListByApiKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	err := r.db.WithContext(ctx).
		Where("api_key_id = ? AND created_at >= ? AND created_at < ?", apiKeyID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return usageLogModelsToService(logs), nil, err
}

func (r *usageLogRepository) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return usageLogModelsToService(logs), nil, err
}

func (r *usageLogRepository) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	err := r.db.WithContext(ctx).
		Where("model = ? AND created_at >= ? AND created_at < ?", modelName, startTime, endTime).
		Order("id DESC").
		Find(&logs).Error
	return usageLogModelsToService(logs), nil, err
}

func (r *usageLogRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&usageLogModel{}, id).Error
}

// GetAccountTodayStats 获取账号今日统计
func (r *usageLogRepository) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	today := timezone.Today()

	var stats struct {
		Requests int64   `gorm:"column:requests"`
		Tokens   int64   `gorm:"column:tokens"`
		Cost     float64 `gorm:"column:cost"`
	}

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select(`
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(actual_cost), 0) as cost
		`).
		Where("account_id = ? AND created_at >= ?", accountID, today).
		Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	return &usagestats.AccountStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
	}, nil
}

// GetAccountWindowStats 获取账号时间窗口内的统计
func (r *usageLogRepository) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	var stats struct {
		Requests int64   `gorm:"column:requests"`
		Tokens   int64   `gorm:"column:tokens"`
		Cost     float64 `gorm:"column:cost"`
	}

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select(`
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(actual_cost), 0) as cost
		`).
		Where("account_id = ? AND created_at >= ?", accountID, startTime).
		Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	return &usagestats.AccountStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
	}, nil
}

// TrendDataPoint represents a single point in trend data
type TrendDataPoint = usagestats.TrendDataPoint

// ModelStat represents usage statistics for a single model
type ModelStat = usagestats.ModelStat

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint = usagestats.UserUsageTrendPoint

// ApiKeyUsageTrendPoint represents API key usage trend data point
type ApiKeyUsageTrendPoint = usagestats.ApiKeyUsageTrendPoint

// GetApiKeyUsageTrend returns usage trend data grouped by API key and date
func (r *usageLogRepository) GetApiKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]ApiKeyUsageTrendPoint, error) {
	var results []ApiKeyUsageTrendPoint

	// Choose date format based on granularity
	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
	} else {
		dateFormat = "YYYY-MM-DD"
	}

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
	}

	return results, nil
}

// GetUserUsageTrend returns usage trend data grouped by user and date
func (r *usageLogRepository) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]UserUsageTrendPoint, error) {
	var results []UserUsageTrendPoint

	// Choose date format based on granularity
	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
	} else {
		dateFormat = "YYYY-MM-DD"
	}

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
	}

	return results, nil
}

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats = usagestats.UserDashboardStats

// GetUserDashboardStats 获取用户专属的仪表盘统计
func (r *usageLogRepository) GetUserDashboardStats(ctx context.Context, userID int64) (*UserDashboardStats, error) {
	var stats UserDashboardStats
	today := timezone.Today()

	// API Key 统计
	r.db.WithContext(ctx).Model(&apiKeyModel{}).
		Where("user_id = ?", userID).
		Count(&stats.TotalApiKeys)

	r.db.WithContext(ctx).Model(&apiKeyModel{}).
		Where("user_id = ? AND status = ?", userID, service.StatusActive).
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
	}
	r.db.WithContext(ctx).Model(&usageLogModel{}).
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
	}
	r.db.WithContext(ctx).Model(&usageLogModel{}).
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

	// 性能指标：RPM 和 TPM（最近1分钟，仅统计该用户的请求）
	stats.Rpm, stats.Tpm = r.getPerformanceStats(ctx, userID)

	return &stats, nil
}

// GetUserUsageTrendByUserID 获取指定用户的使用趋势
func (r *usageLogRepository) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]TrendDataPoint, error) {
	var results []TrendDataPoint

	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
	} else {
		dateFormat = "YYYY-MM-DD"
	}

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
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
	}

	return results, nil
}

// GetUserModelStats 获取指定用户的模型统计
func (r *usageLogRepository) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]ModelStat, error) {
	var results []ModelStat

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
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
	}

	return results, nil
}

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters = usagestats.UsageLogFilters

// ListWithFilters lists usage logs with optional filters (for admin)
func (r *usageLogRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	var logs []usageLogModel
	var total int64

	db := r.db.WithContext(ctx).Model(&usageLogModel{})

	// Apply filters
	if filters.UserID > 0 {
		db = db.Where("user_id = ?", filters.UserID)
	}
	if filters.ApiKeyID > 0 {
		db = db.Where("api_key_id = ?", filters.ApiKeyID)
	}
	if filters.AccountID > 0 {
		db = db.Where("account_id = ?", filters.AccountID)
	}
	if filters.GroupID > 0 {
		db = db.Where("group_id = ?", filters.GroupID)
	}
	if filters.Model != "" {
		db = db.Where("model = ?", filters.Model)
	}
	if filters.Stream != nil {
		db = db.Where("stream = ?", *filters.Stream)
	}
	if filters.BillingType != nil {
		db = db.Where("billing_type = ?", *filters.BillingType)
	}
	if filters.StartTime != nil {
		db = db.Where("created_at >= ?", *filters.StartTime)
	}
	if filters.EndTime != nil {
		db = db.Where("created_at <= ?", *filters.EndTime)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Preload user, api_key, account, and group for display
	if err := db.Preload("User").Preload("ApiKey").Preload("Account").Preload("Group").
		Offset(params.Offset()).Limit(params.Limit()).
		Order("id DESC").Find(&logs).Error; err != nil {
		return nil, nil, err
	}

	return usageLogModelsToService(logs), paginationResultFromTotal(total, params), nil
}

// UsageStats represents usage statistics
type UsageStats = usagestats.UsageStats

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats = usagestats.BatchUserUsageStats

// GetBatchUserUsageStats gets today and total actual_cost for multiple users
func (r *usageLogRepository) GetBatchUserUsageStats(ctx context.Context, userIDs []int64) (map[int64]*BatchUserUsageStats, error) {
	if len(userIDs) == 0 {
		return make(map[int64]*BatchUserUsageStats), nil
	}

	today := timezone.Today()
	result := make(map[int64]*BatchUserUsageStats)

	// Initialize result map
	for _, id := range userIDs {
		result[id] = &BatchUserUsageStats{UserID: id}
	}

	// Get total actual_cost per user
	var totalStats []struct {
		UserID    int64   `gorm:"column:user_id"`
		TotalCost float64 `gorm:"column:total_cost"`
	}
	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select("user_id, COALESCE(SUM(actual_cost), 0) as total_cost").
		Where("user_id IN ?", userIDs).
		Group("user_id").
		Scan(&totalStats).Error
	if err != nil {
		return nil, err
	}

	for _, stat := range totalStats {
		if s, ok := result[stat.UserID]; ok {
			s.TotalActualCost = stat.TotalCost
		}
	}

	// Get today actual_cost per user
	var todayStats []struct {
		UserID    int64   `gorm:"column:user_id"`
		TodayCost float64 `gorm:"column:today_cost"`
	}
	err = r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select("user_id, COALESCE(SUM(actual_cost), 0) as today_cost").
		Where("user_id IN ? AND created_at >= ?", userIDs, today).
		Group("user_id").
		Scan(&todayStats).Error
	if err != nil {
		return nil, err
	}

	for _, stat := range todayStats {
		if s, ok := result[stat.UserID]; ok {
			s.TodayActualCost = stat.TodayCost
		}
	}

	return result, nil
}

// BatchApiKeyUsageStats represents usage stats for a single API key
type BatchApiKeyUsageStats = usagestats.BatchApiKeyUsageStats

// GetBatchApiKeyUsageStats gets today and total actual_cost for multiple API keys
func (r *usageLogRepository) GetBatchApiKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*BatchApiKeyUsageStats, error) {
	if len(apiKeyIDs) == 0 {
		return make(map[int64]*BatchApiKeyUsageStats), nil
	}

	today := timezone.Today()
	result := make(map[int64]*BatchApiKeyUsageStats)

	// Initialize result map
	for _, id := range apiKeyIDs {
		result[id] = &BatchApiKeyUsageStats{ApiKeyID: id}
	}

	// Get total actual_cost per api key
	var totalStats []struct {
		ApiKeyID  int64   `gorm:"column:api_key_id"`
		TotalCost float64 `gorm:"column:total_cost"`
	}
	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select("api_key_id, COALESCE(SUM(actual_cost), 0) as total_cost").
		Where("api_key_id IN ?", apiKeyIDs).
		Group("api_key_id").
		Scan(&totalStats).Error
	if err != nil {
		return nil, err
	}

	for _, stat := range totalStats {
		if s, ok := result[stat.ApiKeyID]; ok {
			s.TotalActualCost = stat.TotalCost
		}
	}

	// Get today actual_cost per api key
	var todayStats []struct {
		ApiKeyID  int64   `gorm:"column:api_key_id"`
		TodayCost float64 `gorm:"column:today_cost"`
	}
	err = r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select("api_key_id, COALESCE(SUM(actual_cost), 0) as today_cost").
		Where("api_key_id IN ? AND created_at >= ?", apiKeyIDs, today).
		Group("api_key_id").
		Scan(&todayStats).Error
	if err != nil {
		return nil, err
	}

	for _, stat := range todayStats {
		if s, ok := result[stat.ApiKeyID]; ok {
			s.TodayActualCost = stat.TodayCost
		}
	}

	return result, nil
}

// GetUsageTrendWithFilters returns usage trend data with optional user/api_key filters
func (r *usageLogRepository) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID int64) ([]TrendDataPoint, error) {
	var results []TrendDataPoint

	var dateFormat string
	if granularity == "hour" {
		dateFormat = "YYYY-MM-DD HH24:00"
	} else {
		dateFormat = "YYYY-MM-DD"
	}

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).
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
		Where("created_at >= ? AND created_at < ?", startTime, endTime)

	if userID > 0 {
		db = db.Where("user_id = ?", userID)
	}
	if apiKeyID > 0 {
		db = db.Where("api_key_id = ?", apiKeyID)
	}

	err := db.Group("date").Order("date ASC").Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetModelStatsWithFilters returns model statistics with optional user/api_key filters
func (r *usageLogRepository) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID int64) ([]ModelStat, error) {
	var results []ModelStat

	db := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select(`
			model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`).
		Where("created_at >= ? AND created_at < ?", startTime, endTime)

	if userID > 0 {
		db = db.Where("user_id = ?", userID)
	}
	if apiKeyID > 0 {
		db = db.Where("api_key_id = ?", apiKeyID)
	}
	if accountID > 0 {
		db = db.Where("account_id = ?", accountID)
	}

	err := db.Group("model").Order("total_tokens DESC").Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetGlobalStats gets usage statistics for all users within a time range
func (r *usageLogRepository) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*UsageStats, error) {
	var stats struct {
		TotalRequests     int64   `gorm:"column:total_requests"`
		TotalInputTokens  int64   `gorm:"column:total_input_tokens"`
		TotalOutputTokens int64   `gorm:"column:total_output_tokens"`
		TotalCacheTokens  int64   `gorm:"column:total_cache_tokens"`
		TotalCost         float64 `gorm:"column:total_cost"`
		TotalActualCost   float64 `gorm:"column:total_actual_cost"`
		AverageDurationMs float64 `gorm:"column:avg_duration_ms"`
	}

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
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
	}

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}, nil
}

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory = usagestats.AccountUsageHistory

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary = usagestats.AccountUsageSummary

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse = usagestats.AccountUsageStatsResponse

// GetAccountUsageStats returns comprehensive usage statistics for an account over a time range
func (r *usageLogRepository) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*AccountUsageStatsResponse, error) {
	daysCount := int(endTime.Sub(startTime).Hours()/24) + 1
	if daysCount <= 0 {
		daysCount = 30
	}

	// Get daily history
	var historyResults []struct {
		Date       string  `gorm:"column:date"`
		Requests   int64   `gorm:"column:requests"`
		Tokens     int64   `gorm:"column:tokens"`
		Cost       float64 `gorm:"column:cost"`
		ActualCost float64 `gorm:"column:actual_cost"`
	}

	err := r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select(`
			TO_CHAR(created_at, 'YYYY-MM-DD') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		`).
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, startTime, endTime).
		Group("date").
		Order("date ASC").
		Scan(&historyResults).Error
	if err != nil {
		return nil, err
	}

	// Build history with labels
	history := make([]AccountUsageHistory, 0, len(historyResults))
	for _, h := range historyResults {
		// Parse date to get label (MM/DD)
		t, _ := time.Parse("2006-01-02", h.Date)
		label := t.Format("01/02")
		history = append(history, AccountUsageHistory{
			Date:       h.Date,
			Label:      label,
			Requests:   h.Requests,
			Tokens:     h.Tokens,
			Cost:       h.Cost,
			ActualCost: h.ActualCost,
		})
	}

	// Calculate summary
	var totalActualCost, totalStandardCost float64
	var totalRequests, totalTokens int64
	var highestCostDay, highestRequestDay *AccountUsageHistory

	for i := range history {
		h := &history[i]
		totalActualCost += h.ActualCost
		totalStandardCost += h.Cost
		totalRequests += h.Requests
		totalTokens += h.Tokens

		if highestCostDay == nil || h.ActualCost > highestCostDay.ActualCost {
			highestCostDay = h
		}
		if highestRequestDay == nil || h.Requests > highestRequestDay.Requests {
			highestRequestDay = h
		}
	}

	actualDaysUsed := len(history)
	if actualDaysUsed == 0 {
		actualDaysUsed = 1
	}

	// Get average duration
	var avgDuration struct {
		AvgDurationMs float64 `gorm:"column:avg_duration_ms"`
	}
	r.db.WithContext(ctx).Model(&usageLogModel{}).
		Select("COALESCE(AVG(duration_ms), 0) as avg_duration_ms").
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, startTime, endTime).
		Scan(&avgDuration)

	summary := AccountUsageSummary{
		Days:              daysCount,
		ActualDaysUsed:    actualDaysUsed,
		TotalCost:         totalActualCost,
		TotalStandardCost: totalStandardCost,
		TotalRequests:     totalRequests,
		TotalTokens:       totalTokens,
		AvgDailyCost:      totalActualCost / float64(actualDaysUsed),
		AvgDailyRequests:  float64(totalRequests) / float64(actualDaysUsed),
		AvgDailyTokens:    float64(totalTokens) / float64(actualDaysUsed),
		AvgDurationMs:     avgDuration.AvgDurationMs,
	}

	// Set today's stats
	todayStr := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date == todayStr {
			summary.Today = &struct {
				Date     string  `json:"date"`
				Cost     float64 `json:"cost"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
			}{
				Date:     history[i].Date,
				Cost:     history[i].ActualCost,
				Requests: history[i].Requests,
				Tokens:   history[i].Tokens,
			}
			break
		}
	}

	// Set highest cost day
	if highestCostDay != nil {
		summary.HighestCostDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Cost     float64 `json:"cost"`
			Requests int64   `json:"requests"`
		}{
			Date:     highestCostDay.Date,
			Label:    highestCostDay.Label,
			Cost:     highestCostDay.ActualCost,
			Requests: highestCostDay.Requests,
		}
	}

	// Set highest request day
	if highestRequestDay != nil {
		summary.HighestRequestDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Requests int64   `json:"requests"`
			Cost     float64 `json:"cost"`
		}{
			Date:     highestRequestDay.Date,
			Label:    highestRequestDay.Label,
			Requests: highestRequestDay.Requests,
			Cost:     highestRequestDay.ActualCost,
		}
	}

	// Get model statistics using the unified method
	models, err := r.GetModelStatsWithFilters(ctx, startTime, endTime, 0, 0, accountID)
	if err != nil {
		models = []ModelStat{}
	}

	return &AccountUsageStatsResponse{
		History: history,
		Summary: summary,
		Models:  models,
	}, nil
}

type usageLogModel struct {
	ID        int64  `gorm:"primaryKey"`
	UserID    int64  `gorm:"index;not null"`
	ApiKeyID  int64  `gorm:"index;not null"`
	AccountID int64  `gorm:"index;not null"`
	RequestID string `gorm:"size:64"`
	Model     string `gorm:"size:100;index;not null"`

	GroupID        *int64 `gorm:"index"`
	SubscriptionID *int64 `gorm:"index"`

	InputTokens         int `gorm:"default:0;not null"`
	OutputTokens        int `gorm:"default:0;not null"`
	CacheCreationTokens int `gorm:"default:0;not null"`
	CacheReadTokens     int `gorm:"default:0;not null"`

	CacheCreation5mTokens int `gorm:"default:0;not null"`
	CacheCreation1hTokens int `gorm:"default:0;not null"`

	InputCost         float64 `gorm:"type:decimal(20,10);default:0;not null"`
	OutputCost        float64 `gorm:"type:decimal(20,10);default:0;not null"`
	CacheCreationCost float64 `gorm:"type:decimal(20,10);default:0;not null"`
	CacheReadCost     float64 `gorm:"type:decimal(20,10);default:0;not null"`
	TotalCost         float64 `gorm:"type:decimal(20,10);default:0;not null"`
	ActualCost        float64 `gorm:"type:decimal(20,10);default:0;not null"`
	RateMultiplier    float64 `gorm:"type:decimal(10,4);default:1;not null"`

	BillingType  int8 `gorm:"type:smallint;default:0;not null"`
	Stream       bool `gorm:"default:false;not null"`
	DurationMs   *int
	FirstTokenMs *int

	CreatedAt time.Time `gorm:"index;not null"`

	User         *userModel             `gorm:"foreignKey:UserID"`
	ApiKey       *apiKeyModel           `gorm:"foreignKey:ApiKeyID"`
	Account      *accountModel          `gorm:"foreignKey:AccountID"`
	Group        *groupModel            `gorm:"foreignKey:GroupID"`
	Subscription *userSubscriptionModel `gorm:"foreignKey:SubscriptionID"`
}

func (usageLogModel) TableName() string { return "usage_logs" }

func usageLogModelToService(m *usageLogModel) *service.UsageLog {
	if m == nil {
		return nil
	}
	return &service.UsageLog{
		ID:                    m.ID,
		UserID:                m.UserID,
		ApiKeyID:              m.ApiKeyID,
		AccountID:             m.AccountID,
		RequestID:             m.RequestID,
		Model:                 m.Model,
		GroupID:               m.GroupID,
		SubscriptionID:        m.SubscriptionID,
		InputTokens:           m.InputTokens,
		OutputTokens:          m.OutputTokens,
		CacheCreationTokens:   m.CacheCreationTokens,
		CacheReadTokens:       m.CacheReadTokens,
		CacheCreation5mTokens: m.CacheCreation5mTokens,
		CacheCreation1hTokens: m.CacheCreation1hTokens,
		InputCost:             m.InputCost,
		OutputCost:            m.OutputCost,
		CacheCreationCost:     m.CacheCreationCost,
		CacheReadCost:         m.CacheReadCost,
		TotalCost:             m.TotalCost,
		ActualCost:            m.ActualCost,
		RateMultiplier:        m.RateMultiplier,
		BillingType:           m.BillingType,
		Stream:                m.Stream,
		DurationMs:            m.DurationMs,
		FirstTokenMs:          m.FirstTokenMs,
		CreatedAt:             m.CreatedAt,
		User:                  userModelToService(m.User),
		ApiKey:                apiKeyModelToService(m.ApiKey),
		Account:               accountModelToService(m.Account),
		Group:                 groupModelToService(m.Group),
		Subscription:          userSubscriptionModelToService(m.Subscription),
	}
}

func usageLogModelsToService(models []usageLogModel) []service.UsageLog {
	out := make([]service.UsageLog, 0, len(models))
	for i := range models {
		if s := usageLogModelToService(&models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func usageLogModelFromService(log *service.UsageLog) *usageLogModel {
	if log == nil {
		return nil
	}
	return &usageLogModel{
		ID:                    log.ID,
		UserID:                log.UserID,
		ApiKeyID:              log.ApiKeyID,
		AccountID:             log.AccountID,
		RequestID:             log.RequestID,
		Model:                 log.Model,
		GroupID:               log.GroupID,
		SubscriptionID:        log.SubscriptionID,
		InputTokens:           log.InputTokens,
		OutputTokens:          log.OutputTokens,
		CacheCreationTokens:   log.CacheCreationTokens,
		CacheReadTokens:       log.CacheReadTokens,
		CacheCreation5mTokens: log.CacheCreation5mTokens,
		CacheCreation1hTokens: log.CacheCreation1hTokens,
		InputCost:             log.InputCost,
		OutputCost:            log.OutputCost,
		CacheCreationCost:     log.CacheCreationCost,
		CacheReadCost:         log.CacheReadCost,
		TotalCost:             log.TotalCost,
		ActualCost:            log.ActualCost,
		RateMultiplier:        log.RateMultiplier,
		BillingType:           log.BillingType,
		Stream:                log.Stream,
		DurationMs:            log.DurationMs,
		FirstTokenMs:          log.FirstTokenMs,
		CreatedAt:             log.CreatedAt,
	}
}

func applyUsageLogModelToService(log *service.UsageLog, m *usageLogModel) {
	if log == nil || m == nil {
		return
	}
	log.ID = m.ID
	log.CreatedAt = m.CreatedAt
}
