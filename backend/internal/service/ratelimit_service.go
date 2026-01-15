package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// RateLimitService 处理限流和过载状态管理
type RateLimitService struct {
	accountRepo         AccountRepository
	usageRepo           UsageLogRepository
	cfg                 *config.Config
	geminiQuotaService  *GeminiQuotaService
	tempUnschedCache    TempUnschedCache
	timeoutCounterCache TimeoutCounterCache
	settingService      *SettingService
	usageCacheMu        sync.RWMutex
	usageCache          map[int64]*geminiUsageCacheEntry
REDACTED

type geminiUsageCacheEntry struct {
	windowStart time.Time
	cachedAt    time.Time
	totals      GeminiUsageTotals
REDACTED

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
REDACTED
REDACTED

// SetTimeoutCounterCache 设置超时计数器缓存（可选依赖）
func (s *RateLimitService) SetTimeoutCounterCache(cache TimeoutCounterCache) {
	s.timeoutCounterCache = cache
REDACTED

// SetSettingService 设置系统设置服务（可选依赖）
func (s *RateLimitService) SetSettingService(settingService *SettingService) {
	s.settingService = settingService
REDACTED

// HandleUpstreamError 处理上游错误响应，标记账号状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) (shouldDisable bool) {
	// apikey 类型账号：检查自定义错误码配置
	// 如果启用且错误码不在列表中，则不处理（不停止调度、不标记限流/过载）
	customErrorCodesEnabled := account.IsCustomErrorCodesEnabled()
	if !account.ShouldHandleErrorCode(statusCode) {
		log.Printf("Account %d: error %d skipped (not in custom error codes)", account.ID, statusCode)
		return false
REDACTED

	tempMatched := s.tryTempUnschedulable(ctx, account, statusCode, responseBody)
	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if upstreamMsg != "" {
		upstreamMsg = truncateForLog([]byte(upstreamMsg), 512)
REDACTED

	switch statusCode {
	case 401:
		// 认证失败：停止调度，记录错误
		msg := "Authentication failed (401): invalid or expired credentials"
		if upstreamMsg != "" {
			msg = "Authentication failed (401): " + upstreamMsg
	REDACTED
		s.handleAuthError(ctx, account, msg)
		shouldDisable = true
	case 402:
		// 支付要求：余额不足或计费问题，停止调度
		msg := "Payment required (402): insufficient balance or billing issue"
		if upstreamMsg != "" {
			msg = "Payment required (402): " + upstreamMsg
	REDACTED
		s.handleAuthError(ctx, account, msg)
		shouldDisable = true
	case 403:
		// 禁止访问：停止调度，记录错误
		msg := "Access forbidden (403): account may be suspended or lack permissions"
		if upstreamMsg != "" {
			msg = "Access forbidden (403): " + upstreamMsg
	REDACTED
		s.handleAuthError(ctx, account, msg)
		shouldDisable = true
	case 429:
		s.handle429(ctx, account, headers)
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
			log.Printf("Account %d received upstream error %d", account.ID, statusCode)
			shouldDisable = false
	REDACTED
REDACTED

	if tempMatched {
		return true
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
				stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil)
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
				log.Printf("[Gemini PreCheck] Account %d reached daily quota (%d/%d), skip until %v", account.ID, used, limit, resetAt)
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
			stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, start, now, 0, 0, account.ID, 0, nil)
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
				log.Printf("[Gemini PreCheck] Account %d reached minute quota (%d/%d), skip until %v", account.ID, used, limit, resetAt)
				return false, nil
		REDACTED
	REDACTED
REDACTED

	return true, nil
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
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		log.Printf("SetError failed for account %d: %v", account.ID, err)
		return
REDACTED
	log.Printf("Account %d disabled due to auth error: %s", account.ID, errorMsg)
REDACTED

// handleCustomErrorCode 处理自定义错误码，停止账号调度
func (s *RateLimitService) handleCustomErrorCode(ctx context.Context, account *Account, statusCode int, errorMsg string) {
	msg := "Custom error code " + strconv.Itoa(statusCode) + ": " + errorMsg
	if err := s.accountRepo.SetError(ctx, account.ID, msg); err != nil {
		log.Printf("SetError failed for account %d: %v", account.ID, err)
		return
REDACTED
	log.Printf("Account %d disabled due to custom error code %d: %s", account.ID, statusCode, errorMsg)
REDACTED

// handle429 处理429限流错误
// 解析响应头获取重置时间，标记账号为限流状态
func (s *RateLimitService) handle429(ctx context.Context, account *Account, headers http.Header) {
	// 解析重置时间戳
	resetTimestamp := headers.Get("anthropic-ratelimit-unified-reset")
	if resetTimestamp == "" {
		// 没有重置时间，使用默认5分钟
		resetAt := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
	REDACTED
		return
REDACTED

	// 解析Unix时间戳
	ts, err := strconv.ParseInt(resetTimestamp, 10, 64)
	if err != nil {
		log.Printf("Parse reset timestamp failed: %v", err)
		resetAt := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
	REDACTED
		return
REDACTED

	resetAt := time.Unix(ts, 0)

	// 标记限流状态
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
		return
REDACTED

	// 根据重置时间反推5h窗口
	windowEnd := resetAt
	windowStart := resetAt.Add(-5 * time.Hour)
	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
		log.Printf("UpdateSessionWindow failed for account %d: %v", account.ID, err)
REDACTED

	log.Printf("Account %d rate limited until %v", account.ID, resetAt)
REDACTED

// handle529 处理529过载错误
// 根据配置设置过载冷却时间
func (s *RateLimitService) handle529(ctx context.Context, account *Account) {
	cooldownMinutes := s.cfg.RateLimit.OverloadCooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = 10 // 默认10分钟
REDACTED

	until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
	if err := s.accountRepo.SetOverloaded(ctx, account.ID, until); err != nil {
		log.Printf("SetOverloaded failed for account %d: %v", account.ID, err)
		return
REDACTED

	log.Printf("Account %d overloaded until %v", account.ID, until)
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

	if needInitWindow && (status == "allowed" || status == "allowed_warning") {
		// 预测时间窗口：从当前时间的整点开始，+5小时为结束
		// 例如：现在是 14:30，窗口为 14:00 ~ 19:00
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		end := start.Add(5 * time.Hour)
		windowStart = &start
		windowEnd = &end
		log.Printf("Account %d: initializing 5h window from %v to %v (status: %s)", account.ID, start, end, status)
REDACTED

	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, windowStart, windowEnd, status); err != nil {
		log.Printf("UpdateSessionWindow failed for account %d: %v", account.ID, err)
REDACTED

	// 如果状态为allowed且之前有限流，说明窗口已重置，清除限流状态
	if status == "allowed" && account.IsRateLimited() {
		if err := s.ClearRateLimit(ctx, account.ID); err != nil {
			log.Printf("ClearRateLimit failed for account %d: %v", account.ID, err)
	REDACTED
REDACTED
REDACTED

// ClearRateLimit 清除账号的限流状态
func (s *RateLimitService) ClearRateLimit(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearRateLimit(ctx, accountID); err != nil {
		return err
REDACTED
	return s.accountRepo.ClearAntigravityQuotaScopes(ctx, accountID)
REDACTED

func (s *RateLimitService) ClearTempUnschedulable(ctx context.Context, accountID int64) error {
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		return err
REDACTED
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
			log.Printf("DeleteTempUnsched failed for account %d: %v", accountID, err)
	REDACTED
REDACTED
	return nil
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
			log.Printf("SetTempUnsched failed for account %d: %v", accountID, err)
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

const tempUnschedBodyMaxBytes = 64 << 10
const tempUnschedMessageMaxBytes = 2048

func (s *RateLimitService) tryTempUnschedulable(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil {
		return false
REDACTED
	if !account.IsTempUnschedulableEnabled() {
		return false
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

	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		log.Printf("SetTempUnschedulable failed for account %d: %v", account.ID, err)
		return false
REDACTED

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			log.Printf("SetTempUnsched cache failed for account %d: %v", account.ID, err)
	REDACTED
REDACTED

	log.Printf("Account %d temp unschedulable until %v (rule %d, code %d)", account.ID, until, ruleIndex, statusCode)
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
		log.Printf("[StreamTimeout] settingService not configured, skipping timeout handling for account %d", account.ID)
		return false
REDACTED

	settings, err := s.settingService.GetStreamTimeoutSettings(ctx)
	if err != nil {
		log.Printf("[StreamTimeout] Failed to get settings: %v", err)
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
			log.Printf("[StreamTimeout] Failed to increment timeout count for account %d: %v", account.ID, err)
			// 继续处理，使用 count=1
			count = 1
	REDACTED
REDACTED

	log.Printf("[StreamTimeout] Account %d timeout count: %d/%d (window: %d min, model: %s)",
		account.ID, count, settings.ThresholdCount, settings.ThresholdWindowMinutes, model)

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

	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		log.Printf("[StreamTimeout] SetTempUnschedulable failed for account %d: %v", account.ID, err)
		return false
REDACTED

	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			log.Printf("[StreamTimeout] SetTempUnsched cache failed for account %d: %v", account.ID, err)
	REDACTED
REDACTED

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			log.Printf("[StreamTimeout] ResetTimeoutCount failed for account %d: %v", account.ID, err)
	REDACTED
REDACTED

	log.Printf("[StreamTimeout] Account %d marked as temp unschedulable until %v (model: %s)", account.ID, until, model)
	return true
REDACTED

// triggerStreamTimeoutError 触发流超时错误状态
func (s *RateLimitService) triggerStreamTimeoutError(ctx context.Context, account *Account, model string) bool {
	errorMsg := "Stream data interval timeout (repeated failures) for model: " + model

	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		log.Printf("[StreamTimeout] SetError failed for account %d: %v", account.ID, err)
		return false
REDACTED

	// 重置超时计数
	if s.timeoutCounterCache != nil {
		if err := s.timeoutCounterCache.ResetTimeoutCount(ctx, account.ID); err != nil {
			log.Printf("[StreamTimeout] ResetTimeoutCount failed for account %d: %v", account.ID, err)
	REDACTED
REDACTED

	log.Printf("[StreamTimeout] Account %d marked as error (model: %s)", account.ID, model)
	return true
REDACTED
