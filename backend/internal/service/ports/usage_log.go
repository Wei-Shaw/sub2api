package ports

import (
	"context"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/pkg/usagestats"
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
REDACTED
