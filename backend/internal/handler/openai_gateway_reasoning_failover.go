package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// openAIPassthroughFailoverState tracks whether this forwarding loop has attempted
// an OpenAI passthrough account. Once it has, every subsequent non-passthrough
// attempt must use a sanitized body because the immutable canonical body can carry
// provider-specific encrypted reasoning produced by that passthrough upstream.
type openAIPassthroughFailoverState struct {
	passthroughSeen bool
	// firstAttemptAccountID 是本次请求循环内第一个尝试的账号，用于切号净化
	// 的请求内检测（后续 attempt 账号不同即跨账号切换）。
	firstAttemptAccountID int64
}

// deriveOpenAIForwardAttemptBody returns the request body for the upcoming forward
// attempt against account. It always derives from the immutable canonical body and
// only removes encrypted reasoning input item(s) when the loop has already attempted
// a passthrough account and the upcoming account is non-passthrough. This remains
// sticky across retries and additional non-passthrough accounts. Attempts before
// any passthrough account, and all passthrough attempts, forward the canonical body
// unchanged. The canonical slice is never mutated.
//
// 切号净化：每次 attempt 先经 TrackOpenAICodexSessionAttemptForTaint 记录会话→
// 账号溯源并在跨账号续写时设置净化标记（service 的 transform/header 层读取，
// 对 id 做确定性改写）；净化发生在 service 层，这里只维护请求内状态与日志。
//
// This method is invoked exactly once per forward attempt, immediately before the
// Forward call, and advances the failover state as a side effect.
func (h *OpenAIGatewayHandler) deriveOpenAIForwardAttemptBody(
	c *gin.Context,
	reqLog *zap.Logger,
	canonicalBody []byte,
	account *service.Account,
	state *openAIPassthroughFailoverState,
) []byte {
	if account != nil && state.firstAttemptAccountID == 0 {
		state.firstAttemptAccountID = account.ID
	}
	if h.gatewayService.TrackOpenAICodexSessionAttemptForTaint(c, account, state.firstAttemptAccountID) {
		if reqLog != nil {
			reqLog.Info("openai.codex_session_taint_rekeyed",
				zap.Int64("account_id", account.ID),
				zap.Int64("first_attempt_account_id", state.firstAttemptAccountID),
			)
		}
	}

	currentPassthrough := account.IsOpenAIPassthroughEnabled()
	if currentPassthrough {
		state.passthroughSeen = true
		return canonicalBody
	}
	if !state.passthroughSeen {
		return canonicalBody
	}

	sanitized, changed, err := service.SanitizeOpenAICrossModeFailoverReasoning(canonicalBody)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("openai.failover_cross_mode_reasoning_sanitize_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
		}
		return canonicalBody
	}
	if !changed {
		return canonicalBody
	}
	if reqLog != nil {
		reqLog.Info("openai.failover_cross_mode_reasoning_stripped",
			zap.Int64("account_id", account.ID),
			zap.Bool("account_passthrough", currentPassthrough),
			zap.Bool("passthrough_seen", true),
		)
	}
	return sanitized
}
