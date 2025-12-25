package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// DashboardService provides aggregated statistics for admin dashboard.
type DashboardService struct {
	usageRepo ports.UsageLogRepository
}

func NewDashboardService(usageRepo ports.UsageLogRepository) *DashboardService {
	return &DashboardService{
		usageRepo: usageRepo,
	}
}

func (s *DashboardService) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	stats, err := s.usageRepo.GetDashboardStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get dashboard stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID int64) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("get usage trend with filters: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID int64) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, 0)
	if err != nil {
		return nil, fmt.Errorf("get model stats with filters: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetApiKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.ApiKeyUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetApiKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get api key usage trend: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get user usage trend: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetBatchUserUsageStats(ctx context.Context, userIDs []int64) (map[int64]*usagestats.BatchUserUsageStats, error) {
	stats, err := s.usageRepo.GetBatchUserUsageStats(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch user usage stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetBatchApiKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*usagestats.BatchApiKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchApiKeyUsageStats(ctx, apiKeyIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
	}
	return stats, nil
}
