package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type UsageLogRepository interface {
	Create(ctx context.Context, log *UsageLog) error
	GetByID(ctx context.Context, id int64) (*UsageLog, error)
	Delete(ctx context.Context, id int64) error

	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByApiKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)

	ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByApiKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)

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
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error)
	GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error)

	// Account stats
	GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error)

	// Aggregated stats (optimized)
	GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetApiKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error)
REDACTED

// apiUsageCache 缓存从 Anthropic API 获取的使用率数据（utilization, resets_at）
type apiUsageCache struct {
	response  *ClaudeUsageResponse
	timestamp time.Time
REDACTED

// windowStatsCache 缓存从本地数据库查询的窗口统计（requests, tokens, cost）
type windowStatsCache struct {
	stats     *WindowStats
	timestamp time.Time
REDACTED

var (
	apiCacheMap         = sync.Map{REDACTED // 缓存 API 响应
	windowStatsCacheMap = sync.Map{REDACTED // 缓存窗口统计
	apiCacheTTL         = 10 * time.Minute
	windowStatsCacheTTL = 1 * time.Minute
)

// WindowStats 窗口期统计
type WindowStats struct {
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	Cost     float64 `json:"cost"`
REDACTED

// UsageProgress 使用量进度
type UsageProgress struct {
	Utilization      float64      `json:"utilization"`            // 使用率百分比 (0-100+，100表示100%)
	ResetsAt         *time.Time   `json:"resets_at"`              // 重置时间
	RemainingSeconds int          `json:"remaining_seconds"`      // 距重置剩余秒数
	WindowStats      *WindowStats `json:"window_stats,omitempty"` // 窗口期统计（从窗口开始到当前的使用量）
REDACTED

// UsageInfo 账号使用量信息
type UsageInfo struct {
	UpdatedAt      *time.Time     `json:"updated_at,omitempty"`       // 更新时间
	FiveHour       *UsageProgress `json:"five_hour"`                  // 5小时窗口
	SevenDay       *UsageProgress `json:"seven_day,omitempty"`        // 7天窗口
	SevenDaySonnet *UsageProgress `json:"seven_day_sonnet,omitempty"` // 7天Sonnet窗口
	GeminiProDaily *UsageProgress `json:"gemini_pro_daily,omitempty"` // Gemini Pro 日配额
	GeminiFlashDaily *UsageProgress `json:"gemini_flash_daily,omitempty"` // Gemini Flash 日配额
REDACTED

// ClaudeUsageResponse Anthropic API返回的usage结构
type ClaudeUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
REDACTED `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
REDACTED `json:"seven_day"`
	SevenDaySonnet struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
REDACTED `json:"seven_day_sonnet"`
REDACTED

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*ClaudeUsageResponse, error)
REDACTED

// AccountUsageService 账号使用量查询服务
type AccountUsageService struct {
	accountRepo         AccountRepository
	usageLogRepo        UsageLogRepository
	usageFetcher        ClaudeUsageFetcher
	geminiQuotaService  *GeminiQuotaService
REDACTED

// NewAccountUsageService 创建AccountUsageService实例
func NewAccountUsageService(accountRepo AccountRepository, usageLogRepo UsageLogRepository, usageFetcher ClaudeUsageFetcher, geminiQuotaService *GeminiQuotaService) *AccountUsageService {
	return &AccountUsageService{
		accountRepo:        accountRepo,
		usageLogRepo:       usageLogRepo,
		usageFetcher:       usageFetcher,
		geminiQuotaService: geminiQuotaService,
REDACTED
REDACTED

// GetUsage 获取账号使用量
// OAuth账号: 调用Anthropic API获取真实数据（需要profile scope），API响应缓存10分钟，窗口统计缓存1分钟
// Setup Token账号: 根据session_window推算5h窗口，7d数据不可用（没有profile scope）
// API Key账号: 不支持usage查询
func (s *AccountUsageService) GetUsage(ctx context.Context, accountID int64) (*UsageInfo, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
REDACTED

	if account.Platform == PlatformGemini {
		return s.getGeminiUsage(ctx, account)
REDACTED

	// 只有oauth类型账号可以通过API获取usage（有profile scope）
	if account.CanGetUsage() {
		var apiResp *ClaudeUsageResponse

		// 1. 检查 API 缓存（10 分钟）
		if cached, ok := apiCacheMap.Load(accountID); ok {
			if cache, ok := cached.(*apiUsageCache); ok && time.Since(cache.timestamp) < apiCacheTTL {
				apiResp = cache.response
		REDACTED
	REDACTED

		// 2. 如果没有缓存，从 API 获取
		if apiResp == nil {
			apiResp, err = s.fetchOAuthUsageRaw(ctx, account)
			if err != nil {
				return nil, err
		REDACTED
			// 缓存 API 响应
			apiCacheMap.Store(accountID, &apiUsageCache{
				response:  apiResp,
				timestamp: time.Now(),
		REDACTED)
	REDACTED

		// 3. 构建 UsageInfo（每次都重新计算 RemainingSeconds）
		now := time.Now()
		usage := s.buildUsageInfo(apiResp, &now)

		// 4. 添加窗口统计（有独立缓存，1 分钟）
		s.addWindowStats(ctx, account, usage)

		return usage, nil
REDACTED

	// Setup Token账号：根据session_window推算（没有profile scope，无法调用usage API）
	if account.Type == AccountTypeSetupToken {
		usage := s.estimateSetupTokenUsage(account)
		// 添加窗口统计
		s.addWindowStats(ctx, account, usage)
		return usage, nil
REDACTED

	// API Key账号不支持usage查询
	return nil, fmt.Errorf("account type %s does not support usage query", account.Type)
REDACTED

func (s *AccountUsageService) getGeminiUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	now := time.Now()
	start := geminiDailyWindowStart(now)

	stats, err := s.usageLogRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID)
	if err != nil {
		return nil, fmt.Errorf("get gemini usage stats failed: %w", err)
REDACTED

	usage := &UsageInfo{
		UpdatedAt: &now,
REDACTED

	quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
	if !ok {
		return usage, nil
REDACTED

	totals := geminiAggregateUsage(stats)
	resetAt := geminiDailyResetTime(now)

	usage.GeminiProDaily = buildGeminiUsageProgress(totals.ProRequests, quota.ProRPD, resetAt, totals.ProTokens, totals.ProCost, now)
	usage.GeminiFlashDaily = buildGeminiUsageProgress(totals.FlashRequests, quota.FlashRPD, resetAt, totals.FlashTokens, totals.FlashCost, now)

	return usage, nil
REDACTED

// addWindowStats 为 usage 数据添加窗口期统计
// 使用独立缓存（1 分钟），与 API 缓存分离
func (s *AccountUsageService) addWindowStats(ctx context.Context, account *Account, usage *UsageInfo) {
	// 修复：即使 FiveHour 为 nil，也要尝试获取统计数据
	// 因为 SevenDay/SevenDaySonnet 可能需要
	if usage.FiveHour == nil && usage.SevenDay == nil && usage.SevenDaySonnet == nil {
		return
REDACTED

	// 检查窗口统计缓存（1 分钟）
	var windowStats *WindowStats
	if cached, ok := windowStatsCacheMap.Load(account.ID); ok {
		if cache, ok := cached.(*windowStatsCache); ok && time.Since(cache.timestamp) < windowStatsCacheTTL {
			windowStats = cache.stats
	REDACTED
REDACTED

	// 如果没有缓存，从数据库查询
	if windowStats == nil {
		var startTime time.Time
		if account.SessionWindowStart != nil {
			startTime = *account.SessionWindowStart
	REDACTED else {
			startTime = time.Now().Add(-5 * time.Hour)
	REDACTED

		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
		if err != nil {
			log.Printf("Failed to get window stats for account %d: %v", account.ID, err)
			return
	REDACTED

		windowStats = &WindowStats{
			Requests: stats.Requests,
			Tokens:   stats.Tokens,
			Cost:     stats.Cost,
	REDACTED

		// 缓存窗口统计（1 分钟）
		windowStatsCacheMap.Store(account.ID, &windowStatsCache{
			stats:     windowStats,
			timestamp: time.Now(),
	REDACTED)
REDACTED

	// 为 FiveHour 添加 WindowStats（5h 窗口统计）
	if usage.FiveHour != nil {
		usage.FiveHour.WindowStats = windowStats
REDACTED
REDACTED

// GetTodayStats 获取账号今日统计
func (s *AccountUsageService) GetTodayStats(ctx context.Context, accountID int64) (*WindowStats, error) {
	stats, err := s.usageLogRepo.GetAccountTodayStats(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get today stats failed: %w", err)
REDACTED

	return &WindowStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
REDACTED, nil
REDACTED

func (s *AccountUsageService) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	stats, err := s.usageLogRepo.GetAccountUsageStats(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get account usage stats failed: %w", err)
REDACTED
	return stats, nil
REDACTED

// fetchOAuthUsageRaw 从 Anthropic API 获取原始响应（不构建 UsageInfo）
func (s *AccountUsageService) fetchOAuthUsageRaw(ctx context.Context, account *Account) (*ClaudeUsageResponse, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	return s.usageFetcher.FetchUsage(ctx, accessToken, proxyURL)
REDACTED

// parseTime 尝试多种格式解析时间
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
REDACTED
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
	REDACTED
REDACTED
	return time.Time{REDACTED, fmt.Errorf("unable to parse time: %s", s)
REDACTED

// buildUsageInfo 构建UsageInfo
func (s *AccountUsageService) buildUsageInfo(resp *ClaudeUsageResponse, updatedAt *time.Time) *UsageInfo {
	info := &UsageInfo{
		UpdatedAt: updatedAt,
REDACTED

	// 5小时窗口 - 始终创建对象（即使 ResetsAt 为空）
	info.FiveHour = &UsageProgress{
		Utilization: resp.FiveHour.Utilization,
REDACTED
	if resp.FiveHour.ResetsAt != "" {
		if fiveHourReset, err := parseTime(resp.FiveHour.ResetsAt); err == nil {
			info.FiveHour.ResetsAt = &fiveHourReset
			info.FiveHour.RemainingSeconds = int(time.Until(fiveHourReset).Seconds())
	REDACTED else {
			log.Printf("Failed to parse FiveHour.ResetsAt: %s, error: %v", resp.FiveHour.ResetsAt, err)
	REDACTED
REDACTED

	// 7天窗口
	if resp.SevenDay.ResetsAt != "" {
		if sevenDayReset, err := parseTime(resp.SevenDay.ResetsAt); err == nil {
			info.SevenDay = &UsageProgress{
				Utilization:      resp.SevenDay.Utilization,
				ResetsAt:         &sevenDayReset,
				RemainingSeconds: int(time.Until(sevenDayReset).Seconds()),
		REDACTED
	REDACTED else {
			log.Printf("Failed to parse SevenDay.ResetsAt: %s, error: %v", resp.SevenDay.ResetsAt, err)
			info.SevenDay = &UsageProgress{
				Utilization: resp.SevenDay.Utilization,
		REDACTED
	REDACTED
REDACTED

	// 7天Sonnet窗口
	if resp.SevenDaySonnet.ResetsAt != "" {
		if sonnetReset, err := parseTime(resp.SevenDaySonnet.ResetsAt); err == nil {
			info.SevenDaySonnet = &UsageProgress{
				Utilization:      resp.SevenDaySonnet.Utilization,
				ResetsAt:         &sonnetReset,
				RemainingSeconds: int(time.Until(sonnetReset).Seconds()),
		REDACTED
	REDACTED else {
			log.Printf("Failed to parse SevenDaySonnet.ResetsAt: %s, error: %v", resp.SevenDaySonnet.ResetsAt, err)
			info.SevenDaySonnet = &UsageProgress{
				Utilization: resp.SevenDaySonnet.Utilization,
		REDACTED
	REDACTED
REDACTED

	return info
REDACTED

// estimateSetupTokenUsage 根据session_window推算Setup Token账号的使用量
func (s *AccountUsageService) estimateSetupTokenUsage(account *Account) *UsageInfo {
	info := &UsageInfo{REDACTED

	// 如果有session_window信息
	if account.SessionWindowEnd != nil {
		remaining := int(time.Until(*account.SessionWindowEnd).Seconds())
		if remaining < 0 {
			remaining = 0
	REDACTED

		// 根据状态估算使用率 (百分比形式，100 = 100%)
		var utilization float64
		switch account.SessionWindowStatus {
		case "rejected":
			utilization = 100.0
		case "allowed_warning":
			utilization = 80.0
		default:
			utilization = 0.0
	REDACTED

		info.FiveHour = &UsageProgress{
			Utilization:      utilization,
			ResetsAt:         account.SessionWindowEnd,
			RemainingSeconds: remaining,
	REDACTED
REDACTED else {
		// 没有窗口信息，返回空数据
		info.FiveHour = &UsageProgress{
			Utilization:      0,
			RemainingSeconds: 0,
	REDACTED
REDACTED

	// Setup Token无法获取7d数据
	return info
REDACTED

func buildGeminiUsageProgress(used, limit int64, resetAt time.Time, tokens int64, cost float64, now time.Time) *UsageProgress {
	if limit <= 0 {
		return nil
REDACTED
	utilization := (float64(used) / float64(limit)) * 100
	remainingSeconds := int(resetAt.Sub(now).Seconds())
	if remainingSeconds < 0 {
		remainingSeconds = 0
REDACTED
	resetCopy := resetAt
	return &UsageProgress{
		Utilization:      utilization,
		ResetsAt:         &resetCopy,
		RemainingSeconds: remainingSeconds,
		WindowStats: &WindowStats{
			Requests: used,
			Tokens:   tokens,
			Cost:     cost,
	REDACTED,
REDACTED
REDACTED
