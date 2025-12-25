package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type UsageLogRepository interface {
	Create(ctx context.Context, log *model.UsageLog) error
	GetByID(ctx context.Context, id int64) (*model.UsageLog, error)
	Delete(ctx context.Context, id int64) error

	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error)
	ListByApiKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error)
	ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]model.UsageLog, *pagination.PaginationResult, error)

	ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error)
	ListByApiKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error)
	ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error)
	ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]model.UsageLog, *pagination.PaginationResult, error)

	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error)
	GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error)

	// Admin dashboard stats
	GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error)
	GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID int64) ([]usagestats.TrendDataPoint, error)
	GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID int64) ([]usagestats.ModelStat, error)
	GetApiKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.ApiKeyUsageTrendPoint, error)
	GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error)
	GetBatchUserUsageStats(ctx context.Context, userIDs []int64) (map[int64]*usagestats.BatchUserUsageStats, error)
	GetBatchApiKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*usagestats.BatchApiKeyUsageStats, error)

	// User dashboard stats
	GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error)
	GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error)
	GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error)

	// Admin usage listing/stats
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]model.UsageLog, *pagination.PaginationResult, error)
	GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error)

	// Account stats
	GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error)
}
