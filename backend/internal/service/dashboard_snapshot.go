package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"golang.org/x/sync/errgroup"
)

type DashboardSnapshotFilters struct {
	UserID      int64
	APIKeyID    int64
	AccountID   int64
	GroupID     int64
	Model       string
	RequestType *int16
	Stream      *bool
	BillingType *int8
}

type DashboardSnapshotQuery struct {
	StartTime   time.Time
	EndTime     time.Time
	Granularity string
	Filters     DashboardSnapshotFilters

	IncludeStats      bool
	IncludeTrend      bool
	IncludeModels     bool
	IncludeGroups     bool
	IncludeUsersTrend bool
	IncludeRanking    bool
	UsersTrendLimit   int
	RankingLimit      int
}

type DashboardSnapshotResult struct {
	Stats        *usagestats.DashboardStats
	Trend        []usagestats.TrendDataPoint
	Models       []usagestats.ModelStat
	Groups       []usagestats.GroupStat
	UserInsights *usagestats.DashboardUserInsights
}

type dashboardUserInsightsRepository interface {
	GetDashboardUserInsights(context.Context, time.Time, time.Time, string, int, int) (*usagestats.DashboardUserInsights, error)
}

func (s *DashboardService) GetDashboardUserInsights(ctx context.Context, startTime, endTime time.Time, granularity string, trendLimit, rankingLimit int) (*usagestats.DashboardUserInsights, error) {
	if repo, ok := s.usageRepo.(dashboardUserInsightsRepository); ok {
		insights, err := repo.GetDashboardUserInsights(ctx, startTime, endTime, granularity, trendLimit, rankingLimit)
		if err != nil {
			return nil, fmt.Errorf("get dashboard user insights: %w", err)
		}
		return insights, nil
	}

	trend, err := s.GetUserUsageTrend(ctx, startTime, endTime, granularity, trendLimit)
	if err != nil {
		return nil, err
	}
	ranking, err := s.GetUserSpendingRanking(ctx, startTime, endTime, rankingLimit)
	if err != nil {
		return nil, err
	}
	return &usagestats.DashboardUserInsights{Trend: trend, Ranking: *ranking}, nil
}

// GetDashboardSnapshot builds the admin dashboard read model with independent queries in parallel.
func (s *DashboardService) GetDashboardSnapshot(ctx context.Context, query DashboardSnapshotQuery) (*DashboardSnapshotResult, error) {
	result := &DashboardSnapshotResult{}
	g, queryCtx := errgroup.WithContext(ctx)
	filters := query.Filters

	if query.IncludeStats {
		g.Go(func() error {
			stats, err := s.GetDashboardStats(queryCtx)
			if err != nil {
				return fmt.Errorf("get dashboard statistics: %w", err)
			}
			result.Stats = stats
			return nil
		})
	}
	if query.IncludeTrend {
		g.Go(func() error {
			trend, err := s.GetUsageTrendWithFilters(queryCtx, query.StartTime, query.EndTime, query.Granularity, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType)
			if err != nil {
				return err
			}
			result.Trend = trend
			return nil
		})
	}
	if query.IncludeModels {
		g.Go(func() error {
			models, err := s.GetModelStatsWithFiltersBySource(queryCtx, query.StartTime, query.EndTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, usagestats.ModelSourceRequested)
			if err != nil {
				return err
			}
			result.Models = models
			return nil
		})
	}
	if query.IncludeGroups {
		g.Go(func() error {
			groups, err := s.GetGroupStatsWithFilters(queryCtx, query.StartTime, query.EndTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType)
			if err != nil {
				return err
			}
			result.Groups = groups
			return nil
		})
	}
	if query.IncludeUsersTrend || query.IncludeRanking {
		g.Go(func() error {
			insights, err := s.GetDashboardUserInsights(queryCtx, query.StartTime, query.EndTime, query.Granularity, query.UsersTrendLimit, query.RankingLimit)
			if err != nil {
				return err
			}
			result.UserInsights = insights
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}
