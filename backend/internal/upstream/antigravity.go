package upstream

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Antigravity endpoint constants, mirroring internal/pkg/antigravity/oauth.go:56-57
// and ForwardBaseURLs() (daily endpoint first, production as the fallback).
const (
	antigravityProdBaseURL  = "https://cloudcode-pa.googleapis.com"
	antigravityDailyBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	// antigravityPathSuffix is appended to a custom base_url for API-key accounts
	// (service/account.go:871-873).
	antigravityPathSuffix = "/antigravity"
)

// AntigravityAdapter speaks the Google Cloud Code / Gemini backend used by the
// Antigravity client.
//
// Replaces per call site: the dual-endpoint fallback order, the /antigravity
// suffix rewrite, and 47 x 401/403/429 literal comparisons spread over
// antigravity_gateway_service.go (3945 lines, 36 base-URL references — the
// highest density of the three).
type AntigravityAdapter struct{}

// NewAntigravityAdapter returns the Antigravity adapter.
func NewAntigravityAdapter() *AntigravityAdapter { return &AntigravityAdapter{} }

// Platform implements Adapter.
func (a *AntigravityAdapter) Platform() Platform { return PlatformAntigravity }

// DefaultBaseURL implements Adapter. It returns the primary (daily) forward URL,
// matching resolveAntigravityForwardBaseURL()'s default.
func (a *AntigravityAdapter) DefaultBaseURL() string { return antigravityDailyBaseURL }

// BaseURLFallbacks implements BaseURLFallbacks: the ordered endpoint list the
// gateway walks on retryable failures.
func (a *AntigravityAdapter) BaseURLFallbacks(creds Credentials) []string {
	if base := strings.TrimSpace(creds.BaseURL); base != "" {
		base = strings.TrimRight(base, "/")
		if creds.Kind == AuthAPIKey {
			base += antigravityPathSuffix
		}
		return []string{base}
	}
	return []string{antigravityDailyBaseURL, antigravityProdBaseURL}
}

// AuthHeaders implements Adapter.
func (a *AntigravityAdapter) AuthHeaders(creds Credentials) (http.Header, error) {
	token := strings.TrimSpace(creds.AccessToken)
	if creds.Kind == AuthAPIKey {
		token = strings.TrimSpace(creds.APIKey)
	}
	if token == "" {
		return nil, fmt.Errorf("upstream: antigravity account (%s) has empty credential", creds.Kind)
	}

	h := make(http.Header, 3)
	h.Set("authorization", "Bearer "+token)
	h.Set("accept", "application/json")
	if beta := mergeBeta(creds.ExtraBeta); beta != "" {
		h.Set("x-goog-api-client", beta)
	}
	return h, nil
}

// BaseURL implements Adapter.
func (a *AntigravityAdapter) BaseURL(creds Credentials) string {
	if fallbacks := a.BaseURLFallbacks(creds); len(fallbacks) > 0 {
		return fallbacks[0]
	}
	return antigravityDailyBaseURL
}

// Classify implements Adapter.
func (a *AntigravityAdapter) Classify(now time.Time, status int, header http.Header, body []byte) Failure {
	f := classifyStatus(now, status, header, body)
	switch {
	case status == http.StatusTooManyRequests:
		// Google distinguishes "this model has no capacity right now" (transient,
		// retry elsewhere) from "this account is out of quota" (account-level).
		if bodyContains(body, "MODEL_CAPACITY_EXHAUSTED") {
			return Failure{Kind: FailureOverload, RetryAt: now.Add(5 * time.Second), Reason: "model capacity"}
		}
		f.RetryAt = retryAtOr(now, f.RetryAt, defaultRateLimitCooldown)
	case status == http.StatusForbidden && bodyContains(body, "RESOURCE_EXHAUSTED"):
		return Failure{Kind: FailureAuth, Reason: "resource exhausted"}
	}
	return f
}

// StickyKey implements Adapter.
func (a *AntigravityAdapter) StickyKey(meta RequestMeta) string {
	return stickyKey(PlatformAntigravity, meta.SessionKey)
}
