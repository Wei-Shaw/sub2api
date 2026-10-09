package service

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
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
// responses, a real flex downgrade, missing declarations, or free-Fast pricing.
func codexClientResponseServiceTier(account *Account, requested, observed string) string {
	if account == nil || !account.IsOpenAIOAuthLike() || !codexOAuthResponseTierIsNonAuthoritative(observed) {
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

// Call after observing the raw upstream payload. Patch only the JSON string
// token; re-encoding the entire response would alter unrelated large numbers,
// tool arguments and passthrough fields.
func normalizeOpenAIResponseServiceTier(account *Account, requested string, payload []byte) []byte {
	if account == nil || !account.IsOpenAIOAuthLike() || requested == "" {
		return payload
	}
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType != "" && !isUpstreamResponseModelTerminalEvent(eventType) {
		return payload
	}
	tier := gjson.GetBytes(payload, "response.service_tier")
	if !tier.Exists() {
		tier = gjson.GetBytes(payload, "service_tier")
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
