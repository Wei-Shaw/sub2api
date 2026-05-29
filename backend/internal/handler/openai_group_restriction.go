package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const openAIGroupCodexOfficialOnlyMessage = "This group only allows Codex official clients"

func (h *OpenAIGatewayHandler) rejectOpenAINonCodexOfficialClient(c *gin.Context, apiKey *service.APIKey, anthropicFormat bool) bool {
	if apiKey == nil {
		return false
	}
	var gatewayService *service.OpenAIGatewayService
	if h != nil {
		gatewayService = h.gatewayService
	}
	result := gatewayService.DetectGroupCodexOfficialRestriction(c, apiKey.Group)
	if !result.Enabled || result.Matched {
		return false
	}

	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	if anthropicFormat {
		h.anthropicErrorResponse(c, http.StatusForbidden, "permission_error", openAIGroupCodexOfficialOnlyMessage)
		return true
	}
	h.errorResponse(c, http.StatusForbidden, "forbidden_error", openAIGroupCodexOfficialOnlyMessage)
	return true
}
