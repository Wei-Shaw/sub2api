package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

var (
	ErrUsageLogNotFound = infraerrors.NotFound("USAGE_LOG_NOT_FOUND", "usage log not found")
)

// CreateUsageLogRequest 创建使用日志请求
type CreateUsageLogRequest struct {
	UserID                int64   `json:"user_id"`
	APIKeyID              int64   `json:"api_key_id"`
	AccountID             int64   `json:"account_id"`
	RequestID             string  `json:"request_id"`
	Model                 string  `json:"model"`
	InputTokens           int     `json:"input_tokens"`
	OutputTokens          int     `json:"output_tokens"`
	CacheCreationTokens   int     `json:"cache_creation_tokens"`
	CacheReadTokens       int     `json:"cache_read_tokens"`
	CacheCreation5mTokens int     `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int     `json:"cache_creation_1h_tokens"`
	InputCost             float64 `json:"input_cost"`
	OutputCost            float64 `json:"output_cost"`
	CacheCreationCost     float64 `json:"cache_creation_cost"`
	CacheReadCost         float64 `json:"cache_read_cost"`
	TotalCost             float64 `json:"total_cost"`
	ActualCost            float64 `json:"actual_cost"`
	RateMultiplier        float64 `json:"rate_multiplier"`
	Stream                bool    `json:"stream"`
	DurationMs            *int    `json:"duration_ms"`
}

// UsageStats 使用统计
type UsageStats struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCacheTokens  int64   `json:"total_cache_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	AverageDurationMs float64 `json:"average_duration_ms"`
}

type UserUsageSummary struct {
	Period            string  `json:"period"`
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCacheTokens  int64   `json:"total_cache_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	TodayRequests     int64   `json:"today_requests"`
	TodayTokens       int64   `json:"today_tokens"`
	TodayActualCost   float64 `json:"today_actual_cost"`
	UsageOverridden   bool    `json:"usage_overridden,omitempty"`
	AverageDurationMs float64 `json:"avg_duration_ms"`
}

type UserUsageOverride = usagestats.UserUsageOverride
type UpdateUserUsageOverrideInput = usagestats.UpdateUserUsageOverrideInput

// UsageService 使用统计服务
type UsageService struct {
	usageRepo            UsageLogRepository
	userRepo             UserRepository
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

// NewUsageService 创建使用统计服务实例
func NewUsageService(usageRepo UsageLogRepository, userRepo UserRepository, entClient *dbent.Client, authCacheInvalidator APIKeyAuthCacheInvalidator) *UsageService {
	return &UsageService{
		usageRepo:            usageRepo,
		userRepo:             userRepo,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// Create 创建使用日志
func (s *UsageService) Create(ctx context.Context, req CreateUsageLogRequest) (*UsageLog, error) {
	// 使用数据库事务保证「使用日志插入」与「扣费」的原子性，避免重复扣费或漏扣风险。
	tx, err := s.entClient.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := ctx
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txCtx = dbent.NewTxContext(ctx, tx)
	}

	// 验证用户存在
	_, err = s.userRepo.GetByID(txCtx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 创建使用日志
	usageLog := &UsageLog{
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		AccountID:             req.AccountID,
		RequestID:             req.RequestID,
		Model:                 req.Model,
		InputTokens:           req.InputTokens,
		OutputTokens:          req.OutputTokens,
		CacheCreationTokens:   req.CacheCreationTokens,
		CacheReadTokens:       req.CacheReadTokens,
		CacheCreation5mTokens: req.CacheCreation5mTokens,
		CacheCreation1hTokens: req.CacheCreation1hTokens,
		InputCost:             req.InputCost,
		OutputCost:            req.OutputCost,
		CacheCreationCost:     req.CacheCreationCost,
		CacheReadCost:         req.CacheReadCost,
		TotalCost:             req.TotalCost,
		ActualCost:            req.ActualCost,
		RateMultiplier:        req.RateMultiplier,
		Stream:                req.Stream,
		DurationMs:            req.DurationMs,
	}

	inserted, err := s.usageRepo.Create(txCtx, usageLog)
	if err != nil {
		return nil, fmt.Errorf("create usage log: %w", err)
	}

	// 扣除用户余额
	balanceUpdated := false
	if inserted && req.ActualCost > 0 {
		if err := s.userRepo.UpdateBalance(txCtx, req.UserID, -req.ActualCost); err != nil {
			return nil, fmt.Errorf("update user balance: %w", err)
		}
		balanceUpdated = true
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit transaction: %w", err)
		}
	}

	s.invalidateUsageCaches(ctx, req.UserID, balanceUpdated)

	return usageLog, nil
}

func (s *UsageService) invalidateUsageCaches(ctx context.Context, userID int64, balanceUpdated bool) {
	if !balanceUpdated || s.authCacheInvalidator == nil {
		return
	}
	s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
}

// GetByID 根据ID获取使用日志
func (s *UsageService) GetByID(ctx context.Context, id int64) (*UsageLog, error) {
	log, err := s.usageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get usage log: %w", err)
	}
	return log, nil
}

// ListByUser 获取用户的使用日志列表
func (s *UsageService) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByUser(ctx, userID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
	}
	return logs, pagination, nil
}

// ListByAPIKey 获取API Key的使用日志列表
func (s *UsageService) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByAPIKey(ctx, apiKeyID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
	}
	return logs, pagination, nil
}

// ListByAccount 获取账号的使用日志列表
func (s *UsageService) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByAccount(ctx, accountID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
	}
	return logs, pagination, nil
}

// GetStatsByUser 获取用户的使用统计
func (s *UsageService) GetStatsByUser(ctx context.Context, userID int64, startTime, endTime time.Time) (*UsageStats, error) {
	if endTime.IsZero() {
		endTime = timezone.Now()
	}
	rangeStats, err := s.usageRepo.GetUserStatsAggregated(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}

	out := &UsageStats{
		TotalRequests:     rangeStats.TotalRequests,
		TotalInputTokens:  rangeStats.TotalInputTokens,
		TotalOutputTokens: rangeStats.TotalOutputTokens,
		TotalCacheTokens:  rangeStats.TotalCacheTokens,
		TotalTokens:       rangeStats.TotalTokens,
		TotalCost:         rangeStats.TotalCost,
		TotalActualCost:   rangeStats.TotalActualCost,
		AverageDurationMs: rangeStats.AverageDurationMs,
	}
	override := s.loadUserUsageOverrideBestEffort(ctx, userID)
	if hasUserUsageTodayOverride(override) {
		todayStart, todayEnd, ok := usageTodayWindow(startTime, endTime)
		if ok {
			todayStats, todayErr := s.usageRepo.GetUserStatsAggregated(ctx, userID, todayStart, todayEnd)
			if todayErr != nil {
				return nil, fmt.Errorf("get user today stats: %w", todayErr)
			}
			applyUsageStatsRangeTodayOverride(out, usageStatsFromAggregate(todayStats), override)
		}
	}
	if usageRangeIncludesAllTime(startTime, endTime) {
		applyUsageStatsExplicitTotalOverride(out, override)
	}
	return out, nil
}

func (s *UsageService) GetUserUsageSummary(ctx context.Context, userID int64, period string, startTime, endTime time.Time) (*UserUsageSummary, error) {
	stats, err := s.GetStatsByUser(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	summary := &UserUsageSummary{
		Period:            period,
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}

	if batchStats, err := s.usageRepo.GetBatchUserUsageStats(ctx, []int64{userID}, time.Time{}, time.Time{}); err == nil {
		if userStats := batchStats[userID]; userStats != nil {
			summary.TodayRequests = userStats.TodayRequests
			summary.TodayTokens = userStats.TodayTokens
			summary.TodayActualCost = userStats.TodayActualCost
			if usageRangeIncludesAllTime(startTime, endTime) {
				summary.TotalTokens = userStats.TotalTokens
				summary.TotalActualCost = userStats.TotalActualCost
				summary.TotalCost = userStats.TotalActualCost
			}
			summary.UsageOverridden = userStats.UsageOverridden
		}
	}

	return summary, nil
}

// GetStatsByAPIKey 获取API Key的使用统计
func (s *UsageService) GetStatsByAPIKey(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*UsageStats, error) {
	stats, err := s.usageRepo.GetAPIKeyStatsAggregated(ctx, apiKeyID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get api key stats: %w", err)
	}

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}, nil
}

func (s *UsageService) GetStatsByAPIKeyForUser(ctx context.Context, userID int64, apiKeyID int64, startTime, endTime time.Time, applyUserOverride bool) (*UsageStats, error) {
	stats, err := s.GetStatsByAPIKey(ctx, apiKeyID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	if !applyUserOverride {
		return stats, nil
	}
	override := s.loadUserUsageOverrideBestEffort(ctx, userID)
	if hasUserUsageTodayOverride(override) {
		todayStart, todayEnd, ok := usageTodayWindow(startTime, endTime)
		if ok {
			todayStats, todayErr := s.usageRepo.GetAPIKeyStatsAggregated(ctx, apiKeyID, todayStart, todayEnd)
			if todayErr != nil {
				return nil, fmt.Errorf("get api key today stats: %w", todayErr)
			}
			applyUsageStatsRangeTodayOverride(stats, usageStatsFromAggregate(todayStats), override)
		}
	}
	if usageRangeIncludesAllTime(startTime, endTime) {
		applyUsageStatsExplicitTotalOverride(stats, override)
	}
	return stats, nil
}

// GetStatsByAccount 获取账号的使用统计
func (s *UsageService) GetStatsByAccount(ctx context.Context, accountID int64, startTime, endTime time.Time) (*UsageStats, error) {
	stats, err := s.usageRepo.GetAccountStatsAggregated(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get account stats: %w", err)
	}

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}, nil
}

// GetStatsByModel 获取模型的使用统计
func (s *UsageService) GetStatsByModel(ctx context.Context, modelName string, startTime, endTime time.Time) (*UsageStats, error) {
	stats, err := s.usageRepo.GetModelStatsAggregated(ctx, modelName, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get model stats: %w", err)
	}

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}, nil
}

// GetDailyStats 获取每日使用统计（最近N天）
func (s *UsageService) GetDailyStats(ctx context.Context, userID int64, days int) ([]map[string]any, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)

	stats, err := s.usageRepo.GetDailyStatsAggregated(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get daily stats: %w", err)
	}

	return stats, nil
}

// Delete 删除使用日志（管理员功能，谨慎使用）
func (s *UsageService) Delete(ctx context.Context, id int64) error {
	if err := s.usageRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete usage log: %w", err)
	}
	return nil
}

// GetUserDashboardStats returns per-user dashboard summary stats.
func (s *UsageService) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	stats, err := s.usageRepo.GetUserDashboardStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user dashboard stats: %w", err)
	}
	return stats, nil
}

func (s *UsageService) GetUserUsageOverride(ctx context.Context, userID int64) (*UserUsageOverride, error) {
	override, err := s.usageRepo.GetUserUsageOverride(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user usage override: %w", err)
	}
	return override, nil
}

func (s *UsageService) UpsertUserUsageOverride(ctx context.Context, userID int64, input UpdateUserUsageOverrideInput) (*UserUsageOverride, error) {
	override, err := s.usageRepo.UpsertUserUsageOverride(ctx, userID, input)
	if err != nil {
		return nil, fmt.Errorf("upsert user usage override: %w", err)
	}
	return override, nil
}

func (s *UsageService) DeleteUserUsageOverride(ctx context.Context, userID int64) error {
	if err := s.usageRepo.DeleteUserUsageOverride(ctx, userID); err != nil {
		return fmt.Errorf("delete user usage override: %w", err)
	}
	return nil
}

func (s *UsageService) loadUserUsageOverrideBestEffort(ctx context.Context, userID int64) *usagestats.UserUsageOverride {
	if s == nil || s.usageRepo == nil || userID <= 0 {
		return nil
	}
	override, err := s.usageRepo.GetUserUsageOverride(ctx, userID)
	if err != nil {
		return nil
	}
	return override
}

func usageStatsFromAggregate(stats *usagestats.UsageStats) *UsageStats {
	if stats == nil {
		return nil
	}
	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	}
}

func hasUserUsageTodayOverride(override *usagestats.UserUsageOverride) bool {
	return override != nil && (override.TodayRequests != nil || override.TodayTokens != nil || override.TodayActualCost != nil)
}

func applyUsageStatsTodayOverride(stats *UsageStats, override *usagestats.UserUsageOverride) {
	if stats == nil || override == nil {
		return
	}
	if override.TodayRequests != nil {
		stats.TotalRequests = *override.TodayRequests
	}
	if override.TodayTokens != nil {
		stats.TotalTokens = *override.TodayTokens
	}
	if override.TodayActualCost != nil {
		stats.TotalActualCost = *override.TodayActualCost
		stats.TotalCost = *override.TodayActualCost
	}
}

func applyUsageStatsRangeTodayOverride(stats *UsageStats, todayStats *UsageStats, override *usagestats.UserUsageOverride) {
	if stats == nil || override == nil {
		return
	}
	if todayStats == nil {
		return
	}
	if override.TodayRequests != nil {
		stats.TotalRequests += *override.TodayRequests - todayStats.TotalRequests
	}
	if override.TodayTokens != nil {
		stats.TotalTokens += *override.TodayTokens - todayStats.TotalTokens
	}
	if override.TodayActualCost != nil {
		delta := *override.TodayActualCost - todayStats.TotalActualCost
		stats.TotalActualCost += delta
		stats.TotalCost += delta
	}
}

func applyUsageStatsTotalOverride(stats *UsageStats, override *usagestats.UserUsageOverride) {
	applyUsageStatsExplicitTotalOverride(stats, override)
}

func applyUsageStatsExplicitTotalOverride(stats *UsageStats, override *usagestats.UserUsageOverride) {
	if stats == nil || override == nil {
		return
	}
	if override.TotalTokens != nil {
		stats.TotalTokens = *override.TotalTokens
	}
	if override.TotalActualCost != nil {
		stats.TotalActualCost = *override.TotalActualCost
		stats.TotalCost = *override.TotalActualCost
	}
}

func applyUserDashboardStatsOverride(stats *usagestats.UserDashboardStats, override *usagestats.UserUsageOverride) {
	if stats == nil || override == nil {
		return
	}
	if override.TodayRequests != nil {
		delta := *override.TodayRequests - stats.TodayRequests
		stats.TodayRequests = *override.TodayRequests
		stats.TotalRequests += delta
		stats.UsageOverridden = true
	}
	if override.TodayTokens != nil {
		delta := *override.TodayTokens - stats.TodayTokens
		stats.TodayTokens = *override.TodayTokens
		if override.TotalTokens == nil {
			stats.TotalTokens += delta
		}
		stats.UsageOverridden = true
	}
	if override.TodayActualCost != nil {
		delta := *override.TodayActualCost - stats.TodayActualCost
		stats.TodayActualCost = *override.TodayActualCost
		stats.TodayCost = *override.TodayActualCost
		if override.TotalActualCost == nil {
			stats.TotalActualCost += delta
			stats.TotalCost += delta
		}
		stats.UsageOverridden = true
	}
	if override.TotalTokens != nil {
		stats.TotalTokens = *override.TotalTokens
		stats.UsageOverridden = true
	}
	if override.TotalActualCost != nil {
		stats.TotalActualCost = *override.TotalActualCost
		stats.TotalCost = *override.TotalActualCost
		stats.UsageOverridden = true
	}
}

func applyAPIKeyUsageStatsUserOverride(stats *usagestats.BatchAPIKeyUsageStats, override *usagestats.UserUsageOverride) {
	if stats == nil || override == nil {
		return
	}
	if override.TodayActualCost != nil {
		delta := *override.TodayActualCost - stats.TodayActualCost
		stats.TodayActualCost = *override.TodayActualCost
		if override.TotalActualCost == nil {
			stats.TotalActualCost += delta
		}
		stats.UsageOverridden = true
	}
	if override.TotalActualCost != nil {
		stats.TotalActualCost = *override.TotalActualCost
		stats.UsageOverridden = true
	}
}

func usageRangeIncludesAllTime(startTime, endTime time.Time) bool {
	return startTime.IsZero()
}

func usageRangeLocation(startTime, endTime time.Time) *time.Location {
	if !startTime.IsZero() && startTime.Location() != nil {
		return startTime.Location()
	}
	if !endTime.IsZero() && endTime.Location() != nil {
		return endTime.Location()
	}
	return timezone.Now().Location()
}

func usageTodayWindow(startTime, endTime time.Time) (time.Time, time.Time, bool) {
	loc := usageRangeLocation(startTime, endTime)
	now := timezone.Now().In(loc)
	rangeEnd := endTime
	if rangeEnd.IsZero() {
		rangeEnd = now
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayEnd := todayStart.AddDate(0, 0, 1)
	effectiveStart := todayStart
	if !startTime.IsZero() && startTime.After(effectiveStart) {
		effectiveStart = startTime
	}
	effectiveEnd := rangeEnd
	if effectiveEnd.After(todayEnd) {
		effectiveEnd = todayEnd
	}
	if !effectiveEnd.After(todayStart) || !effectiveEnd.After(effectiveStart) {
		return time.Time{}, time.Time{}, false
	}
	return effectiveStart, effectiveEnd, true
}

func usageRangeCoversToday(startTime, endTime time.Time) bool {
	if startTime.IsZero() {
		return false
	}
	loc := usageRangeLocation(startTime, endTime)
	now := timezone.Now().In(loc)
	expectedTodayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if !startTime.Equal(expectedTodayStart) {
		return false
	}
	if endTime.IsZero() {
		return true
	}
	return endTime.After(now.Add(-time.Second))
}

// GetAPIKeyDashboardStats returns dashboard summary stats filtered by API Key.
func (s *UsageService) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	stats, err := s.usageRepo.GetAPIKeyDashboardStats(ctx, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("get api key dashboard stats: %w", err)
	}
	return stats, nil
}

func (s *UsageService) GetAPIKeyDashboardStatsForUser(ctx context.Context, userID int64, apiKeyID int64, applyUserOverride bool) (*usagestats.UserDashboardStats, error) {
	stats, err := s.GetAPIKeyDashboardStats(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}
	if applyUserOverride {
		applyUserDashboardStatsOverride(stats, s.loadUserUsageOverrideBestEffort(ctx, userID))
	}
	return stats, nil
}

// GetUserUsageTrendByUserID returns per-user usage trend.
func (s *UsageService) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrendByUserID(ctx, userID, startTime, endTime, granularity)
	if err != nil {
		return nil, fmt.Errorf("get user usage trend: %w", err)
	}
	return trend, nil
}

// GetUserModelStats returns per-user model usage stats.
func (s *UsageService) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetUserModelStats(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get user model stats: %w", err)
	}
	return stats, nil
}

// GetAPIKeyModelStats returns per-model usage stats for a specific API Key.
func (s *UsageService) GetAPIKeyModelStats(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, startTime, endTime, 0, apiKeyID, 0, 0, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get api key model stats: %w", err)
	}
	return stats, nil
}

// GetAPIKeyDailyUsage returns daily usage stats for a user's API key.
func (s *UsageService) GetAPIKeyDailyUsage(ctx context.Context, userID, apiKeyID int64, startTime, endTime time.Time) ([]usagestats.APIKeyDailyUsagePoint, error) {
	trend, err := s.usageRepo.GetUsageTrendWithFilters(ctx, startTime, endTime, "day", userID, apiKeyID, 0, 0, "", nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get api key daily usage: %w", err)
	}

	points := make([]usagestats.APIKeyDailyUsagePoint, 0, len(trend))
	for _, row := range trend {
		points = append(points, usagestats.APIKeyDailyUsagePoint{
			Date:             row.Date,
			Requests:         row.Requests,
			InputTokens:      row.InputTokens,
			OutputTokens:     row.OutputTokens,
			CacheReadTokens:  row.CacheReadTokens,
			CacheWriteTokens: row.CacheCreationTokens,
			TotalTokens:      row.TotalTokens,
			Cost:             row.Cost,
			ActualCost:       row.ActualCost,
		})
	}
	return points, nil
}

// GetBatchAPIKeyUsageStats returns today/total actual_cost for given api keys.
func (s *UsageService) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
	}
	return stats, nil
}

func (s *UsageService) ApplyUserUsageOverrideToSingleAPIKeyStats(ctx context.Context, userID int64, stats *usagestats.BatchAPIKeyUsageStats) {
	applyAPIKeyUsageStatsUserOverride(stats, s.loadUserUsageOverrideBestEffort(ctx, userID))
}

// ListWithFilters lists usage logs with admin filters.
func (s *UsageService) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, result, err := s.usageRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs with filters: %w", err)
	}
	return logs, result, nil
}

// GetGlobalStats returns global usage stats for a time range.
func (s *UsageService) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	stats, err := s.usageRepo.GetGlobalStats(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get global usage stats: %w", err)
	}
	return stats, nil
}

// GetStatsWithFilters returns usage stats with optional filters.
func (s *UsageService) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	stats, err := s.usageRepo.GetStatsWithFilters(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("get usage stats with filters: %w", err)
	}
	return stats, nil
}
