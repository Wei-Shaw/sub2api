package service

import (
	"context"
	"encoding/json"
	"io"
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
)

// checkAccountCredits 通过共享的 AccountUsageService 缓存检查账号是否有足够的 AI Credits。
// 缓存 TTL 不足时会自动从 Google loadCodeAssist API 刷新。
// 返回 true 表示积分可用。
func (s *AntigravityGatewayService) checkAccountCredits(
	ctx context.Context, account *Account,
) bool {
	if account == nil || account.ID == 0 {
		return false
	}

	if s.accountUsageService == nil {
		return true // 无 usage service 时不阻断
	}

	usageInfo, err := s.accountUsageService.GetAntigravityCredits(ctx, account)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway",
			"check_credits: get_credits_failed account=%d err=%v", account.ID, err)
		return true // 出错时假设有积分，不阻断
	}

	if usageInfo == nil || len(usageInfo.AICredits) == 0 {
		logger.LegacyPrintf("service.antigravity_gateway",
			"check_credits: account=%d has_credits=false amount=0 (no credits field)",
			account.ID)
		return false
	}

	for _, credit := range usageInfo.AICredits {
		if credit.CreditType == "GOOGLE_ONE_AI" {
			minimum := credit.MinimumBalance
			if minimum <= 0 {
				minimum = 5
			}
			hasCredits := credit.Amount >= minimum
			logger.LegacyPrintf("service.antigravity_gateway",
				"check_credits: account=%d has_credits=%t amount=%.0f minimum=%.0f",
				account.ID, hasCredits, credit.Amount, minimum)
			return hasCredits
		}
	}

	logger.LegacyPrintf("service.antigravity_gateway",
		"check_credits: account=%d has_credits=false (no GOOGLE_ONE_AI credit)",
		account.ID)
	return false
}

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
	}

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
	}
)

// isCreditsExhausted 检查账号的 AICredits 限流 key 是否生效（积分是否耗尽）。
func (a *Account) isCreditsExhausted() bool {
	if a == nil {
		return false
	}
	return a.isRateLimitActiveForKey(creditsExhaustedKey)
}

// setCreditsExhausted 标记账号积分耗尽：写入 model_rate_limits["AICredits"] + 更新缓存。
func (s *AntigravityGatewayService) setCreditsExhausted(ctx context.Context, account *Account) {
	if account == nil || account.ID == 0 {
		return
	}
	resetAt := time.Now().Add(creditsExhaustedDuration)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, creditsExhaustedKey, resetAt); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "set credits exhausted failed: account=%d err=%v", account.ID, err)
		return
	}
	s.updateAccountModelRateLimitInCache(ctx, account, creditsExhaustedKey, resetAt)
	logger.LegacyPrintf("service.antigravity_gateway", "credits_exhausted_marked account=%d reset_at=%s",
		account.ID, resetAt.UTC().Format(time.RFC3339))
}

// clearCreditsExhausted 清除账号的 AICredits 限流 key。
func (s *AntigravityGatewayService) clearCreditsExhausted(ctx context.Context, account *Account) {
	if account == nil || account.ID == 0 || account.Extra == nil {
		return
	}
	rawLimits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return
	}
	if _, exists := rawLimits[creditsExhaustedKey]; !exists {
		return
	}
	delete(rawLimits, creditsExhaustedKey)
	account.Extra[modelRateLimitsKey] = rawLimits
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		modelRateLimitsKey: rawLimits,
	}); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "clear credits exhausted failed: account=%d err=%v", account.ID, err)
	}
}

// classifyAntigravity429 将 Antigravity 的 429 响应归类为配额耗尽、限流或未知。
func classifyAntigravity429(body []byte) antigravity429Category {
	if len(body) == 0 {
		return antigravity429Unknown
	}
	lowerBody := strings.ToLower(string(body))
	for _, keyword := range antigravityQuotaExhaustedKeywords {
		if strings.Contains(lowerBody, keyword) {
			return antigravity429QuotaExhausted
		}
	}
	if info := parseAntigravitySmartRetryInfo(body); info != nil && !info.IsModelCapacityExhausted {
		return antigravity429RateLimited
	}
	return antigravity429Unknown
}

// injectEnabledCreditTypes 在已序列化的 v1internal JSON body 中注入 AI Credits 类型。
func injectEnabledCreditTypes(body []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	payload["enabledCreditTypes"] = []string{"GOOGLE_ONE_AI"}
	result, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return result
}

// resolveCreditsOveragesModelKey 解析当前请求对应的 overages 状态模型 key。
func resolveCreditsOveragesModelKey(ctx context.Context, account *Account, upstreamModelName, requestedModel string) string {
	modelKey := strings.TrimSpace(upstreamModelName)
	if modelKey != "" {
		return modelKey
	}
	if account == nil {
		return ""
	}
	modelKey = resolveFinalAntigravityModelKey(ctx, account, requestedModel)
	if strings.TrimSpace(modelKey) != "" {
		return modelKey
	}
	return resolveAntigravityModelKey(requestedModel)
}

// shouldMarkCreditsExhausted 判断一次 credits 请求失败是否应标记为 credits 耗尽。
// 此函数在积分注入后失败时调用（预检查注入 + attemptCreditsOveragesRetry 两条路径）。
//   - 429 + 非单模型限流：积分注入后仍 429 → 标记耗尽。
//   - 429 + 单模型限流（"exhausted your capacity on this model"）：该模型免费配额用完，
//     积分注入对此无效，但账号积分对其他模型可能仍可用 → 不标记积分耗尽。
//   - 403 等其他 4xx：检查 body 是否包含积分不足的关键词。
//
// clearCreditsExhausted 会在后续成功时自动清除。
func shouldMarkCreditsExhausted(resp *http.Response, respBody []byte, reqErr error) bool {
	if reqErr != nil || resp == nil {
		return false
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
		return false
	}
	bodyLower := strings.ToLower(string(respBody))
	// 积分注入后仍 429
	if resp.StatusCode == http.StatusTooManyRequests {
		// 单模型配额耗尽：积分注入对此无效，不标记整个账号积分耗尽
		if strings.Contains(bodyLower, "exhausted your capacity on this model") {
			return false
		}
		return true
	}
	// 其他 4xx：关键词匹配（如 403 + "Insufficient credits"）
	for _, keyword := range creditsExhaustedKeywords {
		if strings.Contains(bodyLower, keyword) {
			return true
		}
	}
	return false
}

type creditsOveragesRetryResult struct {
	handled bool
	resp    *http.Response
}

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
		return &creditsOveragesRetryResult{handled: false}
	}

	// Check actual credits balance before attempting retry
	if !s.checkAccountCredits(p.ctx, p.account) {
		s.setCreditsExhausted(p.ctx, p.account)
		modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, modelName, p.requestedModel)
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_no_credits model=%s account=%d (skipping credits retry)",
			p.prefix, modelKey, p.account.ID)
		return &creditsOveragesRetryResult{handled: true}
	}

	modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, modelName, p.requestedModel)
	logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 credit_overages_retry model=%s account=%d (injecting enabledCreditTypes)",
		p.prefix, modelKey, p.account.ID)

	creditsReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, creditsBody)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d build_request_err=%v",
			p.prefix, modelKey, p.account.ID, err)
		return &creditsOveragesRetryResult{handled: true}
	}

	creditsResp, err := p.httpUpstream.Do(creditsReq, p.proxyURL, p.account.ID, p.account.Concurrency)
	if err == nil && creditsResp != nil && creditsResp.StatusCode < 400 {
		s.clearCreditsExhausted(p.ctx, p.account)
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d credit_overages_success model=%s account=%d",
			p.prefix, creditsResp.StatusCode, modelKey, p.account.ID)
		return &creditsOveragesRetryResult{handled: true, resp: creditsResp}
	}

	s.handleCreditsRetryFailure(p.ctx, p.prefix, modelKey, p.account, creditsResp, err)
	return &creditsOveragesRetryResult{handled: true}
}

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
		}
	}

	if shouldMarkCreditsExhausted(creditsResp, creditsRespBody, reqErr) && account != nil {
		s.setCreditsExhausted(ctx, account)
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=true status=%d body=%s",
			prefix, modelKey, account.ID, creditsStatusCode, truncateForLog(creditsRespBody, 200))
		return
	}
	if account != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=false status=%d err=%v body=%s",
			prefix, modelKey, account.ID, creditsStatusCode, reqErr, truncateForLog(creditsRespBody, 200))
	}
}
