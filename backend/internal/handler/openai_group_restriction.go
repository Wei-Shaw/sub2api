package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const openAIGroupCodexOfficialOnlyMessage = "This group only allows Codex official clients"

func (h *OpenAIGatewayHandler) rejectOpenAINonCodexOfficialClient(c *gin.Context, apiKey *service.APIKey, anthropicFormat bool) bool {
	_, rejected := h.applyOpenAIGroupCodexOfficialRestriction(c, apiKey, anthropicFormat)
	return rejected
}

func (h *OpenAIGatewayHandler) applyOpenAIGroupCodexOfficialRestriction(c *gin.Context, apiKey *service.APIKey, anthropicFormat bool) (*service.APIKey, bool) {
	if apiKey == nil {
		return apiKey, false
	}
	var gatewayService *service.OpenAIGatewayService
	if h != nil {
		gatewayService = h.gatewayService
	}
	result := gatewayService.DetectGroupCodexOfficialRestriction(c, apiKey.Group)
	if !result.Enabled || result.Matched {
		return apiKey, false
	}

	if fallbackGroup := h.resolveOpenAIGroupCodexOfficialFallback(c, apiKey.Group); fallbackGroup != nil {
		return cloneAPIKeyWithGroup(apiKey, fallbackGroup), false
	}

	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	if anthropicFormat {
		h.anthropicErrorResponse(c, http.StatusForbidden, "permission_error", openAIGroupCodexOfficialOnlyMessage)
		return apiKey, true
	}
	h.errorResponse(c, http.StatusForbidden, "forbidden_error", openAIGroupCodexOfficialOnlyMessage)
	return apiKey, true
}

func (h *OpenAIGatewayHandler) resolveOpenAIGroupCodexOfficialFallback(c *gin.Context, group *service.Group) *service.Group {
	if group == nil || group.FallbackGroupID == nil || *group.FallbackGroupID <= 0 {
		return nil
	}
	if h == nil || h.apiKeyService == nil {
		return nil
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	log := requestLogger(c, "handler.openai_gateway.group_restriction")

	visited := map[int64]struct{}{}
	if group.ID > 0 {
		visited[group.ID] = struct{}{}
	}
	nextID := *group.FallbackGroupID
	for {
		if _, seen := visited[nextID]; seen {
			log.Warn("openai.group_codex_fallback_cycle_detected", zap.Int64("fallback_group_id", nextID))
			return nil
		}
		visited[nextID] = struct{}{}

		fallbackGroup, err := h.apiKeyService.GetGroupByIDLite(ctx, nextID)
		if err != nil {
			log.Warn("openai.group_codex_fallback_resolve_failed", zap.Int64("fallback_group_id", nextID), zap.Error(err))
			return nil
		}
		if fallbackGroup == nil || fallbackGroup.Platform != service.PlatformOpenAI {
			log.Warn("openai.group_codex_fallback_invalid_platform", zap.Int64("fallback_group_id", nextID))
			return nil
		}
		if !fallbackGroup.CodexOfficialOnly {
			return fallbackGroup
		}
		if fallbackGroup.FallbackGroupID == nil || *fallbackGroup.FallbackGroupID <= 0 {
			log.Warn("openai.group_codex_fallback_target_restricted", zap.Int64("fallback_group_id", nextID))
			return nil
		}
		nextID = *fallbackGroup.FallbackGroupID
	}
}
