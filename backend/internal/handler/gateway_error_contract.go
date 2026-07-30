package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const publicServiceUnavailableMessage = "Service temporarily unavailable, please retry later"

// matchGatewayErrorPassthrough keeps every public gateway protocol on the same
// configured upstream-error contract while preserving full diagnostics in ops logs.
func matchGatewayErrorPassthrough(
	c *gin.Context,
	svc *service.ErrorPassthroughService,
	platform string,
	statusCode int,
	responseBody []byte,
) (int, string, bool) {
	if svc == nil {
		return 0, "", false
	}

	rule := svc.MatchRule(platform, statusCode, responseBody)
	if rule == nil {
		return 0, "", false
	}

	responseCode := statusCode
	if !rule.PassthroughCode && rule.ResponseCode != nil {
		responseCode = *rule.ResponseCode
	}

	message := strings.TrimSpace(service.ExtractClientSafeUpstreamErrorMessage(responseBody))
	if !rule.PassthroughBody && rule.CustomMessage != nil {
		message = strings.TrimSpace(*rule.CustomMessage)
	}
	if message == "" {
		message = publicServiceUnavailableMessage
	}

	if rule.SkipMonitoring && c != nil {
		c.Set(service.OpsSkipPassthroughKey, true)
	}

	return responseCode, message, true
}
