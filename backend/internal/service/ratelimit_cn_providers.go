package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// 国产供应商（kimi/zhipu/deepseek）的响应式冷却辅助。
//
// 与 openai/anthropic 不同：
//   - 余额不足是「可恢复」状态（充值/检测恢复后自动重新调度），不能走 handleAuthError
//     永久置 status=error。这里改为 SetTempUnschedulable，由 CN 余额检测周期任务
//     （cn_provider_balance_check_service.go）在余额恢复后 ClearTempUnschedulable。
//   - Coding Plan 滚动窗口耗尽（429）的冷却终点应是真实的窗口重置时间（已由
//     CNProviderQuotaService 落入 account.Extra 快照），而非默认的秒级兜底。

// cnBalanceExtraSuffixLow 标记账号响应过「余额不足」，供余额检测任务区分
// 「确属余额不足」与「尚未探测」。
const cnBalanceExtraSuffixLow = "balance_low"

const (
	// Qwen Token Plan is a one-time allowance. These markers are written only
	// after the inference endpoint explicitly reports the allowance as exhausted.
	qwenTokenPlanExhaustedExtraKey       = "qwen_token_plan_exhausted"
	qwenTokenPlanExhaustedAtExtraKey     = "qwen_token_plan_exhausted_at"
	qwenTokenPlanExhaustedReasonExtraKey = "qwen_token_plan_exhausted_reason"
)

// cnBalanceLowReasonPrefix 是余额不足临时停调 reason 的稳定前缀。
// 周期余额检测任务据此识别「是我们停调的」并在余额恢复后安全清除——不会误清
// 其他子系统（阈值/限流/401）写入的临时停调。
const cnBalanceLowReasonPrefix = "cn_balance_low"

const kimiConcurrentRequestLimitMessage = "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."

const cnConcurrencyLimitReasonPrefix = "cn_concurrency_limit"

func isCNProviderConcurrencyLimit403(account *Account, upstreamMsg string) bool {
	return account != nil && account.Platform == PlatformKimi &&
		strings.TrimSpace(upstreamMsg) == kimiConcurrentRequestLimitMessage
}

func (s *RateLimitService) handleCNProviderConcurrencyLimit403(
	ctx context.Context,
	account *Account,
) {
	until := time.Now().Add(time.Duration(openAI403CooldownMinutesDefault) * time.Minute)
	reason := cnConcurrencyLimitReasonPrefix + ": " + kimiConcurrentRequestLimitMessage
	s.notifyAccountSchedulingBlocked(account, until, cnConcurrencyLimitReasonPrefix)
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
		slog.Warn("cn_concurrency_limit_set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Info("cn_provider_concurrency_limited",
		"account_id", account.ID,
		"platform", account.Platform,
		"until", until.UTC(),
	)
}

// cnBalanceLowReason 构造余额不足临时停调的 reason（带稳定前缀）。
func cnBalanceLowReason(upstreamMsg string) string {
	if upstreamMsg = strings.TrimSpace(upstreamMsg); upstreamMsg != "" {
		return cnBalanceLowReasonPrefix + ": " + upstreamMsg
	}
	return cnBalanceLowReasonPrefix + ": 余额不足，账号临时停调"
}

// cnProviderResponseIndicatesInsufficientBalance 通过响应体文案识别余额不足
// （智谱 payg 无独立余额端点，仅能靠响应文案识别）。
func cnProviderResponseIndicatesInsufficientBalance(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := strings.ToLower(string(body))
	return strings.Contains(s, "余额不足") ||
		strings.Contains(s, "insufficient balance") ||
		strings.Contains(s, "insufficient_credit") ||
		strings.Contains(s, "balance is not enough") ||
		strings.Contains(s, "no enough balance")
}

// handleCNProviderInsufficientBalance 把余额不足标记为可恢复的临时停调：
// 写入 balance_low 快照 + SetTempUnschedulable 一个余额检测周期，
// 由周期任务在余额恢复后清除。返回前已通知调度阻塞。
func (s *RateLimitService) handleCNProviderInsufficientBalance(
	ctx context.Context,
	account *Account,
	upstreamMsg string,
) {
	msg := cnBalanceLowReason(upstreamMsg)

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		cnExtraKey(account.Platform, cnBalanceExtraSuffixLow): true,
	}); err != nil {
		slog.Warn("cn_balance_low_mark_failed", "account_id", account.ID, "error", err)
	}

	until := time.Now().Add(s.cnBalanceCooldownDuration())
	s.notifyAccountSchedulingBlocked(account, until, "cn_insufficient_balance")
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, msg); err != nil {
		slog.Warn("cn_balance_set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Info("cn_provider_insufficient_balance",
		"account_id", account.ID,
		"platform", account.Platform,
		"until", until.UTC(),
	)
}

// cnBalanceCooldownDuration 返回余额不足临时停调的持续时长（= 2× 余额检测周期，
// 默认 20 分钟）。周期任务会在余额恢复后提前清除，故此处只需保证冷却覆盖到下一次
// 周期检测即可。
func (s *RateLimitService) cnBalanceCooldownDuration() time.Duration {
	minutes := 10
	if s != nil && s.cfg != nil {
		if cfgMin := s.cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes; cfgMin > 0 {
			minutes = cfgMin
		}
	}
	cooldown := time.Duration(minutes) * time.Minute * 2
	if cooldown < time.Minute {
		cooldown = 10 * time.Minute
	}
	return cooldown
}

// cnProviderQuotaSnapshotReset 读取 Coding Plan 账号快照中最早一个仍在未来的窗口
// 重置时间（5h / weekly）。429 多数由 5h 滚动窗口触发，取较早的重置点可避免
// 把账号冷却到 weekly 重置（可达数天）的过度停调；如果确是 weekly 窗口耗尽，
// 周期额度探测刷新快照后阈值评估会再次停调到正确的时间点。
// 无快照或均已过期返回 nil。
func cnProviderQuotaSnapshotReset(account *Account, now time.Time) *time.Time {
	if account == nil || !account.IsCNProvider() || !account.IsCodingPlan() || len(account.Extra) == 0 {
		return nil
	}
	provider := account.Platform
	var earliest *time.Time
	for _, suffix := range []string{cnExtraSuffix5hReset, cnExtraSuffixWeeklyReset} {
		t := parseSchedulingResetAt(account.Extra[cnExtraKey(provider, suffix)])
		if t == nil || !t.After(now) {
			continue
		}
		if earliest == nil || t.Before(*earliest) {
			earliest = t
		}
	}
	return earliest
}

// applyCNProviderReactive429 处理国产供应商的 429 响应。
// 返回 true 表示已处理（调用方应 return），false 表示未命中、继续走默认 429 逻辑。
func (s *RateLimitService) applyCNProviderReactive429(
	ctx context.Context,
	account *Account,
	headers http.Header,
	responseBody []byte,
) bool {
	if account == nil || !account.IsCNProvider() {
		return false
	}
	// Token Plan is a one-time allowance. Its documented exhaustion response is
	// a 429 with explicit quota wording. Do not turn a generic 429, credential
	// error, or transport failure into a permanent scheduling block.
	if account.IsTokenPlan() {
		if qwenTokenPlanQuotaExhausted(responseBody) {
			s.handleQwenTokenPlanExhausted(ctx, account, responseBody)
			return true
		}
		return false
	}
	// 1) 余额不足文案：可恢复临时停调（含智谱 payg 这类无余额端点的场景）。
	if cnProviderResponseIndicatesInsufficientBalance(responseBody) {
		s.handleCNProviderInsufficientBalance(ctx, account, extractUpstreamErrorMessage(responseBody))
		return true
	}
	// 2) Coding Plan 窗口耗尽：冷却到快照中最早的窗口重置点（见
	// cnProviderQuotaSnapshotReset：429 多由 5h 窗口触发，取较早点避免过度停调）。
	if account.IsCodingPlan() {
		if until := cnProviderQuotaSnapshotReset(account, time.Now()); until != nil {
			s.notifyAccountSchedulingBlocked(account, *until, "429")
			if err := s.accountRepo.SetRateLimited(ctx, account.ID, *until); err != nil {
				slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
				return true
			}
			slog.Info("cn_coding_plan_rate_limited",
				"account_id", account.ID,
				"platform", account.Platform,
				"reset_at", *until,
			)
			return true
		}
	}
	return false
}

// qwenTokenPlanQuotaExhausted recognizes Qwen Token Plan allowance exhaustion.
// The caller already limits this to a Token Plan account on HTTP 429, so a
// literal "token-plan" substring is not required.
//
// Documented plan-exhaustion wording (permanently unschedulable):
//   - "Your token-plan ... quota has been exhausted. The quota will reset at ..."
//   - "Allocated quota exceeded" (Token Plan personal FAQ: 5h / 7-day limit used up)
//   - "insufficient_quota" / "You exceeded your current quota"
//   - "quota exhausted" even when the one-time allowance has no reset time
//   - Chinese 额度/配额/限额 + 用尽/耗尽
//
// Documented transient rate limits (must not pause the account):
//   - "Requests rate limit exceeded" / "API-Key Requests rate limit exceeded"
//   - "Request rate increased too quickly"
//   - generic "rate limit exceeded" without quota / allocation evidence
func qwenTokenPlanQuotaExhausted(responseBody []byte) bool {
	if len(responseBody) == 0 {
		return false
	}

	message := strings.ToLower(string(responseBody))
	if qwenTokenPlanTransientRateLimit(message) {
		return false
	}
	if strings.Contains(message, "token-plan") &&
		(strings.Contains(message, "quota") || strings.Contains(message, "exhausted") || strings.Contains(message, "exceeded")) {
		return true
	}
	if strings.Contains(message, "quota") && strings.Contains(message, "exhausted") {
		return true
	}
	if strings.Contains(message, "allocated quota exceeded") {
		return true
	}
	if strings.Contains(message, "insufficient_quota") || strings.Contains(message, "exceeded your current quota") {
		return true
	}
	if (strings.Contains(message, "额度") || strings.Contains(message, "配额") || strings.Contains(message, "限额")) &&
		(strings.Contains(message, "用尽") || strings.Contains(message, "耗尽")) {
		return true
	}
	hasQuota := strings.Contains(message, "quota")
	hasExhaustion := strings.Contains(message, "exhausted") || strings.Contains(message, "exceeded")
	hasReset := strings.Contains(message, "will reset") || strings.Contains(message, "reset at")
	return hasQuota && hasExhaustion && hasReset
}

func qwenTokenPlanTransientRateLimit(message string) bool {
	if strings.Contains(message, "requests rate limit") ||
		strings.Contains(message, "request rate increased too quickly") ||
		strings.Contains(message, "api-key requests rate limit") {
		return true
	}
	if strings.Contains(message, "rate limit exceeded") &&
		!strings.Contains(message, "quota") &&
		!strings.Contains(message, "allocation") &&
		!strings.Contains(message, "token-plan") {
		return true
	}
	return false
}

// handleQwenTokenPlanExhausted permanently removes an exhausted one-time Token
// Plan account from scheduling. This state must never be automatically cleared.
func (s *RateLimitService) handleQwenTokenPlanExhausted(ctx context.Context, account *Account, responseBody []byte) {
	now := time.Now().UTC()
	reason := qwenTokenPlanExhaustionReason(responseBody)
	updates := map[string]any{
		qwenTokenPlanExhaustedExtraKey:       true,
		qwenTokenPlanExhaustedAtExtraKey:     now.Format(time.RFC3339),
		qwenTokenPlanExhaustedReasonExtraKey: reason,
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("qwen_token_plan_exhaustion_mark_failed", "account_id", account.ID, "error", err)
	}

	s.notifyAccountSchedulingBlocked(account, time.Time{}, "qwen_token_plan_exhausted")
	if err := s.accountRepo.SetSchedulable(ctx, account.ID, false); err != nil {
		slog.Warn("qwen_token_plan_pause_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Warn("qwen_token_plan_exhausted", "account_id", account.ID)
}

func qwenTokenPlanExhaustionReason(responseBody []byte) string {
	message := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
	if message == "" {
		message = strings.TrimSpace(string(responseBody))
	}
	message = strings.Join(strings.Fields(message), " ")
	message = sanitizeUpstreamErrorMessage(message)
	message = truncateForLog([]byte(message), 512)
	if message == "" {
		return "Qwen Token Plan quota exhausted"
	}
	return "Qwen Token Plan quota exhausted: " + message
}
