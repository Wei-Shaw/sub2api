package service

import (
	"net/http"
	"strings"
)

const (
	// DefaultDimAgentTestModel is the low-cost model used by the administrator
	// connection test when no explicit model is supplied.
	DefaultDimAgentTestModel = "deepseek-v4.1-flash"

	// DimAgent gates its subscription relay on the official client identity.
	// A valid OAuth credential alone is not enough: requests without the
	// DeepSeek Harness product token are rejected with
	//   403 "该凭证仅限官方客户端使用 (unsupported client)".
	//
	// Verified against the live relay on 2026-10-10 with a real subscription
	// token (same model, same token, only the identity headers varied):
	//   "deepseek-harness/0.2.1-alpha.2 (+https://github.com/deepseek-ai/deepseek-harness)"
	//     + X-Title/HTTP-Referer  -> HTTP 200
	//   "deepseek-harness/9.9.9"  + X-Title/HTTP-Referer  -> HTTP 200 (any version suffix)
	//   no User-Agent override    + X-Title/HTTP-Referer  -> HTTP 403
	//   "DimAgent/0.5.8" + X-Title: DimCode               -> HTTP 403
	// These values mirror APP_IDENTITY/userAgent() in
	// @deepseek-ai/dsh-llm/lib/types/attribution.js and the headers installed by
	// the installed @arcships/dsh-dim-oauth adapter profile.
	DimAgentTitle       = "DeepSeek Harness"
	DimAgentReferer     = "https://dimagent.cn/"
	DimAgentUserAgent   = "deepseek-harness/0.2.1-alpha.2 (+https://github.com/deepseek-ai/deepseek-harness)"
	DimAgentProductName = "deepseek-harness/"
)

// DefaultDimAgentModelIDs is the initial catalog published by the current DSH
// DimAgent provider. Administrators can still sync /v1/models and use account
// model mappings to expose a smaller or aliased catalog to subscribers.
func DefaultDimAgentModelIDs() []string {
	return []string{
		"deepseek-v4.1-flash",
		"deepseek-v4-pro",
		"glm-5.2",
		"glm-5.3",
		"kimi-k3",
		"seed-2.1-pro",
	}
}

// IsDimAgent reports whether this is a managed DimAgent subscription account.
func (a *Account) IsDimAgent() bool {
	return a != nil && a.Platform == PlatformDimAgent
}

// DimAgentUserAgentForAccount returns the outbound User-Agent for this account.
// An administrator-provided credentials.user_agent may pin a newer official
// client version, but only when it still carries the official product token:
// the relay rejects any identity that does not identify as the official client.
func (a *Account) DimAgentUserAgentForAccount() string {
	if a == nil || !a.IsDimAgent() {
		return ""
	}
	custom := strings.TrimSpace(a.GetCredential("user_agent"))
	if strings.HasPrefix(strings.ToLower(custom), DimAgentProductName) {
		return custom
	}
	return DimAgentUserAgent
}

// ApplyDimAgentSubscriptionHeaders installs the exact service-owned profile the
// official DeepSeek Harness client sends, and is applied after client header
// forwarding and account overrides so callers cannot impersonate another
// client. The relay's gate is the `deepseek-harness/` product token; without it
// a valid OAuth credential is still answered with 403 unsupported client.
func (a *Account) ApplyDimAgentSubscriptionHeaders(headers http.Header) {
	if a == nil || !a.IsDimAgent() || headers == nil {
		return
	}
	headers.Set("User-Agent", a.DimAgentUserAgentForAccount())
	headers.Set("X-Title", DimAgentTitle)
	headers.Set("HTTP-Referer", DimAgentReferer)
}

// dimAgentAccessToken is retained as the single outbound credential accessor.
// It deliberately reads the OAuth-issued short-lived access token, never an
// operator-entered API key.
func (a *Account) dimAgentAccessToken() string {
	if a == nil || !a.IsDimAgent() || a.Type != AccountTypeOAuth {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("access_token"))
}
