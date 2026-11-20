package upstream

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultRateLimitCooldown is used when an upstream returns 429 without any
// machine-readable reset hint. It is intentionally generous: waking an account
// too early costs a failed request and an extra scheduler round-trip, while
// waiting too long only costs throughput on a pool that has spare capacity.
const defaultRateLimitCooldown = 60 * time.Second

// ClassifyStatus is the exported, platform-agnostic mapping for callers that only
// have a bare status code and body — e.g. the ops trace writer, which normalises
// historical upstream errors into the same FailureKind vocabulary the live
// scheduler uses. Platform adapters are preferred when the platform is known,
// because they carry the per-provider nuances (429/insufficient_quota,
// Anthropic 400/credit, Google capacity exhaustion).
func ClassifyStatus(status int, header http.Header, body []byte) Failure {
	return classifyStatus(time.Now(), status, header, body)
}

// classifyStatus implements the cross-platform default mapping. Adapters embed
// it and only override the codes where their provider diverges.
func classifyStatus(now time.Time, status int, header http.Header, body []byte) Failure {
	switch {
	case status >= 200 && status < 300:
		return Failure{Kind: FailureNone}

	case status == http.StatusUnauthorized:
		return Failure{Kind: FailureAuth, Reason: "unauthorized"}

	case status == http.StatusForbidden:
		// 403 is account-level on every provider we support: the key is valid but
		// the account is blocked, region-restricted or out of plan. Treat it as a
		// credential failure so the pool stops routing to it.
		return Failure{Kind: FailureAuth, Reason: "forbidden"}

	case status == http.StatusTooManyRequests:
		return Failure{
			Kind:    FailureRateLimit,
			RetryAt: parseRetryAfter(now, header),
			Reason:  "rate limited",
		}

	case status == http.StatusRequestTimeout,
		status == http.StatusInternalServerError,
		status == http.StatusBadGateway,
		status == http.StatusServiceUnavailable,
		status == http.StatusGatewayTimeout,
		status == 529: // Anthropic "overloaded"
		return Failure{
			Kind:    FailureOverload,
			RetryAt: parseRetryAfter(now, header),
			Reason:  "upstream unavailable",
		}

	case status == http.StatusBadRequest,
		status == http.StatusNotFound,
		status == http.StatusMethodNotAllowed,
		status == http.StatusConflict,
		status == http.StatusRequestEntityTooLarge,
		status == http.StatusUnprocessableEntity:
		return Failure{Kind: FailureClient, Reason: "client error"}

	default:
		return Failure{Kind: FailureTransient, Reason: "unexpected status " + strconv.Itoa(status)}
	}
}

// parseRetryAfter understands both forms of the Retry-After header (delta
// seconds and HTTP-date) and the common vendor aliases. It returns zero when the
// upstream gives no hint, which callers resolve to their own default.
func parseRetryAfter(now time.Time, header http.Header) time.Time {
	if header == nil {
		return time.Time{}
	}
	for _, key := range []string{
		"Retry-After",
		"retry-after",
		"x-ratelimit-reset-after",
		"x-ratelimit-reset-requests",
	} {
		raw := strings.TrimSpace(header.Get(key))
		if raw == "" {
			continue
		}
		if secs, err := strconv.ParseFloat(strings.TrimSuffix(raw, "s"), 64); err == nil && secs >= 0 {
			return now.Add(time.Duration(secs * float64(time.Second)))
		}
		if t, err := http.ParseTime(raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// retryAtOr falls back to a default cooldown when the upstream is silent.
func retryAtOr(now, t time.Time, fallback time.Duration) time.Time {
	if t.IsZero() {
		return now.Add(fallback)
	}
	return t
}

// bodyContains is a tiny case-insensitive substring probe used to spot provider
// error codes inside otherwise untyped JSON bodies.
func bodyContains(body []byte, needle string) bool {
	if len(body) == 0 || needle == "" {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), strings.ToLower(needle))
}

// stickyKey builds the deterministic affinity key. It must be stable across
// processes and restarts, so it is a hash rather than a counter.
func stickyKey(platform Platform, sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sessionKey))
	return string(platform) + ":" + hex.EncodeToString(sum[:8])
}

// Header helpers shared by adapters.

func setIfNotEmpty(h http.Header, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		h.Set(key, v)
	}
}

// mergeBeta joins the platform's mandatory beta flags with account-level extras
// without duplicating entries. This exact merge is currently inlined at every
// OAuth call site in gateway_service.go.
func mergeBeta(groups ...string) string {
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for _, group := range groups {
		for _, part := range strings.Split(group, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, dup := seen[part]; dup {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return strings.Join(out, ", ")
}
