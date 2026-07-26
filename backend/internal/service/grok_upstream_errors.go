package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// isGrokContentPolicyRejection identifies request-scoped safety refusals from
// xAI. These failures are caused by the prompt or media, so retrying another
// OAuth account cannot change the outcome and would incorrectly drain a pool.
// Keep this matcher deliberately narrow: account entitlement and suspension
// messages may mention policy but must retain the normal account failover path.
func isGrokContentPolicyRejection(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusForbidden || len(responseBody) == 0 {
		return false
	}
	if grokAccountAccessMessage(string(responseBody)) {
		return false
	}

	var payload any
	if json.Unmarshal(responseBody, &payload) == nil {
		if grokStructuredAccountAccessMarker(payload) {
			return false
		}
		if grokStructuredContentPolicyMarker(payload) {
			return true
		}
	}

	return grokContentPolicyMessage(string(responseBody))
}

func grokStructuredAccountAccessMarker(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalizedKey := normalizeGrokErrorMarker(key)
			switch normalizedKey {
			case "code", "error_code", "type", "category", "reason":
				if marker, ok := child.(string); ok && isGrokAccountAccessCode(marker) {
					return true
				}
			}
			if grokStructuredAccountAccessMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if grokStructuredAccountAccessMarker(child) {
				return true
			}
		}
	}
	return false
}

func grokStructuredContentPolicyMarker(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalizedKey := normalizeGrokErrorMarker(key)
			switch normalizedKey {
			case "code", "error_code", "type", "category", "reason":
				if marker, ok := child.(string); ok && isGrokContentPolicyCode(marker) {
					return true
				}
			}
			if grokStructuredContentPolicyMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if grokStructuredContentPolicyMarker(child) {
				return true
			}
		}
	}
	return false
}

func isGrokContentPolicyCode(value string) bool {
	switch normalizeGrokErrorMarker(value) {
	case "content_filter",
		"content_policy",
		"content_policy_violation",
		"content_moderation",
		"cyber_policy",
		"new_sensitive":
		return true
	default:
		return false
	}
}

func isGrokAccountAccessCode(value string) bool {
	switch normalizeGrokErrorMarker(value) {
	case "account_suspended",
		"account_disabled",
		"user_suspended",
		"user_disabled",
		"subscription_required",
		"entitlement_required",
		"not_entitled",
		"plan_required",
		"permission_denied":
		return true
	default:
		return false
	}
}

func grokAccountAccessMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{
		"account suspended",
		"account has been suspended",
		"account disabled",
		"account has been disabled",
		"user suspended",
		"user has been suspended",
		"subscription required",
		"entitlement required",
		"not entitled",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func grokContentPolicyMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}

	// xAI's media safety responses use these exact phrases. They are specific
	// enough not to classify a generic account-policy or entitlement message.
	for _, phrase := range []string{
		"the moderation feature is not available",
		"image is sensitive",
		"text is sensitive",
		"prohibited content",
		"forbidden content",
		"content policy violation",
		"content policy rejection",
		"content policy rejected",
		"content moderation rejection",
		"content moderation rejected",
		"content moderation blocked",
		"request blocked by content moderation",
		"request rejected by content moderation",
		"request blocked by policy",
		"request rejected by policy",
		"request violates policy",
		"prompt violates content policy",
		"prompt violates policy",
		"input violates content policy",
		"input violates policy",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	return false
}

func grokContentPolicyClientMessage(responseBody []byte) string {
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	if message == "" {
		return "Request blocked by upstream content policy"
	}
	return message
}

// shouldFailoverGrokUpstreamError is body-aware: Grok content refusals must stay
// on the current account and be returned to the caller instead of consuming the
// account pool. A 404 from Grok is treated as account-level (unlike other
// OpenAI-compatible upstreams that may use 404 for request-specific mismatches).
// A 405 is likewise account-level: a custom base_url that does not implement
// /v1/responses fails identically on every retry, so the request must move to
// another account instead of hammering the same one (upstream #4668).
func (s *OpenAIGatewayService) shouldFailoverGrokUpstreamError(statusCode int, responseBody []byte) bool {
	if isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
	}
	switch statusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return true
	}
	return s.shouldFailoverUpstreamError(statusCode)
}

// applyGrokConfiguredUnschedulablePolicy applies an administrator's existing
// temporary unschedulable rules to an account-scoped Grok upstream status
// (non-content 403, or 405 from a custom upstream). It reports true only when
// a rule matched; unmatched responses retain the status-specific fallback.
func (s *OpenAIGatewayService) applyGrokConfiguredUnschedulablePolicy(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if account == nil || !account.IsTempUnschedulableEnabled() {
		return false
	}

	matches := matchTempUnschedulableRules(account, statusCode, responseBody)
	if len(matches) == 0 {
		return false
	}

	match := matches[0]
	// Reuse the central policy implementation when it has a repository. This
	// preserves the existing reason/cache format and avoids duplicating writes.
	if s != nil && s.rateLimitService != nil && s.rateLimitService.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		handled := s.rateLimitService.tryTempUnschedulable(
			stateCtx,
			account,
			statusCode,
			responseBody,
		)
		cancel()
		if handled {
			return true
		}
	}

	// A partially constructed service (for example a unit-test gateway) still
	// honors the configured duration instead of silently falling back to 30m.
	cooldown := time.Duration(match.rule.DurationMinutes) * time.Minute
	if cooldown > 0 {
		s.tempUnscheduleGrok(ctx, account, cooldown, fmt.Sprintf("grok configured status %d rule", statusCode))
	}
	return true
}
