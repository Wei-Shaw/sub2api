package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// preparePromptCompression is deliberately fail-open. It is called only
// after authentication, moderation, billing re-check and session hashing, so
// those decisions always observe the original request body.
func preparePromptCompression(
	c *gin.Context,
	svc *service.PromptCompressionService,
	body []byte,
	protocol, model, cohort string,
	groupID *int64,
	apiKeyID int64,
) []byte {
	if svc == nil || len(body) == 0 {
		return body
	}
	requested := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Sub2API-RTK")))
	if requested == "default" {
		requested = ""
	}
	result, decision := svc.Prepare(c.Request.Context(), body, service.RTKPolicyRequest{
		Protocol: protocol, Model: model, GroupID: derefGroupIDForRTK(groupID), RequestOverride: requested,
		CohortValue: cohort,
	})
	group := int64(0)
	if groupID != nil {
		group = *groupID
	}
	svc.RecordTelemetry(service.PromptCompressionTelemetry{
		RequestID: c.GetString("request_id"), GroupID: group, APIKeyID: apiKeyID,
		Protocol: protocol, Model: model, Mode: decision.Mode, Outcome: result.Outcome,
		SkipReason: result.SkipReason, BeforeBytes: result.BeforeBytes, AfterBytes: result.AfterBytes,
		BeforeTokens: result.BeforeTokens, AfterTokens: result.AfterTokens,
		ChangedTargets: result.ChangedTargets, ProfileRevision: decision.Policy.Revision,
		Duration: result.Duration,
	})
	if result.Outcome == "observed" || result.Applied {
		c.Header("X-Sub2API-RTK", result.Outcome+"; profile="+strconv.FormatUint(decision.Policy.Revision, 10)+"; saved_est="+strconv.Itoa(maxInt(result.BeforeTokens-result.AfterTokens, 0)))
	}
	if result.Applied && len(result.Body) > 0 {
		return result.Body
	}
	return body
}

func derefGroupIDForRTK(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
