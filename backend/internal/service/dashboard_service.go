package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	defaultDashboardStatsFreshTTL       = 15 * time.Second
	defaultDashboardStatsCacheTTL       = 30 * time.Second
	defaultDashboardStatsRefreshTimeout = 30 * time.Second
)

// ErrDashboardStatsCacheMiss 标记仪表盘缓存未命中。
var ErrDashboardStatsCacheMiss = errors.New("仪表盘缓存未命中")

// DashboardStatsCache 定义仪表盘统计缓存接口。
type DashboardStatsCache interface {
	GetDashboardStats(ctx context.Context) (string, error)
	SetDashboardStats(ctx context.Context, data string, ttl time.Duration) error
	DeleteDashboardStats(ctx context.Context) error
}

type dashboardStatsRangeFetcher interface {
	GetDashboardStatsWithRange(ctx context.Context, start, end time.Time) (*usagestats.DashboardStats, error)
}

type dashboardStatsCacheEntry struct {
	Stats     *usagestats.DashboardStats `json:"stats"`
	UpdatedAt int64                      `json:"updated_at"`
}

// DashboardService 提供管理员仪表盘统计服务。
type DashboardService struct {
	usageRepo      UsageLogRepository
	aggRepo        DashboardAggregationRepository
	cache          DashboardStatsCache
	cacheFreshTTL  time.Duration
	cacheTTL       time.Duration
	refreshTimeout time.Duration
	refreshing     int32
	aggEnabled     bool
	aggInterval    time.Duration
	aggLookback    time.Duration
	aggUsageDays   int
}

func NewDashboardService(usageRepo UsageLogRepository, aggRepo DashboardAggregationRepository, cache DashboardStatsCache, cfg *config.Config) *DashboardService {
	freshTTL := defaultDashboardStatsFreshTTL
	cacheTTL := defaultDashboardStatsCacheTTL
	refreshTimeout := defaultDashboardStatsRefreshTimeout
	aggEnabled := true
	aggInterval := time.Minute
	aggLookback := 2 * time.Minute
	aggUsageDays := 90
	if cfg != nil {
		if !cfg.Dashboard.Enabled {
			cache = nil
		}
		if cfg.Dashboard.StatsFreshTTLSeconds > 0 {
			freshTTL = time.Duration(cfg.Dashboard.StatsFreshTTLSeconds) * time.Second
		}
		if cfg.Dashboard.StatsTTLSeconds > 0 {
			cacheTTL = time.Duration(cfg.Dashboard.StatsTTLSeconds) * time.Second
		}
		if cfg.Dashboard.StatsRefreshTimeoutSeconds > 0 {
			refreshTimeout = time.Duration(cfg.Dashboard.StatsRefreshTimeoutSeconds) * time.Second
		}
		aggEnabled = cfg.DashboardAgg.Enabled
		if cfg.DashboardAgg.IntervalSeconds > 0 {
			aggInterval = time.Duration(cfg.DashboardAgg.IntervalSeconds) * time.Second
		}
		if cfg.DashboardAgg.LookbackSeconds > 0 {
			aggLookback = time.Duration(cfg.DashboardAgg.LookbackSeconds) * time.Second
		}
		if cfg.DashboardAgg.Retention.UsageLogsDays > 0 {
			aggUsageDays = cfg.DashboardAgg.Retention.UsageLogsDays
		}
	}
	if aggRepo == nil {
		aggEnabled = false
	}
	return &DashboardService{
		usageRepo:      usageRepo,
		aggRepo:        aggRepo,
		cache:          cache,
		cacheFreshTTL:  freshTTL,
		cacheTTL:       cacheTTL,
		refreshTimeout: refreshTimeout,
		aggEnabled:     aggEnabled,
		aggInterval:    aggInterval,
		aggLookback:    aggLookback,
		aggUsageDays:   aggUsageDays,
	}
}

func (s *DashboardService) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	if s.cache != nil {
		cached, fresh, err := s.getCachedDashboardStats(ctx)
		if err == nil && cached != nil {
			s.refreshAggregationStaleness(cached)
			if !fresh {
				s.refreshDashboardStatsAsync()
			}
			return cached, nil
		}
		if err != nil && !errors.Is(err, ErrDashboardStatsCacheMiss) {
			logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存读取失败: %v", err)
		}
	}

	stats, err := s.refreshDashboardStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get dashboard stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get usage trend with filters: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get model stats with filters: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error) {
	stats, err := s.usageRepo.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get group stats with filters: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) getCachedDashboardStats(ctx context.Context) (*usagestats.DashboardStats, bool, error) {
	data, err := s.cache.GetDashboardStats(ctx)
	if err != nil {
		return nil, false, err
	}

	var entry dashboardStatsCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		s.evictDashboardStatsCache(err)
		return nil, false, ErrDashboardStatsCacheMiss
	}
	if entry.Stats == nil {
		s.evictDashboardStatsCache(errors.New("仪表盘缓存缺少统计数据"))
		return nil, false, ErrDashboardStatsCacheMiss
	}

	age := time.Since(time.Unix(entry.UpdatedAt, 0))
	return entry.Stats, age <= s.cacheFreshTTL, nil
}

func (s *DashboardService) refreshDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	stats, err := s.fetchDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	s.applyAggregationStatus(ctx, stats)
	cacheCtx, cancel := s.cacheOperationContext()
	defer cancel()
	s.saveDashboardStatsCache(cacheCtx, stats)
	return stats, nil
}

func (s *DashboardService) refreshDashboardStatsAsync() {
	if s.cache == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.refreshing, 0, 1) {
		return
	}

	go func() {
		defer atomic.StoreInt32(&s.refreshing, 0)

		ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout)
		defer cancel()

		stats, err := s.fetchDashboardStats(ctx)
		if err != nil {
			logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存异步刷新失败: %v", err)
			return
		}
		s.applyAggregationStatus(ctx, stats)
		cacheCtx, cancel := s.cacheOperationContext()
		defer cancel()
		s.saveDashboardStatsCache(cacheCtx, stats)
	}()
}

func (s *DashboardService) fetchDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	if !s.aggEnabled {
		if fetcher, ok := s.usageRepo.(dashboardStatsRangeFetcher); ok {
			now := time.Now().UTC()
			start := truncateToDayUTC(now.AddDate(0, 0, -s.aggUsageDays))
			return fetcher.GetDashboardStatsWithRange(ctx, start, now)
		}
	}
	return s.usageRepo.GetDashboardStats(ctx)
}

func (s *DashboardService) saveDashboardStatsCache(ctx context.Context, stats *usagestats.DashboardStats) {
	if s.cache == nil || stats == nil {
		return
	}

	entry := dashboardStatsCacheEntry{
		Stats:     stats,
		UpdatedAt: time.Now().Unix(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存序列化失败: %v", err)
		return
	}

	if err := s.cache.SetDashboardStats(ctx, string(data), s.cacheTTL); err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存写入失败: %v", err)
	}
}

func (s *DashboardService) evictDashboardStatsCache(reason error) {
	if s.cache == nil {
		return
	}
	cacheCtx, cancel := s.cacheOperationContext()
	defer cancel()

	if err := s.cache.DeleteDashboardStats(cacheCtx); err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存清理失败: %v", err)
	}
	if reason != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存异常，已清理: %v", reason)
	}
}

func (s *DashboardService) cacheOperationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.refreshTimeout)
}

func (s *DashboardService) applyAggregationStatus(ctx context.Context, stats *usagestats.DashboardStats) {
	if stats == nil {
		return
	}
	updatedAt := s.fetchAggregationUpdatedAt(ctx)
	stats.StatsUpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	stats.StatsStale = s.isAggregationStale(updatedAt, time.Now().UTC())
}

func (s *DashboardService) refreshAggregationStaleness(stats *usagestats.DashboardStats) {
	if stats == nil {
		return
	}
	updatedAt := parseStatsUpdatedAt(stats.StatsUpdatedAt)
	stats.StatsStale = s.isAggregationStale(updatedAt, time.Now().UTC())
}

func (s *DashboardService) fetchAggregationUpdatedAt(ctx context.Context) time.Time {
	if s.aggRepo == nil {
		return time.Unix(0, 0).UTC()
	}
	updatedAt, err := s.aggRepo.GetAggregationWatermark(ctx)
	if err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 读取聚合水位失败: %v", err)
		return time.Unix(0, 0).UTC()
	}
	if updatedAt.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return updatedAt.UTC()
}

func (s *DashboardService) isAggregationStale(updatedAt, now time.Time) bool {
	if !s.aggEnabled {
		return true
	}
	epoch := time.Unix(0, 0).UTC()
	if !updatedAt.After(epoch) {
		return true
	}
	threshold := s.aggInterval + s.aggLookback
	return now.Sub(updatedAt) > threshold
}

func parseStatsUpdatedAt(raw string) time.Time {
	if raw == "" {
		return time.Unix(0, 0).UTC()
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed.UTC()
}

func (s *DashboardService) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
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

func (s *DashboardService) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	ranking, err := s.usageRepo.GetUserSpendingRanking(ctx, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("get user spending ranking: %w", err)
	}
	return ranking, nil
}

func (s *DashboardService) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	stats, err := s.usageRepo.GetUserBreakdownStats(ctx, startTime, endTime, dim, limit)
	if err != nil {
		return nil, fmt.Errorf("get user breakdown stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	stats, err := s.usageRepo.GetBatchUserUsageStats(ctx, userIDs, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get batch user usage stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
	}
	return stats, nil
}
