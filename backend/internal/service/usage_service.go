package service

import (
	"context"
	"fmt"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

var (
	ErrUsageLogNotFound = infraerrors.NotFound("USAGE_LOG_NOT_FOUND", "usage log not found")
)

// CreateUsageLogRequest 创建使用日志请求
type CreateUsageLogRequest struct {
	UserID                int64   `json:"user_id"`
	ApiKeyID              int64   `json:"api_key_id"`
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
REDACTED

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
REDACTED

// UsageService 使用统计服务
type UsageService struct {
	usageRepo UsageLogRepository
	userRepo  UserRepository
REDACTED

// NewUsageService 创建使用统计服务实例
func NewUsageService(usageRepo UsageLogRepository, userRepo UserRepository) *UsageService {
	return &UsageService{
		usageRepo: usageRepo,
		userRepo:  userRepo,
REDACTED
REDACTED

// Create 创建使用日志
func (s *UsageService) Create(ctx context.Context, req CreateUsageLogRequest) (*UsageLog, error) {
	// 验证用户存在
	_, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

	// 创建使用日志
	usageLog := &UsageLog{
		UserID:                req.UserID,
		ApiKeyID:              req.ApiKeyID,
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
REDACTED

	if err := s.usageRepo.Create(ctx, usageLog); err != nil {
		return nil, fmt.Errorf("create usage log: %w", err)
REDACTED

	// 扣除用户余额
	if req.ActualCost > 0 {
		if err := s.userRepo.UpdateBalance(ctx, req.UserID, -req.ActualCost); err != nil {
			return nil, fmt.Errorf("update user balance: %w", err)
	REDACTED
REDACTED

	return usageLog, nil
REDACTED

// GetByID 根据ID获取使用日志
func (s *UsageService) GetByID(ctx context.Context, id int64) (*UsageLog, error) {
	log, err := s.usageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get usage log: %w", err)
REDACTED
	return log, nil
REDACTED

// ListByUser 获取用户的使用日志列表
func (s *UsageService) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByUser(ctx, userID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
REDACTED
	return logs, pagination, nil
REDACTED

// ListByApiKey 获取API Key的使用日志列表
func (s *UsageService) ListByApiKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByApiKey(ctx, apiKeyID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
REDACTED
	return logs, pagination, nil
REDACTED

// ListByAccount 获取账号的使用日志列表
func (s *UsageService) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, pagination, err := s.usageRepo.ListByAccount(ctx, accountID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs: %w", err)
REDACTED
	return logs, pagination, nil
REDACTED

// GetStatsByUser 获取用户的使用统计
func (s *UsageService) GetStatsByUser(ctx context.Context, userID int64, startTime, endTime time.Time) (*UsageStats, error) {
	stats, err := s.usageRepo.GetUserStatsAggregated(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
REDACTED

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
REDACTED, nil
REDACTED

// GetStatsByApiKey 获取API Key的使用统计
func (s *UsageService) GetStatsByApiKey(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*UsageStats, error) {
	stats, err := s.usageRepo.GetApiKeyStatsAggregated(ctx, apiKeyID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get api key stats: %w", err)
REDACTED

	return &UsageStats{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalCost:         stats.TotalCost,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
REDACTED, nil
REDACTED

// GetStatsByAccount 获取账号的使用统计
func (s *UsageService) GetStatsByAccount(ctx context.Context, accountID int64, startTime, endTime time.Time) (*UsageStats, error) {
	logs, _, err := s.usageRepo.ListByAccountAndTimeRange(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("list usage logs: %w", err)
REDACTED

	return s.calculateStats(logs), nil
REDACTED

// GetStatsByModel 获取模型的使用统计
func (s *UsageService) GetStatsByModel(ctx context.Context, modelName string, startTime, endTime time.Time) (*UsageStats, error) {
	logs, _, err := s.usageRepo.ListByModelAndTimeRange(ctx, modelName, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("list usage logs: %w", err)
REDACTED

	return s.calculateStats(logs), nil
REDACTED

// GetDailyStats 获取每日使用统计（最近N天）
func (s *UsageService) GetDailyStats(ctx context.Context, userID int64, days int) ([]map[string]any, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)

	logs, _, err := s.usageRepo.ListByUserAndTimeRange(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("list usage logs: %w", err)
REDACTED

	// 按日期分组统计
	dailyStats := make(map[string]*UsageStats)
	for _, log := range logs {
		dateKey := log.CreatedAt.Format("2006-01-02")
		if _, exists := dailyStats[dateKey]; !exists {
			dailyStats[dateKey] = &UsageStats{REDACTED
	REDACTED

		stats := dailyStats[dateKey]
		stats.TotalRequests++
		stats.TotalInputTokens += int64(log.InputTokens)
		stats.TotalOutputTokens += int64(log.OutputTokens)
		stats.TotalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		stats.TotalTokens += int64(log.TotalTokens())
		stats.TotalCost += log.TotalCost
		stats.TotalActualCost += log.ActualCost

		if log.DurationMs != nil {
			stats.AverageDurationMs += float64(*log.DurationMs)
	REDACTED
REDACTED

	// 计算平均值并转换为数组
	result := make([]map[string]any, 0, len(dailyStats))
	for date, stats := range dailyStats {
		if stats.TotalRequests > 0 {
			stats.AverageDurationMs /= float64(stats.TotalRequests)
	REDACTED

		result = append(result, map[string]any{
			"date":                date,
			"total_requests":      stats.TotalRequests,
			"total_input_tokens":  stats.TotalInputTokens,
			"total_output_tokens": stats.TotalOutputTokens,
			"total_cache_tokens":  stats.TotalCacheTokens,
			"total_tokens":        stats.TotalTokens,
			"total_cost":          stats.TotalCost,
			"total_actual_cost":   stats.TotalActualCost,
			"average_duration_ms": stats.AverageDurationMs,
	REDACTED)
REDACTED

	return result, nil
REDACTED

// calculateStats 计算统计数据
func (s *UsageService) calculateStats(logs []UsageLog) *UsageStats {
	stats := &UsageStats{REDACTED

	for _, log := range logs {
		stats.TotalRequests++
		stats.TotalInputTokens += int64(log.InputTokens)
		stats.TotalOutputTokens += int64(log.OutputTokens)
		stats.TotalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		stats.TotalTokens += int64(log.TotalTokens())
		stats.TotalCost += log.TotalCost
		stats.TotalActualCost += log.ActualCost

		if log.DurationMs != nil {
			stats.AverageDurationMs += float64(*log.DurationMs)
	REDACTED
REDACTED

	// 计算平均持续时间
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs /= float64(stats.TotalRequests)
REDACTED

	return stats
REDACTED

// Delete 删除使用日志（管理员功能，谨慎使用）
func (s *UsageService) Delete(ctx context.Context, id int64) error {
	if err := s.usageRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete usage log: %w", err)
REDACTED
	return nil
REDACTED

// GetUserDashboardStats returns per-user dashboard summary stats.
func (s *UsageService) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	stats, err := s.usageRepo.GetUserDashboardStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user dashboard stats: %w", err)
REDACTED
	return stats, nil
REDACTED

// GetUserUsageTrendByUserID returns per-user usage trend.
func (s *UsageService) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrendByUserID(ctx, userID, startTime, endTime, granularity)
	if err != nil {
		return nil, fmt.Errorf("get user usage trend: %w", err)
REDACTED
	return trend, nil
REDACTED

// GetUserModelStats returns per-user model usage stats.
func (s *UsageService) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetUserModelStats(ctx, userID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get user model stats: %w", err)
REDACTED
	return stats, nil
REDACTED

// GetBatchApiKeyUsageStats returns today/total actual_cost for given api keys.
func (s *UsageService) GetBatchApiKeyUsageStats(ctx context.Context, apiKeyIDs []int64) (map[int64]*usagestats.BatchApiKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchApiKeyUsageStats(ctx, apiKeyIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
REDACTED
	return stats, nil
REDACTED

// ListWithFilters lists usage logs with admin filters.
func (s *UsageService) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error) {
	logs, result, err := s.usageRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list usage logs with filters: %w", err)
REDACTED
	return logs, result, nil
REDACTED

// GetGlobalStats returns global usage stats for a time range.
func (s *UsageService) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	stats, err := s.usageRepo.GetGlobalStats(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get global usage stats: %w", err)
REDACTED
	return stats, nil
REDACTED
