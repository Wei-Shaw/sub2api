package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type UsageLogRepository interface {
	// Create creates a usage log and returns whether it was actually inserted.
	// inserted is false when the insert was skipped due to conflict (idempotent retries).
	Create(ctx context.Context, log *UsageLog) (inserted bool, err error)
	GetByID(ctx context.Context, id int64) (*UsageLog, error)
	Delete(ctx context.Context, id int64) error

	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)

	ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)

	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error)
	GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error)

	// Admin dashboard stats
	GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error)
	GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error)
	GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error)
	GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error)
	GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error)
	GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error)
	GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error)
	GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error)

	// User dashboard stats
	GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error)
	GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error)
	GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error)
	GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error)

	// Admin usage listing/stats
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error)
	GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error)

	// Account stats
	GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error)

	// Aggregated stats (optimized)
	GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error)
REDACTED

type accountWindowStatsBatchReader interface {
	GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error)
REDACTED

// apiUsageCache 缓存从 Anthropic API 获取的使用率数据（utilization, resets_at）
// 同时支持缓存错误响应（负缓存），防止 429 等错误导致的重试风暴
type apiUsageCache struct {
	response  *ClaudeUsageResponse
	err       error // 非 nil 表示缓存的错误（负缓存）
	timestamp time.Time
REDACTED

// windowStatsCache 缓存从本地数据库查询的窗口统计（requests, tokens, cost）
type windowStatsCache struct {
	stats     *WindowStats
	timestamp time.Time
REDACTED

// antigravityUsageCache 缓存 Antigravity 额度数据
type antigravityUsageCache struct {
	usageInfo *UsageInfo
	timestamp time.Time
REDACTED

const (
	apiCacheTTL             = 3 * time.Minute
	apiErrorCacheTTL        = 1 * time.Minute        // 负缓存 TTL：429 等错误缓存 1 分钟
	apiQueryMaxJitter       = 800 * time.Millisecond // 用量查询最大随机延迟
	windowStatsCacheTTL     = 1 * time.Minute
	openAIProbeCacheTTL     = 10 * time.Minute
	openAICodexProbeVersion = "0.104.0"
)

// UsageCache 封装账户使用量相关的缓存
type UsageCache struct {
	apiCache         sync.Map           // accountID -> *apiUsageCache
	windowStatsCache sync.Map           // accountID -> *windowStatsCache
	antigravityCache sync.Map           // accountID -> *antigravityUsageCache
	apiFlight        singleflight.Group // 防止同一账号的并发请求击穿缓存
	openAIProbeCache sync.Map           // accountID -> time.Time
REDACTED

// NewUsageCache 创建 UsageCache 实例
func NewUsageCache() *UsageCache {
	return &UsageCache{REDACTED
REDACTED

// WindowStats 窗口期统计
//
// cost: 账号口径费用（total_cost * account_rate_multiplier）
// standard_cost: 标准费用（total_cost，不含倍率）
// user_cost: 用户/API Key 口径费用（actual_cost，受分组倍率影响）
type WindowStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
REDACTED

// UsageProgress 使用量进度
type UsageProgress struct {
	Utilization      float64      `json:"utilization"`            // 使用率百分比 (0-100+，100表示100%)
	ResetsAt         *time.Time   `json:"resets_at"`              // 重置时间
	RemainingSeconds int          `json:"remaining_seconds"`      // 距重置剩余秒数
	WindowStats      *WindowStats `json:"window_stats,omitempty"` // 窗口期统计（从窗口开始到当前的使用量）
	UsedRequests     int64        `json:"used_requests,omitempty"`
	LimitRequests    int64        `json:"limit_requests,omitempty"`
REDACTED

// AntigravityModelQuota Antigravity 单个模型的配额信息
type AntigravityModelQuota struct {
	Utilization int    `json:"utilization"` // 使用率 0-100
	ResetTime   string `json:"reset_time"`  // 重置时间 ISO8601
REDACTED

// UsageInfo 账号使用量信息
type UsageInfo struct {
	UpdatedAt          *time.Time     `json:"updated_at,omitempty"`           // 更新时间
	FiveHour           *UsageProgress `json:"five_hour"`                      // 5小时窗口
	SevenDay           *UsageProgress `json:"seven_day,omitempty"`            // 7天窗口
	SevenDaySonnet     *UsageProgress `json:"seven_day_sonnet,omitempty"`     // 7天Sonnet窗口
	GeminiSharedDaily  *UsageProgress `json:"gemini_shared_daily,omitempty"`  // Gemini shared pool RPD (Google One / Code Assist)
	GeminiProDaily     *UsageProgress `json:"gemini_pro_daily,omitempty"`     // Gemini Pro 日配额
	GeminiFlashDaily   *UsageProgress `json:"gemini_flash_daily,omitempty"`   // Gemini Flash 日配额
	GeminiSharedMinute *UsageProgress `json:"gemini_shared_minute,omitempty"` // Gemini shared pool RPM (Google One / Code Assist)
	GeminiProMinute    *UsageProgress `json:"gemini_pro_minute,omitempty"`    // Gemini Pro RPM
	GeminiFlashMinute  *UsageProgress `json:"gemini_flash_minute,omitempty"`  // Gemini Flash RPM

	// Antigravity 多模型配额
	AntigravityQuota map[string]*AntigravityModelQuota `json:"antigravity_quota,omitempty"`
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

// ClaudeUsageFetchOptions 包含获取 Claude 用量数据所需的所有选项
type ClaudeUsageFetchOptions struct {
	AccessToken          string       // OAuth access token
	ProxyURL             string       // 代理 URL（可选）
	AccountID            int64        // 账号 ID（用于 TLS 指纹选择）
	EnableTLSFingerprint bool         // 是否启用 TLS 指纹伪装
	Fingerprint          *Fingerprint // 缓存的指纹信息（User-Agent 等）
REDACTED

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*ClaudeUsageResponse, error)
	// FetchUsageWithOptions 使用完整选项获取用量数据，支持 TLS 指纹和自定义 User-Agent
	FetchUsageWithOptions(ctx context.Context, opts *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error)
REDACTED

// AccountUsageService 账号使用量查询服务
type AccountUsageService struct {
	accountRepo             AccountRepository
	usageLogRepo            UsageLogRepository
	usageFetcher            ClaudeUsageFetcher
	geminiQuotaService      *GeminiQuotaService
	antigravityQuotaFetcher *AntigravityQuotaFetcher
	cache                   *UsageCache
	identityCache           IdentityCache
REDACTED

// NewAccountUsageService 创建AccountUsageService实例
func NewAccountUsageService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageFetcher ClaudeUsageFetcher,
	geminiQuotaService *GeminiQuotaService,
	antigravityQuotaFetcher *AntigravityQuotaFetcher,
	cache *UsageCache,
	identityCache IdentityCache,
) *AccountUsageService {
	return &AccountUsageService{
		accountRepo:             accountRepo,
		usageLogRepo:            usageLogRepo,
		usageFetcher:            usageFetcher,
		geminiQuotaService:      geminiQuotaService,
		antigravityQuotaFetcher: antigravityQuotaFetcher,
		cache:                   cache,
		identityCache:           identityCache,
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

	if account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth {
		usage, err := s.getOpenAIUsage(ctx, account)
		if err == nil {
			s.tryClearRecoverableAccountError(ctx, account)
	REDACTED
		return usage, err
REDACTED

	if account.Platform == PlatformGemini {
		usage, err := s.getGeminiUsage(ctx, account)
		if err == nil {
			s.tryClearRecoverableAccountError(ctx, account)
	REDACTED
		return usage, err
REDACTED

	// Antigravity 平台：使用 AntigravityQuotaFetcher 获取额度
	if account.Platform == PlatformAntigravity {
		usage, err := s.getAntigravityUsage(ctx, account)
		if err == nil {
			s.tryClearRecoverableAccountError(ctx, account)
	REDACTED
		return usage, err
REDACTED

	// 只有oauth类型账号可以通过API获取usage（有profile scope）
	if account.CanGetUsage() {
		var apiResp *ClaudeUsageResponse

		// 1. 检查缓存（成功响应 3 分钟 / 错误响应 1 分钟）
		if cached, ok := s.cache.apiCache.Load(accountID); ok {
			if cache, ok := cached.(*apiUsageCache); ok {
				age := time.Since(cache.timestamp)
				if cache.err != nil && age < apiErrorCacheTTL {
					// 负缓存命中：返回缓存的错误，避免重试风暴
					return nil, cache.err
			REDACTED
				if cache.response != nil && age < apiCacheTTL {
					apiResp = cache.response
			REDACTED
		REDACTED
	REDACTED

		// 2. 如果没有有效缓存，通过 singleflight 从 API 获取（防止并发击穿）
		if apiResp == nil {
			// 随机延迟：打散多账号并发请求，避免同一时刻大量相同 TLS 指纹请求
			// 触发上游反滥用检测。延迟范围 0~800ms，仅在缓存未命中时生效。
			jitter := time.Duration(rand.Int64N(int64(apiQueryMaxJitter)))
			select {
			case <-time.After(jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
		REDACTED

			flightKey := fmt.Sprintf("usage:%d", accountID)
			result, flightErr, _ := s.cache.apiFlight.Do(flightKey, func() (any, error) {
				// 再次检查缓存（可能在等待 singleflight 期间被其他请求填充）
				if cached, ok := s.cache.apiCache.Load(accountID); ok {
					if cache, ok := cached.(*apiUsageCache); ok {
						age := time.Since(cache.timestamp)
						if cache.err != nil && age < apiErrorCacheTTL {
							return nil, cache.err
					REDACTED
						if cache.response != nil && age < apiCacheTTL {
							return cache.response, nil
					REDACTED
				REDACTED
			REDACTED
				resp, fetchErr := s.fetchOAuthUsageRaw(ctx, account)
				if fetchErr != nil {
					// 负缓存：缓存错误响应，防止后续请求重复触发 429
					s.cache.apiCache.Store(accountID, &apiUsageCache{
						err:       fetchErr,
						timestamp: time.Now(),
				REDACTED)
					return nil, fetchErr
			REDACTED
				// 缓存成功响应
				s.cache.apiCache.Store(accountID, &apiUsageCache{
					response:  resp,
					timestamp: time.Now(),
			REDACTED)
				return resp, nil
		REDACTED)
			if flightErr != nil {
				return nil, flightErr
		REDACTED
			apiResp, _ = result.(*ClaudeUsageResponse)
	REDACTED

		// 3. 构建 UsageInfo（每次都重新计算 RemainingSeconds）
		now := time.Now()
		usage := s.buildUsageInfo(apiResp, &now)

		// 4. 添加窗口统计（有独立缓存，1 分钟）
		s.addWindowStats(ctx, account, usage)

		s.tryClearRecoverableAccountError(ctx, account)
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

func (s *AccountUsageService) getOpenAIUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	now := time.Now()
	usage := &UsageInfo{UpdatedAt: &nowREDACTED

	if account == nil {
		return usage, nil
REDACTED

	if progress := buildCodexUsageProgressFromExtra(account.Extra, "5h", now); progress != nil {
		usage.FiveHour = progress
REDACTED
	if progress := buildCodexUsageProgressFromExtra(account.Extra, "7d", now); progress != nil {
		usage.SevenDay = progress
REDACTED

	if shouldRefreshOpenAICodexSnapshot(account, usage, now) && s.shouldProbeOpenAICodexSnapshot(account.ID, now) {
		if updates, err := s.probeOpenAICodexSnapshot(ctx, account); err == nil && len(updates) > 0 {
			mergeAccountExtra(account, updates)
			if usage.UpdatedAt == nil {
				usage.UpdatedAt = &now
		REDACTED
			if progress := buildCodexUsageProgressFromExtra(account.Extra, "5h", now); progress != nil {
				usage.FiveHour = progress
		REDACTED
			if progress := buildCodexUsageProgressFromExtra(account.Extra, "7d", now); progress != nil {
				usage.SevenDay = progress
		REDACTED
	REDACTED
REDACTED

	if s.usageLogRepo == nil {
		return usage, nil
REDACTED

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, now.Add(-5*time.Hour)); err == nil {
		windowStats := windowStatsFromAccountStats(stats)
		if hasMeaningfulWindowStats(windowStats) {
			if usage.FiveHour == nil {
				usage.FiveHour = &UsageProgress{Utilization: 0REDACTED
		REDACTED
			usage.FiveHour.WindowStats = windowStats
	REDACTED
REDACTED

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, now.Add(-7*24*time.Hour)); err == nil {
		windowStats := windowStatsFromAccountStats(stats)
		if hasMeaningfulWindowStats(windowStats) {
			if usage.SevenDay == nil {
				usage.SevenDay = &UsageProgress{Utilization: 0REDACTED
		REDACTED
			usage.SevenDay.WindowStats = windowStats
	REDACTED
REDACTED

	return usage, nil
REDACTED

func shouldRefreshOpenAICodexSnapshot(account *Account, usage *UsageInfo, now time.Time) bool {
	if account == nil {
		return false
REDACTED
	if usage == nil {
		return true
REDACTED
	if usage.FiveHour == nil || usage.SevenDay == nil {
		return true
REDACTED
	if account.IsRateLimited() {
		return true
REDACTED
	return isOpenAICodexSnapshotStale(account, now)
REDACTED

func isOpenAICodexSnapshotStale(account *Account, now time.Time) bool {
	if account == nil || !account.IsOpenAIOAuth() || !account.IsOpenAIResponsesWebSocketV2Enabled() {
		return false
REDACTED
	if account.Extra == nil {
		return true
REDACTED
	raw, ok := account.Extra["codex_usage_updated_at"]
	if !ok {
		return true
REDACTED
	ts, err := parseTime(fmt.Sprint(raw))
	if err != nil {
		return true
REDACTED
	return now.Sub(ts) >= openAIProbeCacheTTL
REDACTED

func (s *AccountUsageService) shouldProbeOpenAICodexSnapshot(accountID int64, now time.Time) bool {
	if s == nil || s.cache == nil || accountID <= 0 {
		return true
REDACTED
	if cached, ok := s.cache.openAIProbeCache.Load(accountID); ok {
		if ts, ok := cached.(time.Time); ok && now.Sub(ts) < openAIProbeCacheTTL {
			return false
	REDACTED
REDACTED
	s.cache.openAIProbeCache.Store(accountID, now)
	return true
REDACTED

func (s *AccountUsageService) probeOpenAICodexSnapshot(ctx context.Context, account *Account) (map[string]any, error) {
	if account == nil || !account.IsOAuth() {
		return nil, nil
REDACTED
	accessToken := account.GetOpenAIAccessToken()
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available")
REDACTED
	modelID := openaipkg.DefaultTestModel
	payload := createOpenAITestPayload(modelID, true)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal openai probe payload: %w", err)
REDACTED

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create openai probe request: %w", err)
REDACTED
	req.Host = "chatgpt.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", openAICodexProbeVersion)
	req.Header.Set("User-Agent", codexCLIUserAgent)
	if s.identityCache != nil {
		if fp, fpErr := s.identityCache.GetFingerprint(reqCtx, account.ID); fpErr == nil && fp != nil && strings.TrimSpace(fp.UserAgent) != "" {
			req.Header.Set("User-Agent", strings.TrimSpace(fp.UserAgent))
	REDACTED
REDACTED
	if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
		req.Header.Set("chatgpt-account-id", chatgptAccountID)
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	client, err := httppool.GetClient(httppool.Options{
		ProxyURL:              proxyURL,
		Timeout:               15 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
REDACTED)
	if err != nil {
		return nil, fmt.Errorf("build openai probe client: %w", err)
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai codex probe request failed: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	updates, err := extractOpenAICodexProbeUpdates(resp)
	if err != nil {
		return nil, err
REDACTED
	if len(updates) > 0 {
		go func(accountID int64, updates map[string]any) {
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer updateCancel()
			_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
	REDACTED(account.ID, updates)
		return updates, nil
REDACTED
	return nil, nil
REDACTED

func extractOpenAICodexProbeUpdates(resp *http.Response) (map[string]any, error) {
	if resp == nil {
		return nil, nil
REDACTED
	if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
		updates := buildCodexUsageExtraUpdates(snapshot, time.Now())
		if len(updates) > 0 {
			return updates, nil
	REDACTED
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai codex probe returned status %d", resp.StatusCode)
REDACTED
	return nil, nil
REDACTED

func mergeAccountExtra(account *Account, updates map[string]any) {
	if account == nil || len(updates) == 0 {
		return
REDACTED
	if account.Extra == nil {
		account.Extra = make(map[string]any, len(updates))
REDACTED
	for k, v := range updates {
		account.Extra[k] = v
REDACTED
REDACTED

func (s *AccountUsageService) getGeminiUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	now := time.Now()
	usage := &UsageInfo{
		UpdatedAt: &now,
REDACTED

	if s.geminiQuotaService == nil || s.usageLogRepo == nil {
		return usage, nil
REDACTED

	quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
	if !ok {
		return usage, nil
REDACTED

	dayStart := geminiDailyWindowStart(now)
	stats, err := s.usageLogRepo.GetModelStatsWithFilters(ctx, dayStart, now, 0, 0, account.ID, 0, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get gemini usage stats failed: %w", err)
REDACTED

	dayTotals := geminiAggregateUsage(stats)
	dailyResetAt := geminiDailyResetTime(now)

	// Daily window (RPD)
	if quota.SharedRPD > 0 {
		totalReq := dayTotals.ProRequests + dayTotals.FlashRequests
		totalTokens := dayTotals.ProTokens + dayTotals.FlashTokens
		totalCost := dayTotals.ProCost + dayTotals.FlashCost
		usage.GeminiSharedDaily = buildGeminiUsageProgress(totalReq, quota.SharedRPD, dailyResetAt, totalTokens, totalCost, now)
REDACTED else {
		usage.GeminiProDaily = buildGeminiUsageProgress(dayTotals.ProRequests, quota.ProRPD, dailyResetAt, dayTotals.ProTokens, dayTotals.ProCost, now)
		usage.GeminiFlashDaily = buildGeminiUsageProgress(dayTotals.FlashRequests, quota.FlashRPD, dailyResetAt, dayTotals.FlashTokens, dayTotals.FlashCost, now)
REDACTED

	// Minute window (RPM) - fixed-window approximation: current minute [truncate(now), truncate(now)+1m)
	minuteStart := now.Truncate(time.Minute)
	minuteResetAt := minuteStart.Add(time.Minute)
	minuteStats, err := s.usageLogRepo.GetModelStatsWithFilters(ctx, minuteStart, now, 0, 0, account.ID, 0, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get gemini minute usage stats failed: %w", err)
REDACTED
	minuteTotals := geminiAggregateUsage(minuteStats)

	if quota.SharedRPM > 0 {
		totalReq := minuteTotals.ProRequests + minuteTotals.FlashRequests
		totalTokens := minuteTotals.ProTokens + minuteTotals.FlashTokens
		totalCost := minuteTotals.ProCost + minuteTotals.FlashCost
		usage.GeminiSharedMinute = buildGeminiUsageProgress(totalReq, quota.SharedRPM, minuteResetAt, totalTokens, totalCost, now)
REDACTED else {
		usage.GeminiProMinute = buildGeminiUsageProgress(minuteTotals.ProRequests, quota.ProRPM, minuteResetAt, minuteTotals.ProTokens, minuteTotals.ProCost, now)
		usage.GeminiFlashMinute = buildGeminiUsageProgress(minuteTotals.FlashRequests, quota.FlashRPM, minuteResetAt, minuteTotals.FlashTokens, minuteTotals.FlashCost, now)
REDACTED

	return usage, nil
REDACTED

// getAntigravityUsage 获取 Antigravity 账户额度
func (s *AccountUsageService) getAntigravityUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	if s.antigravityQuotaFetcher == nil || !s.antigravityQuotaFetcher.CanFetch(account) {
		now := time.Now()
		return &UsageInfo{UpdatedAt: &nowREDACTED, nil
REDACTED

	// 1. 检查缓存（10 分钟）
	if cached, ok := s.cache.antigravityCache.Load(account.ID); ok {
		if cache, ok := cached.(*antigravityUsageCache); ok && time.Since(cache.timestamp) < apiCacheTTL {
			// 重新计算 RemainingSeconds
			usage := cache.usageInfo
			if usage.FiveHour != nil && usage.FiveHour.ResetsAt != nil {
				usage.FiveHour.RemainingSeconds = int(time.Until(*usage.FiveHour.ResetsAt).Seconds())
		REDACTED
			return usage, nil
	REDACTED
REDACTED

	// 2. 获取代理 URL
	proxyURL := s.antigravityQuotaFetcher.GetProxyURL(ctx, account)

	// 3. 调用 API 获取额度
	result, err := s.antigravityQuotaFetcher.FetchQuota(ctx, account, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("fetch antigravity quota failed: %w", err)
REDACTED

	// 4. 缓存结果
	s.cache.antigravityCache.Store(account.ID, &antigravityUsageCache{
		usageInfo: result.UsageInfo,
		timestamp: time.Now(),
REDACTED)

	return result.UsageInfo, nil
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
	if cached, ok := s.cache.windowStatsCache.Load(account.ID); ok {
		if cache, ok := cached.(*windowStatsCache); ok && time.Since(cache.timestamp) < windowStatsCacheTTL {
			windowStats = cache.stats
	REDACTED
REDACTED

	// 如果没有缓存，从数据库查询
	if windowStats == nil {
		// 使用统一的窗口开始时间计算逻辑（考虑窗口过期情况）
		startTime := account.GetCurrentWindowStartTime()

		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
		if err != nil {
			log.Printf("Failed to get window stats for account %d: %v", account.ID, err)
			return
	REDACTED

		windowStats = &WindowStats{
			Requests:     stats.Requests,
			Tokens:       stats.Tokens,
			Cost:         stats.Cost,
			StandardCost: stats.StandardCost,
			UserCost:     stats.UserCost,
	REDACTED

		// 缓存窗口统计（1 分钟）
		s.cache.windowStatsCache.Store(account.ID, &windowStatsCache{
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
		Requests:     stats.Requests,
		Tokens:       stats.Tokens,
		Cost:         stats.Cost,
		StandardCost: stats.StandardCost,
		UserCost:     stats.UserCost,
REDACTED, nil
REDACTED

// GetTodayStatsBatch 批量获取账号今日统计，优先走批量 SQL，失败时回退单账号查询。
func (s *AccountUsageService) GetTodayStatsBatch(ctx context.Context, accountIDs []int64) (map[int64]*WindowStats, error) {
	uniqueIDs := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{REDACTED, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
	REDACTED
		if _, exists := seen[accountID]; exists {
			continue
	REDACTED
		seen[accountID] = struct{REDACTED{REDACTED
		uniqueIDs = append(uniqueIDs, accountID)
REDACTED

	result := make(map[int64]*WindowStats, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return result, nil
REDACTED

	startTime := timezone.Today()
	if batchReader, ok := s.usageLogRepo.(accountWindowStatsBatchReader); ok {
		statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, uniqueIDs, startTime)
		if err == nil {
			for _, accountID := range uniqueIDs {
				result[accountID] = windowStatsFromAccountStats(statsByAccount[accountID])
		REDACTED
			return result, nil
	REDACTED
REDACTED

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, accountID := range uniqueIDs {
		id := accountID
		g.Go(func() error {
			stats, err := s.usageLogRepo.GetAccountWindowStats(gctx, id, startTime)
			if err != nil {
				return nil
		REDACTED
			mu.Lock()
			result[id] = windowStatsFromAccountStats(stats)
			mu.Unlock()
			return nil
	REDACTED)
REDACTED

	_ = g.Wait()

	for _, accountID := range uniqueIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = &WindowStats{REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

func windowStatsFromAccountStats(stats *usagestats.AccountStats) *WindowStats {
	if stats == nil {
		return &WindowStats{REDACTED
REDACTED
	return &WindowStats{
		Requests:     stats.Requests,
		Tokens:       stats.Tokens,
		Cost:         stats.Cost,
		StandardCost: stats.StandardCost,
		UserCost:     stats.UserCost,
REDACTED
REDACTED

func hasMeaningfulWindowStats(stats *WindowStats) bool {
	if stats == nil {
		return false
REDACTED
	return stats.Requests > 0 || stats.Tokens > 0 || stats.Cost > 0 || stats.StandardCost > 0 || stats.UserCost > 0
REDACTED

func buildCodexUsageProgressFromExtra(extra map[string]any, window string, now time.Time) *UsageProgress {
	if len(extra) == 0 {
		return nil
REDACTED

	var (
		usedPercentKey string
		resetAfterKey  string
		resetAtKey     string
	)

	switch window {
	case "5h":
		usedPercentKey = "codex_5h_used_percent"
		resetAfterKey = "codex_5h_reset_after_seconds"
		resetAtKey = "codex_5h_reset_at"
	case "7d":
		usedPercentKey = "codex_7d_used_percent"
		resetAfterKey = "codex_7d_reset_after_seconds"
		resetAtKey = "codex_7d_reset_at"
	default:
		return nil
REDACTED

	usedRaw, ok := extra[usedPercentKey]
	if !ok {
		return nil
REDACTED

	progress := &UsageProgress{Utilization: parseExtraFloat64(usedRaw)REDACTED
	if resetAtRaw, ok := extra[resetAtKey]; ok {
		if resetAt, err := parseTime(fmt.Sprint(resetAtRaw)); err == nil {
			progress.ResetsAt = &resetAt
			progress.RemainingSeconds = int(time.Until(resetAt).Seconds())
			if progress.RemainingSeconds < 0 {
				progress.RemainingSeconds = 0
		REDACTED
	REDACTED
REDACTED
	if progress.ResetsAt == nil {
		if resetAfterSeconds := parseExtraInt(extra[resetAfterKey]); resetAfterSeconds > 0 {
			base := now
			if updatedAtRaw, ok := extra["codex_usage_updated_at"]; ok {
				if updatedAt, err := parseTime(fmt.Sprint(updatedAtRaw)); err == nil {
					base = updatedAt
			REDACTED
		REDACTED
			resetAt := base.Add(time.Duration(resetAfterSeconds) * time.Second)
			progress.ResetsAt = &resetAt
			progress.RemainingSeconds = int(time.Until(resetAt).Seconds())
			if progress.RemainingSeconds < 0 {
				progress.RemainingSeconds = 0
		REDACTED
	REDACTED
REDACTED

	return progress
REDACTED

func (s *AccountUsageService) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	stats, err := s.usageLogRepo.GetAccountUsageStats(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get account usage stats failed: %w", err)
REDACTED
	return stats, nil
REDACTED

// fetchOAuthUsageRaw 从 Anthropic API 获取原始响应（不构建 UsageInfo）
// 如果账号开启了 TLS 指纹，则使用 TLS 指纹伪装
// 如果有缓存的 Fingerprint，则使用缓存的 User-Agent 等信息
func (s *AccountUsageService) fetchOAuthUsageRaw(ctx context.Context, account *Account) (*ClaudeUsageResponse, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 构建完整的选项
	opts := &ClaudeUsageFetchOptions{
		AccessToken:          accessToken,
		ProxyURL:             proxyURL,
		AccountID:            account.ID,
		EnableTLSFingerprint: account.IsTLSFingerprintEnabled(),
REDACTED

	// 尝试获取缓存的 Fingerprint（包含 User-Agent 等信息）
	if s.identityCache != nil {
		if fp, err := s.identityCache.GetFingerprint(ctx, account.ID); err == nil && fp != nil {
			opts.Fingerprint = fp
	REDACTED
REDACTED

	return s.usageFetcher.FetchUsageWithOptions(ctx, opts)
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

func (s *AccountUsageService) tryClearRecoverableAccountError(ctx context.Context, account *Account) {
	if account == nil || account.Status != StatusError {
		return
REDACTED

	msg := strings.ToLower(strings.TrimSpace(account.ErrorMessage))
	if msg == "" {
		return
REDACTED

	if !strings.Contains(msg, "token refresh failed") &&
		!strings.Contains(msg, "invalid_client") &&
		!strings.Contains(msg, "missing_project_id") &&
		!strings.Contains(msg, "unauthenticated") {
		return
REDACTED

	if err := s.accountRepo.ClearError(ctx, account.ID); err != nil {
		log.Printf("[usage] failed to clear recoverable account error for account %d: %v", account.ID, err)
		return
REDACTED

	account.Status = StatusActive
	account.ErrorMessage = ""
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

		// 优先使用响应头中存储的真实 utilization 值（0-1 小数，转为 0-100 百分比）
		var utilization float64
		var found bool
		if stored, ok := account.Extra["session_window_utilization"]; ok {
			switch v := stored.(type) {
			case float64:
				utilization = v * 100
				found = true
			case json.Number:
				if f, err := v.Float64(); err == nil {
					utilization = f * 100
					found = true
			REDACTED
		REDACTED
	REDACTED

		// 如果没有存储的 utilization，回退到状态估算
		if !found {
			switch account.SessionWindowStatus {
			case "rejected":
				utilization = 100.0
			case "allowed_warning":
				utilization = 80.0
		REDACTED
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
	// limit <= 0 means "no local quota window" (unknown or unlimited).
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
		UsedRequests:     used,
		LimitRequests:    limit,
		WindowStats: &WindowStats{
			Requests: used,
			Tokens:   tokens,
			Cost:     cost,
	REDACTED,
REDACTED
REDACTED

// GetAccountWindowStats 获取账号在指定时间窗口内的使用统计
// 用于账号列表页面显示当前窗口费用
func (s *AccountUsageService) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return s.usageLogRepo.GetAccountWindowStats(ctx, accountID, startTime)
REDACTED
