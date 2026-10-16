package service

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIResponseServiceTierContextKey = "openai_response_service_tier"

// Capture only the final outbound tier, after group/fast policies and account
// transforms. Each forwarding attempt replaces it, including an omitted tier.
func setOpenAIResponseServiceTier(c *gin.Context, tier *string) {
	if c != nil {
		c.Set(openAIResponseServiceTierContextKey, optionalStringValue(tier))
	}
}

func openAIResponseServiceTier(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(openAIResponseServiceTierContextKey)
}

// Codex's default echo is not authoritative for OAuth billing. Reuse that
// existing decision for the client response, so an API-key relay does not
// reinterpret it as a public-API downgrade. This never changes public API
// responses, a real flex downgrade, missing outbound tiers, or free-Fast pricing.
func codexClientResponseServiceTier(account *Account, requested, observed string) string {
	if account == nil || !account.IsOpenAIOAuthLike() || (strings.TrimSpace(observed) != "" && !codexOAuthResponseTierIsNonAuthoritative(observed)) {
		return observed
	}
	resolution := ResolveOpenAIServiceTierBilling(account, normalizedOpenAIServiceTierValue(requested), observed)
	switch resolution.Billing {
	case OpenAIFastTierPriority, OpenAIFastTierUltrafast, OpenAIFastTierFlex:
		return resolution.Billing
	default:
		return observed
	}
}

// Call after observing the raw upstream payload. Patch only the tier token (or
// insert its missing declaration); re-encoding the entire response would alter
// unrelated large numbers, tool arguments and passthrough fields.
func normalizeOpenAIResponseServiceTier(account *Account, requested string, payload []byte) []byte {
	if account == nil || !account.IsOpenAIOAuthLike() || requested == "" {
		return payload
	}
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType != "" && eventType != "response.created" && eventType != "response.in_progress" && !isUpstreamResponseModelTerminalEvent(eventType) {
		return payload
	}
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return payload
	}
	tier := gjson.GetBytes(payload, "response.service_tier")
	if !tier.Exists() {
		tier = gjson.GetBytes(payload, "service_tier")
	}
	if !tier.Exists() {
		resolved := codexClientResponseServiceTier(account, requested, "")
		if resolved == "" {
			return payload
		}
		path := "service_tier"
		if gjson.GetBytes(payload, "response").IsObject() {
			path = "response.service_tier"
		} else if eventType != "" || (gjson.GetBytes(payload, "object").String() != "response" && !gjson.GetBytes(payload, "model").Exists()) {
			return payload
		}
		if patched, err := sjson.SetBytes(payload, path, resolved); err == nil {
			return patched
		}
		return payload
	}
	if tier.Type != gjson.String {
		return payload
	}
	resolved := codexClientResponseServiceTier(account, requested, tier.String())
	if resolved == tier.String() {
		return payload
	}
	patched := make([]byte, 0, len(payload)+len(resolved))
	patched = append(patched, payload[:tier.Index]...)
	patched = strconv.AppendQuote(patched, resolved)
	return append(patched, payload[tier.Index+len(tier.Raw):]...)
}
