package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
REDACTED

type dashboardStatsCacheEntry struct {
	Stats     *usagestats.DashboardStats `json:"stats"`
	UpdatedAt int64                      `json:"updated_at"`
REDACTED

// DashboardService provides aggregated statistics for admin dashboard.
type DashboardService struct {
	usageRepo      UsageLogRepository
	cache          DashboardStatsCache
	cacheFreshTTL  time.Duration
	cacheTTL       time.Duration
	refreshTimeout time.Duration
	refreshing     int32
REDACTED

func NewDashboardService(usageRepo UsageLogRepository, cache DashboardStatsCache, cfg *config.Config) *DashboardService {
	freshTTL := defaultDashboardStatsFreshTTL
	cacheTTL := defaultDashboardStatsCacheTTL
	refreshTimeout := defaultDashboardStatsRefreshTimeout
	if cfg != nil {
		if !cfg.Dashboard.Enabled {
			cache = nil
	REDACTED
		if cfg.Dashboard.StatsFreshTTLSeconds > 0 {
			freshTTL = time.Duration(cfg.Dashboard.StatsFreshTTLSeconds) * time.Second
	REDACTED
		if cfg.Dashboard.StatsTTLSeconds > 0 {
			cacheTTL = time.Duration(cfg.Dashboard.StatsTTLSeconds) * time.Second
	REDACTED
		if cfg.Dashboard.StatsRefreshTimeoutSeconds > 0 {
			refreshTimeout = time.Duration(cfg.Dashboard.StatsRefreshTimeoutSeconds) * time.Second
	REDACTED
REDACTED
	return &DashboardService{
		usageRepo:      usageRepo,
		cache:          cache,
		cacheFreshTTL:  freshTTL,
		cacheTTL:       cacheTTL,
		refreshTimeout: refreshTimeout,
REDACTED
REDACTED

func (s *DashboardService) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	if s.cache != nil {
		cached, fresh, err := s.getCachedDashboardStats(ctx)
		if err == nil && cached != nil {
			if !fresh {
				s.refreshDashboardStatsAsync()
		REDACTED
			return cached, nil
	REDACTED
		if err != nil && !errors.Is(err, ErrDashboardStatsCacheMiss) {
			log.Printf("[Dashboard] 仪表盘缓存读取失败: %v", err)
	REDACTED
REDACTED

	stats, err := s.refreshDashboardStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get dashboard stats: %w", err)
REDACTED
	return stats, nil
REDACTED

func (s *DashboardService) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID int64) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("get usage trend with filters: %w", err)
REDACTED
	return trend, nil
REDACTED

func (s *DashboardService) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID int64) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, 0)
	if err != nil {
		return nil, fmt.Errorf("get model stats with filters: %w", err)
REDACTED
	return stats, nil
REDACTED

func (s *DashboardService) getCachedDashboardStats(ctx context.Context) (*usagestats.DashboardStats, bool, error) {
	data, err := s.cache.GetDashboardStats(ctx)
	if err != nil {
		return nil, false, err
REDACTED

	var entry dashboardStatsCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, false, err
REDACTED
	if entry.Stats == nil {
		return nil, false, errors.New("仪表盘缓存缺少统计数据")
REDACTED

	age := time.Since(time.Unix(entry.UpdatedAt, 0))
	return entry.Stats, age <= s.cacheFreshTTL, nil
REDACTED

func (s *DashboardService) refreshDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	stats, err := s.usageRepo.GetDashboardStats(ctx)
	if err != nil {
		return nil, err
REDACTED
	s.saveDashboardStatsCache(ctx, stats)
	return stats, nil
REDACTED

func (s *DashboardService) refreshDashboardStatsAsync() {
	if s.cache == nil {
		return
REDACTED
	if !atomic.CompareAndSwapInt32(&s.refreshing, 0, 1) {
		return
REDACTED

	go func() {
		defer atomic.StoreInt32(&s.refreshing, 0)

		ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout)
		defer cancel()

		stats, err := s.usageRepo.GetDashboardStats(ctx)
		if err != nil {
			log.Printf("[Dashboard] 仪表盘缓存异步刷新失败: %v", err)
			return
	REDACTED
		s.saveDashboardStatsCache(ctx, stats)
REDACTED()
REDACTED

func (s *DashboardService) saveDashboardStatsCache(ctx context.Context, stats *usagestats.DashboardStats) {
	if s.cache == nil || stats == nil {
		return
REDACTED

	entry := dashboardStatsCacheEntry{
		Stats:     stats,
		UpdatedAt: time.Now().Unix(),
REDACTED
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[Dashboard] 仪表盘缓存序列化失败: %v", err)
		return
REDACTED

	if err := s.cache.SetDashboardStats(ctx, string(data), s.cacheTTL); err != nil {
		log.Printf("[Dashboard] 仪表盘缓存写入失败: %v", err)
REDACTED
REDACTED

func (s *DashboardService) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get api key usage trend: %w", err)
REDACTED
	return trend, nil
REDACTED

func (s *DashboardService) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get user usage trend: %w", err)
REDACTED
	return trend, nil
REDACTED

func (s *DashboardService) GetBatchUserUsageStats(ctx context.Context, userIDs []int64) (map[int64]*usagestats.BatchUserUsageStats, error) {
	stats, err := s.usageRepo.GetBatchUserUsageStats(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch user usage stats: %w", err)
REDACTED
	return stats, nil
REDACTED

func (s *DashboardService) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
REDACTED
	return stats, nil
REDACTED
