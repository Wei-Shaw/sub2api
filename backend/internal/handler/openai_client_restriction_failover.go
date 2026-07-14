package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIClientRestrictionFailoverState struct {
	lastResult          *service.CodexClientRestrictionDetectionResult
	lastAccountID       int64
	lastAccountPlatform string
}

// excludeSelection releases any slot reserved by the scheduler and excludes
// an account whose client policy does not allow the current request. The caller
// can then run the normal selection loop again without consuming an upstream
// failover attempt.
func (s *openAIClientRestrictionFailoverState) excludeSelection(
	gatewayService *service.OpenAIGatewayService,
	c *gin.Context,
	body []byte,
	selection *service.AccountSelectionResult,
	excludedIDs map[int64]struct{},
) (service.CodexClientRestrictionDetectionResult, bool) {
	if gatewayService == nil || selection == nil || selection.Account == nil {
		return service.CodexClientRestrictionDetectionResult{}, false
	}

	result := gatewayService.DetectOpenAIClientRestriction(c, selection.Account, body)
	if !result.Enabled || result.Matched {
		return result, false
	}

	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
		selection.ReleaseFunc = nil
	}
	selection.Acquired = false
	if excludedIDs != nil {
		excludedIDs[selection.Account.ID] = struct{}{}
	}
	resultCopy := result
	s.lastResult = &resultCopy
	s.lastAccountID = selection.Account.ID
	s.lastAccountPlatform = selection.Account.Platform
	return result, true
}

func (s *openAIClientRestrictionFailoverState) rejectIfExhausted(
	h *OpenAIGatewayHandler,
	c *gin.Context,
	streamStarted bool,
) bool {
	if s == nil || s.lastResult == nil || h == nil {
		return false
	}
	setOpsSelectedAccount(c, s.lastAccountID, s.lastAccountPlatform)
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	h.handleStreamingAwareError(
		c,
		http.StatusForbidden,
		"forbidden_error",
		service.CodexClientRestrictionMessage(*s.lastResult),
		streamStarted,
	)
	return true
}
