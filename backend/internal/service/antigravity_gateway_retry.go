package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// antigravityRetryLoopParams 重试循环的参数
type antigravityRetryLoopParams struct {
	ctx             context.Context
	prefix          string
	account         *Account
	proxyURL        string
	accessToken     string
	action          string
	body            []byte
	c               *gin.Context
	httpUpstream    HTTPUpstream
	settingService  *SettingService
	accountRepo     AccountRepository // 用于智能重试的模型级别限流
	handleError     func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult
	requestedModel  string // 用于限流检查的原始请求模型
	isStickySession bool   // 是否为粘性会话（用于账号切换时的缓存计费判断）
	groupID         int64  // 用于模型级限流时清除粘性会话
	sessionHash     string // 用于模型级限流时清除粘性会话
}

// antigravityRetryLoopResult 重试循环的结果
type antigravityRetryLoopResult struct {
	resp *http.Response
}

// resolveAntigravityForwardBaseURL 解析转发用 base URL。
//
// 默认使用生产端点 cloudcode-pa.googleapis.com（antigravity.BaseURLs 的首个地址，
// 与账号 OAuth 登录/测试连接所用的 antigravity.BaseURL 一致）。
//
// 历史上这里改用 ForwardBaseURLs()（把 daily/sandbox 排到首位）并默认取首个地址，
// 导致网关把带生产 OAuth token 的请求发到 daily-cloudcode-pa.sandbox.googleapis.com，
// 上游拒绝 → 账号被 401「Invalid bearer token」/502 打入临时不可调度且无法恢复
// （见 #3611 / #2962）。后台「测试连接」用的是生产端点，所以「测试成功但网关 401」。
//
// daily/sandbox 端点仅供内部联调，需显式设置
// GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL=daily（或 sandbox）才启用。
func resolveAntigravityForwardBaseURL() string {
	baseURLs := antigravity.BaseURLs
	if len(baseURLs) == 0 {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(antigravityForwardBaseURLEnv)))
	if (mode == "daily" || mode == "sandbox") && len(baseURLs) > 1 {
		return baseURLs[1]
	}
	return baseURLs[0]
}

// smartRetryAction 智能重试的处理结果
type smartRetryAction int

const (
	smartRetryActionContinue      smartRetryAction = iota // 继续默认重试逻辑
	smartRetryActionBreakWithResp                         // 结束循环并返回 resp
	smartRetryActionContinueURL                           // 继续 URL fallback 循环
)

// smartRetryResult 智能重试的结果
type smartRetryResult struct {
	action      smartRetryAction
	resp        *http.Response
	err         error
	switchError *AntigravityAccountSwitchError // 模型限流时返回账号切换信号
}

// handleSmartRetry 处理 OAuth 账号的智能重试逻辑
// 将 429/503 限流处理逻辑抽取为独立函数，减少 antigravityRetryLoop 的复杂度
func (s *AntigravityGatewayService) handleSmartRetry(p antigravityRetryLoopParams, resp *http.Response, respBody []byte, baseURL string, urlIdx int, availableURLs []string) *smartRetryResult {
	// "Resource has been exhausted" 是 URL 级别限流，切换 URL（仅 429）
	if resp.StatusCode == http.StatusTooManyRequests && isURLLevelRateLimit(respBody) && urlIdx < len(availableURLs)-1 {
		logger.LegacyPrintf("service.antigravity_gateway", "%s URL fallback (429): %s -> %s", p.prefix, baseURL, availableURLs[urlIdx+1])
		return &smartRetryResult{action: smartRetryActionContinueURL}
	}

	category := antigravity429Unknown
	if resp.StatusCode == http.StatusTooManyRequests {
		category = classifyAntigravity429(respBody)
	}

	// 判断是否触发智能重试
	shouldSmartRetry, shouldRateLimitModel, waitDuration, modelName, isModelCapacityExhausted := shouldTriggerAntigravitySmartRetry(p.account, respBody)

	// AI Credits 超量请求：
	// 仅在上游明确返回免费配额耗尽时才允许切换到 credits。
	if resp.StatusCode == http.StatusTooManyRequests &&
		category == antigravity429QuotaExhausted &&
		p.account.IsOveragesEnabled() &&
		!p.account.isCreditsExhausted() {
		result := s.attemptCreditsOveragesRetry(p, baseURL, modelName, waitDuration, resp.StatusCode, respBody)
		if result.handled && result.resp != nil {
			return &smartRetryResult{
				action: smartRetryActionBreakWithResp,
				resp:   result.resp,
			}
		}
	}

	// 情况1: retryDelay >= 阈值，限流模型并切换账号
	if shouldRateLimitModel {
		// 单账号 503 退避重试模式：不设限流、不切换账号，改为原地等待+重试
		// 谷歌上游 503 (MODEL_CAPACITY_EXHAUSTED) 通常是暂时性的，等几秒就能恢复。
		// 多账号场景下切换账号是最优选择，但单账号场景下设限流毫无意义（只会导致双重等待）。
		if resp.StatusCode == http.StatusServiceUnavailable && isSingleAccountRetry(p.ctx) {
			return s.handleSingleAccountRetryInPlace(p, resp, respBody, baseURL, waitDuration, modelName)
		}

		rateLimitDuration := waitDuration
		if rateLimitDuration <= 0 {
			rateLimitDuration = antigravityDefaultRateLimitDuration
		}
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d oauth_long_delay model=%s account=%d upstream_retry_delay=%v body=%s (model rate limit, switch account)",
			p.prefix, resp.StatusCode, modelName, p.account.ID, rateLimitDuration, truncateForLog(respBody, 200))

		resetAt := time.Now().Add(rateLimitDuration)
		if !s.setAntigravityModelRateLimits(p.ctx, p.accountRepo, p.account, modelName, p.prefix, resp.StatusCode, resetAt, false) {
			p.handleError(p.ctx, p.prefix, p.account, resp.StatusCode, resp.Header, respBody, p.requestedModel, p.groupID, p.sessionHash, p.isStickySession)
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d rate_limited account=%d (no model mapping)", p.prefix, resp.StatusCode, p.account.ID)
		}
		s.clearStickySession(p.ctx, p.groupID, p.sessionHash)

		// 返回账号切换信号，让上层切换账号重试
		return &smartRetryResult{
			action: smartRetryActionBreakWithResp,
			switchError: &AntigravityAccountSwitchError{
				OriginalAccountID: p.account.ID,
				RateLimitedModel:  modelName,
				IsStickySession:   p.isStickySession,
			},
		}
	}

	// 情况2: retryDelay < 阈值（或 MODEL_CAPACITY_EXHAUSTED），智能重试
	if shouldSmartRetry {
		var lastRetryResp *http.Response
		var lastRetryBody []byte

		// MODEL_CAPACITY_EXHAUSTED 使用独立的重试参数（60 次，固定 1s 间隔）
		maxAttempts := antigravitySmartRetryMaxAttempts
		if isModelCapacityExhausted {
			maxAttempts = antigravityModelCapacityRetryMaxAttempts
			waitDuration = antigravityModelCapacityRetryWait

			// 全局去重：如果其他 goroutine 已在重试同一模型且尚在 cooldown 中，直接返回 503
			if modelName != "" {
				modelCapacityExhaustedMu.RLock()
				cooldownUntil, exists := modelCapacityExhaustedUntil[modelName]
				modelCapacityExhaustedMu.RUnlock()
				if exists && time.Now().Before(cooldownUntil) {
					log.Printf("%s status=%d model_capacity_exhausted_dedup model=%s account=%d cooldown_until=%v (skip retry)",
						p.prefix, resp.StatusCode, modelName, p.account.ID, cooldownUntil.Format("15:04:05"))
					return &smartRetryResult{
						action: smartRetryActionBreakWithResp,
						resp: &http.Response{
							StatusCode: resp.StatusCode,
							Header:     resp.Header.Clone(),
							Body:       io.NopCloser(bytes.NewReader(respBody)),
						},
					}
				}
			}
		}

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			log.Printf("%s status=%d oauth_smart_retry attempt=%d/%d delay=%v model=%s account=%d",
				p.prefix, resp.StatusCode, attempt, maxAttempts, waitDuration, modelName, p.account.ID)

			timer := time.NewTimer(waitDuration)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				log.Printf("%s status=context_canceled_during_smart_retry", p.prefix)
				return &smartRetryResult{action: smartRetryActionBreakWithResp, err: p.ctx.Err()}
			case <-timer.C:
			}

			// 智能重试：创建新请求
			retryReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, p.body)
			if err != nil {
				logger.LegacyPrintf("service.antigravity_gateway", "%s status=smart_retry_request_build_failed error=%v", p.prefix, err)
				p.handleError(p.ctx, p.prefix, p.account, resp.StatusCode, resp.Header, respBody, p.requestedModel, p.groupID, p.sessionHash, p.isStickySession)
				return &smartRetryResult{
					action: smartRetryActionBreakWithResp,
					resp: &http.Response{
						StatusCode: resp.StatusCode,
						Header:     resp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(respBody)),
					},
				}
			}

			retryResp, retryErr := p.httpUpstream.Do(retryReq, p.proxyURL, p.account.ID, p.account.Concurrency)
			if retryErr == nil && retryResp != nil && retryResp.StatusCode != http.StatusTooManyRequests && retryResp.StatusCode != http.StatusServiceUnavailable {
				log.Printf("%s status=%d smart_retry_success attempt=%d/%d", p.prefix, retryResp.StatusCode, attempt, maxAttempts)
				// 重试成功，清除 MODEL_CAPACITY_EXHAUSTED cooldown
				if isModelCapacityExhausted && modelName != "" {
					modelCapacityExhaustedMu.Lock()
					delete(modelCapacityExhaustedUntil, modelName)
					modelCapacityExhaustedMu.Unlock()
				}
				return &smartRetryResult{action: smartRetryActionBreakWithResp, resp: retryResp}
			}

			// 网络错误时，继续重试
			if retryErr != nil || retryResp == nil {
				log.Printf("%s status=smart_retry_network_error attempt=%d/%d error=%v", p.prefix, attempt, maxAttempts, retryErr)
				continue
			}

			// 重试失败，关闭之前的响应
			if lastRetryResp != nil {
				_ = lastRetryResp.Body.Close()
			}
			lastRetryResp = retryResp
			if retryResp != nil {
				lastRetryBody, _ = io.ReadAll(io.LimitReader(retryResp.Body, 8<<10))
				_ = retryResp.Body.Close()
			}

			// 解析新的重试信息，用于下次重试的等待时间（MODEL_CAPACITY_EXHAUSTED 使用固定循环，跳过）
			if !isModelCapacityExhausted && attempt < maxAttempts && lastRetryBody != nil {
				newShouldRetry, _, newWaitDuration, _, _ := shouldTriggerAntigravitySmartRetry(p.account, lastRetryBody)
				if newShouldRetry && newWaitDuration > 0 {
					waitDuration = newWaitDuration
				}
			}
		}

		// 所有重试都失败
		rateLimitDuration := waitDuration
		if rateLimitDuration <= 0 {
			rateLimitDuration = antigravityDefaultRateLimitDuration
		}
		retryBody := lastRetryBody
		if retryBody == nil {
			retryBody = respBody
		}

		// MODEL_CAPACITY_EXHAUSTED：模型容量不足，切换账号无意义
		// 直接返回上游错误响应，不设置模型限流，不切换账号
		if isModelCapacityExhausted {
			// 设置 cooldown，让后续请求快速失败，避免重复重试
			if modelName != "" {
				modelCapacityExhaustedMu.Lock()
				modelCapacityExhaustedUntil[modelName] = time.Now().Add(antigravityModelCapacityCooldown)
				modelCapacityExhaustedMu.Unlock()
			}
			log.Printf("%s status=%d smart_retry_exhausted_model_capacity attempts=%d model=%s account=%d body=%s (model capacity exhausted, not switching account)",
				p.prefix, resp.StatusCode, maxAttempts, modelName, p.account.ID, truncateForLog(retryBody, 200))
			return &smartRetryResult{
				action: smartRetryActionBreakWithResp,
				resp: &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(retryBody)),
				},
			}
		}

		// 单账号 503 退避重试模式：智能重试耗尽后不设限流、不切换账号，
		// 直接返回 503 让 Handler 层的单账号退避循环做最终处理。
		if resp.StatusCode == http.StatusServiceUnavailable && isSingleAccountRetry(p.ctx) {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d smart_retry_exhausted_single_account attempts=%d model=%s account=%d body=%s (return 503 directly)",
				p.prefix, resp.StatusCode, antigravitySmartRetryMaxAttempts, modelName, p.account.ID, truncateForLog(retryBody, 200))
			return &smartRetryResult{
				action: smartRetryActionBreakWithResp,
				resp: &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(retryBody)),
				},
			}
		}

		log.Printf("%s status=%d smart_retry_exhausted attempts=%d model=%s account=%d upstream_retry_delay=%v body=%s (switch account)",
			p.prefix, resp.StatusCode, maxAttempts, modelName, p.account.ID, rateLimitDuration, truncateForLog(retryBody, 200))

		resetAt := time.Now().Add(rateLimitDuration)
		s.setAntigravityModelRateLimits(p.ctx, p.accountRepo, p.account, modelName, p.prefix, resp.StatusCode, resetAt, true)

		// 清除粘性会话绑定，避免下次请求仍命中限流账号
		s.clearStickySession(p.ctx, p.groupID, p.sessionHash)

		// 返回账号切换信号，让上层切换账号重试
		return &smartRetryResult{
			action: smartRetryActionBreakWithResp,
			switchError: &AntigravityAccountSwitchError{
				OriginalAccountID: p.account.ID,
				RateLimitedModel:  modelName,
				IsStickySession:   p.isStickySession,
			},
		}
	}

	// 未触发智能重试，继续默认重试逻辑
	return &smartRetryResult{action: smartRetryActionContinue}
}

// handleSingleAccountRetryInPlace 单账号 503 退避重试的原地重试逻辑。
//
// 在多账号场景下，收到 503 + 长 retryDelay（≥ 7s）时会设置模型限流 + 切换账号；
// 但在单账号场景下，设限流毫无意义（因为切换回来的还是同一个账号，还要等限流过期）。
// 此方法改为在 Service 层原地等待 + 重试，避免双重等待问题：
//
//	旧流程：Service 设限流 → Handler 退避等待 → Service 等限流过期 → 再请求（总耗时 = 退避 + 限流）
//	新流程：Service 直接等 retryDelay → 重试 → 成功/再等 → 重试...（总耗时 ≈ 实际 retryDelay × 重试次数）
//
// 约束：
//   - 单次等待不超过 antigravitySingleAccountSmartRetryMaxWait
//   - 总累计等待不超过 antigravitySingleAccountSmartRetryTotalMaxWait
//   - 最多重试 antigravitySingleAccountSmartRetryMaxAttempts 次
func (s *AntigravityGatewayService) handleSingleAccountRetryInPlace(
	p antigravityRetryLoopParams,
	resp *http.Response,
	respBody []byte,
	baseURL string,
	waitDuration time.Duration,
	modelName string,
) *smartRetryResult {
	// 限制单次等待时间
	if waitDuration > antigravitySingleAccountSmartRetryMaxWait {
		waitDuration = antigravitySingleAccountSmartRetryMaxWait
	}
	if waitDuration < antigravitySmartRetryMinWait {
		waitDuration = antigravitySmartRetryMinWait
	}

	logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d single_account_503_retry_in_place model=%s account=%d upstream_retry_delay=%v (retrying in-place instead of rate-limiting)",
		p.prefix, resp.StatusCode, modelName, p.account.ID, waitDuration)

	var lastRetryResp *http.Response
	var lastRetryBody []byte
	totalWaited := time.Duration(0)

	for attempt := 1; attempt <= antigravitySingleAccountSmartRetryMaxAttempts; attempt++ {
		// 检查累计等待是否超限
		if totalWaited+waitDuration > antigravitySingleAccountSmartRetryTotalMaxWait {
			remaining := antigravitySingleAccountSmartRetryTotalMaxWait - totalWaited
			if remaining <= 0 {
				logger.LegacyPrintf("service.antigravity_gateway", "%s single_account_503_retry: total_wait_exceeded total=%v max=%v, giving up",
					p.prefix, totalWaited, antigravitySingleAccountSmartRetryTotalMaxWait)
				break
			}
			waitDuration = remaining
		}

		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d single_account_503_retry attempt=%d/%d delay=%v total_waited=%v model=%s account=%d",
			p.prefix, resp.StatusCode, attempt, antigravitySingleAccountSmartRetryMaxAttempts, waitDuration, totalWaited, modelName, p.account.ID)

		timer := time.NewTimer(waitDuration)
		select {
		case <-p.ctx.Done():
			timer.Stop()
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=context_canceled_during_single_account_retry", p.prefix)
			return &smartRetryResult{action: smartRetryActionBreakWithResp, err: p.ctx.Err()}
		case <-timer.C:
		}
		totalWaited += waitDuration

		// 创建新请求
		retryReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, p.body)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s single_account_503_retry: request_build_failed error=%v", p.prefix, err)
			break
		}

		retryResp, retryErr := p.httpUpstream.Do(retryReq, p.proxyURL, p.account.ID, p.account.Concurrency)
		if retryErr == nil && retryResp != nil && retryResp.StatusCode != http.StatusTooManyRequests && retryResp.StatusCode != http.StatusServiceUnavailable {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d single_account_503_retry_success attempt=%d/%d total_waited=%v",
				p.prefix, retryResp.StatusCode, attempt, antigravitySingleAccountSmartRetryMaxAttempts, totalWaited)
			// 关闭之前的响应
			if lastRetryResp != nil {
				_ = lastRetryResp.Body.Close()
			}
			return &smartRetryResult{action: smartRetryActionBreakWithResp, resp: retryResp}
		}

		// 网络错误时继续重试
		if retryErr != nil || retryResp == nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s single_account_503_retry: network_error attempt=%d/%d error=%v",
				p.prefix, attempt, antigravitySingleAccountSmartRetryMaxAttempts, retryErr)
			continue
		}

		// 关闭之前的响应
		if lastRetryResp != nil {
			_ = lastRetryResp.Body.Close()
		}
		lastRetryResp = retryResp
		lastRetryBody, _ = io.ReadAll(io.LimitReader(retryResp.Body, 8<<10))
		_ = retryResp.Body.Close()

		// 解析新的重试信息，更新下次等待时间
		if attempt < antigravitySingleAccountSmartRetryMaxAttempts && lastRetryBody != nil {
			_, _, newWaitDuration, _, _ := shouldTriggerAntigravitySmartRetry(p.account, lastRetryBody)
			if newWaitDuration > 0 {
				waitDuration = newWaitDuration
				if waitDuration > antigravitySingleAccountSmartRetryMaxWait {
					waitDuration = antigravitySingleAccountSmartRetryMaxWait
				}
				if waitDuration < antigravitySmartRetryMinWait {
					waitDuration = antigravitySmartRetryMinWait
				}
			}
		}
	}

	// 所有重试都失败，不设限流，直接返回 503
	// Handler 层的单账号退避循环会做最终处理
	retryBody := lastRetryBody
	if retryBody == nil {
		retryBody = respBody
	}
	logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d single_account_503_retry_exhausted attempts=%d total_waited=%v model=%s account=%d body=%s (return 503 directly)",
		p.prefix, resp.StatusCode, antigravitySingleAccountSmartRetryMaxAttempts, totalWaited, modelName, p.account.ID, truncateForLog(retryBody, 200))

	return &smartRetryResult{
		action: smartRetryActionBreakWithResp,
		resp: &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(retryBody)),
		},
	}
}

// antigravityRetryLoop 执行带 URL fallback 的重试循环
func (s *AntigravityGatewayService) antigravityRetryLoop(p antigravityRetryLoopParams) (*antigravityRetryLoopResult, error) {
	// 预检查：模型限流 + overages 启用 + 积分未耗尽 → 直接注入 AI Credits
	overagesInjected := false
	if p.requestedModel != "" && p.account.Platform == PlatformAntigravity &&
		p.account.IsOveragesEnabled() && !p.account.isCreditsExhausted() &&
		p.account.isModelRateLimitedWithContext(p.ctx, p.requestedModel) {
		if creditsBody := injectEnabledCreditTypes(p.body); creditsBody != nil {
			p.body = creditsBody
			overagesInjected = true
			logger.LegacyPrintf("service.antigravity_gateway", "%s pre_check: model_rate_limited_credits_inject model=%s account=%d (injecting enabledCreditTypes)",
				p.prefix, p.requestedModel, p.account.ID)
		}
	}

	// 预检查：如果账号已限流，直接返回切换信号
	if p.requestedModel != "" {
		if remaining := p.account.GetRateLimitRemainingTimeWithContext(p.ctx, p.requestedModel); remaining > 0 {
			// 已注入积分的请求不再受普通模型限流预检查阻断。
			if overagesInjected {
				logger.LegacyPrintf("service.antigravity_gateway", "%s pre_check: credits_injected_ignore_rate_limit remaining=%v model=%s account=%d",
					p.prefix, remaining.Truncate(time.Millisecond), p.requestedModel, p.account.ID)
			} else if isSingleAccountRetry(p.ctx) {
				// 单账号 503 退避重试模式：跳过限流预检查，直接发请求。
				// 首次请求设的限流是为了多账号调度器跳过该账号，在单账号模式下无意义。
				// 如果上游确实还不可用，handleSmartRetry → handleSingleAccountRetryInPlace
				// 会在 Service 层原地等待+重试，不需要在预检查这里等。
				logger.LegacyPrintf("service.antigravity_gateway", "%s pre_check: single_account_retry skipping rate_limit remaining=%v model=%s account=%d (will retry in-place if 503)",
					p.prefix, remaining.Truncate(time.Millisecond), p.requestedModel, p.account.ID)
			} else {
				logger.LegacyPrintf("service.antigravity_gateway", "%s pre_check: rate_limit_switch remaining=%v model=%s account=%d",
					p.prefix, remaining.Truncate(time.Millisecond), p.requestedModel, p.account.ID)
				return nil, &AntigravityAccountSwitchError{
					OriginalAccountID: p.account.ID,
					RateLimitedModel:  p.requestedModel,
					IsStickySession:   p.isStickySession,
				}
			}
		}
	}

	baseURL := resolveAntigravityForwardBaseURL()
	if baseURL == "" {
		return nil, errors.New("no antigravity forward base url configured")
	}
	availableURLs := []string{baseURL}

	var resp *http.Response
	var usedBaseURL string
	logBody := p.settingService != nil && p.settingService.cfg != nil && p.settingService.cfg.Gateway.LogUpstreamErrorBody
	maxBytes := 2048
	if p.settingService != nil && p.settingService.cfg != nil && p.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes > 0 {
		maxBytes = p.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
	}
	getUpstreamDetail := func(body []byte) string {
		if !logBody {
			return ""
		}
		return truncateString(string(body), maxBytes)
	}

urlFallbackLoop:
	for urlIdx, baseURL := range availableURLs {
		usedBaseURL = baseURL
		allAttemptsInternal500 := true // 追踪本轮所有 attempt 是否全部命中 INTERNAL 500
		for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
			select {
			case <-p.ctx.Done():
				logger.LegacyPrintf("service.antigravity_gateway", "%s status=context_canceled error=%v", p.prefix, p.ctx.Err())
				return nil, p.ctx.Err()
			default:
			}

			upstreamReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, p.body)
			if err != nil {
				return nil, err
			}

			resp, err = p.httpUpstream.Do(upstreamReq, p.proxyURL, p.account.ID, p.account.Concurrency)
			if err == nil && resp == nil {
				err = errors.New("upstream returned nil response")
			}
			if err != nil {
				safeErr := sanitizeUpstreamErrorMessage(err.Error())
				appendOpsUpstreamError(p.c, OpsUpstreamErrorEvent{
					Platform:           p.account.Platform,
					AccountID:          p.account.ID,
					AccountName:        p.account.Name,
					UpstreamStatusCode: 0,
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Kind:               "request_error",
					Message:            safeErr,
				})
				if shouldAntigravityFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
					logger.LegacyPrintf("service.antigravity_gateway", "%s URL fallback (connection error): %s -> %s", p.prefix, baseURL, availableURLs[urlIdx+1])
					continue urlFallbackLoop
				}
				if attempt < antigravityMaxRetries {
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=request_failed retry=%d/%d error=%v", p.prefix, attempt, antigravityMaxRetries, err)
					if !sleepAntigravityBackoffWithContext(p.ctx, attempt) {
						logger.LegacyPrintf("service.antigravity_gateway", "%s status=context_canceled_during_backoff", p.prefix)
						return nil, p.ctx.Err()
					}
					continue
				}
				logger.LegacyPrintf("service.antigravity_gateway", "%s status=request_failed retries_exhausted error=%v", p.prefix, err)
				setOpsUpstreamError(p.c, 0, safeErr, "")
				return nil, fmt.Errorf("upstream request failed after retries: %w", err)
			}

			// 统一处理错误响应
			if resp.StatusCode >= 400 {
				respBody := s.readUpstreamErrorBody(resp)
				_ = resp.Body.Close()

				if overagesInjected && shouldMarkCreditsExhausted(resp, respBody, nil) {
					modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, "", p.requestedModel)
					s.handleCreditsRetryFailure(p.ctx, p.prefix, modelKey, p.account, &http.Response{
						StatusCode: resp.StatusCode,
						Header:     resp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(respBody)),
					}, nil)
				}

				// ★ 统一入口：自定义错误码 + 临时不可调度
				if handled, outStatus, policyErr := s.applyErrorPolicy(p, resp.StatusCode, resp.Header, respBody); handled {
					if policyErr != nil {
						return nil, policyErr
					}
					resp = &http.Response{
						StatusCode: outStatus,
						Header:     resp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(respBody)),
					}
					break urlFallbackLoop
				}

				// 429/503 限流处理：区分 URL 级别限流、智能重试和账户配额限流
				if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
					// 尝试智能重试处理（OAuth 账号专用）
					smartResult := s.handleSmartRetry(p, resp, respBody, baseURL, urlIdx, availableURLs)
					switch smartResult.action {
					case smartRetryActionContinueURL:
						continue urlFallbackLoop
					case smartRetryActionBreakWithResp:
						if smartResult.err != nil {
							return nil, smartResult.err
						}
						// 模型限流时返回切换账号信号
						if smartResult.switchError != nil {
							return nil, smartResult.switchError
						}
						resp = smartResult.resp
						break urlFallbackLoop
					}
					// smartRetryActionContinue: 继续默认重试逻辑

					// 账户/模型配额限流，重试 3 次（指数退避）- 默认逻辑（非 OAuth 账号或解析失败）
					if attempt < antigravityMaxRetries {
						upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
						upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
						appendOpsUpstreamError(p.c, OpsUpstreamErrorEvent{
							Platform:           p.account.Platform,
							AccountID:          p.account.ID,
							AccountName:        p.account.Name,
							UpstreamStatusCode: resp.StatusCode,
							UpstreamRequestID:  resp.Header.Get("x-request-id"),
							UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
							Kind:               "retry",
							Message:            upstreamMsg,
							Detail:             getUpstreamDetail(respBody),
						})
						logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d retry=%d/%d body=%s", p.prefix, resp.StatusCode, attempt, antigravityMaxRetries, truncateForLog(respBody, 200))
						if !sleepAntigravityBackoffWithContext(p.ctx, attempt) {
							logger.LegacyPrintf("service.antigravity_gateway", "%s status=context_canceled_during_backoff", p.prefix)
							return nil, p.ctx.Err()
						}
						continue
					}

					// 重试用尽，标记账户限流
					p.handleError(p.ctx, p.prefix, p.account, resp.StatusCode, resp.Header, respBody, p.requestedModel, p.groupID, p.sessionHash, p.isStickySession)
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d rate_limited base_url=%s body=%s", p.prefix, resp.StatusCode, baseURL, truncateForLog(respBody, 200))
					resp = &http.Response{
						StatusCode: resp.StatusCode,
						Header:     resp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(respBody)),
					}
					break urlFallbackLoop
				}

				// 其他可重试错误（500/502/504/529，不包括 429 和 503）
				if shouldRetryAntigravityError(resp.StatusCode) {
					if attempt < antigravityMaxRetries {
						upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
						upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
						appendOpsUpstreamError(p.c, OpsUpstreamErrorEvent{
							Platform:           p.account.Platform,
							AccountID:          p.account.ID,
							AccountName:        p.account.Name,
							UpstreamStatusCode: resp.StatusCode,
							UpstreamRequestID:  resp.Header.Get("x-request-id"),
							UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
							Kind:               "retry",
							Message:            upstreamMsg,
							Detail:             getUpstreamDetail(respBody),
						})
						logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d retry=%d/%d body=%s", p.prefix, resp.StatusCode, attempt, antigravityMaxRetries, truncateForLog(respBody, 500))
						if !sleepAntigravityBackoffWithContext(p.ctx, attempt) {
							logger.LegacyPrintf("service.antigravity_gateway", "%s status=context_canceled_during_backoff", p.prefix)
							return nil, p.ctx.Err()
						}
						// 追踪 INTERNAL 500：非匹配的 attempt 清除标记
						if !isAntigravityInternalServerError(resp.StatusCode, respBody) {
							allAttemptsInternal500 = false
						}
						continue
					}
				}

				// INTERNAL 500 渐进惩罚：3 次重试全部命中特定 500 时递增计数器并惩罚
				if allAttemptsInternal500 && isAntigravityInternalServerError(resp.StatusCode, respBody) {
					s.handleInternal500RetryExhausted(p.ctx, p.prefix, p.account)
				}

				// 其他 4xx 错误或重试用尽，直接返回
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
				}
				break urlFallbackLoop
			}

			// 成功响应（< 400）
			break urlFallbackLoop
		}
	}

	if resp != nil && resp.StatusCode < 400 && usedBaseURL != "" {
		antigravity.DefaultURLAvailability.MarkSuccess(usedBaseURL)
	}

	// 成功响应时清零 INTERNAL 500 连续失败计数器（覆盖所有成功路径，含 smart retry）
	if resp != nil && resp.StatusCode < 400 {
		s.resetInternal500Counter(p.ctx, p.prefix, p.account.ID)
	}

	return &antigravityRetryLoopResult{resp: resp}, nil
}

// shouldRetryAntigravityError 判断是否应该重试
func shouldRetryAntigravityError(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
}

// isURLLevelRateLimit 判断是否为 URL 级别的限流（应切换 URL 重试）
// "Resource has been exhausted" 是 URL/节点级别限流，切换 URL 可能成功
// "exhausted your capacity on this model" 是账户/模型配额限流，切换 URL 无效
func isURLLevelRateLimit(body []byte) bool {
	// 快速检查：包含 "Resource has been exhausted" 且不包含 "capacity on this model"
	bodyStr := string(body)
	return strings.Contains(bodyStr, "Resource has been exhausted") &&
		!strings.Contains(bodyStr, "capacity on this model")
}

// isAntigravityConnectionError 判断是否为连接错误（网络超时、DNS 失败、连接拒绝）
func isAntigravityConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// 检查超时错误
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// 检查连接错误（DNS 失败、连接拒绝）
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

// shouldAntigravityFallbackToNextURL 判断是否应切换到下一个 URL
// 仅连接错误和 HTTP 429 触发 URL 降级
func shouldAntigravityFallbackToNextURL(err error, statusCode int) bool {
	if isAntigravityConnectionError(err) {
		return true
	}
	return statusCode == http.StatusTooManyRequests
}

// getSessionID 从 gin.Context 获取 session_id（用于日志追踪）
func getSessionID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetHeader("session_id")
}

// logPrefix 生成统一的日志前缀
func logPrefix(sessionID, accountName string) string {
	if sessionID != "" {
		return fmt.Sprintf("[antigravity-Forward] session=%s account=%s", sessionID, accountName)
	}
	return fmt.Sprintf("[antigravity-Forward] account=%s", accountName)
}

func (s *AntigravityGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

// isGoogleProjectConfigError 判断（已提取的小写）错误消息是否属于 Google 服务端配置类问题。
// 只精确匹配已知的服务端侧错误，避免对客户端请求错误做无意义重试。
// 适用于所有走 Google 后端的平台（Antigravity、Gemini）。
func isGoogleProjectConfigError(lowerMsg string) bool {
	// Google 间歇性 Bug：Project ID 有效但被临时识别失败
	return strings.Contains(lowerMsg, "invalid project resource name")
}

// googleConfigErrorCooldown 服务端配置类 400 错误的临时封禁时长
const googleConfigErrorCooldown = 1 * time.Minute

// tempUnscheduleGoogleConfigError 对服务端配置类 400 错误触发临时封禁，
// 避免短时间内反复调度到同一个有问题的账号。
func tempUnscheduleGoogleConfigError(ctx context.Context, repo AccountRepository, accountID int64, logPrefix string) {
	until := time.Now().Add(googleConfigErrorCooldown)
	reason := "400: invalid project resource name (auto temp-unschedule 1m)"
	if err := repo.SetTempUnschedulable(ctx, accountID, until, reason); err != nil {
		log.Printf("%s temp_unschedule_failed account=%d error=%v", logPrefix, accountID, err)
	} else {
		log.Printf("%s temp_unscheduled account=%d until=%v reason=%q", logPrefix, accountID, until.Format("15:04:05"), reason)
	}
}

// emptyResponseCooldown 空流式响应的临时封禁时长
const emptyResponseCooldown = 1 * time.Minute

// tempUnscheduleEmptyResponse 对空流式响应触发临时封禁，
// 避免短时间内反复调度到同一个返回空响应的账号。
func tempUnscheduleEmptyResponse(ctx context.Context, repo AccountRepository, accountID int64, logPrefix string) {
	until := time.Now().Add(emptyResponseCooldown)
	reason := "empty stream response (auto temp-unschedule 1m)"
	if err := repo.SetTempUnschedulable(ctx, accountID, until, reason); err != nil {
		log.Printf("%s temp_unschedule_failed account=%d error=%v", logPrefix, accountID, err)
	} else {
		log.Printf("%s temp_unscheduled account=%d until=%v reason=%q", logPrefix, accountID, until.Format("15:04:05"), reason)
	}
}

// sleepAntigravityBackoffWithContext 带 context 取消检查的退避等待
// 返回 true 表示正常完成等待，false 表示 context 已取消
func sleepAntigravityBackoffWithContext(ctx context.Context, attempt int) bool {
	delay := antigravityRetryBaseDelay * time.Duration(1<<uint(attempt-1))
	if delay > antigravityRetryMaxDelay {
		delay = antigravityRetryMaxDelay
	}

	// +/- 20% jitter
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(float64(delay) * 0.2 * (r.Float64()*2 - 1))
	sleepFor := delay + jitter
	if sleepFor < 0 {
		sleepFor = 0
	}

	timer := time.NewTimer(sleepFor)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false
	case <-timer.C:
		return true
	}
}

// isSingleAccountRetry 检查 context 中是否设置了单账号退避重试标记
func isSingleAccountRetry(ctx context.Context) bool {
	v, _ := SingleAccountRetryFromContext(ctx)
	return v
}

// setModelRateLimitByModelName 使用官方模型 ID 设置模型级限流
// 直接使用上游返回的模型 ID（如 claude-sonnet-4-5）作为限流 key
// 返回是否已成功设置（若模型名为空或 repo 为 nil 将返回 false）
func setModelRateLimitByModelName(ctx context.Context, repo AccountRepository, accountID int64, modelName, prefix string, statusCode int, resetAt time.Time, afterSmartRetry bool) bool {
	if repo == nil || modelName == "" {
		return false
	}
	// 直接使用官方模型 ID 作为 key，不再转换为 scope
	if err := repo.SetModelRateLimit(ctx, accountID, modelName, resetAt); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limit_failed model=%s error=%v", prefix, statusCode, modelName, err)
		return false
	}
	if afterSmartRetry {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limited_after_smart_retry model=%s account=%d reset_in=%v", prefix, statusCode, modelName, accountID, time.Until(resetAt).Truncate(time.Second))
	} else {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limited model=%s account=%d reset_in=%v", prefix, statusCode, modelName, accountID, time.Until(resetAt).Truncate(time.Second))
	}
	return true
}

func (s *AntigravityGatewayService) setAntigravityModelRateLimits(ctx context.Context, repo AccountRepository, account *Account, modelName, prefix string, statusCode int, resetAt time.Time, afterSmartRetry bool) bool {
	if account == nil || repo == nil {
		return false
	}
	keys := antigravityModelRateLimitKeys(modelName)
	if len(keys) == 0 {
		return false
	}

	success := false
	for _, key := range keys {
		if setModelRateLimitByModelName(ctx, repo, account.ID, key, prefix, statusCode, resetAt, afterSmartRetry) {
			s.updateAccountModelRateLimitInCache(ctx, account, key, resetAt)
			success = true
		}
	}
	return success
}

func (s *AntigravityGatewayService) clearStickySession(ctx context.Context, groupID int64, sessionHash string) {
	if s == nil || s.cache == nil || strings.TrimSpace(sessionHash) == "" {
		return
	}
	if err := s.cache.DeleteSessionAccountID(ctx, groupID, sessionHash); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] sticky_session_clear_failed group_id=%d session=%s err=%v", groupID, shortSessionHash(sessionHash), err)
	}
}

func antigravityFallbackCooldownSeconds() (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(antigravityFallbackSecondsEnv))
	if raw == "" {
		return 0, false
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// antigravitySmartRetryInfo 智能重试所需的信息
type antigravitySmartRetryInfo struct {
	RetryDelay               time.Duration // 重试延迟时间
	ModelName                string        // 限流的模型名称（如 "claude-sonnet-4-5"）
	IsModelCapacityExhausted bool          // 是否为模型容量不足（MODEL_CAPACITY_EXHAUSTED）
}

// parseAntigravitySmartRetryInfo 解析 Google RPC RetryInfo 和 ErrorInfo 信息
// 返回解析结果，如果解析失败或不满足条件返回 nil
//
// 支持两种情况：
// 1. 429 RESOURCE_EXHAUSTED + RATE_LIMIT_EXCEEDED：
//   - error.status == "RESOURCE_EXHAUSTED"
//   - error.details[].reason == "RATE_LIMIT_EXCEEDED"
//
// 2. 503 UNAVAILABLE + MODEL_CAPACITY_EXHAUSTED：
//   - error.status == "UNAVAILABLE"
//   - error.details[].reason == "MODEL_CAPACITY_EXHAUSTED"
//
// 必须满足以下条件才会返回有效值：
// - error.details[] 中存在 @type == "type.googleapis.com/google.rpc.RetryInfo" 的元素
// - 该元素包含 retryDelay 字段，格式为 "数字s"（如 "0.201506475s"）
func parseAntigravitySmartRetryInfo(body []byte) *antigravitySmartRetryInfo {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		return nil
	}

	// 检查 status 是否符合条件
	// 情况1: 429 RESOURCE_EXHAUSTED (需要进一步检查 reason == RATE_LIMIT_EXCEEDED)
	// 情况2: 503 UNAVAILABLE (需要进一步检查 reason == MODEL_CAPACITY_EXHAUSTED)
	status, _ := errObj["status"].(string)
	isResourceExhausted := status == googleRPCStatusResourceExhausted
	isUnavailable := status == googleRPCStatusUnavailable

	if !isResourceExhausted && !isUnavailable {
		return nil
	}

	details, ok := errObj["details"].([]any)
	if !ok {
		return nil
	}

	var retryDelay time.Duration
	var modelName string
	var hasRateLimitExceeded bool      // 429 需要此 reason
	var hasModelCapacityExhausted bool // 503 需要此 reason

	for _, d := range details {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}

		atType, _ := dm["@type"].(string)

		// 从 ErrorInfo 提取模型名称和 reason
		if atType == googleRPCTypeErrorInfo {
			if meta, ok := dm["metadata"].(map[string]any); ok {
				if model, ok := meta["model"].(string); ok {
					modelName = normalizeAntigravityModelName(model)
				}
			}
			// 检查 reason
			if reason, ok := dm["reason"].(string); ok {
				if reason == googleRPCReasonModelCapacityExhausted {
					hasModelCapacityExhausted = true
				}
				if reason == googleRPCReasonRateLimitExceeded {
					hasRateLimitExceeded = true
				}
			}
			continue
		}

		// 从 RetryInfo 提取重试延迟
		if atType == googleRPCTypeRetryInfo {
			delay, ok := dm["retryDelay"].(string)
			if !ok || delay == "" {
				continue
			}
			// 使用 time.ParseDuration 解析，支持所有 Go duration 格式
			// 例如: "0.5s", "10s", "4m50s", "1h30m", "200ms" 等
			dur, err := time.ParseDuration(delay)
			if err != nil {
				logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] failed to parse retryDelay: %s error=%v", delay, err)
				continue
			}
			retryDelay = dur
		}
	}

	// 验证条件
	// 情况1: RESOURCE_EXHAUSTED 需要有 RATE_LIMIT_EXCEEDED reason
	// 情况2: UNAVAILABLE 需要有 MODEL_CAPACITY_EXHAUSTED reason
	if isResourceExhausted && !hasRateLimitExceeded {
		return nil
	}
	if isUnavailable && !hasModelCapacityExhausted {
		return nil
	}

	// 必须有模型名才返回有效结果
	if modelName == "" {
		return nil
	}

	// 如果上游未提供 retryDelay，使用默认限流时间
	if retryDelay <= 0 {
		retryDelay = antigravityDefaultRateLimitDuration
	}

	return &antigravitySmartRetryInfo{
		RetryDelay:               retryDelay,
		ModelName:                modelName,
		IsModelCapacityExhausted: hasModelCapacityExhausted,
	}
}

// shouldTriggerAntigravitySmartRetry 判断是否应该触发智能重试
// 返回：
//   - shouldRetry: 是否应该智能重试（retryDelay < antigravityRateLimitThreshold，或 MODEL_CAPACITY_EXHAUSTED）
//   - shouldRateLimitModel: 是否应该限流模型并切换账号（仅 RATE_LIMIT_EXCEEDED 且 retryDelay >= 阈值）
//   - waitDuration: 等待时间
//   - modelName: 限流的模型名称
//   - isModelCapacityExhausted: 是否为模型容量不足（MODEL_CAPACITY_EXHAUSTED）
func shouldTriggerAntigravitySmartRetry(account *Account, respBody []byte) (shouldRetry bool, shouldRateLimitModel bool, waitDuration time.Duration, modelName string, isModelCapacityExhausted bool) {
	if account.Platform != PlatformAntigravity {
		return false, false, 0, "", false
	}

	info := parseAntigravitySmartRetryInfo(respBody)
	if info == nil {
		return false, false, 0, "", false
	}

	// MODEL_CAPACITY_EXHAUSTED（模型容量不足）：所有账号共享同一模型容量池
	// 切换账号无意义，使用固定 1s 间隔重试
	if info.IsModelCapacityExhausted {
		return true, false, antigravityModelCapacityRetryWait, info.ModelName, true
	}

	// RATE_LIMIT_EXCEEDED（账号级限流）：
	// retryDelay >= 阈值：直接限流模型，不重试
	// 注意：如果上游未提供 retryDelay，parseAntigravitySmartRetryInfo 已设置为默认 30s
	if info.RetryDelay >= antigravityRateLimitThreshold {
		return false, true, info.RetryDelay, info.ModelName, false
	}

	// retryDelay < 阈值：智能重试
	waitDuration = info.RetryDelay
	if waitDuration < antigravitySmartRetryMinWait {
		waitDuration = antigravitySmartRetryMinWait
	}

	return true, false, waitDuration, info.ModelName, false
}

// handleModelRateLimitParams 模型级限流处理参数
type handleModelRateLimitParams struct {
	ctx             context.Context
	prefix          string
	account         *Account
	statusCode      int
	body            []byte
	cache           GatewayCache
	groupID         int64
	sessionHash     string
	isStickySession bool
}

// handleModelRateLimitResult 模型级限流处理结果
type handleModelRateLimitResult struct {
	Handled      bool                           // 是否已处理
	ShouldRetry  bool                           // 是否等待后重试
	WaitDuration time.Duration                  // 等待时间
	SwitchError  *AntigravityAccountSwitchError // 账号切换错误
}

// handleModelRateLimit 处理模型级限流（在原有逻辑之前调用）
// 仅处理 429/503，解析模型名和 retryDelay
// - MODEL_CAPACITY_EXHAUSTED: 返回 Handled=true（实际重试由 handleSmartRetry 处理）
// - RATE_LIMIT_EXCEEDED + retryDelay < 阈值: 返回 ShouldRetry=true，由调用方等待后重试
// - RATE_LIMIT_EXCEEDED + retryDelay >= 阈值: 设置模型限流 + 清除粘性会话 + 返回 SwitchError
func (s *AntigravityGatewayService) handleModelRateLimit(p *handleModelRateLimitParams) *handleModelRateLimitResult {
	if p.statusCode != 429 && p.statusCode != 503 {
		return &handleModelRateLimitResult{Handled: false}
	}

	info := parseAntigravitySmartRetryInfo(p.body)
	if info == nil || info.ModelName == "" {
		return &handleModelRateLimitResult{Handled: false}
	}

	// MODEL_CAPACITY_EXHAUSTED：模型容量不足，所有账号共享同一容量池
	// 切换账号无意义，不设置模型限流（实际重试由 handleSmartRetry 处理）
	if info.IsModelCapacityExhausted {
		log.Printf("%s status=%d model_capacity_exhausted model=%s (not switching account, retry handled by smart retry)",
			p.prefix, p.statusCode, info.ModelName)
		return &handleModelRateLimitResult{
			Handled: true,
		}
	}

	// RATE_LIMIT_EXCEEDED: < antigravityRateLimitThreshold: 等待后重试
	if info.RetryDelay < antigravityRateLimitThreshold {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limit_wait model=%s wait=%v",
			p.prefix, p.statusCode, info.ModelName, info.RetryDelay)
		return &handleModelRateLimitResult{
			Handled:      true,
			ShouldRetry:  true,
			WaitDuration: info.RetryDelay,
		}
	}

	// RATE_LIMIT_EXCEEDED: >= antigravityRateLimitThreshold: 设置限流 + 清除粘性会话 + 切换账号
	s.setModelRateLimitAndClearSession(p, info)

	return &handleModelRateLimitResult{
		Handled: true,
		SwitchError: &AntigravityAccountSwitchError{
			OriginalAccountID: p.account.ID,
			RateLimitedModel:  info.ModelName,
			IsStickySession:   p.isStickySession,
		},
	}
}

// setModelRateLimitAndClearSession 设置模型限流并清除粘性会话
func (s *AntigravityGatewayService) setModelRateLimitAndClearSession(p *handleModelRateLimitParams, info *antigravitySmartRetryInfo) {
	resetAt := time.Now().Add(info.RetryDelay)
	logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limited model=%s account=%d reset_in=%v",
		p.prefix, p.statusCode, info.ModelName, p.account.ID, info.RetryDelay)

	s.setAntigravityModelRateLimits(p.ctx, s.accountRepo, p.account, info.ModelName, p.prefix, p.statusCode, resetAt, false)

	// 清除粘性会话绑定
	if p.cache != nil && p.sessionHash != "" {
		_ = p.cache.DeleteSessionAccountID(p.ctx, p.groupID, p.sessionHash)
	}
}

// updateAccountModelRateLimitInCache 立即更新 Redis 中账号的模型限流状态
func (s *AntigravityGatewayService) updateAccountModelRateLimitInCache(ctx context.Context, account *Account, modelKey string, resetAt time.Time) {
	if s.schedulerSnapshot == nil || account == nil || modelKey == "" {
		return
	}

	// 更新账号对象的 Extra 字段
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}

	limits, _ := account.Extra["model_rate_limits"].(map[string]any)
	if limits == nil {
		limits = make(map[string]any)
		account.Extra["model_rate_limits"] = limits
	}

	limits[modelKey] = map[string]any{
		"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}

	// 更新 Redis 快照
	if err := s.schedulerSnapshot.UpdateAccountInCache(ctx, account); err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] cache_update_failed account=%d model=%s err=%v", account.ID, modelKey, err)
	}
}

func (s *AntigravityGatewayService) handleUpstreamError(
	ctx context.Context, prefix string, account *Account,
	statusCode int, headers http.Header, body []byte,
	requestedModel string,
	groupID int64, sessionHash string, isStickySession bool,
) *handleModelRateLimitResult {
	// 遵守自定义错误码策略：未命中则跳过所有限流处理
	if !account.ShouldHandleErrorCode(statusCode) {
		return nil
	}
	// 模型级限流处理（优先）
	result := s.handleModelRateLimit(&handleModelRateLimitParams{
		ctx:             ctx,
		prefix:          prefix,
		account:         account,
		statusCode:      statusCode,
		body:            body,
		cache:           s.cache,
		groupID:         groupID,
		sessionHash:     sessionHash,
		isStickySession: isStickySession,
	})
	if result.Handled {
		return result
	}

	// 503 仅处理模型限流（MODEL_CAPACITY_EXHAUSTED），非模型限流不做额外处理
	// 避免将普通的 503 错误误判为账号问题
	if statusCode == 503 {
		return nil
	}

	// 429：尝试解析模型级限流，解析失败时兜底为账号级限流
	if statusCode == 429 {
		if logBody, maxBytes := s.getLogConfig(); logBody {
			logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity-Debug] 429 response body: %s", truncateString(string(body), maxBytes))
		}

		resetAt := ParseGeminiRateLimitResetTime(body)
		defaultDur := s.getDefaultRateLimitDuration()

		// 尝试解析模型 key 并设置模型级限流
		//
		// 注意：requestedModel 可能是"映射前"的请求模型名（例如 claude-opus-4-6），
		// 调度与限流判定使用的是 Antigravity 最终模型名（包含映射与 thinking 后缀）。
		// 因此这里必须写入最终模型 key，确保后续调度能正确避开已限流模型。
		modelKey := resolveFinalAntigravityModelKey(ctx, account, requestedModel)
		if strings.TrimSpace(modelKey) == "" {
			// 极少数情况下无法映射（理论上不应发生：能转发成功说明映射已通过），
			// 保持旧行为作为兜底，避免完全丢失模型级限流记录。
			modelKey = resolveAntigravityModelKey(requestedModel)
		}
		if modelKey != "" {
			ra := s.resolveResetTime(resetAt, defaultDur)
			if !s.setAntigravityModelRateLimits(ctx, s.accountRepo, account, modelKey, prefix, statusCode, ra, false) {
				logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 model_rate_limit_set_failed model=%s", prefix, modelKey)
			} else {
				logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 model_rate_limited model=%s account=%d reset_at=%v reset_in=%v",
					prefix, modelKey, account.ID, ra.Format("15:04:05"), time.Until(ra).Truncate(time.Second))
			}
			return nil
		}

		// 无法解析模型 key，兜底为账号级限流
		ra := s.resolveResetTime(resetAt, defaultDur)
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 rate_limited account=%d reset_at=%v reset_in=%v (fallback)",
			prefix, account.ID, ra.Format("15:04:05"), time.Until(ra).Truncate(time.Second))
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, ra); err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 rate_limit_set_failed account=%d error=%v", prefix, account.ID, err)
		}
		return nil
	}
	// 其他错误码继续使用 rateLimitService
	if s.rateLimitService == nil {
		return nil
	}
	shouldDisable := s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
	if shouldDisable {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d marked_error", prefix, statusCode)
	}
	return nil
}

// getDefaultRateLimitDuration 获取默认限流时间
func (s *AntigravityGatewayService) getDefaultRateLimitDuration() time.Duration {
	defaultDur := antigravityDefaultRateLimitDuration
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.AntigravityFallbackCooldownMinutes > 0 {
		defaultDur = time.Duration(s.settingService.cfg.Gateway.AntigravityFallbackCooldownMinutes) * time.Minute
	}
	if override, ok := antigravityFallbackCooldownSeconds(); ok {
		defaultDur = override
	}
	return defaultDur
}

// resolveResetTime 根据解析的重置时间或默认时长计算重置时间点
func (s *AntigravityGatewayService) resolveResetTime(resetAt *int64, defaultDur time.Duration) time.Time {
	if resetAt != nil {
		return time.Unix(*resetAt, 0)
	}
	return time.Now().Add(defaultDur)
}
