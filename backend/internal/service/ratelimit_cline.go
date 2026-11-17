package service

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Cline 上游错误的响应式处理。依据官方错误说明（https://docs.cline.bot/api/errors）与
// Cline 客户端的错误分类（cline/cline：sdk/packages/llms/src/providers/errors.ts、
// apps/vscode/src/services/error/ClineError.ts）：
//
//   - 402 insufficient_credits：积分不足；
//   - 429 SPEND_LIMIT_EXCEEDED：组织为成员设的花费上限；
//   - INFERENCE_CAP_ERROR：推理额度上限（客户端归为 QuotaExceeded，文案由服务端给出）；
//   - "You have reached your … ClinePass limit … Please try again later."：ClinePass
//     5 小时 / 7 天 / 30 天滚动用量上限，文案不带重置时间；
//   - "the user is not subscribed to required model plan" 与 "organization accounts
//     cannot use individual model inference subscriptions"：没有可用的 ClinePass 订阅；
//   - "Free limit reached on model … try again in …"：免费模型的每日额度，只限该模型。
//
// 只读取结构化 error.code 与错误消息；ClinePass / 免费模型限额兼容官方固定文案。
// metadata 等字段可能回显请求内容，不能据此冷却钱包；未识别的错误交回默认逻辑。
//
// 作用范围：积分与 ClinePass 是同一个 Key 下互不影响的两个钱包，一边超限只冷却该边的
// 全部模型（模型级冷却键，见 clineWalletRateLimitKey），不停整个账号；钱包按请求模型
// 判定——cline-pass/* 请求只会被订阅额度拒绝，其余付费模型只会被积分拒绝。

type clineErrorKind int

const (
	clineErrorNone clineErrorKind = iota
	// clineCreditsExhausted 积分不足：冷却积分钱包，用量探测发现余额恢复后解除。
	clineCreditsExhausted
	// clineSpendLimit 组织花费上限：冷却后由下一次请求重新确认。
	clineSpendLimit
	// clineInferenceCap 推理额度上限：归属由请求模型所在的钱包决定。
	clineInferenceCap
	// clinePassLimit ClinePass 用量上限：冷却到对应窗口的重置时间。
	clinePassLimit
	// clinePassUnavailable 没有可用的 ClinePass 订阅（未订阅或组织账户）。
	clinePassUnavailable
	// clineFreeModelLimit 免费模型的每日额度：只限当前模型。
	clineFreeModelLimit
)

const (
	clinePassLimitReason       = "cline_pass_limit"
	clinePassUnavailableReason = "cline_pass_unavailable"
	clineSpendLimitReason      = "cline_spend_limit"
	clineInferenceCapReason    = "cline_inference_cap"
	clineFreeModelLimitReason  = "cline_free_model_limit"
	clineCreditsReason         = "cline_credits_exhausted"

	// clineLimitRecheck 是无法得知重置时间时的冷却：到期后由下一次请求重新确认
	// （管理员可能调高上限、订阅可能续费、滚动窗口逐步释放）。
	clineLimitRecheck = time.Hour
	// ClinePass 7 天 / 30 天窗口没有用量快照时的冷却：窗口滚动释放，时间点无从得知，
	// 冷却较长但远短于窗口，由下一次请求或用量探测重新确认。
	clinePassWeeklyRecheck  = 6 * time.Hour
	clinePassMonthlyRecheck = 24 * time.Hour
	// clinePassMaxCooldown 限制按快照重置时间冷却的上限（最长的 30 天窗口）。
	clinePassMaxCooldown = 31 * 24 * time.Hour
	// clineFreeModelMaxCooldown 是从免费模型文案解析出的重置时长上限（每日额度）。
	clineFreeModelMaxCooldown = 25 * time.Hour
)

var (
	// clinePassLimitPattern 与 Cline 客户端的判定一致："you have reached your" 开头、
	// "please try again later." 结尾，中间含 "clinepass limit"。
	clinePassLimitPattern = regexp.MustCompile(`(?is)you have reached your(.*?)clinepass limit.*?please try again later\.`)
	clineTryAgainPattern  = regexp.MustCompile(`(?i)\btry\s+again\s+in\s+`)
)

// parseClineError 返回错误分类与命中的限额消息，后者用于解析窗口和重试时间。
func parseClineError(body []byte) (clineErrorKind, string) {
	if !gjson.ValidBytes(body) {
		return clineErrorNone, ""
	}
	err := gjson.GetBytes(body, "error")
	if !err.IsObject() {
		return clineErrorNone, ""
	}
	if code := err.Get("code"); code.Type == gjson.String {
		if kind := classifyClineErrorCode(code.String()); kind != clineErrorNone {
			return kind, ""
		}
	}
	// 官方客户端也识别 details.code 中的组织花费上限。
	if code := err.Get("details.code"); code.Type == gjson.String && classifyClineErrorCode(code.String()) == clineSpendLimit {
		return clineSpendLimit, ""
	}
	for _, path := range []string{"message", "details.message"} {
		message := err.Get(path)
		if message.Type != gjson.String {
			continue
		}
		if kind := classifyClineLimitMessage(message.String()); kind != clineErrorNone {
			return kind, message.String()
		}
	}
	return clineErrorNone, ""
}

func classifyClineErrorCode(code string) clineErrorKind {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "insufficient_credits":
		return clineCreditsExhausted
	case "spend_limit_exceeded":
		return clineSpendLimit
	case "inference_cap_error":
		return clineInferenceCap
	default:
		return clineErrorNone
	}
}

func classifyClineLimitMessage(message string) clineErrorKind {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "organization accounts cannot use individual model inference subscriptions"),
		strings.Contains(text, "the user is not subscribed to required model plan"):
		return clinePassUnavailable
	case strings.Contains(text, "free limit reached on model"):
		return clineFreeModelLimit
	case clinePassLimitPattern.MatchString(message):
		return clinePassLimit
	default:
		return clineErrorNone
	}
}

// clineErrorWallet 返回错误本身指明的钱包冷却键，用于请求模型未知的情况。
func clineErrorWallet(kind clineErrorKind) string {
	switch kind {
	case clinePassLimit, clinePassUnavailable, clineInferenceCap:
		return clinePassRateLimitKey
	case clineCreditsExhausted, clineSpendLimit:
		return clineCreditsRateLimitKey
	default:
		return ""
	}
}

// clinePassWindow 返回 ClinePass 超限文案所指的窗口（5h / weekly / monthly）。
func clinePassWindow(body []byte) string {
	match := clinePassLimitPattern.FindSubmatch(body)
	if match == nil {
		return "5h"
	}
	window := strings.ToLower(string(match[1]))
	switch {
	case strings.Contains(window, "month"):
		return "monthly"
	case strings.Contains(window, "week"):
		return "weekly"
	default:
		return "5h"
	}
}

// clinePassLimitCooldown 冷却到用量快照中该窗口的重置时间；没有可用快照时按窗口估计。
func clinePassLimitCooldown(account *Account, window string, now time.Time) time.Duration {
	if account != nil && account.Extra != nil {
		resetKey := map[string]string{
			"5h":      cnExtraSuffix5hReset,
			"weekly":  cnExtraSuffixWeeklyReset,
			"monthly": cnExtraSuffixMonthlyReset,
		}[window]
		if resetAt := parseSchedulingResetAt(account.Extra[cnExtraKey(PlatformCline, resetKey)]); resetAt != nil {
			if wait := resetAt.Sub(now); wait > 0 && wait <= clinePassMaxCooldown {
				return wait
			}
		}
	}
	switch window {
	case "monthly":
		return clinePassMonthlyRecheck
	case "weekly":
		return clinePassWeeklyRecheck
	default:
		return clineLimitRecheck
	}
}

// clineFreeModelResetAfter 解析 "try again in 3h 20m"；无法解析或超出上限时返回 0。
func clineFreeModelResetAfter(body []byte) time.Duration {
	text := string(body)
	loc := clineTryAgainPattern.FindStringIndex(text)
	if loc == nil {
		return 0
	}
	after := parseOpenCodeGoUsageLimitResetDuration("resets in " + text[loc[1]:])
	if after <= 0 || after > clineFreeModelMaxCooldown {
		return 0
	}
	return after
}

// handleClineError 处理已识别的 Cline 上游错误：冷却对应钱包（或免费模型本身），不停
// 整个账号。handled=false 时由调用方继续走默认逻辑。
func (s *RateLimitService) handleClineError(
	ctx context.Context,
	account *Account,
	statusCode int,
	responseBody []byte,
	upstreamMsg string,
) bool {
	kind, limitMessage := parseClineError(responseBody)
	if kind == clineErrorNone && statusCode == http.StatusPaymentRequired {
		// 402 只会来自积分不足（ClinePass 超限走 429 等其他状态码）。
		kind = clineCreditsExhausted
	}
	if kind == clineErrorNone {
		return false
	}
	now := time.Now()
	modelKey := modelRateLimitKeyForUpstreamModelNotFound(ctx, account, tempUnschedulableModel(ctx, nil))

	scope := modelKey
	if kind != clineFreeModelLimit {
		if scope = clineWalletRateLimitKey(modelKey); scope == "" {
			scope = clineErrorWallet(kind)
		}
	}
	if scope == "" {
		return false
	}

	var cooldown time.Duration
	var reason string
	switch kind {
	case clineCreditsExhausted:
		cooldown, reason = s.cnBalanceCooldownDuration(), clineCreditsReason
	case clineSpendLimit:
		cooldown, reason = clineLimitRecheck, clineSpendLimitReason
	case clineInferenceCap:
		cooldown, reason = clineLimitRecheck, clineInferenceCapReason
		if scope == clinePassRateLimitKey {
			cooldown = clinePassLimitCooldown(account, "5h", now)
		}
	case clinePassLimit:
		cooldown, reason = clinePassLimitCooldown(account, clinePassWindow([]byte(limitMessage)), now), clinePassLimitReason
	case clinePassUnavailable:
		cooldown, reason = clineLimitRecheck, clinePassUnavailableReason
	case clineFreeModelLimit:
		cooldown, reason = clineFreeModelResetAfter([]byte(limitMessage)), clineFreeModelLimitReason
		if cooldown <= 0 {
			cooldown = clineLimitRecheck
		}
	}
	until := now.Add(cooldown)
	if msg := strings.TrimSpace(upstreamMsg); msg != "" {
		reason += ": " + msg
	}

	setAccountModelRateLimitSnapshot(account, scope, until, reason, now)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, scope, until, reason); err != nil {
		slog.Warn("cline_limit_set_failed", "account_id", account.ID, "scope", scope, "error", err)
		return false
	}
	slog.Info("cline_limit", "account_id", account.ID, "scope", scope, "model", modelKey,
		"status_code", statusCode, "until", until.UTC())
	return true
}
