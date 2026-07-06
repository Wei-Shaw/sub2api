package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
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
	openAI403CounterCache OpenAI403CounterCache
	settingService        *SettingService
	tokenCacheInvalidator TokenCacheInvalidator
	runtimeBlocker        AccountRuntimeBlocker
	usageCacheMu          sync.RWMutex
	usageCache            map[int64]*geminiUsageCacheEntry
REDACTED

type AccountRuntimeBlocker interface {
	BlockAccountScheduling(account *Account, until time.Time, reason string)
	ClearAccountSchedulingBlock(accountID int64)
REDACTED

// SuccessfulTestRecoveryResult 表示测试成功后恢复了哪些运行时状态。
type SuccessfulTestRecoveryResult struct {
	ClearedError     bool
	ClearedRateLimit bool
REDACTED

// AccountRecoveryOptions 控制账号恢复时的附加行为。
type AccountRecoveryOptions struct {
	InvalidateToken bool
REDACTED

type geminiUsageCacheEntry struct {
	windowStart time.Time
	cachedAt    time.Time
	totals      GeminiUsageTotals
REDACTED

type geminiUsageTotalsBatchProvider interface {
	GetGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, startTime, endTime time.Time) (map[int64]GeminiUsageTotals, error)
REDACTED

const geminiPrecheckCacheTTL = time.Minute

const (
	defaultRateLimit429CooldownSeconds = 5
	maxRateLimit429CooldownSeconds     = 7200
)

const (
	openAIImageRateLimitDefaultCooldown = time.Minute
	openAIImageRateLimitReason          = "openai_image_rate_limited"
)

var openAIImageTryAgainPattern = regexp.MustCompile(`(?i)try again in\s+([0-9]+(?:\.[0-9]+)?)\s*(ms|s|sec|secs|second|seconds|m|min|mins|minute|minutes)`)

const (
	openAI403CooldownMinutesDefault = 10
	openAI403DisableThreshold       = 3
	openAI403CounterWindowMinutes   = 180
)

// NewRateLimitService 创建RateLimitService实例
func NewRateLimitService(accountRepo AccountRepository, usageRepo UsageLogRepository, cfg *config.Config, geminiQuotaService *GeminiQuotaService, tempUnschedCache TempUnschedCache) *RateLimitService {
	return &RateLimitService{
		accountRepo:        accountRepo,
		usageRepo:          usageRepo,
		cfg:                cfg,
		geminiQuotaService: geminiQuotaService,
		tempUnschedCache:   tempUnschedCache,
		usageCache:         make(map[int64]*geminiUsageCacheEntry),
REDACTED
REDACTED

// SetTimeoutCounterCache 设置超时计数器缓存（可选依赖）
func (s *RateLimitService) SetTimeoutCounterCache(cache TimeoutCounterCache) {
	s.timeoutCounterCache = cache
REDACTED

// SetOpenAI403CounterCache 设置 OpenAI 403 连续失败计数器（可选依赖）
func (s *RateLimitService) SetOpenAI403CounterCache(cache OpenAI403CounterCache) {
	s.openAI403CounterCache = cache
REDACTED

// SetSettingService 设置系统设置服务（可选依赖）
func (s *RateLimitService) SetSettingService(settingService *SettingService) {
	s.settingService = settingService
REDACTED

// SetTokenCacheInvalidator 设置 token 缓存清理器（可选依赖）
func (s *RateLimitService) SetTokenCacheInvalidator(invalidator TokenCacheInvalidator) {
	s.tokenCacheInvalidator = invalidator
REDACTED

func (s *RateLimitService) SetAccountRuntimeBlocker(blocker AccountRuntimeBlocker) {
	s.runtimeBlocker = blocker
REDACTED

func (s *RateLimitService) notifyAccountSchedulingBlocked(account *Account, until time.Time, reason string) {
	if s == nil || s.runtimeBlocker == nil || account == nil {
		return
REDACTED
	s.runtimeBlocker.BlockAccountScheduling(account, until, reason)
REDACTED

func (s *RateLimitService) notifyAccountSchedulingBlockCleared(accountID int64) {
	if s == nil || s.runtimeBlocker == nil || accountID <= 0 {
		return
REDACTED
	s.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
REDACTED

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
	REDACTED
		slog.Info("account_error_code_skipped", "account_id", account.ID, "status_code", statusCode)
		return ErrorPolicySkipped
REDACTED
	if account.IsPoolMode() {
		return ErrorPolicySkipped
REDACTED
	if s.tryTempUnschedulable(ctx, account, statusCode, responseBody) {
		return ErrorPolicyTempUnscheduled
REDACTED
	return ErrorPolicyNone
REDACTED

// HandleUpstreamError 处理上游错误响应，标记账号状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte, requestedModel ...string) (shouldDisable bool) {
	customErrorCodesEnabled := account.IsCustomErrorCodesEnabled()

	// 池模式默认不标记本地账号状态；仅当用户显式配置自定义错误码时按本地策略处理。
	if account.IsPoolMode() && !customErrorCodesEnabled {
		slog.Info("pool_mode_error_skipped", "account_id", account.ID, "status_code", statusCode)
		return false
REDACTED

	// apikey 类型账号：检查自定义错误码配置
	// 如果启用且错误码不在列表中，则不处理（不停止调度、不标记限流/过载）
	if !account.ShouldHandleErrorCode(statusCode) {
		slog.Info("account_error_code_skipped", "account_id", account.ID, "status_code", statusCode)
		return false
REDACTED

	if len(requestedModel) > 0 && s.HandleUpstreamModelNotFound(ctx, account, requestedModel[0], statusCode, responseBody) {
		return true
REDACTED

	// Anthropic official 5h / 7d window exhaustion is a hard account limit.
	// It must take precedence over user-configured 429 temp-unsched rules,
	// otherwise a broad "rate limit" keyword rule can shorten a multi-hour
	// cooldown to a local temporary pause.
	if statusCode == http.StatusTooManyRequests && account.Platform == PlatformAnthropic {
		// 7d_oi 是 Fable 模型专属的 7d 窗口：只标记模型级限流，账号对其他模型仍可调度。
		fableLimited := s.persistAnthropicFableWindowLimit(ctx, account, headers)
		if s.persistAnthropicExhaustedWindowLimit(ctx, account, headers) {
			return false
	REDACTED
		if fableLimited {
			return false
	REDACTED
REDACTED

	// 先尝试临时不可调度规则（401除外）
	// 如果匹配成功，直接返回，不执行后续禁用逻辑
	if statusCode != 401 {
		if s.tryTempUnschedulable(ctx, account, statusCode, responseBody) {
			return true
	REDACTED
REDACTED

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if upstreamMsg != "" {
		upstreamMsg = truncateForLog([]byte(upstreamMsg), 512)
REDACTED

	switch statusCode {
	case 400:
		// "organization has been disabled" → 永久禁用
		if strings.Contains(strings.ToLower(upstreamMsg), "organization has been disabled") {
			msg := "Organization disabled (400): " + upstreamMsg
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
	REDACTED else if account.Platform == PlatformAnthropic && strings.Contains(strings.ToLower(upstreamMsg), "credit balance") {
			// Anthropic API key 余额不足（语义等同 402），停止调度
			msg := "Credit balance exhausted (400): " + upstreamMsg
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
	REDACTED else if strings.Contains(strings.ToLower(upstreamMsg), "identity verification is required") {
			// KYC 身份验证要求 → 永久禁用，账号需完成身份验证后才能恢复
			msg := "Identity verification required (400): " + upstreamMsg
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
	REDACTED
		// 其他 400 错误（如参数问题）不处理，不禁用账号
	case 401:
		// 外审第9轮:Spark 影子无独立凭据,401 是母账号 token 问题——失效缓存 / refresh_token 判断 /
		// 永久禁用 / 临时不可调度都必须落到凭据 owner(母账号),否则影子(无 refresh_token)必中
		// "refresh_token missing"永久禁用分支、母账号 token cache 也不会被清,把母账号可恢复的 token
		// 问题变成影子永久死亡。母账号被标记 temp-unschedulable 后由 parentHealthyForShadow 级联排除影子。
		// 非影子时 resolveCredentialAccount 返回自身;母账号缺失/损坏(orphan 影子,罕见)时回退到原 account。
		authAccount := account
		if resolved, rerr := resolveCredentialAccount(ctx, s.accountRepo, account); rerr == nil && resolved != nil {
			authAccount = resolved
	REDACTED
		// OpenAI: token_invalidated / token_revoked 表示 token 被永久作废（非过期），直接标记 error
		openai401Code := extractUpstreamErrorCode(responseBody)
		if authAccount.Platform == PlatformOpenAI && (openai401Code == "token_invalidated" || openai401Code == "token_revoked") {
			msg := "Token revoked (401): account authentication permanently revoked"
			if upstreamMsg != "" {
				msg = "Token revoked (401): " + upstreamMsg
		REDACTED
			s.handleAuthError(ctx, authAccount, msg)
			shouldDisable = true
			break
	REDACTED
		// OpenAI: {"detail":"Unauthorized"REDACTED 表示 token 完全无效（非标准 OpenAI 错误格式），直接标记 error
		if authAccount.Platform == PlatformOpenAI && gjson.GetBytes(responseBody, "detail").String() == "Unauthorized" {
			msg := "Unauthorized (401): account authentication failed permanently"
			if upstreamMsg != "" {
				msg = "Unauthorized (401): " + upstreamMsg
		REDACTED
			s.handleAuthError(ctx, authAccount, msg)
			shouldDisable = true
			break
	REDACTED
		// OAuth 账号在 401 错误时临时不可调度（给 token 刷新窗口）；非 OAuth 账号保持原有 SetError 行为。
		if authAccount.Type == AccountTypeOAuth {
			// 1. 失效缓存
			if s.tokenCacheInvalidator != nil {
				if err := s.tokenCacheInvalidator.InvalidateToken(ctx, authAccount); err != nil {
					slog.Warn("oauth_401_invalidate_cache_failed", "account_id", authAccount.ID, "error", err)
			REDACTED
		REDACTED
			// 缺少 refresh_token 的 OAuth 账号无法在冷却期内自愈（后台刷新服务也会跳过），
			// 直接走 SetError 永久禁用，避免冷却结束后再被选中产生一发无意义的 502。
			if strings.TrimSpace(authAccount.GetCredential("refresh_token")) == "" {
				msg := "Authentication failed (401): refresh_token missing, cannot recover"
				if upstreamMsg != "" {
					msg = "OAuth 401 (no refresh_token): " + upstreamMsg
			REDACTED
				s.handleAuthError(ctx, authAccount, msg)
				shouldDisable = true
				break
		REDACTED
			// 2. 临时不可调度，替代 SetError（保持 status=active 让刷新服务能拾取）
			// 注意：此处不再写回 account.Credentials/expires_at。
			// 原实现使用请求开始时的 account 快照整列覆盖 credentials JSONB（见
			// persistAccountCredentials → accountRepository.UpdateCredentials → SetCredentials），
			// 在另一个 worker 刚刷新完 refresh_token 的窄窗口内会把新 refresh_token 回滚为旧值，
			// 导致下一周期用旧 refresh_token 调上游拿到 invalid_grant 后，
			// tryRecoverFromRefreshRace 重读 DB 发现 currentRT == usedRT 也救不回来，账号被错误 disable。
			// 这里仅依赖 InvalidateToken + SetTempUnschedulable 让账号在冷却期内不被调度，
			// 冷却结束后由 token_provider 的 NeedsRefresh / token_refresh_service 走带分布式锁的正路刷新。
			msg := "Authentication failed (401): invalid or expired credentials"
			if upstreamMsg != "" {
				msg = "OAuth 401: " + upstreamMsg
		REDACTED
			if authAccount.Platform == PlatformAntigravity {
				extraUpdates := antigravityForceTokenRefreshExtra("401_invalid")
				if err := s.accountRepo.UpdateExtra(ctx, authAccount.ID, extraUpdates); err != nil {
					slog.Warn("antigravity_401_force_refresh_mark_failed", "account_id", authAccount.ID, "error", err)
			REDACTED else {
					if authAccount.Extra == nil {
						authAccount.Extra = make(map[string]any, len(extraUpdates))
				REDACTED
					for k, v := range extraUpdates {
						authAccount.Extra[k] = v
				REDACTED
					slog.Info("antigravity_401_force_refresh_marked", "account_id", authAccount.ID)
			REDACTED
		REDACTED
			cooldownMinutes := s.cfg.RateLimit.OAuth401CooldownMinutes
			if cooldownMinutes <= 0 {
				cooldownMinutes = 10
		REDACTED
			until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
			s.notifyAccountSchedulingBlocked(authAccount, until, "oauth_401")
			if err := s.accountRepo.SetTempUnschedulable(ctx, authAccount.ID, until, msg); err != nil {
				slog.Warn("oauth_401_set_temp_unschedulable_failed", "account_id", authAccount.ID, "error", err)
		REDACTED
			shouldDisable = true
	REDACTED else {
			// 非 OAuth：保持 SetError 行为
			msg := "Authentication failed (401): invalid or expired credentials"
			if upstreamMsg != "" {
				msg = "Authentication failed (401): " + upstreamMsg
		REDACTED
			s.handleAuthError(ctx, authAccount, msg)
			shouldDisable = true
	REDACTED
	case 402:
		// OpenAI: deactivated_workspace 表示工作区已停用，直接标记 error
		if account.Platform == PlatformOpenAI && gjson.GetBytes(responseBody, "detail.code").String() == "deactivated_workspace" {
			msg := "Workspace deactivated (402): workspace has been deactivated"
			s.handleAuthError(ctx, account, msg)
			shouldDisable = true
			break
	REDACTED
		// 支付要求：余额不足或计费问题，停止调度
		msg := "Payment required (402): insufficient balance or billing issue"
		if upstreamMsg != "" {
			msg = "Payment required (402): " + upstreamMsg
	REDACTED
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
		REDACTED
			s.handleCustomErrorCode(ctx, account, statusCode, msg)
			shouldDisable = true
	REDACTED else if statusCode >= 500 {
			// 未启用自定义错误码时：仅记录5xx错误
			slog.Warn("account_upstream_error", "account_id", account.ID, "status_code", statusCode)
			shouldDisable = false
	REDACTED
REDACTED

	return shouldDisable
REDACTED

// PreCheckUsage proactively checks local quota before dispatching a request.
// Returns false when the account should be skipped.
func (s *RateLimitService) PreCheckUsage(ctx context.Context, account *Account, requestedModel string) (bool, error) {
	if account == nil || account.Platform != PlatformGemini {
		return true, nil
REDACTED
	if s.usageRepo == nil || s.geminiQuotaService == nil {
		return true, nil
REDACTED

	quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
	if !ok {
		return true, nil
REDACTED

	now := time.Now()
	modelClass := geminiModelClassFromName(requestedModel)

	// 1) Daily quota precheck (RPD; resets at PST midnight)
	{
		var limit int64
		if quota.SharedRPD > 0 {
			limit = quota.SharedRPD
	REDACTED else {
			switch modelClass {
			case geminiModelFlash:
				limit = quota.FlashRPD
			default:
				limit = quota.ProRPD
		REDACTED
	REDACTED

		if limit > 0 {
			start := geminiDailyWindowStart(now)
			totals, ok := s.getGeminiUsageTotals(account.ID, start, now)
			if !ok {
				stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil, nil, nil)
				if err != nil {
					return true, err
			REDACTED
				totals = geminiAggregateUsage(stats)
				s.setGeminiUsageTotals(account.ID, start, now, totals)
		REDACTED

			var used int64
			if quota.SharedRPD > 0 {
				used = totals.ProRequests + totals.FlashRequests
		REDACTED else {
				switch modelClass {
				case geminiModelFlash:
					used = totals.FlashRequests
				default:
					used = totals.ProRequests
			REDACTED
		REDACTED

			if used >= limit {
				resetAt := geminiDailyResetTime(now)
				// NOTE:
				// - This is a local precheck to reduce upstream 429s.
				// - Do NOT mark the account as rate-limited here; rate_limit_reset_at should reflect real upstream 429s.
				slog.Info("gemini_precheck_daily_quota_reached", "account_id", account.ID, "used", used, "limit", limit, "reset_at", resetAt)
				return false, nil
		REDACTED
	REDACTED
REDACTED

	// 2) Minute quota precheck (RPM; fixed window current minute)
	{
		var limit int64
		if quota.SharedRPM > 0 {
			limit = quota.SharedRPM
	REDACTED else {
			switch modelClass {
			case geminiModelFlash:
				limit = quota.FlashRPM
			default:
				limit = quota.ProRPM
		REDACTED
	REDACTED

		if limit > 0 {
			start := now.Truncate(time.Minute)
			stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil, nil, nil)
			if err != nil {
				return true, err
		REDACTED
			totals := geminiAggregateUsage(stats)

			var used int64
			if quota.SharedRPM > 0 {
				used = totals.ProRequests + totals.FlashRequests
		REDACTED else {
				switch modelClass {
				case geminiModelFlash:
					used = totals.FlashRequests
				default:
					used = totals.ProRequests
			REDACTED
		REDACTED

			if used >= limit {
				resetAt := start.Add(time.Minute)
				// Do not persist "rate limited" status from local precheck. See note above.
				slog.Info("gemini_precheck_minute_quota_reached", "account_id", account.ID, "used", used, "limit", limit, "reset_at", resetAt)
				return false, nil
		REDACTED
	REDACTED
REDACTED

	return true, nil
REDACTED

// PreCheckUsageBatch performs quota precheck for multiple accounts in one request.
// Returned map value=false means the account should be skipped.
func (s *RateLimitService) PreCheckUsageBatch(ctx context.Context, accounts []*Account, requestedModel string) (map[int64]bool, error) {
	result := make(map[int64]bool, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
	REDACTED
		result[account.ID] = true
REDACTED

	if len(accounts) == 0 || requestedModel == "" {
		return result, nil
REDACTED
	if s.usageRepo == nil || s.geminiQuotaService == nil {
		return result, nil
REDACTED

	modelClass := geminiModelClassFromName(requestedModel)
	now := time.Now()
	dailyStart := geminiDailyWindowStart(now)
	minuteStart := now.Truncate(time.Minute)

	type quotaAccount struct {
		account *Account
		quota   GeminiQuota
REDACTED
	quotaAccounts := make([]quotaAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Platform != PlatformGemini {
			continue
	REDACTED
		quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
		if !ok {
			continue
	REDACTED
		quotaAccounts = append(quotaAccounts, quotaAccount{
			account: account,
			quota:   quota,
	REDACTED)
REDACTED
	if len(quotaAccounts) == 0 {
		return result, nil
REDACTED

	// 1) Daily precheck (cached + batch DB fallback)
	dailyTotalsByID := make(map[int64]GeminiUsageTotals, len(quotaAccounts))
	dailyMissIDs := make([]int64, 0, len(quotaAccounts))
	for _, item := range quotaAccounts {
		limit := geminiDailyLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
	REDACTED
		accountID := item.account.ID
		if totals, ok := s.getGeminiUsageTotals(accountID, dailyStart, now); ok {
			dailyTotalsByID[accountID] = totals
			continue
	REDACTED
		dailyMissIDs = append(dailyMissIDs, accountID)
REDACTED
	if len(dailyMissIDs) > 0 {
		totalsBatch, err := s.getGeminiUsageTotalsBatch(ctx, dailyMissIDs, dailyStart, now)
		if err != nil {
			return result, err
	REDACTED
		for _, accountID := range dailyMissIDs {
			totals := totalsBatch[accountID]
			dailyTotalsByID[accountID] = totals
			s.setGeminiUsageTotals(accountID, dailyStart, now, totals)
	REDACTED
REDACTED
	for _, item := range quotaAccounts {
		limit := geminiDailyLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
	REDACTED
		accountID := item.account.ID
		used := geminiUsedRequests(item.quota, modelClass, dailyTotalsByID[accountID], true)
		if used >= limit {
			resetAt := geminiDailyResetTime(now)
			slog.Info("gemini_precheck_daily_quota_reached_batch", "account_id", accountID, "used", used, "limit", limit, "reset_at", resetAt)
			result[accountID] = false
	REDACTED
REDACTED

	// 2) Minute precheck (batch DB)
	minuteIDs := make([]int64, 0, len(quotaAccounts))
	for _, item := range quotaAccounts {
		accountID := item.account.ID
		if !result[accountID] {
			continue
	REDACTED
		if geminiMinuteLimit(item.quota, modelClass) <= 0 {
			continue
	REDACTED
		minuteIDs = append(minuteIDs, accountID)
REDACTED
	if len(minuteIDs) == 0 {
		return result, nil
REDACTED

	minuteTotalsByID, err := s.getGeminiUsageTotalsBatch(ctx, minuteIDs, minuteStart, now)
	if err != nil {
		return result, err
REDACTED
	for _, item := range quotaAccounts {
		accountID := item.account.ID
		if !result[accountID] {
			continue
	REDACTED

		limit := geminiMinuteLimit(item.quota, modelClass)
		if limit <= 0 {
			continue
	REDACTED

		used := geminiUsedRequests(item.quota, modelClass, minuteTotalsByID[accountID], false)
		if used >= limit {
			resetAt := minuteStart.Add(time.Minute)
			slog.Info("gemini_precheck_minute_quota_reached_batch", "account_id", accountID, "used", used, "limit", limit, "reset_at", resetAt)
			result[accountID] = false
	REDACTED
REDACTED

	return result, nil
REDACTED

func (s *RateLimitService) getGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]GeminiUsageTotals, error) {
	result := make(map[int64]GeminiUsageTotals, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
REDACTED

	ids := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{REDACTED, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
	REDACTED
		if _, ok := seen[accountID]; ok {
			continue
	REDACTED
		seen[accountID] = struct{REDACTED{REDACTED
		ids = append(ids, accountID)
REDACTED
	if len(ids) == 0 {
		return result, nil
REDACTED

	if batchReader, ok := s.usageRepo.(geminiUsageTotalsBatchProvider); ok {
		stats, err := batchReader.GetGeminiUsageTotalsBatch(ctx, ids, start, end)
		if err != nil {
			return nil, err
	REDACTED
		for _, accountID := range ids {
			result[accountID] = stats[accountID]
	REDACTED
		return result, nil
REDACTED

	for _, accountID := range ids {
		stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, end, 0, 0, accountID, 0, nil, nil, nil)
		if err != nil {
			return nil, err
	REDACTED
		result[accountID] = geminiAggregateUsage(stats)
REDACTED
	return result, nil
REDACTED

func geminiDailyLimit(quota GeminiQuota, modelClass geminiModelClass) int64 {
	if quota.SharedRPD > 0 {
		return quota.SharedRPD
REDACTED
	switch modelClass {
	case geminiModelFlash:
		return quota.FlashRPD
	default:
		return quota.ProRPD
REDACTED
REDACTED

func geminiMinuteLimit(quota GeminiQuota, modelClass geminiModelClass) int64 {
	if quota.SharedRPM > 0 {
		return quota.SharedRPM
REDACTED
	switch modelClass {
	case geminiModelFlash:
		return quota.FlashRPM
	default:
		return quota.ProRPM
REDACTED
REDACTED

func geminiUsedRequests(quota GeminiQuota, modelClass geminiModelClass, totals GeminiUsageTotals, daily bool) int64 {
	if daily {
		if quota.SharedRPD > 0 {
			return totals.ProRequests + totals.FlashRequests
	REDACTED
REDACTED else {
		if quota.SharedRPM > 0 {
			return totals.ProRequests + totals.FlashRequests
	REDACTED
REDACTED
	switch modelClass {
	case geminiModelFlash:
		return totals.FlashRequests
	default:
		return totals.ProRequests
REDACTED
REDACTED

func (s *RateLimitService) getGeminiUsageTotals(accountID int64, windowStart, now time.Time) (GeminiUsageTotals, bool) {
	s.usageCacheMu.RLock()
	defer s.usageCacheMu.RUnlock()

	if s.usageCache == nil {
		return GeminiUsageTotals{REDACTED, false
REDACTED

	entry, ok := s.usageCache[accountID]
	if !ok || entry == nil {
		return GeminiUsageTotals{REDACTED, false
REDACTED
	if !entry.windowStart.Equal(windowStart) {
		return GeminiUsageTotals{REDACTED, false
REDACTED
	if now.Sub(entry.cachedAt) >= geminiPrecheckCacheTTL {
		return GeminiUsageTotals{REDACTED, false
REDACTED
	return entry.totals, true
REDACTED

func (s *RateLimitService) setGeminiUsageTotals(accountID int64, windowStart, now time.Time, totals GeminiUsageTotals) {
	s.usageCacheMu.Lock()
	defer s.usageCacheMu.Unlock()
	if s.usageCache == nil {
		s.usageCache = make(map[int64]*geminiUsageCacheEntry)
REDACTED
	s.usageCache[accountID] = &geminiUsageCacheEntry{
		windowStart: windowStart,
		cachedAt:    now,
		totals:      totals,
REDACTED
REDACTED

// GeminiCooldown returns the fallback cooldown duration for Gemini 429s based on tier.
func (s *RateLimitService) GeminiCooldown(ctx context.Context, account *Account) time.Duration {
	if account == nil {
		return 5 * time.Minute
REDACTED
	if s.geminiQuotaService == nil {
		return 5 * time.Minute
REDACTED
	return s.geminiQuotaService.CooldownForAccount(ctx, account)
REDACTED

// handleAuthError 处理认证类错误(401/403)，停止账号调度
func (s *RateLimitService) handleAuthError(ctx context.Context, account *Account, errorMsg string) {
	s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "auth_error")
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		slog.Warn("account_set_error_failed", "account_id", account.ID, "error", err)
		return
REDACTED
	slog.Warn("account_disabled_auth_error", "account_id", account.ID, "error", errorMsg)
REDACTED

func buildForbiddenErrorMessage(prefix string, upstreamMsg string, responseBody []byte, fallback string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, " ") {
		prefix += " "
REDACTED

	if msg := strings.TrimSpace(upstreamMsg); msg != "" {
		return prefix + msg
REDACTED

	rawBody := bytes.TrimSpace(responseBody)
	if len(rawBody) > 0 {
		if json.Valid(rawBody) {
			var compact bytes.Buffer
			if err := json.Compact(&compact, rawBody); err == nil {
				return prefix + truncateForLog(compact.Bytes(), 512)
		REDACTED
	REDACTED
		return prefix + truncateForLog(rawBody, 512)
REDACTED

	return prefix + fallback
REDACTED

// handle403 处理 403 Forbidden 错误
// Antigravity 平台区分 validation/violation/generic 三种类型，均 SetError 永久禁用；
// 其他平台保持原有 SetError 行为。
func (s *RateLimitService) handle403(ctx context.Context, account *Account, upstreamMsg string, responseBody []byte) (shouldDisable bool) {
	if account.Platform == PlatformAntigravity {
		return s.handleAntigravity403(ctx, account, upstreamMsg, responseBody)
REDACTED
	if account.Platform == PlatformOpenAI {
		return s.handleOpenAI403(ctx, account, upstreamMsg, responseBody)
REDACTED
	// 非 Antigravity 平台：保持原有行为
	msg := buildForbiddenErrorMessage(
		"Access forbidden (403):",
		upstreamMsg,
		responseBody,
		"account may be suspended or lack permissions",
	)
	s.handleAuthError(ctx, account, msg)
	return true
REDACTED

func (s *RateLimitService) handleOpenAI403(ctx context.Context, account *Account, upstreamMsg string, responseBody []byte) (shouldDisable bool) {
	msg := buildForbiddenErrorMessage(
		"Access forbidden (403):",
		upstreamMsg,
		responseBody,
		"account may be suspended or lack permissions",
	)

	if s.openAI403CounterCache == nil {
		s.handleAuthError(ctx, account, msg)
		return true
REDACTED

	count, err := s.openAI403CounterCache.IncrementOpenAI403Count(ctx, account.ID, openAI403CounterWindowMinutes)
	if err != nil {
		slog.Warn("openai_403_increment_failed", "account_id", account.ID, "error", err)
		s.handleAuthError(ctx, account, msg)
		return true
REDACTED

	if count >= openAI403DisableThreshold {
		msg = fmt.Sprintf("%s | consecutive_403=%d/%d", msg, count, openAI403DisableThreshold)
		s.handleAuthError(ctx, account, msg)
		return true
REDACTED

	until := time.Now().Add(time.Duration(openAI403CooldownMinutesDefault) * time.Minute)
	reason := fmt.Sprintf("OpenAI 403 temporary cooldown (%d/%d): %s", count, openAI403DisableThreshold, msg)
	s.notifyAccountSchedulingBlocked(account, until, "openai_403_temp")
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("openai_403_set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		s.handleAuthError(ctx, account, msg)
		return true
REDACTED

	slog.Warn(
		"openai_403_temp_unschedulable",
		"account_id", account.ID,
		"until", until,
		"count", count,
		"threshold", openAI403DisableThreshold,
	)
	return true
REDACTED

// handleAntigravity403 处理 Antigravity 平台的 403 错误
// validation（需要验证）→ 永久 SetError（需人工去 Google 验证后恢复）
// violation（违规封号）→ 永久 SetError（需人工处理）
// generic（通用禁止）→ 永久 SetError
func (s *RateLimitService) handleAntigravity403(ctx context.Context, account *Account, upstreamMsg string, responseBody []byte) (shouldDisable bool) {
	fbType := classifyForbiddenType(string(responseBody))

	switch fbType {
	case forbiddenTypeValidation:
		// VALIDATION_REQUIRED: 永久禁用，需人工去 Google 验证后手动恢复
		msg := buildForbiddenErrorMessage(
			"Validation required (403):",
			upstreamMsg,
			responseBody,
			"account needs Google verification",
		)
		if validationURL := extractValidationURL(string(responseBody)); validationURL != "" {
			msg += " | validation_url: " + validationURL
	REDACTED
		s.handleAuthError(ctx, account, msg)
		return true

	case forbiddenTypeViolation:
		// 违规封号: 永久禁用，需人工处理
		msg := buildForbiddenErrorMessage(
			"Account violation (403):",
			upstreamMsg,
			responseBody,
			"terms of service violation",
		)
		s.handleAuthError(ctx, account, msg)
		return true

	default:
		// 通用 403: 保持原有行为
		msg := buildForbiddenErrorMessage(
			"Access forbidden (403):",
			upstreamMsg,
			responseBody,
			"account may be suspended or lack permissions",
		)
		s.handleAuthError(ctx, account, msg)
		return true
REDACTED
REDACTED

// handleCustomErrorCode 处理自定义错误码，停止账号调度
func (s *RateLimitService) handleCustomErrorCode(ctx context.Context, account *Account, statusCode int, errorMsg string) {
	msg := "Custom error code " + strconv.Itoa(statusCode) + ": " + errorMsg
	s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "custom_error_code")
	if err := s.accountRepo.SetError(ctx, account.ID, msg); err != nil {
		slog.Warn("account_set_error_failed", "account_id", account.ID, "status_code", statusCode, "error", err)
		return
REDACTED
	slog.Warn("account_disabled_custom_error", "account_id", account.ID, "status_code", statusCode, "error", errorMsg)
REDACTED

// handle429 处理429限流错误
// 解析响应头获取重置时间，标记账号为限流状态
func (s *RateLimitService) handle429(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	// Spark 影子：限流/熔断状态 100% 由 QueryUsage(/wham/usage body 的 codex_bengalfox)驱动。
	// /responses 的 429 携带的 x-codex-*/usage_limit_reached 是 global codex 道(plan/spec §8),
	// 套到影子会把 spark 误耦合到 global 窗口——即便 spark 仍有配额也会被冷却到 global reset,
	// 单影子场景直接变成无可用账号(外审第8轮 P1)。整段跳过;影子的 codex_* 仅由 account_usage 的
	// QueryUsage→persistOpenAICodexProbeSnapshot 维护,枯竭由调度守卫处理。
	if account.IsShadow() {
		return
REDACTED
	// 1. OpenAI 平台：优先尝试解析 x-codex-* 响应头（用于 rate_limit_exceeded）
	if account.Platform == PlatformOpenAI {
		persistOpenAI429PlanType(ctx, s.accountRepo, account, responseBody)
		s.persistOpenAICodexSnapshot(ctx, account, headers)
		if resetAt := s.calculateOpenAI429ResetTime(headers); resetAt != nil {
			s.notifyAccountSchedulingBlocked(account, *resetAt, "429")
			if err := s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt); err != nil {
				slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
				return
		REDACTED
			slog.Info("openai_account_rate_limited", "account_id", account.ID, "reset_at", *resetAt)
			return
	REDACTED
REDACTED

	// 2. Anthropic 平台：尝试解析 per-window 头（5h / 7d），选择实际触发的窗口
	if result := calculateAnthropic429ResetTime(headers); result != nil {
		s.notifyAccountSchedulingBlocked(account, result.resetAt, "429")
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, result.resetAt); err != nil {
			slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
			return
	REDACTED

		// 更新 session window：优先使用 5h-reset 头精确计算，否则从 resetAt 反推
		windowEnd := result.resetAt
		if result.fiveHourReset != nil {
			windowEnd = *result.fiveHourReset
	REDACTED
		windowStart := windowEnd.Add(-5 * time.Hour)
		if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
			slog.Warn("rate_limit_update_session_window_failed", "account_id", account.ID, "error", err)
	REDACTED

		slog.Info("anthropic_account_rate_limited", "account_id", account.ID, "reset_at", result.resetAt, "reset_in", time.Until(result.resetAt).Truncate(time.Second))
		return
REDACTED

	// 3. 尝试从响应头解析重置时间（Anthropic 聚合头，向后兼容）
	resetTimestamp := headers.Get("anthropic-ratelimit-unified-reset")

	// 4. 如果响应头没有，尝试从响应体解析（OpenAI usage_limit_reached, Gemini）
	if resetTimestamp == "" {
		switch account.Platform {
		case PlatformOpenAI:
			// 尝试解析 OpenAI 的 usage_limit_reached 错误
			if resetAt := parseOpenAIRateLimitResetTime(responseBody); resetAt != nil {
				resetTime := time.Unix(*resetAt, 0)
				s.notifyAccountSchedulingBlocked(account, resetTime, "429")
				if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetTime); err != nil {
					slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
					return
			REDACTED
				slog.Info("account_rate_limited", "account_id", account.ID, "platform", account.Platform, "reset_at", resetTime, "reset_in", time.Until(resetTime).Truncate(time.Second))
				return
		REDACTED
		case PlatformGemini, PlatformAntigravity:
			// 尝试解析 Gemini 格式（用于其他平台）
			if resetAt := ParseGeminiRateLimitResetTime(responseBody); resetAt != nil {
				resetTime := time.Unix(*resetAt, 0)
				s.notifyAccountSchedulingBlocked(account, resetTime, "429")
				if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetTime); err != nil {
					slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
					return
			REDACTED
				slog.Info("account_rate_limited", "account_id", account.ID, "platform", account.Platform, "reset_at", resetTime, "reset_in", time.Until(resetTime).Truncate(time.Second))
				return
		REDACTED
	REDACTED

		// Anthropic 平台：没有限流重置时间的 429 可能是非真实限流（如 Extra usage required），
		// 不标记账号限流状态，直接透传错误给客户端
		if account.Platform == PlatformAnthropic {
			slog.Warn("rate_limit_429_no_reset_time_skipped",
				"account_id", account.ID,
				"platform", account.Platform,
				"reason", "no rate limit reset time in headers, likely not a real rate limit")
			return
	REDACTED

		// 其他平台：没有重置时间，使用可配置的秒级默认回避，避免误伤长时间不可调度。
		s.apply429FallbackRateLimit(ctx, account, "no_reset_time")
		return
REDACTED

	// 解析Unix时间戳
	ts, err := strconv.ParseInt(resetTimestamp, 10, 64)
	if err != nil {
		slog.Warn("rate_limit_reset_parse_failed", "reset_timestamp", resetTimestamp, "error", err)
		s.apply429FallbackRateLimit(ctx, account, "reset_parse_failed")
		return
REDACTED

	resetAt := time.Unix(ts, 0)

	// 标记限流状态
	s.notifyAccountSchedulingBlocked(account, resetAt, "429")
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
		return
REDACTED

	// 根据重置时间反推5h窗口
	windowEnd := resetAt
	windowStart := resetAt.Add(-5 * time.Hour)
	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
		slog.Warn("rate_limit_update_session_window_failed", "account_id", account.ID, "error", err)
REDACTED

	slog.Info("account_rate_limited", "account_id", account.ID, "reset_at", resetAt)
REDACTED

func (s *RateLimitService) apply429FallbackRateLimit(ctx context.Context, account *Account, reason string) {
	cooldown, enabled := s.get429FallbackCooldown(ctx, account)
	if !enabled {
		slog.Info("rate_limit_429_fallback_ignored", "account_id", account.ID, "platform", account.Platform, "reason", reason)
		return
REDACTED

	resetAt := time.Now().Add(cooldown)
	slog.Warn("rate_limit_429_fallback_used", "account_id", account.ID, "platform", account.Platform, "reason", reason, "using_default", cooldown.String())
	s.notifyAccountSchedulingBlocked(account, resetAt, "429_fallback")
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
REDACTED
REDACTED

func (s *RateLimitService) get429FallbackCooldown(ctx context.Context, account *Account) (time.Duration, bool) {
	if s.settingService != nil {
		settings, err := s.settingService.GetRateLimit429CooldownSettings(ctx)
		if err == nil && settings != nil {
			if !settings.Enabled {
				return 0, false
		REDACTED
			seconds := clampRateLimit429CooldownSeconds(settings.CooldownSeconds)
			return time.Duration(seconds) * time.Second, true
	REDACTED
		slog.Warn("rate_limit_429_settings_read_failed", "account_id", account.ID, "error", err)
REDACTED

	seconds := defaultRateLimit429CooldownSeconds
	seconds = clampRateLimit429CooldownSeconds(seconds)
	return time.Duration(seconds) * time.Second, true
REDACTED

func clampRateLimit429CooldownSeconds(seconds int) int {
	if seconds < 1 {
		return 1
REDACTED
	if seconds > maxRateLimit429CooldownSeconds {
		return maxRateLimit429CooldownSeconds
REDACTED
	return seconds
REDACTED

// calculateOpenAI429ResetTime 从 OpenAI 429 响应头计算正确的重置时间
// 返回 nil 表示无法从响应头中确定重置时间
func calculateOpenAI429ResetTime(headers http.Header) *time.Time {
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return nil
REDACTED

	normalized := snapshot.Normalize()
	if normalized == nil {
		return nil
REDACTED

	now := time.Now()

	// 判断哪个限制被触发（used_percent >= 100）
	is7dExhausted := normalized.Used7dPercent != nil && *normalized.Used7dPercent >= 100
	is5hExhausted := normalized.Used5hPercent != nil && *normalized.Used5hPercent >= 100

	// 优先使用被触发限制的重置时间
	if is7dExhausted && normalized.Reset7dSeconds != nil {
		resetAt := now.Add(time.Duration(*normalized.Reset7dSeconds) * time.Second)
		slog.Info("openai_429_7d_limit_exhausted", "reset_after_seconds", *normalized.Reset7dSeconds, "reset_at", resetAt)
		return &resetAt
REDACTED
	if is5hExhausted && normalized.Reset5hSeconds != nil {
		resetAt := now.Add(time.Duration(*normalized.Reset5hSeconds) * time.Second)
		slog.Info("openai_429_5h_limit_exhausted", "reset_after_seconds", *normalized.Reset5hSeconds, "reset_at", resetAt)
		return &resetAt
REDACTED

	// 都未达到100%但收到429，使用较长的重置时间
	var maxResetSecs int
	if normalized.Reset7dSeconds != nil && *normalized.Reset7dSeconds > maxResetSecs {
		maxResetSecs = *normalized.Reset7dSeconds
REDACTED
	if normalized.Reset5hSeconds != nil && *normalized.Reset5hSeconds > maxResetSecs {
		maxResetSecs = *normalized.Reset5hSeconds
REDACTED
	if maxResetSecs > 0 {
		resetAt := now.Add(time.Duration(maxResetSecs) * time.Second)
		slog.Info("openai_429_using_max_reset", "max_reset_seconds", maxResetSecs, "reset_at", resetAt)
		return &resetAt
REDACTED

	return nil
REDACTED

func (s *RateLimitService) calculateOpenAI429ResetTime(headers http.Header) *time.Time {
	return calculateOpenAI429ResetTime(headers)
REDACTED

// anthropic429Result holds the parsed Anthropic 429 rate-limit information.
type anthropic429Result struct {
	resetAt       time.Time  // The correct reset time to use for SetRateLimited
	fiveHourReset *time.Time // 5h window reset timestamp (for session window calculation), nil if not available
REDACTED

type anthropicWindowLimit struct {
	window  string
	resetAt time.Time
	reason  string
REDACTED

func selectAnthropicExhaustedWindow(headers http.Header, now time.Time) *anthropicWindowLimit {
	reset5h, ok5hReset := parseAnthropicWindowReset(headers, "5h", now)
	reset7d, ok7dReset := parseAnthropicWindowReset(headers, "7d", now)

	exceeded5h := isAnthropic5hRejected(headers) || isAnthropicWindowExceeded(headers, "5h")
	exceeded7d := isAnthropicWindowExceeded(headers, "7d")

	if exceeded7d && ok7dReset {
		return &anthropicWindowLimit{
			window:  "7d",
			resetAt: reset7d,
			reason:  "anthropic_7d_window_exhausted",
	REDACTED
REDACTED
	if exceeded5h && ok5hReset {
		return &anthropicWindowLimit{
			window:  "5h",
			resetAt: reset5h,
			reason:  "anthropic_5h_window_exhausted",
	REDACTED
REDACTED
	return nil
REDACTED

func isAnthropic5hRejected(headers http.Header) bool {
	return isAnthropicWindowRejected(headers, "5h")
REDACTED

func isAnthropicWindowRejected(headers http.Header, window string) bool {
	return strings.EqualFold(strings.TrimSpace(headers.Get("anthropic-ratelimit-unified-"+window+"-status")), "rejected")
REDACTED

func parseAnthropicWindowReset(headers http.Header, window string, now time.Time) (time.Time, bool) {
	maxAge := 8 * 24 * time.Hour
	if window == "5h" {
		maxAge = 6 * time.Hour
REDACTED
	return parseAnthropicResetTimestamp(headers.Get("anthropic-ratelimit-unified-"+window+"-reset"), now, maxAge)
REDACTED

// parseAnthropicResetTimestamp 解析 Anthropic reset 头的 Unix 时间戳（自动识别毫秒），
// 并校验落在 (now, now+maxAge] 的合理区间内。
func parseAnthropicResetTimestamp(raw string, now time.Time, maxAge time.Duration) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{REDACTED, false
REDACTED
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{REDACTED, false
REDACTED
	if ts > 1e11 {
		ts = ts / 1000
REDACTED
	resetAt := time.Unix(ts, 0)
	if !resetAt.After(now) || resetAt.After(now.Add(maxAge)) {
		return time.Time{REDACTED, false
REDACTED
	return resetAt, true
REDACTED

func shouldPersistAnthropicWindowLimit(account *Account, limit *anthropicWindowLimit, now time.Time) bool {
	if account == nil || limit == nil || !limit.resetAt.After(now) {
		return false
REDACTED
	if account.RateLimitResetAt == nil {
		return true
REDACTED
	if !account.RateLimitResetAt.After(now) {
		return true
REDACTED
	return limit.resetAt.After(*account.RateLimitResetAt)
REDACTED

func (s *RateLimitService) persistAnthropicExhaustedWindowLimit(ctx context.Context, account *Account, headers http.Header) bool {
	if s == nil || s.accountRepo == nil || account == nil {
		return false
REDACTED
	now := time.Now()
	limit := selectAnthropicExhaustedWindow(headers, now)
	if limit == nil {
		return false
REDACTED
	if !shouldPersistAnthropicWindowLimit(account, limit, now) {
		slog.Info("anthropic_window_rate_limit_kept",
			"account_id", account.ID,
			"window", limit.window,
			"reset_at", limit.resetAt,
			"existing_reset_at", account.RateLimitResetAt)
		return true
REDACTED

	s.notifyAccountSchedulingBlocked(account, limit.resetAt, limit.reason)
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, limit.resetAt); err != nil {
		slog.Warn("anthropic_window_rate_limit_set_failed",
			"account_id", account.ID,
			"window", limit.window,
			"reset_at", limit.resetAt,
			"error", err)
		return true
REDACTED
	slog.Info("anthropic_window_rate_limited",
		"account_id", account.ID,
		"window", limit.window,
		"reset_at", limit.resetAt,
		"reset_in", time.Until(limit.resetAt).Truncate(time.Second))
	return true
REDACTED

const anthropicFableWindowReason = "anthropic_7d_oi_window_exhausted"

// selectAnthropicFableWindowLimit parses the Anthropic 7d_oi per-model window
// headers (the Fable-only 7d window, e.g. anthropic-ratelimit-unified-7d_oi-*).
// Unlike 5h/7d, exhaustion of this window only limits the Fable model family —
// the account must stay schedulable for other models.
//
// The 7d_oi surpassed-threshold header carries a float ("1.0") rather than
// "true", so exhaustion is detected via status=rejected or utilization >= 1.0.
// When the 7d_oi reset header is missing, the aggregated
// anthropic-ratelimit-unified-reset is used (it mirrors the binding claim's
// reset when 7d_oi is the representative claim).
func selectAnthropicFableWindowLimit(headers http.Header, now time.Time) *anthropicWindowLimit {
	if !isAnthropicWindowRejected(headers, "7d_oi") && !isAnthropicWindowExceeded(headers, "7d_oi") {
		return nil
REDACTED
	resetAt, ok := parseAnthropicWindowReset(headers, "7d_oi", now)
	if !ok {
		resetAt, ok = parseAnthropicAggregateReset(headers, now)
REDACTED
	if !ok {
		return nil
REDACTED
	return &anthropicWindowLimit{
		window:  "7d_oi",
		resetAt: resetAt,
		reason:  anthropicFableWindowReason,
REDACTED
REDACTED

// parseAnthropicAggregateReset parses the aggregated
// anthropic-ratelimit-unified-reset header with the same sanity checks as the
// per-window variant (7d scale).
func parseAnthropicAggregateReset(headers http.Header, now time.Time) (time.Time, bool) {
	return parseAnthropicResetTimestamp(headers.Get("anthropic-ratelimit-unified-reset"), now, 8*24*time.Hour)
REDACTED

// persistAnthropicFableWindowLimit marks the Fable model family as rate limited
// when the 7d_oi window is exhausted. Returns true when the 7d_oi window was the
// (or a) trigger of this 429, so the caller must not fall through to logic that
// would mark the whole account as rate limited.
func (s *RateLimitService) persistAnthropicFableWindowLimit(ctx context.Context, account *Account, headers http.Header) bool {
	if s == nil || s.accountRepo == nil || account == nil {
		return false
REDACTED
	now := time.Now()
	limit := selectAnthropicFableWindowLimit(headers, now)
	if limit == nil {
		return false
REDACTED
	// 429 响应头本身携带最新的窗口用量（7d_oi utilization=1.0）。限流期内
	// Fable 请求不再调度到该账号，若不在此处采样，7d F 进度条会冻结在
	// 限流前的旧值直到窗口重置。
	s.samplePassiveUsageFromHeaders(ctx, account, headers)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, anthropicFableRateLimitKey, limit.resetAt, limit.reason); err != nil {
		slog.Warn("anthropic_fable_window_rate_limit_set_failed",
			"account_id", account.ID,
			"scope", anthropicFableRateLimitKey,
			"reset_at", limit.resetAt,
			"error", err)
		return true
REDACTED
	slog.Info("anthropic_fable_window_model_rate_limited",
		"account_id", account.ID,
		"scope", anthropicFableRateLimitKey,
		"reset_at", limit.resetAt,
		"reset_in", time.Until(limit.resetAt).Truncate(time.Second))
	return true
REDACTED

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
REDACTED

	var reset5h, reset7d *time.Time
	if ts, err := strconv.ParseInt(reset5hStr, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		reset5h = &t
REDACTED
	if ts, err := strconv.ParseInt(reset7dStr, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		reset7d = &t
REDACTED

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
	REDACTED
	case is5hExceeded:
		chosen = reset5h
	case is7dExceeded:
		chosen = reset7d
	default:
		// Neither flag clearly exceeded — pick the sooner reset as best guess
		chosen = pickSooner(reset5h, reset7d)
REDACTED

	if chosen == nil {
		return nil
REDACTED
	return &anthropic429Result{resetAt: *chosen, fiveHourReset: reset5hREDACTED
REDACTED

// isAnthropicWindowExceeded checks whether a given Anthropic rate-limit window
// (e.g. "5h" or "7d") has been exceeded, using utilization and surpassed-threshold headers.
func isAnthropicWindowExceeded(headers http.Header, window string) bool {
	prefix := "anthropic-ratelimit-unified-" + window + "-"

	// Check surpassed-threshold first (most explicit signal)
	if st := headers.Get(prefix + "surpassed-threshold"); strings.EqualFold(st, "true") {
		return true
REDACTED

	// Fall back to utilization >= 1.0
	if utilStr := headers.Get(prefix + "utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil && util >= 1.0-1e-9 {
			// Use a small epsilon to handle floating point: treat 0.9999999... as >= 1.0
			return true
	REDACTED
REDACTED

	return false
REDACTED

// pickSooner returns whichever of the two time pointers is earlier.
// If only one is non-nil, it is returned. If both are nil, returns nil.
func pickSooner(a, b *time.Time) *time.Time {
	switch {
	case a != nil && b != nil:
		if a.Before(*b) {
			return a
	REDACTED
		return b
	case a != nil:
		return a
	default:
		return b
REDACTED
REDACTED

func (s *RateLimitService) persistOpenAICodexSnapshot(ctx context.Context, account *Account, headers http.Header) {
	if s == nil || s.accountRepo == nil || account == nil || headers == nil {
		return
REDACTED
	// spark 影子的 codex_* 仅由 QueryUsage(/wham/usage bengalfox 道)更新,不能被 /responses 的
	// x-codex-* 全局头快照污染(外审第7轮 P1,与 updateCodexUsageSnapshot 同口径)。
	if account.IsShadow() {
		return
REDACTED
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return
REDACTED
	updates := buildCodexUsageExtraUpdates(snapshot, time.Now())
	if len(updates) == 0 {
		return
REDACTED
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("openai_codex_snapshot_persist_failed", "account_id", account.ID, "error", err)
REDACTED
REDACTED

// parseOpenAIRateLimitResetTime 解析 OpenAI 格式的 429 响应，返回重置时间的 Unix 时间戳
// OpenAI 的 usage_limit_reached 错误格式：
//
//	{
//	  "error": {
//	    "message": "The usage limit has been reached",
//	    "type": "usage_limit_reached",
//	    "resets_at": 1769404154,
//	    "resets_in_seconds": 133107
//	  REDACTED
//REDACTED
func parseOpenAIRateLimitResetTime(body []byte) *int64 {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
REDACTED

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		return nil
REDACTED

	// 检查是否为 usage_limit_reached 或 rate_limit_exceeded 类型
	errType, _ := errObj["type"].(string)
	if errType != "usage_limit_reached" && errType != "rate_limit_exceeded" {
		return nil
REDACTED

	// 优先使用 resets_at（Unix 时间戳）
	if resetsAt, ok := errObj["resets_at"].(float64); ok {
		ts := int64(resetsAt)
		return &ts
REDACTED
	if resetsAt, ok := errObj["resets_at"].(string); ok {
		if ts, err := strconv.ParseInt(resetsAt, 10, 64); err == nil {
			return &ts
	REDACTED
REDACTED

	// 如果没有 resets_at，尝试使用 resets_in_seconds
	if resetsInSeconds, ok := errObj["resets_in_seconds"].(float64); ok {
		ts := time.Now().Unix() + int64(resetsInSeconds)
		return &ts
REDACTED
	if resetsInSeconds, ok := errObj["resets_in_seconds"].(string); ok {
		if sec, err := strconv.ParseInt(resetsInSeconds, 10, 64); err == nil {
			ts := time.Now().Unix() + sec
			return &ts
	REDACTED
REDACTED

	return nil
REDACTED

func parseOpenAIRateLimitPlanType(body []byte) string {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
REDACTED

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		return ""
REDACTED

	errType, _ := errObj["type"].(string)
	if errType != "usage_limit_reached" && errType != "rate_limit_exceeded" {
		return ""
REDACTED

	planType, _ := errObj["plan_type"].(string)
	return strings.ToLower(strings.TrimSpace(planType))
REDACTED

func persistOpenAI429PlanType(ctx context.Context, repo AccountRepository, account *Account, body []byte) {
	if repo == nil || account == nil || account.Platform != PlatformOpenAI {
		return
REDACTED
	// spark 影子账号恒不持凭据:即便收到带 plan_type 的 429,也不能把 plan_type 写进影子 credentials
	// ——该路径走 repo.BulkUpdate 直写、不经 persistAccountCredentials 守卫(外审第7轮 P1)。
	// plan_type 由母账号在自己的请求上维护,影子跳过。
	if account.IsCredentialShadow() {
		return
REDACTED

	planType := parseOpenAIRateLimitPlanType(body)
	if planType == "" {
		return
REDACTED

	current := strings.TrimSpace(account.GetCredential("plan_type"))
	if strings.EqualFold(current, planType) {
		return
REDACTED

	if _, err := repo.BulkUpdate(ctx, []int64{account.IDREDACTED, AccountBulkUpdate{
REDACTED"plan_type": planTypeREDACTED,
REDACTED); err != nil {
		slog.Warn("openai_429_plan_type_sync_failed", "account_id", account.ID, "plan_type", planType, "error", err)
		return
REDACTED

	if account.Credentials == nil {
		account.Credentials = make(map[string]any, 1)
REDACTED
	account.Credentials["plan_type"] = planType
	slog.Info("openai_429_plan_type_synced", "account_id", account.ID, "previous_plan_type", current, "plan_type", planType)
REDACTED

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
	REDACTED
REDACTED
	// 回退到配置文件
	if settings == nil {
		cooldown := s.cfg.RateLimit.OverloadCooldownMinutes
		if cooldown <= 0 {
			cooldown = 10
	REDACTED
		settings = &OverloadCooldownSettings{Enabled: true, CooldownMinutes: cooldownREDACTED
REDACTED

	if !settings.Enabled {
		slog.Info("account_529_ignored", "account_id", account.ID, "reason", "overload_cooldown_disabled")
		return
REDACTED

	cooldownMinutes := settings.CooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = 10
REDACTED

	until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
	s.notifyAccountSchedulingBlocked(account, until, "529")
	if err := s.accountRepo.SetOverloaded(ctx, account.ID, until); err != nil {
		slog.Warn("overload_set_failed", "account_id", account.ID, "error", err)
		return
REDACTED

	slog.Info("account_overloaded", "account_id", account.ID, "until", until)
REDACTED

// UpdateSessionWindow 从成功响应更新5h窗口状态
func (s *RateLimitService) UpdateSessionWindow(ctx context.Context, account *Account, headers http.Header) {
	status := headers.Get("anthropic-ratelimit-unified-5h-status")
	if status == "" {
		return
REDACTED

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
		REDACTED
			end := time.Unix(ts, 0)
			// 校验时间戳是否在合理范围内（不早于 5h 前，不晚于 7 天后）
			minAllowed := time.Now().Add(-5 * time.Hour)
			maxAllowed := time.Now().Add(7 * 24 * time.Hour)
			if end.Before(minAllowed) || end.After(maxAllowed) {
				slog.Warn("account_session_window_header_out_of_range", "account_id", account.ID, "raw_reset", resetStr, "parsed_end", end)
		REDACTED else if needInitWindow || account.SessionWindowEnd == nil || !end.Equal(*account.SessionWindowEnd) {
				// 窗口需要初始化，或者真实重置时间与已存储的不同，则更新
				start := end.Add(-5 * time.Hour)
				windowStart = &start
				windowEnd = &end
				slog.Info("account_session_window_from_header", "account_id", account.ID, "window_start", start, "window_end", end, "status", status)
		REDACTED
	REDACTED else {
			slog.Warn("account_session_window_header_parse_failed", "account_id", account.ID, "raw_reset", resetStr, "error", err)
	REDACTED
REDACTED

	// 回退：如果没有真实重置时间且需要初始化窗口，使用预测
	if windowEnd == nil && needInitWindow && (status == "allowed" || status == "allowed_warning") {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		end := start.Add(5 * time.Hour)
		windowStart = &start
		windowEnd = &end
		slog.Info("account_session_window_initialized", "account_id", account.ID, "window_start", start, "window_end", end, "status", status)
REDACTED

	// 窗口重置时清除旧的 utilization 和被动采样数据，避免残留上个窗口的数据
	if windowEnd != nil && needInitWindow {
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			"session_window_utilization":      nil,
			"passive_usage_7d_utilization":    nil,
			"passive_usage_7d_reset":          nil,
			"passive_usage_7d_oi_utilization": nil,
			"passive_usage_7d_oi_reset":       nil,
			"passive_usage_sampled_at":        nil,
	REDACTED)
REDACTED

	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, windowStart, windowEnd, status); err != nil {
		slog.Warn("session_window_update_failed", "account_id", account.ID, "error", err)
REDACTED

	// 被动采样：从响应头收集 5h + 7d + 7d_oi utilization，合并为一次 DB 写入
	s.samplePassiveUsageFromHeaders(ctx, account, headers)

	// 如果状态为allowed且之前有限流，说明窗口已重置，清除限流状态
	if status == "allowed" && account.IsRateLimited() {
		if err := s.ClearRateLimit(ctx, account.ID); err != nil {
			slog.Warn("rate_limit_clear_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED
REDACTED

// samplePassiveUsageFromHeaders 从 Anthropic 响应头收集 5h/7d/7d_oi 的
// utilization 与 reset 被动采样数据，合并为一次 Extra 写入。无数据时不写。
func (s *RateLimitService) samplePassiveUsageFromHeaders(ctx context.Context, account *Account, headers http.Header) {
	extraUpdates := make(map[string]any, 6)
	// 5h utilization（0-1 小数），供 estimateSetupTokenUsage 使用
	if utilStr := headers.Get("anthropic-ratelimit-unified-5h-utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil {
			extraUpdates["session_window_utilization"] = util
	REDACTED
REDACTED
	// 7d utilization（0-1 小数）
	if utilStr := headers.Get("anthropic-ratelimit-unified-7d-utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil {
			extraUpdates["passive_usage_7d_utilization"] = util
	REDACTED
REDACTED
	// 7d reset timestamp
	if resetStr := headers.Get("anthropic-ratelimit-unified-7d-reset"); resetStr != "" {
		if ts, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			if ts > 1e11 {
				ts = ts / 1000
		REDACTED
			extraUpdates["passive_usage_7d_reset"] = ts
	REDACTED
REDACTED
	// 7d_oi (Fable 专属 7d 窗口) utilization（0-1 小数）
	if utilStr := headers.Get("anthropic-ratelimit-unified-7d_oi-utilization"); utilStr != "" {
		if util, err := strconv.ParseFloat(utilStr, 64); err == nil {
			extraUpdates["passive_usage_7d_oi_utilization"] = util
	REDACTED
REDACTED
	// 7d_oi reset timestamp
	if resetStr := headers.Get("anthropic-ratelimit-unified-7d_oi-reset"); resetStr != "" {
		if ts, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			if ts > 1e11 {
				ts = ts / 1000
		REDACTED
			extraUpdates["passive_usage_7d_oi_reset"] = ts
	REDACTED
REDACTED
	if len(extraUpdates) > 0 {
		extraUpdates["passive_usage_sampled_at"] = time.Now().UTC().Format(time.RFC3339)
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, extraUpdates); err != nil {
			slog.Warn("passive_usage_update_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED
REDACTED

// ClearRateLimit 清除账号的限流状态
func (s *RateLimitService) ClearRateLimit(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearRateLimit(ctx, accountID); err != nil {
		return err
REDACTED
	if err := s.accountRepo.ClearAntigravityQuotaScopes(ctx, accountID); err != nil {
		return err
REDACTED
	if err := s.accountRepo.ClearModelRateLimits(ctx, accountID); err != nil {
		return err
REDACTED
	// 清除限流时一并清理临时不可调度状态，避免周限/窗口重置后仍被本地临时状态阻断。
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		return err
REDACTED
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
			slog.Warn("temp_unsched_cache_delete_failed", "account_id", accountID, "error", err)
	REDACTED
REDACTED
	s.ResetOpenAI403Counter(ctx, accountID)
	s.notifyAccountSchedulingBlockCleared(accountID)
	return nil
REDACTED

func (s *RateLimitService) ResetOpenAI403Counter(ctx context.Context, accountID int64) {
	if s == nil || s.openAI403CounterCache == nil || accountID <= 0 {
		return
REDACTED
	if err := s.openAI403CounterCache.ResetOpenAI403Count(ctx, accountID); err != nil {
		slog.Warn("openai_403_reset_failed", "account_id", accountID, "error", err)
REDACTED
REDACTED

// RecoverAccountState 按需恢复账号的可恢复运行时状态。
func (s *RateLimitService) RecoverAccountState(ctx context.Context, accountID int64, options AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED

	result := &SuccessfulTestRecoveryResult{REDACTED
	if account.Status == StatusError {
		if err := s.accountRepo.ClearError(ctx, accountID); err != nil {
			return nil, err
	REDACTED
		result.ClearedError = true
		if options.InvalidateToken && s.tokenCacheInvalidator != nil && account.IsOAuth() {
			if invalidateErr := s.tokenCacheInvalidator.InvalidateToken(ctx, account); invalidateErr != nil {
				slog.Warn("recover_account_state_invalidate_token_failed", "account_id", accountID, "error", invalidateErr)
		REDACTED
	REDACTED
REDACTED

	if hasRecoverableRuntimeState(account) {
		if err := s.ClearRateLimit(ctx, accountID); err != nil {
			return nil, err
	REDACTED
		result.ClearedRateLimit = true
REDACTED
	if result.ClearedError || result.ClearedRateLimit {
		s.ResetOpenAI403Counter(ctx, accountID)
		if result.ClearedError && !result.ClearedRateLimit {
			s.notifyAccountSchedulingBlockCleared(accountID)
	REDACTED
REDACTED

	return result, nil
REDACTED

// RecoverAccountAfterSuccessfulTest 将一次成功测试视为正常请求，
// 按需恢复 error / rate-limit / overload / temp-unsched / model-rate-limit 等运行时状态。
func (s *RateLimitService) RecoverAccountAfterSuccessfulTest(ctx context.Context, accountID int64) (*SuccessfulTestRecoveryResult, error) {
	return s.RecoverAccountState(ctx, accountID, AccountRecoveryOptions{REDACTED)
REDACTED

func (s *RateLimitService) ClearTempUnschedulable(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		return err
REDACTED
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
			slog.Warn("temp_unsched_cache_delete_failed", "account_id", accountID, "error", err)
	REDACTED
REDACTED
	// 同时清除模型级别限流
	if err := s.accountRepo.ClearModelRateLimits(ctx, accountID); err != nil {
		slog.Warn("clear_model_rate_limits_on_temp_unsched_reset_failed", "account_id", accountID, "error", err)
REDACTED
	s.notifyAccountSchedulingBlockCleared(accountID)
	return nil
REDACTED

func hasRecoverableRuntimeState(account *Account) bool {
	if account == nil {
		return false
REDACTED
	if account.RateLimitedAt != nil || account.RateLimitResetAt != nil || account.OverloadUntil != nil || account.TempUnschedulableUntil != nil {
		return true
REDACTED
	if len(account.Extra) == 0 {
		return false
REDACTED
	return hasNonEmptyMapValue(account.Extra, "model_rate_limits") ||
		hasNonEmptyMapValue(account.Extra, "antigravity_quota_scopes")
REDACTED

func hasNonEmptyMapValue(extra map[string]any, key string) bool {
	raw, ok := extra[key]
	if !ok || raw == nil {
		return false
REDACTED
	switch typed := raw.(type) {
	case map[string]any:
		return len(typed) > 0
	case map[string]string:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return true
REDACTED
REDACTED

func (s *RateLimitService) GetTempUnschedStatus(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	now := time.Now().Unix()
	if s.tempUnschedCache != nil {
		state, err := s.tempUnschedCache.GetTempUnsched(ctx, accountID)
		if err != nil {
			return nil, err
	REDACTED
		if state != nil && state.UntilUnix > now {
			return state, nil
	REDACTED
REDACTED

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	if account.TempUnschedulableUntil == nil {
		return nil, nil
REDACTED
	if account.TempUnschedulableUntil.Unix() <= now {
		return nil, nil
REDACTED

	state := &TempUnschedState{
		UntilUnix: account.TempUnschedulableUntil.Unix(),
REDACTED

	if account.TempUnschedulableReason != "" {
		var parsed TempUnschedState
		if err := json.Unmarshal([]byte(account.TempUnschedulableReason), &parsed); err == nil {
			if parsed.UntilUnix == 0 {
				parsed.UntilUnix = state.UntilUnix
		REDACTED
			state = &parsed
	REDACTED else {
			state.ErrorMessage = account.TempUnschedulableReason
	REDACTED
REDACTED

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, accountID, state); err != nil {
			slog.Warn("temp_unsched_cache_set_failed", "account_id", accountID, "error", err)
	REDACTED
REDACTED

	return state, nil
REDACTED

func (s *RateLimitService) HandleTempUnschedulable(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil {
		return false
REDACTED
	if !account.ShouldHandleErrorCode(statusCode) {
		return false
REDACTED
	return s.tryTempUnschedulable(ctx, account, statusCode, responseBody)
REDACTED

func (s *RateLimitService) HandleOpenAIImageRateLimit(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) bool {
	if s == nil || account == nil || s.accountRepo == nil {
		return false
REDACTED
	if account.Platform != PlatformOpenAI {
		return false
REDACTED
	if !account.ShouldHandleErrorCode(statusCode) {
		slog.Info("openai_image_rate_limit_skipped_by_error_code_policy", "account_id", account.ID, "status_code", statusCode)
		return false
REDACTED
	if !isOpenAIImageRateLimitError(statusCode, responseBody) {
		return false
REDACTED

	resetAt := openAIImageRateLimitResetAt(headers, responseBody)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, openAIImageGenerationRateLimitKey, resetAt, openAIImageRateLimitReason); err != nil {
		slog.Warn("openai_image_rate_limit_set_model_rate_limit_failed", "account_id", account.ID, "scope", openAIImageGenerationRateLimitKey, "error", err)
		return true
REDACTED
	slog.Info("openai_image_rate_limited", "account_id", account.ID, "scope", openAIImageGenerationRateLimitKey, "reset_at", resetAt, "reset_in", time.Until(resetAt).Truncate(time.Second))
	return true
REDACTED

func isOpenAIImageRateLimitError(statusCode int, body []byte) bool {
	if statusCode != http.StatusTooManyRequests || len(body) == 0 {
		return false
REDACTED
	lower := strings.ToLower(string(body))
	for _, marker := range []string{
		"for limit gpt-image",
		"input-images per min",
		"gpt-image-2-codex",
		"gpt-image",
REDACTED {
		if strings.Contains(lower, marker) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func openAIImageRateLimitResetAt(headers http.Header, body []byte) time.Time {
	now := time.Now()
	if resetAt := parseRetryAfterResetTime(headers, now); resetAt != nil && resetAt.After(now) {
		return *resetAt
REDACTED
	if resetAt := calculateOpenAI429ResetTime(headers); resetAt != nil && resetAt.After(now) {
		return *resetAt
REDACTED
	if resetUnix := parseOpenAIRateLimitResetTime(body); resetUnix != nil {
		if resetAt := time.Unix(*resetUnix, 0); resetAt.After(now) {
			return resetAt
	REDACTED
REDACTED
	if cooldown := parseOpenAIImageTryAgainCooldown(body); cooldown > 0 {
		return now.Add(cooldown)
REDACTED
	return now.Add(openAIImageRateLimitDefaultCooldown)
REDACTED

func parseRetryAfterResetTime(headers http.Header, now time.Time) *time.Time {
	if headers == nil {
		return nil
REDACTED
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
REDACTED
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		resetAt := now.Add(time.Duration(seconds * float64(time.Second)))
		return &resetAt
REDACTED
	if parsed, err := http.ParseTime(raw); err == nil {
		return &parsed
REDACTED
	return nil
REDACTED

func parseOpenAIImageTryAgainCooldown(body []byte) time.Duration {
	if len(body) == 0 {
		return 0
REDACTED
	match := openAIImageTryAgainPattern.FindSubmatch(body)
	if len(match) != 3 {
		return 0
REDACTED
	value, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil || value <= 0 {
		return 0
REDACTED
	switch strings.ToLower(string(match[2])) {
	case "ms":
		return time.Duration(value * float64(time.Millisecond))
	case "s", "sec", "secs", "second", "seconds":
		return time.Duration(value * float64(time.Second))
	case "m", "min", "mins", "minute", "minutes":
		return time.Duration(value * float64(time.Minute))
	default:
		return 0
REDACTED
REDACTED

const upstreamModelNotFoundCooldown = 30 * time.Minute
const upstreamModelNotFoundReason = "upstream_404_model_not_found"
const tempUnschedBodyMaxBytes = 64 << 10
const tempUnschedMessageMaxBytes = 2048

func (s *RateLimitService) HandleUpstreamModelNotFound(ctx context.Context, account *Account, requestedModel string, statusCode int, responseBody []byte) bool {
	if s == nil || account == nil || s.accountRepo == nil {
		return false
REDACTED
	if !account.ShouldHandleErrorCode(statusCode) {
		return false
REDACTED
	if !isUpstreamModelNotFoundError(statusCode, responseBody) {
		return false
REDACTED
	modelKey := modelRateLimitKeyForUpstreamModelNotFound(ctx, account, requestedModel)
	if modelKey == "" {
		return false
REDACTED
	resetAt := time.Now().Add(upstreamModelNotFoundCooldown)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, modelKey, resetAt, upstreamModelNotFoundReason); err != nil {
		slog.Warn("upstream_model_not_found_set_model_rate_limit_failed", "account_id", account.ID, "model", modelKey, "error", err)
		return true
REDACTED
	slog.Info("upstream_model_not_found_model_rate_limited", "account_id", account.ID, "model", modelKey, "reset_at", resetAt)
	return true
REDACTED

func modelRateLimitKeyForUpstreamModelNotFound(ctx context.Context, account *Account, requestedModel string) string {
	modelKey := strings.TrimSpace(requestedModel)
	if account == nil || modelKey == "" {
		return modelKey
REDACTED
	if account.Platform == PlatformAntigravity {
		if resolved := strings.TrimSpace(resolveFinalAntigravityModelKey(ctx, account, modelKey)); resolved != "" {
			return resolved
	REDACTED
		return modelKey
REDACTED
	if mapped := strings.TrimSpace(account.GetMappedModel(modelKey)); mapped != "" {
		return mapped
REDACTED
	return modelKey
REDACTED

func (s *RateLimitService) tryTempUnschedulable(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil {
		return false
REDACTED
	if !account.IsTempUnschedulableEnabled() {
		return false
REDACTED
	// 401 首次命中可临时不可调度（给 token 刷新窗口）；
	// 若历史上已因 401 进入过临时不可调度，则本次应升级为 error（返回 false 交由默认错误逻辑处理）。
	// Antigravity 跳过：其 401 由 applyErrorPolicy 的 temp_unschedulable_rules 自行控制，无需升级逻辑。
	if statusCode == http.StatusUnauthorized && account.Platform != PlatformAntigravity {
		reason := account.TempUnschedulableReason
		// 缓存可能没有 reason，从 DB 回退读取
		if reason == "" {
			if dbAcc, err := s.accountRepo.GetByID(ctx, account.ID); err == nil && dbAcc != nil {
				reason = dbAcc.TempUnschedulableReason
		REDACTED
	REDACTED
		if wasTempUnschedByStatusCode(reason, statusCode) {
			slog.Info("401_escalated_to_error", "account_id", account.ID,
				"reason", "previous temp-unschedulable was also 401")
			return false
	REDACTED
REDACTED
	rules := account.GetTempUnschedulableRules()
	if len(rules) == 0 {
		return false
REDACTED
	if statusCode <= 0 || len(responseBody) == 0 {
		return false
REDACTED

	body := responseBody
	if len(body) > tempUnschedBodyMaxBytes {
		body = body[:tempUnschedBodyMaxBytes]
REDACTED
	bodyLower := strings.ToLower(string(body))

	for idx, rule := range rules {
		if rule.ErrorCode != statusCode || len(rule.Keywords) == 0 {
			continue
	REDACTED
		matchedKeyword := matchTempUnschedKeyword(bodyLower, rule.Keywords)
		if matchedKeyword == "" {
			continue
	REDACTED

		if s.triggerTempUnschedulable(ctx, account, rule, idx, statusCode, matchedKeyword, responseBody) {
			return true
	REDACTED
REDACTED

	return false
REDACTED

func wasTempUnschedByStatusCode(reason string, statusCode int) bool {
	if statusCode <= 0 {
		return false
REDACTED
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
REDACTED

	var state TempUnschedState
	if err := json.Unmarshal([]byte(reason), &state); err != nil {
		return false
REDACTED
	return state.StatusCode == statusCode
REDACTED

func matchTempUnschedKeyword(bodyLower string, keywords []string) string {
	if bodyLower == "" {
		return ""
REDACTED
	for _, keyword := range keywords {
		k := strings.TrimSpace(keyword)
		if k == "" {
			continue
	REDACTED
		if strings.Contains(bodyLower, strings.ToLower(k)) {
			return k
	REDACTED
REDACTED
	return ""
REDACTED

func (s *RateLimitService) triggerTempUnschedulable(ctx context.Context, account *Account, rule TempUnschedulableRule, ruleIndex int, statusCode int, matchedKeyword string, responseBody []byte) bool {
	if account == nil {
		return false
REDACTED
	if rule.DurationMinutes <= 0 {
		return false
REDACTED

	now := time.Now()
	until := now.Add(time.Duration(rule.DurationMinutes) * time.Minute)

	state := &TempUnschedState{
		UntilUnix:       until.Unix(),
		TriggeredAtUnix: now.Unix(),
		StatusCode:      statusCode,
		MatchedKeyword:  matchedKeyword,
		RuleIndex:       ruleIndex,
		ErrorMessage:    truncateTempUnschedMessage(responseBody, tempUnschedMessageMaxBytes),
REDACTED

	reason := ""
	if raw, err := json.Marshal(state); err == nil {
		reason = string(raw)
REDACTED
	if reason == "" {
		reason = strings.TrimSpace(state.ErrorMessage)
REDACTED

	s.notifyAccountSchedulingBlocked(account, until, "temp_unschedulable")
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("temp_unsched_set_failed", "account_id", account.ID, "error", err)
		return false
REDACTED

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			slog.Warn("temp_unsched_cache_set_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	slog.Info("account_temp_unschedulable", "account_id", account.ID, "until", until, "rule_index", ruleIndex, "status_code", statusCode)
	return true
REDACTED

func truncateTempUnschedMessage(body []byte, maxBytes int) string {
	if maxBytes <= 0 || len(body) == 0 {
		return ""
REDACTED
	if len(body) > maxBytes {
		body = body[:maxBytes]
REDACTED
	return strings.TrimSpace(string(body))
REDACTED

// HandleStreamTimeout 处理流数据超时
// 根据系统设置决定是否标记账户为临时不可调度或错误状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleStreamTimeout(ctx context.Context, account *Account, model string) bool {
	if account == nil {
		return false
REDACTED

	// 获取系统设置
	if s.settingService == nil {
		slog.Warn("stream_timeout_setting_service_missing", "account_id", account.ID)
		return false
REDACTED

	settings, err := s.settingService.GetStreamTimeoutSettings(ctx)
	if err != nil {
		slog.Warn("stream_timeout_get_settings_failed", "account_id", account.ID, "error", err)
		return false
REDACTED

	if !settings.Enabled {
		return false
REDACTED

	if settings.Action == StreamTimeoutActionNone {
		return false
REDACTED

	// 增加超时计数
	var count int64 = 1
	if s.timeoutCounterCache != nil {
		count, err = s.timeoutCounterCache.IncrementTimeoutCount(ctx, account.ID, settings.ThresholdWindowMinutes)
		if err != nil {
			slog.Warn("stream_timeout_increment_count_failed", "account_id", account.ID, "error", err)
			// 继续处理，使用 count=1
			count = 1
	REDACTED
REDACTED

	slog.Info("stream_timeout_count", "account_id", account.ID, "count", count, "threshold", settings.ThresholdCount, "window_minutes", settings.ThresholdWindowMinutes, "model", model)

	// 检查是否达到阈值
	if count < int64(settings.ThresholdCount) {
		return false
REDACTED

	// 达到阈值，执行相应操作
	switch settings.Action {
	case StreamTimeoutActionTempUnsched:
		return s.triggerStreamTimeoutTempUnsched(ctx, account, settings, model)
	case StreamTimeoutActionError:
		return s.triggerStreamTimeoutError(ctx, account, model)
	default:
		return false
REDACTED
REDACTED

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
REDACTED

	reason := ""
	if raw, err := json.Marshal(state); err == nil {
		reason = string(raw)
REDACTED
	if reason == "" {
		reason = state.ErrorMessage
REDACTED

	s.notifyAccountSchedulingBlocked(account, until, "stream_timeout_temp_unschedulable")
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("stream_timeout_set_temp_unsched_failed", "account_id", account.ID, "error", err)
		return false
REDACTED

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			slog.Warn("stream_timeout_set_temp_unsched_cache_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			slog.Warn("stream_timeout_reset_count_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	slog.Info("stream_timeout_temp_unschedulable", "account_id", account.ID, "until", until, "model", model)
	return true
REDACTED

// triggerStreamTimeoutError 触发流超时错误状态
func (s *RateLimitService) triggerStreamTimeoutError(ctx context.Context, account *Account, model string) bool {
	errorMsg := "Stream data interval timeout (repeated failures) for model: " + model

	s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "stream_timeout_error")
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		slog.Warn("stream_timeout_set_error_failed", "account_id", account.ID, "error", err)
		return false
REDACTED

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			slog.Warn("stream_timeout_reset_count_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	slog.Warn("stream_timeout_account_error", "account_id", account.ID, "model", model)
	return true
REDACTED
