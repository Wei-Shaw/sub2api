package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// creditsExhaustedKey 是 model_rate_limits 中标记积分耗尽的特殊 key。
	// 与普通模型限流完全同构：通过 SetModelRateLimit / isRateLimitActiveForKey 读写。
	creditsExhaustedKey      = "AICredits"
	creditsExhaustedDuration = 5 * time.Hour

	// credits 降级响应重试参数
	creditsRetryMaxAttempts  = 3
	creditsRetryBaseInterval = 500 * time.Millisecond
)

// creditsRetryableErrorCodes 是降级响应中可重试的错误码集合。
// forbidden 是稳定的封号状态，不属于可恢复的瞬态错误，不重试。
var creditsRetryableErrorCodes = map[string]bool{
	errorCodeUnauthenticated: true,
	errorCodeRateLimited:     true,
	errorCodeNetworkError:    true,
REDACTED

// isAntigravityDegradedResponse 检查 UsageInfo 是否为可重试的降级响应。
// 仅检测 3 个瞬态错误码（unauthenticated/rate_limited/network_error），
// forbidden 是稳定的封号状态，不属于降级。
func isAntigravityDegradedResponse(info *UsageInfo) bool {
	if info == nil || info.ErrorCode == "" {
		return false
REDACTED
	return creditsRetryableErrorCodes[info.ErrorCode]
REDACTED

// checkAccountCredits 通过共享的 AccountUsageService 缓存检查账号是否有足够的 AI Credits。
// 缓存 TTL 不足时会自动从 Google loadCodeAssist API 刷新。
// 检测到降级响应时会清除缓存并重试，最终 fail-open（返回 true）。
func (s *AntigravityGatewayService) checkAccountCredits(
	ctx context.Context, account *Account,
) bool {
	if account == nil || account.ID == 0 {
		return false
REDACTED
	if s.accountUsageService == nil {
		return true // 无 usage service 时不阻断
REDACTED

	usageInfo, err := s.accountUsageService.GetAntigravityCredits(ctx, account)
	if err != nil {
		slog.Error("check_credits: get_credits_failed",
			"account_id", account.ID, "error", err)
		return true // 出错时 fail-open
REDACTED

	// 非降级响应：直接检查积分余额
	if !isAntigravityDegradedResponse(usageInfo) {
		return s.logCreditsResult(account, usageInfo)
REDACTED

	// 降级响应：清除缓存后重试
	return s.retryCreditsOnDegraded(ctx, account, usageInfo)
REDACTED

// retryCreditsOnDegraded 在检测到降级响应后，清除缓存并重试获取 credits。
// 使用指数退避（500ms → 1s → 2s），最多重试 creditsRetryMaxAttempts 次。
// 所有重试失败后 fail-open（返回 true），不做熔断。
func (s *AntigravityGatewayService) retryCreditsOnDegraded(
	ctx context.Context, account *Account, lastInfo *UsageInfo,
) bool {
	for attempt := 1; attempt <= creditsRetryMaxAttempts; attempt++ {
		delay := creditsRetryBaseInterval << (attempt - 1) // 指数退避：500ms, 1s, 2s
		slog.Warn("check_credits: degraded response, retrying",
			"account_id", account.ID,
			"attempt", attempt,
			"max_attempts", creditsRetryMaxAttempts,
			"error_code", lastInfo.ErrorCode,
			"delay", delay,
		)

		select {
		case <-ctx.Done():
			slog.Warn("check_credits: context cancelled during retry, fail-open",
				"account_id", account.ID, "attempt", attempt)
			return true
		case <-time.After(delay):
	REDACTED

		// 清除缓存，强制下次 GetAntigravityCredits 重新拉取
		s.accountUsageService.InvalidateAntigravityCreditsCache(account.ID)

		info, err := s.accountUsageService.GetAntigravityCredits(ctx, account)
		if err != nil {
			slog.Error("check_credits: retry get_credits_failed",
				"account_id", account.ID, "attempt", attempt, "error", err)
			continue
	REDACTED

		// 重试成功（不再是降级响应）：检查积分余额
		if !isAntigravityDegradedResponse(info) {
			slog.Info("check_credits: retry succeeded",
				"account_id", account.ID, "attempt", attempt)
			return s.logCreditsResult(account, info)
	REDACTED
		lastInfo = info
REDACTED

	// 所有重试失败：fail-open，不做熔断
	slog.Warn("check_credits: all retries exhausted, fail-open",
		"account_id", account.ID,
		"last_error_code", lastInfo.ErrorCode,
	)
	return true
REDACTED

// logCreditsResult 检查积分并记录不足日志，返回是否有积分。
func (s *AntigravityGatewayService) logCreditsResult(account *Account, info *UsageInfo) bool {
	hasCredits := hasEnoughCredits(info)
	if !hasCredits {
		slog.Warn("check_credits: insufficient credits",
			"account_id", account.ID)
REDACTED
	return hasCredits
REDACTED

// hasEnoughCredits 检查 UsageInfo 中是否有足够的 GOOGLE_ONE_AI 积分。
// 返回 true 表示积分可用，false 表示积分不足或无积分信息。
func hasEnoughCredits(info *UsageInfo) bool {
	if info == nil || len(info.AICredits) == 0 {
		return false
REDACTED

	for _, credit := range info.AICredits {
		if credit.CreditType == "GOOGLE_ONE_AI" {
			minimum := credit.MinimumBalance
			if minimum <= 0 {
				minimum = 5
		REDACTED
			return credit.Amount >= minimum
	REDACTED
REDACTED

	return false
REDACTED

type antigravity429Category string

const (
	antigravity429Unknown        antigravity429Category = "unknown"
	antigravity429RateLimited    antigravity429Category = "rate_limited"
	antigravity429QuotaExhausted antigravity429Category = "quota_exhausted"
)

var (
	antigravityQuotaExhaustedKeywords = []string{
		"quota_exhausted",
		"quota exhausted",
REDACTED

	creditsExhaustedKeywords = []string{
		"google_one_ai",
		"insufficient credit",
		"insufficient credits",
		"not enough credit",
		"not enough credits",
		"credit exhausted",
		"credits exhausted",
		"credit balance",
		"minimumcreditamountforusage",
		"minimum credit amount for usage",
		"minimum credit",
		"resource has been exhausted",
REDACTED
)

// isCreditsExhausted 检查账号的 AICredits 限流 key 是否生效（积分是否耗尽）。
func (a *Account) isCreditsExhausted() bool {
	if a == nil {
		return false
REDACTED
	return a.isRateLimitActiveForKey(creditsExhaustedKey)
REDACTED

// setCreditsExhausted 标记账号积分耗尽：写入 model_rate_limits["AICredits"] + 更新缓存。
func (s *AntigravityGatewayService) setCreditsExhausted(ctx context.Context, account *Account) {
	if account == nil || account.ID == 0 {
		return
REDACTED
	resetAt := time.Now().Add(creditsExhaustedDuration)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, creditsExhaustedKey, resetAt); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "set credits exhausted failed: account=%d err=%v", account.ID, err)
		return
REDACTED
	s.updateAccountModelRateLimitInCache(ctx, account, creditsExhaustedKey, resetAt)
	logger.LegacyPrintf("service.antigravity_gateway", "credits_exhausted_marked account=%d reset_at=%s",
		account.ID, resetAt.UTC().Format(time.RFC3339))
REDACTED

// clearCreditsExhausted 清除账号的 AICredits 限流 key。
func (s *AntigravityGatewayService) clearCreditsExhausted(ctx context.Context, account *Account) {
	if account == nil || account.ID == 0 || account.Extra == nil {
		return
REDACTED
	rawLimits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return
REDACTED
	if _, exists := rawLimits[creditsExhaustedKey]; !exists {
		return
REDACTED
	delete(rawLimits, creditsExhaustedKey)
	account.Extra[modelRateLimitsKey] = rawLimits
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		modelRateLimitsKey: rawLimits,
REDACTED); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "clear credits exhausted failed: account=%d err=%v", account.ID, err)
REDACTED
	// 同步更新 Redis 调度快照，避免其他节点/请求延迟感知
	if s.schedulerSnapshot != nil {
		_ = s.schedulerSnapshot.UpdateAccountInCache(ctx, account)
REDACTED
REDACTED

// classifyAntigravity429 将 Antigravity 的 429 响应归类为配额耗尽、限流或未知。
func classifyAntigravity429(body []byte) antigravity429Category {
	if len(body) == 0 {
		return antigravity429Unknown
REDACTED
	lowerBody := strings.ToLower(string(body))
	for _, keyword := range antigravityQuotaExhaustedKeywords {
		if strings.Contains(lowerBody, keyword) {
			return antigravity429QuotaExhausted
	REDACTED
REDACTED
	if info := parseAntigravitySmartRetryInfo(body); info != nil && !info.IsModelCapacityExhausted {
		return antigravity429RateLimited
REDACTED
	return antigravity429Unknown
REDACTED

// injectEnabledCreditTypes 在已序列化的 v1internal JSON body 中注入 AI Credits 类型。
func injectEnabledCreditTypes(body []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
REDACTED
	payload["enabledCreditTypes"] = []string{"GOOGLE_ONE_AI"REDACTED
	result, err := json.Marshal(payload)
	if err != nil {
		return nil
REDACTED
	return result
REDACTED

// resolveCreditsOveragesModelKey 解析当前请求对应的 overages 状态模型 key。
func resolveCreditsOveragesModelKey(ctx context.Context, account *Account, upstreamModelName, requestedModel string) string {
	modelKey := strings.TrimSpace(upstreamModelName)
	if modelKey != "" {
		return modelKey
REDACTED
	if account == nil {
		return ""
REDACTED
	modelKey = resolveFinalAntigravityModelKey(ctx, account, requestedModel)
	if strings.TrimSpace(modelKey) != "" {
		return modelKey
REDACTED
	return resolveAntigravityModelKey(requestedModel)
REDACTED

// shouldMarkCreditsExhausted 判断一次 credits 请求失败是否应标记为 credits 耗尽。
// 注意：不再检查 isURLLevelRateLimit。此函数仅在积分重试失败后调用，
// 如果注入 enabledCreditTypes 后仍返回 "Resource has been exhausted"，
// 说明积分也已耗尽，应该标记。clearCreditsExhausted 会在后续成功时自动清除。
func shouldMarkCreditsExhausted(resp *http.Response, respBody []byte, reqErr error) bool {
	if reqErr != nil || resp == nil {
		return false
REDACTED
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
		return false
REDACTED
	if info := parseAntigravitySmartRetryInfo(respBody); info != nil {
		return false
REDACTED
	bodyLower := strings.ToLower(string(respBody))
	for _, keyword := range creditsExhaustedKeywords {
		if strings.Contains(bodyLower, keyword) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

type creditsOveragesRetryResult struct {
	handled bool
	resp    *http.Response
REDACTED

// attemptCreditsOveragesRetry 在确认免费配额耗尽后，尝试注入 AI Credits 继续请求。
func (s *AntigravityGatewayService) attemptCreditsOveragesRetry(
	p antigravityRetryLoopParams,
	baseURL string,
	modelName string,
	waitDuration time.Duration,
	originalStatusCode int,
	respBody []byte,
) *creditsOveragesRetryResult {
	creditsBody := injectEnabledCreditTypes(p.body)
	if creditsBody == nil {
		return &creditsOveragesRetryResult{handled: falseREDACTED
REDACTED

	// Check actual credits balance before attempting retry
	if !s.checkAccountCredits(p.ctx, p.account) {
		s.setCreditsExhausted(p.ctx, p.account)
		modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, modelName, p.requestedModel)
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_no_credits model=%s account=%d (skipping credits retry)",
			p.prefix, modelKey, p.account.ID)
		return &creditsOveragesRetryResult{handled: trueREDACTED
REDACTED

	modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, modelName, p.requestedModel)
	logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 credit_overages_retry model=%s account=%d (injecting enabledCreditTypes)",
		p.prefix, modelKey, p.account.ID)

	creditsReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, creditsBody)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d build_request_err=%v",
			p.prefix, modelKey, p.account.ID, err)
		return &creditsOveragesRetryResult{handled: trueREDACTED
REDACTED

	creditsResp, err := p.httpUpstream.Do(creditsReq, p.proxyURL, p.account.ID, p.account.Concurrency)
	if err == nil && creditsResp != nil && creditsResp.StatusCode < 400 {
		s.clearCreditsExhausted(p.ctx, p.account)
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d credit_overages_success model=%s account=%d",
			p.prefix, creditsResp.StatusCode, modelKey, p.account.ID)
		return &creditsOveragesRetryResult{handled: true, resp: creditsRespREDACTED
REDACTED

	s.handleCreditsRetryFailure(p.ctx, p.prefix, modelKey, p.account, creditsResp, err)
	return &creditsOveragesRetryResult{handled: trueREDACTED
REDACTED

func (s *AntigravityGatewayService) handleCreditsRetryFailure(
	ctx context.Context,
	prefix string,
	modelKey string,
	account *Account,
	creditsResp *http.Response,
	reqErr error,
) {
	var creditsRespBody []byte
	creditsStatusCode := 0
	if creditsResp != nil {
		creditsStatusCode = creditsResp.StatusCode
		if creditsResp.Body != nil {
			creditsRespBody, _ = io.ReadAll(io.LimitReader(creditsResp.Body, 64<<10))
			_ = creditsResp.Body.Close()
	REDACTED
REDACTED

	if shouldMarkCreditsExhausted(creditsResp, creditsRespBody, reqErr) && account != nil {
		s.setCreditsExhausted(ctx, account)
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=true status=%d body=%s",
			prefix, modelKey, account.ID, creditsStatusCode, truncateForLog(creditsRespBody, 200))
		return
REDACTED
	if account != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=false status=%d err=%v body=%s",
			prefix, modelKey, account.ID, creditsStatusCode, reqErr, truncateForLog(creditsRespBody, 200))
REDACTED
REDACTED
