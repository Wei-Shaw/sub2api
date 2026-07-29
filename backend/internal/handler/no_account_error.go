package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// rateLimitedFromNoAccounts 判断「无可用账号」的错误是否实为「分组内账号全部处于上游限流冷却中」。
// 是则返回 (retryAfterSeconds, 面向客户端的限流文案, true)，调用方据此返回 429 rate_limit_error + Retry-After，
// 而非误导性的 503 No available accounts。否则返回 (0, "", false)，调用方回退原 503 行为。
//
// 之所以放在 handler 层统一判定：service 层把账号全限流包装成 *service.AllAccountsRateLimitedError，
// 其 Unwrap 返回 service.ErrNoAvailableAccounts，所以现有 errors.Is 分支与 ops 容量分类完全不受影响。
//
// 文案口径：ResetAt 已是候选账号中最早恢复者（service.computeAllAccountsRateLimited 取 min），
// 所以面向客户端直接报「最早恢复的那个账号还要多久可用」，时长用 human-readable（~2h35m）而非裸秒数（~9339s），
// 人/客户端不用心算；Retry-After header 仍返回秒（retryAfter，HTTP 规范要求）。
func rateLimitedFromNoAccounts(err error) (int, string, bool) {
	var rlErr *service.AllAccountsRateLimitedError
	if !errors.As(err, &rlErr) {
		return 0, "", false
	}
	retryAfter := int(time.Until(rlErr.ResetAt).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	msg := fmt.Sprintf("All upstream accounts are rate limited; the soonest one becomes available in ~%s (at %s).",
		humanizeDuration(retryAfter), rlErr.ResetAt.Format("15:04:05 MST"))
	return retryAfter, msg, true
}

// humanizeDuration 把秒数格式化成紧凑可读时长：最高非零量级 + 紧邻下一量级（仅当其非零）。
// 90061→"1d1h"、9339→"2h35m"、150→"2m30s"、3600→"1h"、60→"1m"、8→"8s"。
// 末尾零量级直接丢（"1h" 而非 "1h0m"），量级足够表达「还要多久」即可，避免 "~9339s" 这种要心算的裸秒数。
func humanizeDuration(totalSeconds int) string {
	if totalSeconds < 1 {
		totalSeconds = 1
	}
	d := time.Duration(totalSeconds) * time.Second
	units := []struct {
		v int
		s string
	}{
		{int(d / (24 * time.Hour)), "d"},
		{int(d % (24 * time.Hour) / time.Hour), "h"},
		{int(d % time.Hour / time.Minute), "m"},
		{int(d % time.Minute / time.Second), "s"},
	}

	for i, u := range units {
		if u.v == 0 {
			continue // 跳到首个非零量级
		}
		out := fmt.Sprintf("%d%s", u.v, u.s)
		if i+1 < len(units) && units[i+1].v != 0 {
			out += fmt.Sprintf("%d%s", units[i+1].v, units[i+1].s)
		}
		return out
	}
	return "0s" // 不可达（已 clamp >=1s），兜底
}

// noAccountErrorClassification describes the HTTP response to emit when
// account selection failed with ErrNoAvailableAccounts. Handlers obtain it
// via classifyNoAccountError and choose between:
//
//   - 404 model_not_found — the group has accounts, but none of them are
//     configured to serve the requested model (config / typo / unsupported
//     model). Returning 503 here misleads operators and trips reverse-proxy
//     health checks; 404 lets the client surface the real problem.
//
//   - 503 api_error — accounts that could serve the model exist but are
//     temporarily exhausted (rate limit, quota auto-pause, runtime block) OR
//     the group has no accounts at all. Both stay on 503 because retrying
//     after a backoff can plausibly succeed (or, in the empty-pool case, the
//     operator may be in the middle of adding accounts).
type noAccountErrorClassification struct {
	Status        int
	ErrType       string
	Message       string
	ModelNotFound bool // true when this is a 404 model_not_found classification
}

// classifyNoAccountError decides between 404 model_not_found and 503
// api_error for "no available accounts" failures.
//
// The classifier intentionally does not consume the original error: the
// selection layer never tells us *why* the pool came up empty (rate-limited
// vs. unsupported model are both wrapped as ErrNoAvailableAccounts). Instead
// we re-check pool composition through DiagnoseModelAvailabilityForPlatform.
// Its dedicated database query considers only persistent eligibility
// (active status + schedulable setting) and model_mapping, bypassing scheduler
// snapshots and transient filters. That guarantees a 404 is only returned
// when persistent account/group/model configuration must change before the
// request can succeed.
//
// routingModel is the model name that account selection actually compared
// against (i.e. after group-level dispatch mapping). displayModel is the
// raw model the caller asked for; it is used only in the user-facing error
// message so that internal mapping details don't leak. Most callers pass
// the same value for both.
//
// platform is the platform the request was routed to (use
// service.PlatformOpenAI / PlatformAnthropic / PlatformGemini). It is
// required because Anthropic/Gemini routes additionally surface
// mixed-scheduled Antigravity accounts; passing the wrong platform would
// flip a legitimate 503 to a misleading 404 (or vice versa).
func classifyNoAccountError(
	ctx context.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
	platform string,
) noAccountErrorClassification {
	fallback := noAccountErrorClassification{
		Status:  http.StatusServiceUnavailable,
		ErrType: "api_error",
		Message: "Service temporarily unavailable",
	}

	routingModel = strings.TrimSpace(routingModel)
	displayModel = strings.TrimSpace(displayModel)
	if displayModel == "" {
		displayModel = routingModel
	}
	if diag == nil || apiKey == nil || apiKey.GroupID == nil || routingModel == "" {
		return fallback
	}

	result := diag.DiagnoseModelAvailabilityForPlatform(ctx, apiKey.GroupID, routingModel, platform)
	if result.HasAccountsInPool && !result.HasModelSupport {
		return noAccountErrorClassification{
			Status:        http.StatusNotFound,
			ErrType:       "model_not_found",
			Message:       fmt.Sprintf("Model %q is not supported by any configured account in this group", displayModel),
			ModelNotFound: true,
		}
	}
	return fallback
}

// classifyNoAccountErrorFromGin is a thin wrapper that forwards the gin
// context's underlying request context. Most call sites already have a
// *gin.Context handy, so this keeps the call sites uncluttered.
func classifyNoAccountErrorFromGin(
	c *gin.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
	platform string,
) noAccountErrorClassification {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return classifyNoAccountError(ctx, diag, apiKey, routingModel, displayModel, platform)
}

func classifyOpenAICompatibleNoAccountErrorFromGin(
	c *gin.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
) noAccountErrorClassification {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return classifyNoAccountErrorFromGin(
		c,
		diag,
		apiKey,
		routingModel,
		displayModel,
		openAICompatibleRequestPlatform(ctx, apiKey),
	)
}

func openAICompatibleSelectionErrorForLog(err error, platform string) error {
	if err == nil || platform != service.PlatformGrok {
		return err
	}
	message := strings.ReplaceAll(err.Error(), "OpenAI accounts", "Grok accounts")
	if message == err.Error() {
		return err
	}
	return fmt.Errorf("%s", message)
}
