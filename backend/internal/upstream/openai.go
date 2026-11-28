package upstream

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAI constants, mirroring the values currently inlined in
// openai_gateway_service.go:
//
//	codexCLIVersion = "0.125.0"   (openai_gateway_service.go:65)
//	"version" header             (openai_gateway_service.go:3736)
//	"originator"/"session_id"    (openai_gateway_service.go:80-96)
//	"openai-beta"                (openai_gateway_service.go:93)
const (
	openAIDefaultBaseURL = "https://api.openai.com"
	// openAICodexCLIVersion is the Codex CLI version advertised in the version
	// header. Bumping it is a one-line change here instead of a hunt through the
	// gateway when the real CLI ships a new build.
	openAICodexCLIVersion = "0.125.0"
	// openAICodexOriginator is the originator the official Codex CLI sends.
	openAICodexOriginator = "codex_cli_rs"
)

// OpenAIAdapter speaks the OpenAI API and the ChatGPT/Codex backend.
//
// Replaces per call site: bearer/API-key branching, the "version" + "originator"
// fingerprint headers, chatgpt-account-id handling, and 21 x 401/403/429 literal
// comparisons currently spread over openai_gateway_service.go (7062 lines).
type OpenAIAdapter struct{}

// NewOpenAIAdapter returns the OpenAI adapter.
func NewOpenAIAdapter() *OpenAIAdapter { return &OpenAIAdapter{} }

// Platform implements Adapter.
func (a *OpenAIAdapter) Platform() Platform { return PlatformOpenAI }

// DefaultBaseURL implements Adapter.
func (a *OpenAIAdapter) DefaultBaseURL() string { return openAIDefaultBaseURL }

// AuthHeaders implements Adapter.
func (a *OpenAIAdapter) AuthHeaders(creds Credentials) (http.Header, error) {
	token := strings.TrimSpace(creds.APIKey)
	if creds.Kind == AuthOAuth {
		token = strings.TrimSpace(creds.AccessToken)
	}
	if token == "" {
		return nil, fmt.Errorf("upstream: openai account (%s) has empty credential", creds.Kind)
	}

	h := make(http.Header, 5)
	h.Set("authorization", "Bearer "+token)
	h.Set("accept", "application/json")

	// ChatGPT-backend routing: the account id selects which subscription the
	// request is billed against.
	if id := strings.TrimSpace(creds.AccountID); id != "" && creds.Kind == AuthOAuth {
		h.Set("chatgpt-account-id", id)
	}

	// Client fingerprint. Only the Codex CLI path sends version/originator; a
	// plain API call must not pretend to be the CLI.
	if strings.EqualFold(strings.TrimSpace(creds.ImpersonateClient), "codex") {
		h.Set("version", openAICodexCLIVersion)
		h.Set("originator", openAICodexOriginator)
	}

	if beta := mergeBeta(creds.ExtraBeta); beta != "" {
		h.Set("openai-beta", beta)
	}
	return h, nil
}

// BaseURL implements Adapter.
func (a *OpenAIAdapter) BaseURL(creds Credentials) string {
	base := strings.TrimSpace(creds.BaseURL)
	if base == "" {
		return openAIDefaultBaseURL
	}
	return strings.TrimRight(base, "/")
}

// Classify implements Adapter.
func (a *OpenAIAdapter) Classify(now time.Time, status int, header http.Header, body []byte) Failure {
	f := classifyStatus(now, status, header, body)
	switch {
	case f.Kind == FailureRateLimit:
		// OpenAI overloads the 429 status: a true rate limit is transient, while
		// "insufficient_quota" means this account's plan is exhausted and must be
		// pulled from the pool. Today that distinction is a body sniff at each
		// call site.
		if bodyContains(body, "insufficient_quota") {
			return Failure{Kind: FailureAuth, Reason: "quota exhausted"}
		}
		f.RetryAt = retryAtOr(now, f.RetryAt, defaultRateLimitCooldown)
	case status == http.StatusForbidden && bodyContains(body, "unsupported_country"):
		return Failure{Kind: FailureAuth, Reason: "region blocked"}
	case status == http.StatusServiceUnavailable:
		f = Failure{Kind: FailureOverload, RetryAt: retryAtOr(now, f.RetryAt, 10*time.Second), Reason: "unavailable"}
	}
	return f
}

// StickyKey implements Adapter. Codex conversation continuity is keyed on the
// client-supplied session_id (openai_gateway_service.go:1251-1334).
func (a *OpenAIAdapter) StickyKey(meta RequestMeta) string {
	return stickyKey(PlatformOpenAI, meta.SessionKey)
}
