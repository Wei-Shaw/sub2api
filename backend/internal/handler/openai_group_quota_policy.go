package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

type openAIGroupQuotaPolicyResolver interface {
	Resolve(groupID int64, model string) service.GroupQuotaPolicyResolution
}

// SetGroupQuotaPolicyService attaches the cached policy resolver at startup.
func (h *OpenAIGatewayHandler) SetGroupQuotaPolicyService(s *service.GroupQuotaPolicyService) {
	if s == nil {
		h.groupQuotaPolicy = nil
		return
	}
	h.groupQuotaPolicy = s
}

// Resolve once before scheduling; retries use this same immutable decision.
// Compact and composite routes retain their existing specialized model chain.
func (h *OpenAIGatewayHandler) resolveOpenAIGroupQuotaPolicy(apiKey *service.APIKey, model string, compact bool) service.GroupQuotaPolicyResolution {
	unchanged := service.GroupQuotaPolicyResolution{SourceModel: model, TargetModel: model}
	if h == nil || h.groupQuotaPolicy == nil || compact || apiKey == nil || apiKey.GroupID == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformOpenAI {
		return unchanged
	}
	resolved := h.groupQuotaPolicy.Resolve(*apiKey.GroupID, model)
	if !resolved.Applied || strings.TrimSpace(resolved.TargetModel) == "" {
		return unchanged
	}
	return resolved
}

func logOpenAIGroupQuotaPolicy(logger *zap.Logger, policy service.GroupQuotaPolicyResolution) {
	if logger == nil || !policy.Applied {
		return
	}
	logger.Info("openai.group_quota_policy_applied",
		zap.String("rule_id", policy.RuleID), zap.String("rule_name", policy.RuleName),
		zap.Int64("policy_revision", policy.Revision),
		zap.String("source_model", policy.SourceModel), zap.String("target_model", policy.TargetModel))
}

func quotaPolicyInflightEstimate(model string, body []byte, policy service.GroupQuotaPolicyResolution) service.InflightEstimateRequest {
	req := tokenInflightEstimate(model, body)
	if policy.Applied {
		req.AdditionalUpstreamModel = policy.TargetModel
	}
	return req
}

// A WS account, response chain and balance reservation belong to its connection.
// Re-select them at a turn boundary when a policy changes the actual upstream
// target. Changes between aliases with the same supported target can continue.
func openAIQuotaPolicyTurnRequiresReconnect(account *service.Account, previous, next *openAIWSTurnChannelMappingSnapshot) bool {
	if previous == nil || next == nil || (!previous.quotaPolicy.Applied && !next.quotaPolicy.Applied) {
		return false
	}
	if account == nil || !account.IsModelSupported(next.forwardModel) {
		return true
	}
	previousTarget := service.ResolveOpenAIAccountUpstreamModelForRequest(account, previous.forwardModel, false)
	nextTarget := service.ResolveOpenAIAccountUpstreamModelForRequest(account, next.forwardModel, false)
	return previousTarget != nextTarget
}

// A proxy restart renumbers the retried client turn to one. Copy its complete
// frozen mapping so account selection, model forwarding and billing identity
// cannot fall back to the connection's initial turn or resample policy state.
func openAIWSTurnMappingForRetry(current, fallback *openAIWSTurnChannelMappingSnapshot) *openAIWSTurnChannelMappingSnapshot {
	if current == nil {
		current = fallback
	}
	if current == nil {
		return nil
	}
	retry := *current
	retry.turn = 1
	return &retry
}
