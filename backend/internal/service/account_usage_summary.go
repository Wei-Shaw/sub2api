package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

func (s *AccountUsageService) GetAccountsUsageTrend(ctx context.Context, accountIDs []int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageLogRepo.GetUsageTrendForAccounts(ctx, accountIDs, startTime, endTime, granularity)
	if err != nil {
		return nil, fmt.Errorf("get accounts usage trend failed: %w", err)
	}
	return trend, nil
}

func (s *AccountUsageService) GetAccountsModelStats(ctx context.Context, accountIDs []int64, startTime, endTime time.Time, source string) ([]usagestats.ModelStat, error) {
	stats, err := s.usageLogRepo.GetModelStatsForAccounts(ctx, accountIDs, startTime, endTime, source)
	if err != nil {
		return nil, fmt.Errorf("get accounts model stats failed: %w", err)
	}
	return stats, nil
}
