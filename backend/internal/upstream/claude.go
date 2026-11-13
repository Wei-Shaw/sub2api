package upstream

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Anthropic constants, mirroring what gateway_service.go hardcodes today:
//
//	"anthropic-version" is set at gateway_service.go:10466
//	"anthropic-beta"    is set/merged at gateway_service.go:6430
//	"oauth-2025-04-20"  is the OAuth beta required for claude.ai accounts
const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicVersionHeader  = "anthropic-version"
	anthropicVersionValue   = "2023-06-01"
	anthropicBetaHeader     = "anthropic-beta"
	anthropicOAuthBeta      = "oauth-2025-04-20"
)

// ClaudeAdapter speaks the Anthropic Messages API for both Console API keys and
// claude.ai OAuth accounts.
//
// Replaces, per call site, the auth/base-url/beta/status-code branching that is
// currently spread over gateway_service.go (9471 lines, 12 x "anthropic-version",
// 51 x "anthropic-beta", 18 x 401/403/429 literals).
type ClaudeAdapter struct{}

// NewClaudeAdapter returns the Anthropic adapter.
func NewClaudeAdapter() *ClaudeAdapter { return &ClaudeAdapter{} }

// Platform implements Adapter.
func (a *ClaudeAdapter) Platform() Platform { return PlatformClaude }

// DefaultBaseURL implements Adapter.
func (a *ClaudeAdapter) DefaultBaseURL() string { return anthropicDefaultBaseURL }

// AuthHeaders implements Adapter.
func (a *ClaudeAdapter) AuthHeaders(creds Credentials) (http.Header, error) {
	h := make(http.Header, 4)
	h.Set(anthropicVersionHeader, anthropicVersionValue)
	h.Set("accept", "application/json")

	switch creds.Kind {
	case AuthAPIKey:
		if strings.TrimSpace(creds.APIKey) == "" {
			return nil, fmt.Errorf("upstream: claude api-key account has empty key")
		}
		// Anthropic-compatible relays are split on this: the native scheme is
		// x-api-key, but some deployments only accept Authorization: Bearer with
		// the same key (service/anthropic_apikey_auth.go). The account decides;
		// the adapter owns the wire format.
		if creds.APIKeyScheme == APIKeySchemeAuthorizationBearer {
			h.Set("authorization", "Bearer "+creds.APIKey)
		} else {
			h.Set("x-api-key", creds.APIKey)
		}
	case AuthOAuth:
		if strings.TrimSpace(creds.AccessToken) == "" {
			return nil, fmt.Errorf("upstream: claude oauth account has empty access token")
		}
		h.Set("authorization", "Bearer "+creds.AccessToken)
		// OAuth accounts only work when the OAuth beta is present; account-level
		// betas are merged in, never appended blindly (duplicates are rejected by
		// the upstream with a 400).
		if beta := mergeBeta(anthropicOAuthBeta, creds.ExtraBeta); beta != "" {
			h.Set(anthropicBetaHeader, beta)
		}
	default:
		return nil, fmt.Errorf("upstream: claude account has unknown auth kind %q", creds.Kind)
	}
	return h, nil
}

// BaseURL implements Adapter.
//
// Note the quirk being normalised here: service.Account.GetBaseURL() returns ""
// for every non-API-key account (account.go:863-866), so the empty string is a
// sentinel meaning "use the platform default" rather than an error. The adapter
// resolves the sentinel once instead of leaving it to 23 call sites.
func (a *ClaudeAdapter) BaseURL(creds Credentials) string {
	base := strings.TrimSpace(creds.BaseURL)
	if base == "" {
		return anthropicDefaultBaseURL
	}
	return strings.TrimRight(base, "/")
}

// Classify implements Adapter.
func (a *ClaudeAdapter) Classify(now time.Time, status int, header http.Header, body []byte) Failure {
	f := classifyStatus(now, status, header, body)
	switch {
	case f.Kind == FailureRateLimit:
		f.RetryAt = retryAtOr(now, f.RetryAt, defaultRateLimitCooldown)
	case status == 529 || bodyContains(body, "overloaded_error"):
		// Anthropic signals capacity pressure with 529 or an {"type":"overloaded_error"}
		// envelope; both are transient and must not disable the credential.
		f = Failure{Kind: FailureOverload, RetryAt: retryAtOr(now, f.RetryAt, 5*time.Second), Reason: "overloaded"}
	case status == http.StatusBadRequest && bodyContains(body, "credit balance is too low"):
		// Out of credit arrives as a 400 but is an account-level condition.
		f = Failure{Kind: FailureAuth, Reason: "credit exhausted"}
	}
	return f
}

// StickyKey implements Adapter. Claude conversations are pinned by the
// OAuth metadata user_id / metadata.session_id that the caller resolves.
func (a *ClaudeAdapter) StickyKey(meta RequestMeta) string {
	return stickyKey(PlatformClaude, meta.SessionKey)
}
