package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// anthropicThinkingMode is the wire-level thinking contract learned from an upstream
// validation response. Unknown is intentionally the zero value: requests are left
// untouched until the upstream gives an explicit, machine-readable compatibility signal.
type anthropicThinkingMode uint8

const (
	anthropicThinkingModeUnknown anthropicThinkingMode = iota
	anthropicThinkingModeAdaptive
	anthropicThinkingModeLegacy
)

// adaptAnthropicThinkingForUpstreamError performs a narrowly-scoped, bidirectional
// compatibility retry for Anthropic-compatible endpoints. It does not guess from a
// model prefix or rewrite arbitrary 400 responses: only an explicit thinking-mode
// rejection can trigger a rewrite.
func adaptAnthropicThinkingForUpstreamError(body, responseBody []byte) ([]byte, anthropicThinkingMode, bool) {
	if !gjson.ValidBytes(body) {
		return body, anthropicThinkingModeUnknown, false
	}

	thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()))
	if thinkingType == "" {
		return body, anthropicThinkingModeUnknown, false
	}

	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	if message == "" {
		message = strings.ToLower(strings.TrimSpace(string(responseBody)))
	}

	switch thinkingType {
	case "enabled", "disabled":
		if !anthropicThinkingTypeRejected(message, thinkingType) || !strings.Contains(message, "adaptive") {
			return body, anthropicThinkingModeUnknown, false
		}
		modified, applied := anthropicThinkingAdaptiveBody(body)
		if !applied {
			return body, anthropicThinkingModeUnknown, false
		}
		return modified, anthropicThinkingModeAdaptive, true

	case "adaptive":
		if !anthropicThinkingTypeRejected(message, thinkingType) ||
			(!strings.Contains(message, "enabled") && !strings.Contains(message, "budget_tokens") && !strings.Contains(message, "extended thinking")) {
			return body, anthropicThinkingModeUnknown, false
		}
		return anthropicThinkingLegacyBody(body)
	default:
		return body, anthropicThinkingModeUnknown, false
	}
}

func anthropicThinkingTypeRejected(message, thinkingType string) bool {
	if !strings.Contains(message, "thinking.type") && !strings.Contains(message, "thinking type") {
		return false
	}
	if strings.Contains(message, thinkingType+" thinking is not supported") ||
		strings.Contains(message, thinkingType+" mode is not supported") {
		return true
	}
	marker := "thinking.type." + thinkingType
	if markerIndex := strings.Index(message, marker); markerIndex >= 0 {
		// Only inspect the clause attached to this exact type. A response that
		// rejects enabled and recommends adaptive must not be interpreted as a
		// rejection of adaptive on the next retry.
		clause := message[markerIndex+len(marker):]
		if nextMarker := strings.Index(clause, "thinking.type."); nextMarker >= 0 {
			clause = clause[:nextMarker]
		}
		return strings.Contains(clause, "not supported") ||
			strings.Contains(clause, "unsupported") ||
			strings.Contains(clause, "not allowed") ||
			strings.Contains(clause, "does not support") ||
			strings.Contains(clause, "invalid")
	}
	// Some compatible providers return a schema error such as
	// "thinking.type must be adaptive" without naming the rejected literal.
	return strings.Contains(message, "thinking.type") &&
		(strings.Contains(message, "must be") || strings.Contains(message, "expected") || strings.Contains(message, "should be"))
}

func anthropicThinkingAdaptiveBody(body []byte) ([]byte, bool) {
	modified, err := sjson.SetBytes(body, "thinking.type", "adaptive")
	if err != nil {
		return body, false
	}
	// budget_tokens belongs to legacy extended thinking and is rejected by
	// adaptive-only models. The official replacement is output_config.effort;
	// do not invent an effort value from a token budget.
	if modified, err = sjson.DeleteBytes(modified, "thinking.budget_tokens"); err != nil {
		return body, false
	}
	return modified, true
}

func anthropicThinkingLegacyBody(body []byte) ([]byte, anthropicThinkingMode, bool) {
	maxTokens := gjson.GetBytes(body, "max_tokens").Int()
	if maxTokens <= 1024 {
		// Legacy thinking requires budget_tokens >= 1024 and (outside the
		// interleaved beta) budget_tokens < max_tokens. Do not silently raise
		// max_tokens and increase cost for a request that cannot be converted safely.
		return body, anthropicThinkingModeUnknown, false
	}

	budget := gjson.GetBytes(body, "thinking.budget_tokens").Int()
	if budget < 1024 || budget >= maxTokens {
		budget = maxTokens / 2
		if budget > BudgetRectifyBudgetTokens {
			budget = BudgetRectifyBudgetTokens
		}
		if budget < 1024 {
			budget = 1024
		}
	}

	modified, err := sjson.SetBytes(body, "thinking.type", "enabled")
	if err != nil {
		return body, anthropicThinkingModeUnknown, false
	}
	if modified, err = sjson.SetBytes(modified, "thinking.budget_tokens", budget); err != nil {
		return body, anthropicThinkingModeUnknown, false
	}
	// effort is the adaptive steering control and can itself be rejected by
	// legacy-only models. Keep other output_config fields, if any.
	if modified, err = sjson.DeleteBytes(modified, "output_config.effort"); err != nil {
		return body, anthropicThinkingModeUnknown, false
	}
	if len(gjson.GetBytes(modified, "output_config").Map()) == 0 {
		if modified, err = sjson.DeleteBytes(modified, "output_config"); err != nil {
			return body, anthropicThinkingModeUnknown, false
		}
	}
	return modified, anthropicThinkingModeLegacy, true
}

func anthropicThinkingModeCacheKey(account *Account, model string) string {
	accountID := int64(0)
	baseURL := claudeAPIURL
	if account != nil {
		accountID = account.ID
		if configured := strings.TrimSpace(account.GetBaseURL()); configured != "" {
			baseURL = configured
		}
	}
	return fmt.Sprintf("%d\x00%s\x00%s", accountID, strings.ToLower(baseURL), strings.ToLower(strings.TrimSpace(model)))
}

func (s *GatewayService) applyCachedAnthropicThinkingMode(account *Account, model string, body []byte) ([]byte, bool) {
	if s == nil {
		return body, false
	}
	cached, ok := s.anthropicThinkingModes.Load(anthropicThinkingModeCacheKey(account, model))
	if !ok {
		return body, false
	}
	mode, ok := cached.(anthropicThinkingMode)
	if !ok {
		return body, false
	}
	switch mode {
	case anthropicThinkingModeAdaptive:
		if strings.EqualFold(gjson.GetBytes(body, "thinking.type").String(), "enabled") ||
			strings.EqualFold(gjson.GetBytes(body, "thinking.type").String(), "disabled") {
			modified, applied := anthropicThinkingAdaptiveBody(body)
			return modified, applied
		}
	case anthropicThinkingModeLegacy:
		if strings.EqualFold(gjson.GetBytes(body, "thinking.type").String(), "adaptive") {
			modified, _, applied := anthropicThinkingLegacyBody(body)
			return modified, applied
		}
	}
	return body, false
}

func (s *GatewayService) rememberAnthropicThinkingMode(account *Account, model string, mode anthropicThinkingMode) {
	if s == nil || mode == anthropicThinkingModeUnknown {
		return
	}
	s.anthropicThinkingModes.Store(anthropicThinkingModeCacheKey(account, model), mode)
}
