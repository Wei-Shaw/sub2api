package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxClientAdmissionVetoAttempts = service.MaxCodexClientAdmissionVetoAttempts

func recordOpenAIClientAdmissionVeto(failedAccountIDs map[int64]struct{}, accountID int64, vetoCount *int) bool {
	failedAccountIDs[accountID] = struct{}{}
	*vetoCount = *vetoCount + 1
	return *vetoCount < maxClientAdmissionVetoAttempts
}

func allExcludedOpenAIAccountsWereClientVetoed(failedAccountIDs map[int64]struct{}, vetoCount int) bool {
	return vetoCount > 0 && len(failedAccountIDs) == vetoCount
}

func (h *OpenAIGatewayHandler) handleOpenAIClientAdmissionExhausted(
	c *gin.Context,
	ctx context.Context,
	streamStarted bool,
	anthropicFormat bool,
) {
	err := service.CodexClientAdmissionErrorFromContext(ctx)
	if h.handleOpenAICodexAdmissionError(c, err, streamStarted, anthropicFormat) {
		return
	}
	if anthropicFormat {
		h.anthropicStreamingAwareError(c, http.StatusForbidden, "permission_error", "This account only allows Codex official clients", streamStarted)
		return
	}
	h.handleStreamingAwareError(c, http.StatusForbidden, "forbidden_error", "This account only allows Codex official clients", streamStarted)
}

func (h *OpenAIGatewayHandler) handleOpenAICodexAdmissionError(
	c *gin.Context,
	err error,
	streamStarted bool,
	anthropicFormat bool,
) bool {
	if !errors.Is(err, service.ErrCodexClientRestricted) {
		return false
	}
	result, ok := service.CodexClientRestrictionResultFromError(err)
	if !ok {
		result = service.CodexClientRestrictionDetectionResult{
			Enabled: true,
			Matched: false,
		}
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	message := service.CodexClientRestrictionMessage(result)
	if anthropicFormat {
		h.anthropicStreamingAwareError(c, http.StatusForbidden, "permission_error", message, streamStarted)
	} else {
		h.handleStreamingAwareError(c, http.StatusForbidden, "forbidden_error", message, streamStarted)
	}
	return true
}
