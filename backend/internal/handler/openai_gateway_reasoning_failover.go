package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// openAIPassthroughFailoverState tracks whether this forwarding loop has attempted
// an OpenAI passthrough account. Once it has, every subsequent non-passthrough
// attempt must use a sanitized body because the immutable canonical body can carry
// provider-specific encrypted reasoning produced by that passthrough upstream.
type openAIPassthroughFailoverState struct {
	passthroughSeen bool
}

// deriveOpenAIForwardAttemptBody returns the request body for the upcoming forward
// attempt against account. It always derives from the immutable canonical body and
// only removes encrypted reasoning input item(s) when the loop has already attempted
// a passthrough account and the upcoming account is non-passthrough. This remains
// sticky across retries and additional non-passthrough accounts. Attempts before
// any passthrough account, and all passthrough attempts, forward the canonical body
// unchanged. The canonical slice is never mutated.
//
// This method is invoked exactly once per forward attempt, immediately before the
// Forward call, and advances the failover state as a side effect.
func (h *OpenAIGatewayHandler) deriveOpenAIForwardAttemptBody(
	reqLog *zap.Logger,
	canonicalBody []byte,
	account *service.Account,
	state *openAIPassthroughFailoverState,
) []byte {
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
