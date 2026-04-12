package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

// RateLimitService 处理限流和过载状态管理
type RateLimitService struct {
	accountRepo           AccountRepository
	usageRepo             UsageLogRepository
	cfg                   *config.Config
	geminiQuotaService    *GeminiQuotaService
	tempUnschedCache      TempUnschedCache
	timeoutCounterCache   TimeoutCounterCache
	settingService        *SettingService
	tokenCacheInvalidator TokenCacheInvalidator
	usageCacheMu          sync.RWMutex
	usageCache            map[int64]*geminiUsageCacheEntry
}

// SuccessfulTestRecoveryResult 表示测试成功后恢复了哪些运行时状态。
type SuccessfulTestRecoveryResult struct {
	ClearedError     bool
	ClearedRateLimit bool
}

// AccountRecoveryOptions 控制账号恢复时的附加行为。
type AccountRecoveryOptions struct {
	InvalidateToken bool
}

type geminiUsageCacheEntry struct {
	windowStart time.Time
	cachedAt    time.Time
	totals      GeminiUsageTotals
}

type geminiUsageTotalsBatchProvider interface {
	GetGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, startTime, endTime time.Time) (map[int64]GeminiUsageTotals, error)
}

const geminiPrecheckCacheTTL = time.Minute

// NewRateLimitService 创建RateLimitService实例
func NewRateLimitService(accountRepo AccountRepository, usageRepo UsageLogRepository, cfg *config.Config, geminiQuotaService *GeminiQuotaService, tempUnschedCache TempUnschedCache) *RateLimitService {
	return &RateLimitService{
		accountRepo:        accountRepo,
		usageRepo:          usageRepo,
		cfg:                cfg,
		geminiQuotaService: geminiQuotaService,
		tempUnschedCache:   tempUnschedCache,
		usageCache:         make(map[int64]*geminiUsageCacheEntry),
	}
}

// SetTimeoutCounterCache 设置超时计数器缓存（可选依赖）
func (s *RateLimitService) SetTimeoutCounterCache(cache TimeoutCounterCache) {
	s.timeoutCounterCache = cache
}

// SetSettingService 设置系统设置服务（可选依赖）
func (s *RateLimitService) SetSettingService(settingService *SettingService) {
	s.settingService = settingService
}

// SetTokenCacheInvalidator 设置 token 缓存清理器（可选依赖）
func (s *RateLimitService) SetTokenCacheInvalidator(invalidator TokenCacheInvalidator) {
	s.tokenCacheInvalidator = invalidator
}

// ErrorPolicyResult 表示错误策略检查的结果
type ErrorPolicyResult int

const (
	ErrorPolicyNone            ErrorPolicyResult = iota // 未命中任何策略，继续默认逻辑
	ErrorPolicySkipped                                  // 自定义错误码开启但未命中，跳过处理
	ErrorPolicyMatched                                  // 自定义错误码命中，应停止调度
	ErrorPolicyTempUnscheduled                          // 临时不可调度规则命中
)

// CheckErrorPolicy 检查自定义错误码和临时不可调度规则。
// 自定义错误码开启时覆盖后续所有逻辑（包括临时不可调度）。
func (s *RateLimitService) CheckErrorPolicy(ctx context.Context, account *Account, statusCode int, responseBody []byte) ErrorPolicyResult {
	if account.IsCustomErrorCodesEnabled() {
		if account.ShouldHandleErrorCode(statusCode) {
			return ErrorPolicyMatched
		}
		slog.Info("account_error_code_skipped", "account_id", account.ID, "status_code", statusCode)
		return ErrorPolicySkipped
	}
	if account.IsPoolMode() {
		return ErrorPolicySkipped
	}
	if s.tryTempUnschedulable(ctx, account, statusCode, responseBody) {
		return ErrorPolicyTempUnscheduled
	}
	return ErrorPolicyNone
}

// HandleUpstreamError 处理上游错误响应，标记账号状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) (shouldDisable bool) {
	customErrorCodesEnabled := account.IsCustomErrorCodesEnabled()

	// 池模式默认不标记本地账号状态；仅当用户显式配置自定义错误码时按本地策略处理。
	if account.IsPoolMode() && !customErrorCodesEnabled {
		slog.Info("pool_mode_error_skipped", "account_id", account.ID, "status_code", statusCode)
		return false
	}

	// apikey 类型账号：检查自定义错误码配置
	// 如果启用且错误码不在列表中，则不处理（不停止调度、不标记限流/过载）
	if !account.ShouldHandleErrorCode(statusCode) {
		slog.Info("account_error_code_skipped", "account_id", account.ID, "status_code", statusCode)
		return false
	}

	// 先尝试临时不可调度规则（401除外）
	// 如果匹配成功，直接返回，不执行后续禁用逻辑
	if statusCode != 401 {
		if s.tryTempUnschedulable(ctx, account, statusCode, responseBody) {
			return true
		}
	}

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if upstreamMsg != "" {
		upstreamMsg = truncateForLog([]byte(upstreamMsg), 512)
	}

	switch statusCode {
	case 400:
		// "organization has been disabled" → 永久禁用
		if strings.Contains(strings.ToLower(upstreamMsg), "organization has been disabled") {
			msg := "Organization disabled (400): " + upstreamMsg
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
		} else if account.Platform == PlatformAnthropic && strings.Contains(strings.ToLower(upstreamMsg), "credit balance") {
			// Anthropic API key 余额不足（语义等同 402），停止调度
			msg := "Credit balance exhausted (400): " + upstreamMsg
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
		}
		// 其他 400 错误（如参数问题）不处理，不禁用账号
	case 401:
		// OpenAI: token_invalidated / token_revoked 表示 token 被永久作废（非过期），直接标记 error
		openai401Code := extractUpstreamErrorCode(responseBody)
		if account.Platform == PlatformOpenAI && (openai401Code == "token_invalidated" || openai401Code == "token_revoked") {
			msg := "Token revoked (401): account authentication permanently revoked"
			if upstreamMsg != "" {
				msg = "Token revoked (401): " + upstreamMsg
			}
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
			break
		}
		// OpenAI: {"detail":"Unauthorized"} 表示 token 完全无效（非标准 OpenAI 错误格式），直接标记 error
		if account.Platform == PlatformOpenAI && gjson.GetBytes(responseBody, "detail").String() == "Unauthorized" {
			msg := "Unauthorized (401): account authentication failed permanently"
			if upstreamMsg != "" {
				msg = "Unauthorized (401): " + upstreamMsg
			}
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
			break
		}
		// OAuth 账号在 401 错误时临时不可调度（给 token 刷新窗口）；非 OAuth 账号保持原有 SetError 行为。
		// Antigravity 除外：其 401 由 applyErrorPolicy 的 temp_unschedulable_rules 自行控制。
		if account.Type == AccountTypeOAuth && account.Platform != PlatformAntigravity {
			// 1. 失效缓存
			if s.tokenCacheInvalidator != nil {
				if err := s.tokenCacheInvalidator.InvalidateToken(ctx, account); err != nil {
					slog.Warn("oauth_401_invalidate_cache_failed", "account_id", account.ID, "error", err)
				}
			}
			// 2. 设置 expires_at 为当前时间，强制下次请求刷新 token
			if account.Credentials == nil {
				account.Credentials = make(map[string]any)
			}
			account.Credentials["expires_at"] = time.Now().Format(time.RFC3339)
			if err := persistAccountCredentials(ctx, s.accountRepo, account, account.Credentials); err != nil {
				slog.Warn("oauth_401_force_refresh_update_failed", "account_id", account.ID, "error", err)
			} else {
				slog.Info("oauth_401_force_refresh_set", "account_id", account.ID, "platform", account.Platform)
			}
			// 3. 临时不可调度，替代 SetError（保持 status=active 让刷新服务能拾取）
			msg := "Authentication failed (401): invalid or expired credentials"
			if upstreamMsg != "" {
				msg = "OAuth 401: " + upstreamMsg
			}
			cooldownMinutes := s.cfg.RateLimit.OAuth401CooldownMinutes
			if cooldownMinutes <= 0 {
				cooldownMinutes = 10
			}
			until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
			if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, msg); err != nil {
				slog.Warn("oauth_401_set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
			}
			shouldDisable = true
		} else {
			// 非 OAuth / Antigravity OAuth：保持 SetError 行为
			msg := "Authentication failed (401): invalid or expired credentials"
			if upstreamMsg != "" {
				msg = "Authentication failed (401): " + upstreamMsg
			}
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
		}
	case 402:
		// OpenAI: deactivated_workspace 表示工作区已停用，直接标记 error
		if account.Platform == PlatformOpenAI && gjson.GetBytes(responseBody, "detail.code").String() == "deactivated_workspace" {
			msg := "Workspace deactivated (402): workspace has been deactivated"
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
			break
		}
		// 支付要求：余额不足或计费问题，停止调度
		msg := "Payment required (402): insufficient balance or billing issue"
		if upstreamMsg != "" {
			msg = "Payment required (402): " + upstreamMsg
		}
		s.handleAuthError(ctx, account, msg)
		shouldDisable = true
	case 403:
		logger.LegacyPrintf(
			"service.ratelimit",
			"[HandleUpstreamErrorRaw] account_id=%d platform=%s type=%s status=403 request_id=%s cf_ray=%s upstream_msg=%s raw_body=%s",
			account.ID,
			account.Platform,
			account.Type,
			strings.TrimSpace(headers.Get("x-request-id")),
			strings.TrimSpace(headers.Get("cf-ray")),
			upstreamMsg,
			truncateForLog(responseBody, 1024),
		)
		shouldDisable = s.handle403(ctx, account, upstreamMsg, responseBody)
	case 429:
		s.handle429(ctx, account, headers, responseBody)
		shouldDisable = false
	case 529:
		s.handle529(ctx, account)
		shouldDisable = false
	default:
		// 自定义错误码启用时：在列表中的错误码都应该停止调度
		if customErrorCodesEnabled {
			msg := "Custom error code triggered"
			if upstreamMsg != "" {
				msg = upstreamMsg
			}
			s.handleCustomErrorCode(ctx, account, statusCode, msg)
			shouldDisable = true
		} else if statusCode >= 500 {
			// 未启用自定义错误码时：仅记录5xx错误
			slog.Warn("account_upstream_error", "account_id", account.ID, "status_code", statusCode)
			shouldDisable = false
		}
	}

	return shouldDisable
}

// PreCheckUsage proactively checks local quota before dispatching a request.
// Returns false when the account should be skipped.
func (s *RateLimitService) PreCheckUsage(ctx context.Context, account *Account, requestedModel string) (bool, error) {
	if account == nil || account.Platform != PlatformGemini {
		return true, nil
	}
	if s.usageRepo == nil || s.geminiQuotaService == nil {
		return true, nil
	}

	quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
	if !ok {
		return true, nil
	}

	now := time.Now()
	modelClass := geminiModelClassFromName(requestedModel)

	// 1) Daily quota precheck (RPD; resets at PST midnight)
	{
		var limit int64
		if quota.SharedRPD > 0 {
			limit = quota.SharedRPD
		} else {
			switch modelClass {
			case geminiModelFlash:
				limit = quota.FlashRPD
			default:
				limit = quota.ProRPD
			}
		}

		if limit > 0 {
			start := geminiDailyWindowStart(now)
			totals, ok := s.getGeminiUsageTotals(account.ID, start, now)
			if !ok {
				stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil, nil, nil)
				if err != nil {
					return true, err
				}
				totals = geminiAggregateUsage(stats)
				s.setGeminiUsageTotals(account.ID, start, now, totals)
			}

			var used int64
			if quota.SharedRPD > 0 {
				used = totals.ProRequests + totals.FlashRequests
			} else {
				switch modelClass {
				case geminiModelFlash:
					used = totals.FlashRequests
				default:
					used = totals.ProRequests
				}
			}

			if used >= limit {
				resetAt := geminiDailyResetTime(now)
				// NOTE:
				// - This is a local precheck to reduce upstream 429s.
				// - Do NOT mark the account as rate-limited here; rate_limit_reset_at should reflect real upstream 429s.
				slog.Info("gemini_precheck_daily_quota_reached", "account_id", account.ID, "used", used, "limit", limit, "reset_at", resetAt)
				return false, nil
			}
		}
	}

	// 2) Minute quota precheck (RPM; fixed window current minute)
	{
		var limit int64
		if quota.SharedRPM > 0 {
			limit = quota.SharedRPM
		} else {
			switch modelClass {
			case geminiModelFlash:
				limit = quota.FlashRPM
			default:
				limit = quota.ProRPM
			}
		}

		if limit > 0 {
			start := now.Truncate(time.Minute)
			stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil, nil, nil)
			if err != nil {
				return true, err
			}
			totals := geminiAggregateUsage(stats)

			var used int64
			if quota.SharedRPM > 0 {
				used = totals.ProRequests + totals.FlashRequests
			} else {
				switch modelClass {
				case geminiModelFlash:
					used = totals.FlashRequests
				default:
					used = totals.ProRequests
				}
			}

			if used >= limit {
				resetAt := start.Add(time.Minute)
				// Do not persist "rate limited" status from local precheck. See note above.
				slog.Info("gemini_precheck_minute_quota_reached", "account_id", account.ID, "used", used, "limit", limit, "reset_at", resetAt)
				return false, nil
			}
		}
	}

	return true, nil
}

// PreCheckUsageBatch performs quota precheck for multiple accounts in one request.
// Returned map value=false means the account should be skipped.
func (s *RateLimitService) PreCheckUsageBatch(ctx context.Context, accounts []*Account, requestedModel string) (map[int64]bool, error) {
	result := make(map[int64]bool, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		result[account.ID] = true
	}

	if len(accounts) == 0 || requestedModel == "" {
		return result, nil
	}
	if s.usageRepo == nil || s.geminiQuotaService == nil {
		return result, nil
	}

	modelClass := geminiModelClassFromName(requestedModel)
	now := time.Now()
	dailyStart := geminiDailyWindowStart(now)
	minuteStart := now.Truncate(time.Minute)

	type quotaAccount struct {
		account *Account
		quota   GeminiQuota
	}
	quotaAccounts := make([]quotaAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Platform != PlatformGemini {
			continue
		}
		quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
		if !ok {
			continue
		}
		quotaAccounts = append(quotaAccounts, quotaAccount{
			account: account,
			quota:   quota,
		})
	}
	if len(quotaAccounts) == 0 {
		return result, nil
	}

	// 1) Daily precheck (cached + batch DB fallback)
	dailyTotalsByID := make(map[int64]GeminiUsageTotals, len(quotaAccounts))
	dailyMissIDs := make([]int64, 0, len(quotaAccounts))
	for _, item := range quotaAccounts {
		limit := geminiDailyLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
		}
		accountID := item.account.ID
		if totals, ok := s.getGeminiUsageTotals(accountID, dailyStart, now); ok {
			dailyTotalsByID[accountID] = totals
			continue
		}
		dailyMissIDs = append(dailyMissIDs, accountID)
	}
	if len(dailyMissIDs) > 0 {
		totalsBatch, err := s.getGeminiUsageTotalsBatch(ctx, dailyMissIDs, dailyStart, now)
		if err != nil {
			return result, err
		}
		for _, accountID := range dailyMissIDs {
			totals := totalsBatch[accountID]
			dailyTotalsByID[accountID] = totals
			s.setGeminiUsageTotals(accountID, dailyStart, now, totals)
		}
	}
	for _, item := range quotaAccounts {
		limit := geminiDailyLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
		}
		accountID := item.account.ID
		used := geminiUsedRequests(item.quota, modelClass, dailyTotalsByID[accountID], true)
		if used >= limit {
			resetAt := geminiDailyResetTime(now)
			slog.Info("gemini_precheck_daily_quota_reached_batch", "account_id", accountID, "used", used, "limit", limit, "reset_at", resetAt)
			result[accountID] = false
		}
	}

	// 2) Minute precheck (batch DB)
	minuteIDs := make([]int64, 0, len(quotaAccounts))
	for _, item := range quotaAccounts {
		accountID := item.account.ID
		if !result[accountID] {
			continue
		}
		if geminiMinuteLimit(item.quota, modelClass) <= 0 {
			continue
		}
		minuteIDs = append(minuteIDs, accountID)
	}
	if len(minuteIDs) == 0 {
		return result, nil
	}

	minuteTotalsByID, err := s.getGeminiUsageTotalsBatch(ctx, minuteIDs, minuteStart, now)
	if err != nil {
		return result, err
	}
	for _, item := range quotaAccounts {
		accountID := item.account.ID
		if !result[accountID] {
			continue
		}

		limit := geminiMinuteLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
		}

		used := geminiUsedRequests(item.quota, modelClass, minuteTotalsByID[accountID], false)
		if used >= limit {
			resetAt := minuteStart.Add(time.Minute)
			slog.Info("gemini_precheck_minute_quota_reached_batch", "account_id", accountID, "used", used, "limit", limit, "reset_at", resetAt)
			result[accountID] = false
		}
	}

	return result, nil
}

func (s *RateLimitService) getGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]GeminiUsageTotals, error) {
	result := make(map[int64]GeminiUsageTotals, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}

	ids := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if _, ok := seen[accountID]; ok {
			continue
		}
		seen[accountID] = struct{}{}
		ids = append(ids, accountID)
	}
	if len(ids) == 0 {
		return result, nil
	}

	if batchReader, ok := s.usageRepo.(geminiUsageTotalsBatchProvider); ok {
		stats, err := batchReader.GetGeminiUsageTotalsBatch(ctx, ids, start, end)
		if err != nil {
			return nil, err
		}
		for _, accountID := range ids {
			result[accountID] = stats[accountID]
		}
		return result, nil
	}

	for _, accountID := range ids {
		stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, end, 0, 0, accountID, 0, nil, nil, nil)
		if err != nil {
			return nil, err
		}
		result[accountID] = geminiAggregateUsage(stats)
	}
	return result, nil
}

func geminiDailyLimit(quota GeminiQuota, modelClass geminiModelClass) int64 {
	if quota.SharedRPD > 0 {
		return quota.SharedRPD
	}
	switch modelClass {
	case geminiModelFlash:
		return quota.FlashRPD
	default:
		return quota.ProRPD
	}
}

func geminiMinuteLimit(quota GeminiQuota, modelClass geminiModelClass) int64 {
	if quota.SharedRPM > 0 {
		return quota.SharedRPM
	}
	switch modelClass {
	case geminiModelFlash:
		return quota.FlashRPM
	default:
		return quota.ProRPM
	}
}

func geminiUsedRequests(quota GeminiQuota, modelClass geminiModelClass, totals GeminiUsageTotals, daily bool) int64 {
	if daily {
		if quota.SharedRPD > 0 {
			return totals.ProRequests + totals.FlashRequests
		}
	} else {
		if quota.SharedRPM > 0 {
			return totals.ProRequests + totals.FlashRequests
		}
	}
	switch modelClass {
	case geminiModelFlash:
		return totals.FlashRequests
	default:
		return totals.ProRequests
	}
}

func (s *RateLimitService) getGeminiUsageTotals(accountID int64, windowStart, now time.Time) (GeminiUsageTotals, bool) {
	s.usageCacheMu.RLock()
	defer s.usageCacheMu.RUnlock()

	if s.usageCache == nil {
		return GeminiUsageTotals{}, false
	}

	entry, ok := s.usageCache[accountID]
	if !ok || entry == nil {
		return GeminiUsageTotals{}, false
	}
	if !entry.windowStart.Equal(windowStart) {
		return GeminiUsageTotals{}, false
	}
	if now.Sub(entry.cachedAt) >= geminiPrecheckCacheTTL {
		return GeminiUsageTotals{}, false
	}
	return entry.totals, true
}

func (s *RateLimitService) setGeminiUsageTotals(accountID int64, windowStart, now time.Time, totals GeminiUsageTotals) {
	s.usageCacheMu.Lock()
	defer s.usageCacheMu.Unlock()
	if s.usageCache == nil {
		s.usageCache = make(map[int64]*geminiUsageCacheEntry)
	}
	s.usageCache[accountID] = &geminiUsageCacheEntry{
		windowStart: windowStart,
		cachedAt:    now,
		totals:      totals,
	}
}

// GeminiCooldown returns the fallback cooldown duration for Gemini 429s based on tier.
func (s *RateLimitService) GeminiCooldown(ctx context.Context, account *Account) time.Duration {
	if account == nil {
		return 5 * time.Minute
	}
	if s.geminiQuotaService == nil {
		return 5 * time.Minute
	}
	return s.geminiQuotaService.CooldownForAccount(ctx, account)
}

// handleAuthError 处理认证类错误(401/403)，停止账号调度
func (s *RateLimitService) handleAuthError(ctx context.Context, account *Account, errorMsg string) {
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		slog.Warn("account_set_error_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Warn("account_disabled_auth_error", "account_id", account.ID, "error", errorMsg)
}

// handle403 处理 403 Forbidden 错误
// Antigravity 平台区分 validation/violation/generic 三种类型，均 SetError 永久禁用；
// 其他平台保持原有 SetError 行为。
func (s *RateLimitService) handle403(ctx context.Context, account *Account, upstreamMsg string, responseBody []byte) (shouldDisable bool) {
	if account.Platform == PlatformAntigravity {
		return s.handleAntigravity403(ctx, account, upstreamMsg, responseBody)
	}
	// 非 Antigravity 平台：保持原有行为
	msg := "Access forbidden (403): account may be suspended or lack permissions"
	if upstreamMsg != "" {
		msg = "Access forbidden (403): " + upstreamMsg
	}
	s.handleAuthError(ctx, account, msg)
	return true
}

// handleAntigravity403 处理 Antigravity 平台的 403 错误
// validation（需要验证）→ 永久 SetError（需人工去 Google 验证后恢复）
// violation（违规封号）→ 永久 SetError（需人工处理）
// generic（通用禁止）→ 永久 SetError
func (s *RateLimitService) handleAntigravity403(ctx context.Context, account *Account, upstreamMsg string, responseBody []byte) (shouldDisable bool) {
	fbType := classifyForbiddenType(string(responseBody))

	switch fbType {
	case forbiddenTypeValidation:
		// VALIDATION_REQUIRED: 永久禁用，需人工去 Google 验证后手动恢复
		msg := "Validation required (403): account needs Google verification"
		if upstreamMsg != "" {
			msg = "Validation required (403): " + upstreamMsg
		}
		if validationURL := extractValidationURL(string(responseBody)); validationURL != "" {
			msg += " | validation_url: " + validationURL
		}
		s.handleAuthError(ctx, account, msg)
		return true

	case forbiddenTypeViolation:
		// 违规封号: 永久禁用，需人工处理
		msg := "Account violation (403): terms of service violation"
		if upstreamMsg != "" {
			msg = "Account violation (403): " + upstreamMsg
		}
		s.handleAuthError(ctx, account, msg)
		return true

	default:
		// 通用 403: 保持原有行为
		msg := "Access forbidden (403): account may be suspended or lack permissions"
		if upstreamMsg != "" {
			msg = "Access forbidden (403): " + upstreamMsg
		}
		s.handleAuthError(ctx, account, msg)
		return true
	}
}

// handleCustomErrorCode 处理自定义错误码，停止账号调度
func (s *RateLimitService) handleCustomErrorCode(ctx context.Context, account *Account, statusCode int, errorMsg string) {
	msg := "Custom error code " + strconv.Itoa(statusCode) + ": " + errorMsg
	if err := s.accountRepo.SetError(ctx, account.ID, msg); err != nil {
		slog.Warn("account_set_error_failed", "account_id", account.ID, "status_code", statusCode, "error", err)
		return
	}
	slog.Warn("account_disabled_custom_error", "account_id", account.ID, "status_code", statusCode, "error", errorMsg)
}

// handle429 处理429限流错误
// 解析响应头获取重置时间，标记账号为限流状态
func (s *RateLimitService) handle429(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	// 1. OpenAI 平台：优先尝试解析 x-codex-* 响应头（用于 rate_limit_exceeded）
	if account.Platform == PlatformOpenAI {
		s.persistOpenAICodexSnapshot(ctx, account, headers)
		if resetAt := s.calculateOpenAI429ResetTime(headers); resetAt != nil {
			if err := s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt); err != nil {
				slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
				return
			}
			slog.Info("openai_account_rate_limited", "account_id", account.ID, "reset_at", *resetAt)
			return
		}
	}

	// 2. Anthropic 平台：尝试解析 per-window 头（5h / 7d），选择实际触发的窗口
	if result := calculateAnthropic429ResetTime(headers); result != nil {
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, result.resetAt); err != nil {
			slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
			return
		}

		// 更新 session window：优先使用 5h-reset 头精确计算，否则从 resetAt 反推
		windowEnd := result.resetAt
		if result.fiveHourReset != nil {
			windowEnd = *result.fiveHourReset
		}
		windowStart := windowEnd.Add(-5 * time.Hour)
		if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
			slog.Warn("rate_limit_update_session_window_failed", "account_id", account.ID, "error", err)
		}

		slog.Info("anthropic_account_rate_limited", "account_id", account.ID, "reset_at", result.resetAt, "reset_in", time.Until(result.resetAt).Truncate(time.Second))
		return
	}

	// 3. 尝试从响应头解析重置时间（Anthropic 聚合头，向后兼容）
	resetTimestamp := headers.Get("anthropic-ratelimit-unified-reset")

	// 4. 如果响应头没有，尝试从响应体解析（OpenAI usage_limit_reached, Gemini）
	if resetTimestamp == "" {
		switch account.Platform {
		case PlatformOpenAI:
			// 尝试解析 OpenAI 的 usage_limit_reached 错误
			if resetAt := parseOpenAIRateLimitResetTime(responseBody); resetAt != nil {
				resetTime := time.Unix(*resetAt, 0)
				if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetTime); err != nil {
					slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
					return
				}
				slog.Info("account_rate_limited", "account_id", account.ID, "platform", account.Platform, "reset_at", resetTime, "reset_in", time.Until(resetTime).Truncate(time.Second))
				return
			}
		case PlatformGemini, PlatformAntigravity:
			// 尝试解析 Gemini 格式（用于其他平台）
			if resetAt := ParseGeminiRateLimitResetTime(responseBody); resetAt != nil {
				resetTime := time.Unix(*resetAt, 0)
				if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetTime); err != nil {
					slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
					return
				}
				slog.Info("account_rate_limited", "account_id", account.ID, "platform", account.Platform, "reset_at", resetTime, "reset_in", time.Until(resetTime).Truncate(time.Second))
				return
			}
		}

		// Anthropic 平台：没有限流重置时间的 429 可能是非真实限流（如 Extra usage required），
		// 不标记账号限流状态，直接透传错误给客户端
		if account.Platform == PlatformAnthropic {
			slog.Warn("rate_limit_429_no_reset_time_skipped",
				"account_id", account.ID,
				"platform", account.Platform,
				"reason", "no rate limit reset time in headers, likely not a real rate limit")
			return
		}

		// 其他平台：没有重置时间，使用默认5分钟
		resetAt := time.Now().Add(5 * time.Minute)
		slog.Warn("rate_limit_no_reset_time", "account_id", account.ID, "platform", account.Platform, "using_default", "5m")
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
		}
		return
	}

	// 解析Unix时间戳
	ts, err := strconv.ParseInt(resetTimestamp, 10, 64)
	if err != nil {
		slog.Warn("rate_limit_reset_parse_failed", "reset_timestamp", resetTimestamp, "error", err)
		resetAt := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
		}
		return
	}

	resetAt := time.Unix(ts, 0)

	// 标记限流状态
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
		return
	}

	// 根据重置时间反推5h窗口
	windowEnd := resetAt
	windowStart := resetAt.Add(-5 * time.Hour)
	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
		slog.Warn("rate_limit_update_session_window_failed", "account_id", account.ID, "error", err)
	}

	slog.Info("account_rate_limited", "account_id", account.ID, "reset_at", resetAt)
}

// calculateOpenAI429ResetTime 从 OpenAI 429 响应头计算正确的重置时间
// 返回 nil 表示无法从响应头中确定重置时间
func (s *RateLimitService) calculateOpenAI429ResetTime(headers http.Header) *time.Time {
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return nil
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		return nil
	}

	now := time.Now()

	// 判断哪个限制被触发（used_percent >= 100）
	is7dExhausted := normalized.Used7dPercent != nil && *normalized.Used7dPercent >= 100
	is5hExhausted := normalized.Used5hPercent != nil && *normalized.Used5hPercent >= 100

	// 优先使用被触发限制的重置时间
	if is7dExhausted && normalized.Reset7dSeconds != nil {
		resetAt := now.Add(time.Duration(*normalized.Reset7dSeconds) * time.Second)
		slog.Info("openai_429_7d_limit_exhausted", "reset_after_seconds", *normalized.Reset7dSeconds, "reset_at", resetAt)
		return &resetAt
	}
	if is5hExhausted && normalized.Reset5hSeconds != nil {
		resetAt := now.Add(time.Duration(*normalized.Reset5hSeconds) * time.Second)
		slog.Info("openai_429_5h_limit_exhausted", "reset_after_seconds", *normalized.Reset5hSeconds, "reset_at", resetAt)
		return &resetAt
	}

	// 都未达到100%但收到429，使用较长的重置时间
	var maxResetSecs int
	if normalized.Reset7dSeconds != nil && *normalized.Reset7dSeconds > maxResetSecs {
		maxResetSecs = *normalized.Reset7dSeconds
	}
	if normalized.Reset5hSeconds != nil && *normalized.Reset5hSeconds > maxResetSecs {
		maxResetSecs = *normalized.Reset5hSeconds
	}
	if maxResetSecs > 0 {
		resetAt := now.Add(time.Duration(maxResetSecs) * time.Second)
		slog.Info("openai_429_using_max_reset", "max_reset_seconds", maxResetSecs, "reset_at", resetAt)
		return &resetAt
	}

	return nil
}

// anthropic429Result holds the parsed Anthropic 429 rate-limit information.
type anthropic429Result struct {
	resetAt       time.Time  // The correct reset time to use for SetRateLimited
	fiveHourReset *time.Time // 5h window reset timestamp (for session window calculation), nil if not available
}

// calculateAnthropic429ResetTime parses Anthropic's per-window rate-limit headers
// to determine which window (5h or 7d) actually triggered the 429.
//
// Headers used:
//   - anthropic-ratelimit-unified-5h-utilization / anthropic-ratelimit-unified-5h-surpassed-threshold
//   - anthropic-ratelimit-unified-5h-reset
//   - anthropic-ratelimit-unified-7d-utilization / anthropic-ratelimit-unified-7d-surpassed-threshold
//   - anthropic-ratelimit-unified-7d-reset
//
// Returns nil when the per-window headers are absent (caller should fall back to
// the aggregated anthropic-ratelimit-unified-reset header).
func calculateAnthropic429ResetTime(headers http.Header) *anthropic429Result {
	reset5hStr := headers.Get("anthropic-ratelimit-unified-5h-reset")
	reset7dStr := headers.Get("anthropic-ratelimit-unified-7d-reset")

	if reset5hStr == "" && reset7dStr == "" {
		return nil
	}

	var reset5h, reset7d *time.Time
	if ts, err := strconv.ParseInt(reset5hStr, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		reset5h = &t
	}
	if ts, err := strconv.ParseInt(reset7dStr, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		reset7d = &t
	}

	is5hExceeded := isAnthropicWindowExceeded(headers, "5h")
	is7dExceeded := isAnthropicWindowExceeded(headers, "7d")

	slog.Info("anthropic_429_window_analysis",
		"is_5h_exceeded", is5hExceeded,
		"is_7d_exceeded", is7dExceeded,
		"reset_5h", reset5hStr,
		"reset_7d", reset7dStr,
	)

	// Select the correct reset time based on which window(s) are exceeded.
	var chosen *time.Time
	switch {
	case is5hExceeded && is7dExceeded:
		// Both exceeded → prefer 7d (longer cooldown), fall back to 5h
		chosen = reset7d
		if chosen == nil {
			chosen = reset5h
		}
	case is5hExceeded:
		chosen = reset5h
	case is7dExceeded:
		chosen = reset7d
	default:
		// Neither flag clearly exceeded — pick the sooner reset as best guess
		chosen = pickSooner(reset5h, reset7d)
	}

	if chosen == nil {
		return nil
	}
	return &anthropic429Result{resetAt: *chosen, fiveHourReset: reset5h}
}

// isAnthropicWindowExceeded checks whether a given Anthropic rate-limit window
// (e.g. "5h" or "7d") has been exceeded, using utilization and surpassed-threshold headers.
func isAnthropicWindowExceeded(headers http.Header, window string) bool {
	prefix := "anthropic-ratelimit-unified-" + window + "-"

	// Check surpassed-threshold first (most explicit signal)
	if st := headers.Get(prefix + "surpassed-threshold"); strings.EqualFold(st, "true") {
		return true
	}

	// Fall back to utilization >= 1.0
	if utilStr := headers.Get(prefix + "utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil && util >= 1.0-1e-9 {
			// Use a small epsilon to handle floating point: treat 0.9999999... as >= 1.0
			return true
		}
	}

	return false
}

// pickSooner returns whichever of the two time pointers is earlier.
// If only one is non-nil, it is returned. If both are nil, returns nil.
func pickSooner(a, b *time.Time) *time.Time {
	switch {
	case a != nil && b != nil:
		if a.Before(*b) {
			return a
		}
		return b
	case a != nil:
		return a
	default:
		return b
	}
}

func (s *RateLimitService) persistOpenAICodexSnapshot(ctx context.Context, account *Account, headers http.Header) {
	if s == nil || s.accountRepo == nil || account == nil || headers == nil {
		return
	}
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return
	}
	updates := buildCodexUsageExtraUpdates(snapshot, time.Now())
	if len(updates) == 0 {
		return
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("openai_codex_snapshot_persist_failed", "account_id", account.ID, "error", err)
	}
}

// parseOpenAIRateLimitResetTime 解析 OpenAI 格式的 429 响应，返回重置时间的 Unix 时间戳
// OpenAI 的 usage_limit_reached 错误格式：
//
//	{
//	  "error": {
//	    "message": "The usage limit has been reached",
//	    "type": "usage_limit_reached",
//	    "resets_at": 1769404154,
//	    "resets_in_seconds": 133107
//	  }
//	}
func parseOpenAIRateLimitResetTime(body []byte) *int64 {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		return nil
	}

	// 检查是否为 usage_limit_reached 或 rate_limit_exceeded 类型
	errType, _ := errObj["type"].(string)
	if errType != "usage_limit_reached" && errType != "rate_limit_exceeded" {
		return nil
	}

	// 优先使用 resets_at（Unix 时间戳）
	if resetsAt, ok := errObj["resets_at"].(float64); ok {
		ts := int64(resetsAt)
		return &ts
	}
	if resetsAt, ok := errObj["resets_at"].(string); ok {
		if ts, err := strconv.ParseInt(resetsAt, 10, 64); err == nil {
			return &ts
		}
	}

	// 如果没有 resets_at，尝试使用 resets_in_seconds
	if resetsInSeconds, ok := errObj["resets_in_seconds"].(float64); ok {
		ts := time.Now().Unix() + int64(resetsInSeconds)
		return &ts
	}
	if resetsInSeconds, ok := errObj["resets_in_seconds"].(string); ok {
		if sec, err := strconv.ParseInt(resetsInSeconds, 10, 64); err == nil {
			ts := time.Now().Unix() + sec
			return &ts
		}
	}

	return nil
}

// handle529 处理529过载错误
// 根据配置决定是否暂停账号调度及冷却时长
func (s *RateLimitService) handle529(ctx context.Context, account *Account) {
	var settings *OverloadCooldownSettings
	if s.settingService != nil {
		var err error
		settings, err = s.settingService.GetOverloadCooldownSettings(ctx)
		if err != nil {
			slog.Warn("overload_settings_read_failed", "account_id", account.ID, "error", err)
			settings = nil
		}
	}
	// 回退到配置文件
	if settings == nil {
		cooldown := s.cfg.RateLimit.OverloadCooldownMinutes
		if cooldown <= 0 {
			cooldown = 10
		}
		settings = &OverloadCooldownSettings{Enabled: true, CooldownMinutes: cooldown}
	}

	if !settings.Enabled {
		slog.Info("account_529_ignored", "account_id", account.ID, "reason", "overload_cooldown_disabled")
		return
	}

	cooldownMinutes := settings.CooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = 10
	}

	until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
	if err := s.accountRepo.SetOverloaded(ctx, account.ID, until); err != nil {
		slog.Warn("overload_set_failed", "account_id", account.ID, "error", err)
		return
	}

	slog.Info("account_overloaded", "account_id", account.ID, "until", until)
}

// UpdateSessionWindow 从成功响应更新5h窗口状态
func (s *RateLimitService) UpdateSessionWindow(ctx context.Context, account *Account, headers http.Header) {
	status := headers.Get("anthropic-ratelimit-unified-5h-status")
	if status == "" {
		return
	}

	// 检查是否需要初始化时间窗口
	// 对于 Setup Token 账号，首次成功请求时需要预测时间窗口
	var windowStart, windowEnd *time.Time
	needInitWindow := account.SessionWindowEnd == nil || time.Now().After(*account.SessionWindowEnd)

	// 优先使用响应头中的真实重置时间（比预测更准确）
	if resetStr := headers.Get("anthropic-ratelimit-unified-5h-reset"); resetStr != "" {
		if ts, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			// 检测可能的毫秒时间戳（秒级约为 1e9，毫秒约为 1e12）
			if ts > 1e11 {
				slog.Warn("account_session_window_header_millis_detected", "account_id", account.ID, "raw_reset", resetStr)
				ts = ts / 1000
			}
			end := time.Unix(ts, 0)
			// 校验时间戳是否在合理范围内（不早于 5h 前，不晚于 7 天后）
			minAllowed := time.Now().Add(-5 * time.Hour)
			maxAllowed := time.Now().Add(7 * 24 * time.Hour)
			if end.Before(minAllowed) || end.After(maxAllowed) {
				slog.Warn("account_session_window_header_out_of_range", "account_id", account.ID, "raw_reset", resetStr, "parsed_end", end)
			} else if needInitWindow || account.SessionWindowEnd == nil || !end.Equal(*account.SessionWindowEnd) {
				// 窗口需要初始化，或者真实重置时间与已存储的不同，则更新
				start := end.Add(-5 * time.Hour)
				windowStart = &start
				windowEnd = &end
				slog.Info("account_session_window_from_header", "account_id", account.ID, "window_start", start, "window_end", end, "status", status)
			}
		} else {
			slog.Warn("account_session_window_header_parse_failed", "account_id", account.ID, "raw_reset", resetStr, "error", err)
		}
	}

	// 回退：如果没有真实重置时间且需要初始化窗口，使用预测
	if windowEnd == nil && needInitWindow && (status == "allowed" || status == "allowed_warning") {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		end := start.Add(5 * time.Hour)
		windowStart = &start
		windowEnd = &end
		slog.Info("account_session_window_initialized", "account_id", account.ID, "window_start", start, "window_end", end, "status", status)
	}

	// 窗口重置时清除旧的 utilization 和被动采样数据，避免残留上个窗口的数据
	if windowEnd != nil && needInitWindow {
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			"session_window_utilization":   nil,
			"passive_usage_7d_utilization": nil,
			"passive_usage_7d_reset":       nil,
			"passive_usage_sampled_at":     nil,
		})
	}

	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, windowStart, windowEnd, status); err != nil {
		slog.Warn("session_window_update_failed", "account_id", account.ID, "error", err)
	}

	// 被动采样：从响应头收集 5h + 7d utilization，合并为一次 DB 写入
	extraUpdates := make(map[string]any, 4)
	// 5h utilization（0-1 小数），供 estimateSetupTokenUsage 使用
	if utilStr := headers.Get("anthropic-ratelimit-unified-5h-utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil {
			extraUpdates["session_window_utilization"] = util
		}
	}
	// 7d utilization（0-1 小数）
	if utilStr := headers.Get("anthropic-ratelimit-unified-7d-utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil {
			extraUpdates["passive_usage_7d_utilization"] = util
		}
	}
	// 7d reset timestamp
	if resetStr := headers.Get("anthropic-ratelimit-unified-7d-reset"); resetStr != "" {
		if ts, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			if ts > 1e11 {
				ts = ts / 1000
			}
			extraUpdates["passive_usage_7d_reset"] = ts
		}
	}
	if len(extraUpdates) > 0 {
		extraUpdates["passive_usage_sampled_at"] = time.Now().UTC().Format(time.RFC3339)
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, extraUpdates); err != nil {
			slog.Warn("passive_usage_update_failed", "account_id", account.ID, "error", err)
		}
	}

	// 如果状态为allowed且之前有限流，说明窗口已重置，清除限流状态
	if status == "allowed" && account.IsRateLimited() {
		if err := s.ClearRateLimit(ctx, account.ID); err != nil {
			slog.Warn("rate_limit_clear_failed", "account_id", account.ID, "error", err)
		}
	}
}

// ClearRateLimit 清除账号的限流状态
func (s *RateLimitService) ClearRateLimit(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearRateLimit(ctx, accountID); err != nil {
		return err
	}
	if err := s.accountRepo.ClearAntigravityQuotaScopes(ctx, accountID); err != nil {
		return err
	}
	if err := s.accountRepo.ClearModelRateLimits(ctx, accountID); err != nil {
		return err
	}
	// 清除限流时一并清理临时不可调度状态，避免周限/窗口重置后仍被本地临时状态阻断。
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		return err
	}
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
			slog.Warn("temp_unsched_cache_delete_failed", "account_id", accountID, "error", err)
		}
	}
	return nil
}

// RecoverAccountState 按需恢复账号的可恢复运行时状态。
func (s *RateLimitService) RecoverAccountState(ctx context.Context, accountID int64, options AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	result := &SuccessfulTestRecoveryResult{}
	if account.Status == StatusError {
		if err := s.accountRepo.ClearError(ctx, accountID); err != nil {
			return nil, err
		}
		result.ClearedError = true
		if options.InvalidateToken && s.tokenCacheInvalidator != nil && account.IsOAuth() {
			if invalidateErr := s.tokenCacheInvalidator.InvalidateToken(ctx, account); invalidateErr != nil {
				slog.Warn("recover_account_state_invalidate_token_failed", "account_id", accountID, "error", invalidateErr)
			}
		}
	}

	if hasRecoverableRuntimeState(account) {
		if err := s.ClearRateLimit(ctx, accountID); err != nil {
			return nil, err
		}
		result.ClearedRateLimit = true
	}

	return result, nil
}

// RecoverAccountAfterSuccessfulTest 将一次成功测试视为正常请求，
// 按需恢复 error / rate-limit / overload / temp-unsched / model-rate-limit 等运行时状态。
func (s *RateLimitService) RecoverAccountAfterSuccessfulTest(ctx context.Context, accountID int64) (*SuccessfulTestRecoveryResult, error) {
	return s.RecoverAccountState(ctx, accountID, AccountRecoveryOptions{})
}

func (s *RateLimitService) ClearTempUnschedulable(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		return err
	}
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
			slog.Warn("temp_unsched_cache_delete_failed", "account_id", accountID, "error", err)
		}
	}
	// 同时清除模型级别限流
	if err := s.accountRepo.ClearModelRateLimits(ctx, accountID); err != nil {
		slog.Warn("clear_model_rate_limits_on_temp_unsched_reset_failed", "account_id", accountID, "error", err)
	}
	return nil
}

func hasRecoverableRuntimeState(account *Account) bool {
	if account == nil {
		return false
	}
	if account.RateLimitedAt != nil || account.RateLimitResetAt != nil || account.OverloadUntil != nil || account.TempUnschedulableUntil != nil {
		return true
	}
	if len(account.Extra) == 0 {
		return false
	}
	return hasNonEmptyMapValue(account.Extra, "model_rate_limits") ||
		hasNonEmptyMapValue(account.Extra, "antigravity_quota_scopes")
}

func hasNonEmptyMapValue(extra map[string]any, key string) bool {
	raw, ok := extra[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case map[string]any:
		return len(typed) > 0
	case map[string]string:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func (s *RateLimitService) GetTempUnschedStatus(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	now := time.Now().Unix()
	if s.tempUnschedCache != nil {
		state, err := s.tempUnschedCache.GetTempUnsched(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if state != nil && state.UntilUnix > now {
			return state, nil
		}
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.TempUnschedulableUntil == nil {
		return nil, nil
	}
	if account.TempUnschedulableUntil.Unix() <= now {
		return nil, nil
	}

	state := &TempUnschedState{
		UntilUnix: account.TempUnschedulableUntil.Unix(),
	}

	if account.TempUnschedulableReason != "" {
		var parsed TempUnschedState
		if err := json.Unmarshal([]byte(account.TempUnschedulableReason), &parsed); err == nil {
			if parsed.UntilUnix == 0 {
				parsed.UntilUnix = state.UntilUnix
			}
			state = &parsed
		} else {
			state.ErrorMessage = account.TempUnschedulableReason
		}
	}

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, accountID, state); err != nil {
			slog.Warn("temp_unsched_cache_set_failed", "account_id", accountID, "error", err)
		}
	}

	return state, nil
}

func (s *RateLimitService) HandleTempUnschedulable(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil {
		return false
	}
	if !account.ShouldHandleErrorCode(statusCode) {
		return false
	}
	return s.tryTempUnschedulable(ctx, account, statusCode, responseBody)
}

const tempUnschedBodyMaxBytes = 64 << 10
const tempUnschedMessageMaxBytes = 2048

func (s *RateLimitService) tryTempUnschedulable(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil {
		return false
	}
	if !account.IsTempUnschedulableEnabled() {
		return false
	}
	// 401 首次命中可临时不可调度（给 token 刷新窗口）；
	// 若历史上已因 401 进入过临时不可调度，则本次应升级为 error（返回 false 交由默认错误逻辑处理）。
	// Antigravity 跳过：其 401 由 applyErrorPolicy 的 temp_unschedulable_rules 自行控制，无需升级逻辑。
	if statusCode == http.StatusUnauthorized && account.Platform != PlatformAntigravity {
		reason := account.TempUnschedulableReason
		// 缓存可能没有 reason，从 DB 回退读取
		if reason == "" {
			if dbAcc, err := s.accountRepo.GetByID(ctx, account.ID); err == nil && dbAcc != nil {
				reason = dbAcc.TempUnschedulableReason
			}
		}
		if wasTempUnschedByStatusCode(reason, statusCode) {
			slog.Info("401_escalated_to_error", "account_id", account.ID,
				"reason", "previous temp-unschedulable was also 401")
			return false
		}
	}
	rules := account.GetTempUnschedulableRules()
	if len(rules) == 0 {
		return false
	}
	if statusCode <= 0 || len(responseBody) == 0 {
		return false
	}

	body := responseBody
	if len(body) > tempUnschedBodyMaxBytes {
		body = body[:tempUnschedBodyMaxBytes]
	}
	bodyLower := strings.ToLower(string(body))

	for idx, rule := range rules {
		if rule.ErrorCode != statusCode || len(rule.Keywords) == 0 {
			continue
		}
		matchedKeyword := matchTempUnschedKeyword(bodyLower, rule.Keywords)
		if matchedKeyword == "" {
			continue
		}

		if s.triggerTempUnschedulable(ctx, account, rule, idx, statusCode, matchedKeyword, responseBody) {
			return true
		}
	}

	return false
}

func wasTempUnschedByStatusCode(reason string, statusCode int) bool {
	if statusCode <= 0 {
		return false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
	}

	var state TempUnschedState
	if err := json.Unmarshal([]byte(reason), &state); err != nil {
		return false
	}
	return state.StatusCode == statusCode
}

func matchTempUnschedKeyword(bodyLower string, keywords []string) string {
	if bodyLower == "" {
		return ""
	}
	for _, keyword := range keywords {
		k := strings.TrimSpace(keyword)
		if k == "" {
			continue
		}
		if strings.Contains(bodyLower, strings.ToLower(k)) {
			return k
		}
	}
	return ""
}

func (s *RateLimitService) triggerTempUnschedulable(ctx context.Context, account *Account, rule TempUnschedulableRule, ruleIndex int, statusCode int, matchedKeyword string, responseBody []byte) bool {
	if account == nil {
		return false
	}
	if rule.DurationMinutes <= 0 {
		return false
	}

	now := time.Now()
	until := now.Add(time.Duration(rule.DurationMinutes) * time.Minute)

	state := &TempUnschedState{
		UntilUnix:       until.Unix(),
		TriggeredAtUnix: now.Unix(),
		StatusCode:      statusCode,
		MatchedKeyword:  matchedKeyword,
		RuleIndex:       ruleIndex,
		ErrorMessage:    truncateTempUnschedMessage(responseBody, tempUnschedMessageMaxBytes),
	}

	reason := ""
	if raw, err := json.Marshal(state); err == nil {
		reason = string(raw)
	}
	if reason == "" {
		reason = strings.TrimSpace(state.ErrorMessage)
	}

	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("temp_unsched_set_failed", "account_id", account.ID, "error", err)
		return false
	}

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			slog.Warn("temp_unsched_cache_set_failed", "account_id", account.ID, "error", err)
		}
	}

	slog.Info("account_temp_unschedulable", "account_id", account.ID, "until", until, "rule_index", ruleIndex, "status_code", statusCode)
	return true
}

func truncateTempUnschedMessage(body []byte, maxBytes int) string {
	if maxBytes <= 0 || len(body) == 0 {
		return ""
	}
	if len(body) > maxBytes {
		body = body[:maxBytes]
	}
	return strings.TrimSpace(string(body))
}

// HandleStreamTimeout 处理流数据超时
// 根据系统设置决定是否标记账户为临时不可调度或错误状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleStreamTimeout(ctx context.Context, account *Account, model string) bool {
	if account == nil {
		return false
	}

	// 获取系统设置
	if s.settingService == nil {
		slog.Warn("stream_timeout_setting_service_missing", "account_id", account.ID)
		return false
	}

	settings, err := s.settingService.GetStreamTimeoutSettings(ctx)
	if err != nil {
		slog.Warn("stream_timeout_get_settings_failed", "account_id", account.ID, "error", err)
		return false
	}

	if !settings.Enabled {
		return false
	}

	if settings.Action == StreamTimeoutActionNone {
		return false
	}

	// 增加超时计数
	var count int64 = 1
	if s.timeoutCounterCache != nil {
		count, err = s.timeoutCounterCache.IncrementTimeoutCount(ctx, account.ID, settings.ThresholdWindowMinutes)
		if err != nil {
			slog.Warn("stream_timeout_increment_count_failed", "account_id", account.ID, "error", err)
			// 继续处理，使用 count=1
			count = 1
		}
	}

	slog.Info("stream_timeout_count", "account_id", account.ID, "count", count, "threshold", settings.ThresholdCount, "window_minutes", settings.ThresholdWindowMinutes, "model", model)

	// 检查是否达到阈值
	if count < int64(settings.ThresholdCount) {
		return false
	}

	// 达到阈值，执行相应操作
	switch settings.Action {
	case StreamTimeoutActionTempUnsched:
		return s.triggerStreamTimeoutTempUnsched(ctx, account, settings, model)
	case StreamTimeoutActionError:
		return s.triggerStreamTimeoutError(ctx, account, model)
	default:
		return false
	}
}

// triggerStreamTimeoutTempUnsched 触发流超时临时不可调度
func (s *RateLimitService) triggerStreamTimeoutTempUnsched(ctx context.Context, account *Account, settings *StreamTimeoutSettings, model string) bool {
	now := time.Now()
	until := now.Add(time.Duration(settings.TempUnschedMinutes) * time.Minute)

	state := &TempUnschedState{
		UntilUnix:       until.Unix(),
		TriggeredAtUnix: now.Unix(),
		StatusCode:      0, // 超时没有状态码
		MatchedKeyword:  "stream_timeout",
		RuleIndex:       -1, // 表示系统级规则
		ErrorMessage:    "Stream data interval timeout for model: " + model,
	}

	reason := ""
	if raw, err := json.Marshal(state); err == nil {
		reason = string(raw)
	}
	if reason == "" {
		reason = state.ErrorMessage
	}

	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("stream_timeout_set_temp_unsched_failed", "account_id", account.ID, "error", err)
		return false
	}

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			slog.Warn("stream_timeout_set_temp_unsched_cache_failed", "account_id", account.ID, "error", err)
		}
	}

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			slog.Warn("stream_timeout_reset_count_failed", "account_id", account.ID, "error", err)
		}
	}

	slog.Info("stream_timeout_temp_unschedulable", "account_id", account.ID, "until", until, "model", model)
	return true
}

// triggerStreamTimeoutError 触发流超时错误状态
func (s *RateLimitService) triggerStreamTimeoutError(ctx context.Context, account *Account, model string) bool {
	errorMsg := "Stream data interval timeout (repeated failures) for model: " + model

	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		slog.Warn("stream_timeout_set_error_failed", "account_id", account.ID, "error", err)
		return false
	}

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			slog.Warn("stream_timeout_reset_count_failed", "account_id", account.ID, "error", err)
		}
	}

	slog.Warn("stream_timeout_account_error", "account_id", account.ID, "model", model)
	return true
}
