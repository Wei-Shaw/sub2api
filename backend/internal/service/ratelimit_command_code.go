package service

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Command Code 用量超限的响应式处理，依据官方错误说明
// （https://commandcode.ai/docs/troubleshooting/errors）：
//
//   - Rate limit：短时间请求过多，很快恢复，走默认 429 逻辑；
//   - Usage exceeded，三种来源：
//     1. 5 小时 / 每周滚动窗口用满，文案如
//     "You've reached your 5-hour usage limit. Resets in 2h 41m (3:00 PM)"；
//     2. 月度积分耗尽；
//     3. 组织为成员设的花费上限，分成员总额度与成员某个模型的额度两种，后者文案如
//     "You've reached the $10.00/mo spending limit your organization set for GLM-5. It resets in 3 days."
//
// 文档没有给出状态码与响应体结构，因此按文案分类，402 / 403 / 429 同样处理；
// 页面路径中的 usage_exceeded 可能是错误码，一并识别。未识别的错误交回默认逻辑。
//
// 窗口用满写带原因的临时停调（而非 SetRateLimited）：充值积分不受滚动窗口限制，
// 额度探测发现充值积分后只清除这一类停调（见 queryCommandCodeUsage）。

type commandCodeUsageErrorKind int

const (
	commandCodeUsageErrorNone commandCodeUsageErrorKind = iota
	// commandCodeUsageWindowLimit 滚动窗口用满：冷却到窗口重置。
	commandCodeUsageWindowLimit
	// commandCodeUsageCreditsExhausted 积分耗尽：可恢复停调，积分恢复后由周期检测解除。
	commandCodeUsageCreditsExhausted
	// commandCodeUsageSpendLimit 组织花费上限：模型级只限当前模型，成员总额度按账号冷却。
	commandCodeUsageSpendLimit
)

const (
	// commandCodeUsageLimitReason 是窗口用满临时停调的原因前缀，充值后据此只清除这一类停调。
	commandCodeUsageLimitReason = "command_code_usage_limit"
	commandCodeSpendLimitReason = "command_code_spend_limit"
	// 从文案解析出的重置时长上限：窗口最长为每周，花费上限最长按月。
	commandCodeMaxWindowCooldown     = 8 * 24 * time.Hour
	commandCodeMaxSpendLimitCooldown = 32 * 24 * time.Hour
	// commandCodeAccountSpendLimitRecheck 是账号级花费上限（或作用域不明）的最长冷却：
	// 管理员可能随时调高上限，到期后由下一次请求重新确认。
	commandCodeAccountSpendLimitRecheck = time.Hour
)

// commandCodeModelSpendLimitPattern 匹配组织为成员某个模型设的花费上限
// （"... spending limit your organization set for GLM-5."）；成员总额度没有 "for <模型>"。
var commandCodeModelSpendLimitPattern = regexp.MustCompile(`(?i)spending limit your organization set for\s+([^.\s][^.]*)`)

// commandCodeSpendLimitIsModelScoped 报告花费上限是否只针对单个模型。
func commandCodeSpendLimitIsModelScoped(body []byte) bool {
	match := commandCodeModelSpendLimitPattern.FindSubmatch(body)
	if match == nil {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(string(match[1])))
	return target != "" && !strings.HasPrefix(target, "you")
}

func classifyCommandCodeUsageError(body []byte) commandCodeUsageErrorKind {
	text := strings.ToLower(string(body))
	switch {
	case strings.Contains(text, "spending limit"):
		return commandCodeUsageSpendLimit
	case strings.Contains(text, "usage limit"):
		return commandCodeUsageWindowLimit
	case cnProviderResponseIndicatesInsufficientBalance(body),
		strings.Contains(text, "insufficient credit"),
		strings.Contains(text, "out of credits"),
		strings.Contains(text, "usage_exceeded"),
		strings.Contains(text, "usage exceeded"):
		return commandCodeUsageCreditsExhausted
	default:
		return commandCodeUsageErrorNone
	}
}

// commandCodeResetAt 解析文案中的 "Resets in 2h 41m" / "resets in 3 days"；无法解析
// 或超过 maxWait 时返回 nil。
func commandCodeResetAt(body []byte, now time.Time, maxWait time.Duration) *time.Time {
	after := parseOpenCodeGoUsageLimitResetDuration(extractUpstreamErrorMessage(body))
	if after <= 0 {
		after = parseOpenCodeGoUsageLimitResetDuration(string(body))
	}
	if after <= 0 || after > maxWait {
		return nil
	}
	until := now.Add(after)
	return &until
}

// handleCommandCodeUsageError 处理已识别的 Command Code 用量超限；handled=false 时由
// 调用方继续走默认逻辑。shouldDisable 与同类状态码的既有口径一致：429 不置位；
// 只限单个模型时也不置位，避免调用方把整个账号拉黑。
func (s *RateLimitService) handleCommandCodeUsageError(
	ctx context.Context,
	account *Account,
	statusCode int,
	responseBody []byte,
	upstreamMsg string,
) (handled, shouldDisable bool) {
	accountWide := statusCode != http.StatusTooManyRequests
	now := time.Now()
	switch classifyCommandCodeUsageError(responseBody) {
	case commandCodeUsageWindowLimit:
		until := commandCodeResetAt(responseBody, now, commandCodeMaxWindowCooldown)
		if until == nil {
			until = cnProviderQuotaSnapshotReset(account, now)
		}
		if until == nil {
			return false, false
		}
		reason := commandCodeUsageLimitReason
		if msg := strings.TrimSpace(upstreamMsg); msg != "" {
			reason += ": " + msg
		}
		s.notifyAccountSchedulingBlocked(account, *until, commandCodeUsageLimitReason)
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, *until, reason); err != nil {
			slog.Warn("command_code_usage_limit_set_failed", "account_id", account.ID, "error", err)
			return false, false
		}
		slog.Info("command_code_usage_limit", "account_id", account.ID, "status_code", statusCode, "reset_at", until.UTC())
		return true, accountWide
	case commandCodeUsageCreditsExhausted:
		s.handleCNProviderInsufficientBalance(ctx, account, upstreamMsg)
		return true, accountWide
	case commandCodeUsageSpendLimit:
		return true, s.handleCommandCodeSpendLimit(ctx, account, statusCode, responseBody, now)
	default:
		return false, false
	}
}

// handleCommandCodeSpendLimit 处理组织花费上限：文案指明模型且请求模型已知时只限该
// 模型，冷却到文案中的重置时间；成员总额度或作用域不明时整个账号冷却，但最长
// commandCodeAccountSpendLimitRecheck，避免管理员调高上限后账号仍停用数天，也不会
// 因累积 403 被永久禁用。
func (s *RateLimitService) handleCommandCodeSpendLimit(ctx context.Context, account *Account, statusCode int, responseBody []byte, now time.Time) (shouldDisable bool) {
	reset := commandCodeResetAt(responseBody, now, commandCodeMaxSpendLimitCooldown)
	modelKey := modelRateLimitKeyForUpstreamModelNotFound(ctx, account, tempUnschedulableModel(ctx, nil))
	if modelKey != "" && commandCodeSpendLimitIsModelScoped(responseBody) {
		until := reset
		if until == nil {
			fallback, ok := s.get429FallbackCooldown(ctx, account)
			if !ok || fallback <= 0 {
				fallback = time.Duration(defaultRateLimit429CooldownSeconds) * time.Second
			}
			next := now.Add(fallback)
			until = &next
		}
		if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, modelKey, *until, commandCodeSpendLimitReason); err != nil {
			slog.Warn("command_code_spend_limit_set_failed", "account_id", account.ID, "model", modelKey, "error", err)
		}
		slog.Info("command_code_spend_limit", "account_id", account.ID, "model", modelKey, "status_code", statusCode, "reset_at", until.UTC())
		return false
	}
	until := now.Add(commandCodeAccountSpendLimitRecheck)
	if reset != nil && reset.Before(until) {
		until = *reset
	}
	s.notifyAccountSchedulingBlocked(account, until, commandCodeSpendLimitReason)
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, commandCodeSpendLimitReason); err != nil {
		slog.Warn("command_code_spend_limit_set_failed", "account_id", account.ID, "error", err)
	}
	slog.Info("command_code_spend_limit", "account_id", account.ID, "status_code", statusCode, "until", until.UTC())
	return statusCode != http.StatusTooManyRequests
}
