package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// openAIPassthroughFailoverState tracks account and passthrough transitions for
// one Responses forwarding loop. Replayable reasoning may be stripped after a
// provider transition; opaque native compaction must instead stop the attempt.
type openAIPassthroughFailoverState struct {
	passthroughSeen                     bool
	lastAccountID                       int64
	accountSwitched                     bool
	nativeCompactionCrossAccountBlocked bool
}

// deriveOpenAIForwardAttemptBody returns the request body for the upcoming forward
// attempt against account. It always derives from the immutable canonical body.
// Replayable reasoning is removed when the loop crosses provider modes. Native
// compaction is never deleted: a cross-account retry is blocked because the
// gateway has no owner evidence for that ciphertext. The canonical slice is never
// mutated.
//
// This method is invoked exactly once per forward attempt, immediately before the
// Forward call, and advances the failover state as a side effect.
func (h *OpenAIGatewayHandler) deriveOpenAIForwardAttemptBody(
	reqLog *zap.Logger,
	canonicalBody []byte,
	account *service.Account,
	state *openAIPassthroughFailoverState,
) []byte {
	if account == nil || state == nil {
		return canonicalBody
	}
	if state.lastAccountID != 0 && state.lastAccountID != account.ID {
		state.accountSwitched = true
	}
	state.lastAccountID = account.ID
	if state.accountSwitched && service.HasOpenAIEncryptedCompactionInput(canonicalBody) {
		// Native compaction is opaque state owned by the account that minted it.
		// Dropping it would silently shorten the conversation, so stop instead of
		// replaying an incomplete history on another account.
		state.nativeCompactionCrossAccountBlocked = true
		return canonicalBody
	}
	if state.accountSwitched && openAIRequestCarriesAccountBoundContinuation(canonicalBody) {
		sanitized, changed, err := service.SanitizeOpenAICrossModeFailoverReasoning(canonicalBody)
		if err != nil {
			if reqLog != nil {
				reqLog.Warn("openai.failover_account_bound_encrypted_content_sanitize_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		} else if changed {
			if reqLog != nil {
				reqLog.Info("openai.failover_account_bound_encrypted_content_stripped",
					zap.Int64("account_id", account.ID),
				)
			}
			return sanitized
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

func openAIRequestCarriesAccountBoundContinuation(body []byte) bool {
	return gjson.GetBytes(body, "previous_response_id").Exists() || service.HasCompactionTriggerInInput(body)
}
