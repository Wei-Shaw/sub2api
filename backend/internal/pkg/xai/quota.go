package xai

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const GrokFreeRolling24hTokenLimit int64 = 1_000_000

var grokFreeRolling24hTokenLimits = map[int64]struct{}{
	GrokFreeRolling24hTokenLimit: {},
	2_000_000:                    {}, // Legacy Free limit observed before July 2026.
}

func IsGrokFreeRolling24hTokenLimit(limit int64) bool {
	_, ok := grokFreeRolling24hTokenLimits[limit]
	return ok
}

type QuotaWindow struct {
	Limit     *int64 `json:"limit,omitempty"`
	Remaining *int64 `json:"remaining,omitempty"`
	ResetUnix *int64 `json:"reset_unix,omitempty"`
	ResetAt   string `json:"reset_at,omitempty"`
}

type QuotaSnapshot struct {
	Requests          *QuotaWindow      `json:"requests,omitempty"`
	Tokens            *QuotaWindow      `json:"tokens,omitempty"`
	RetryAfterSeconds *int              `json:"retry_after_seconds,omitempty"`
	SubscriptionTier  string            `json:"subscription_tier,omitempty"`
	EntitlementStatus string            `json:"entitlement_status,omitempty"`
	StatusCode        int               `json:"status_code,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	HeadersObserved   bool              `json:"headers_observed"`
	ObservationSource string            `json:"observation_source,omitempty"`
	LastProbeAt       string            `json:"last_probe_at,omitempty"`
	LastHeadersSeenAt string            `json:"last_headers_seen_at,omitempty"`
	UpdatedAt         string            `json:"updated_at"`
}

func (s *QuotaSnapshot) HasObservedHeaders() bool {
	if s == nil {
		return false
	}
	return s.HeadersObserved ||
		s.Requests != nil ||
		s.Tokens != nil ||
		s.RetryAfterSeconds != nil ||
		s.SubscriptionTier != "" ||
		s.EntitlementStatus != "" ||
		len(s.Headers) > 0
}

var quotaHeaderAllowlist = []string{
	"x-ratelimit-limit-requests",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-reset-requests",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-remaining-tokens",
	"x-ratelimit-reset-tokens",
	"retry-after",
	"x-subscription-tier",
	"xai-subscription-tier",
	"x-entitlement-status",
	"xai-entitlement-status",
}

func ParseQuotaHeaders(headers http.Header, statusCode int) *QuotaSnapshot {
	return parseQuotaHeaders(headers, statusCode, "", false)
}

func ObserveQuotaHeaders(headers http.Header, statusCode int, source string) *QuotaSnapshot {
	return parseQuotaHeaders(headers, statusCode, source, true)
}

func parseQuotaHeaders(headers http.Header, statusCode int, source string, keepEmpty bool) *QuotaSnapshot {
	if headers == nil && !keepEmpty {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	snapshot := &QuotaSnapshot{
		Requests:          parseQuotaWindow(headers, "requests"),
		Tokens:            parseQuotaWindow(headers, "tokens"),
		StatusCode:        statusCode,
		Headers:           make(map[string]string),
		ObservationSource: strings.TrimSpace(source),
		UpdatedAt:         now,
	}
	if snapshot.ObservationSource == "active_probe" {
		snapshot.LastProbeAt = now
	}
	if retryAfter := parseRetryAfter(headers.Get("retry-after")); retryAfter != nil {
		snapshot.RetryAfterSeconds = retryAfter
	}
	snapshot.SubscriptionTier = firstHeader(headers, "xai-subscription-tier", "x-subscription-tier")
	snapshot.EntitlementStatus = firstHeader(headers, "xai-entitlement-status", "x-entitlement-status")

	for _, name := range quotaHeaderAllowlist {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			snapshot.Headers[name] = value
		}
	}

	if snapshot.Requests == nil &&
		snapshot.Tokens == nil &&
		snapshot.RetryAfterSeconds == nil &&
		snapshot.SubscriptionTier == "" &&
		snapshot.EntitlementStatus == "" &&
		len(snapshot.Headers) == 0 {
		if keepEmpty {
			return snapshot
		}
		return nil
	}
	snapshot.HeadersObserved = true
	snapshot.LastHeadersSeenAt = now
	return snapshot
}

func parseQuotaWindow(headers http.Header, dimension string) *QuotaWindow {
	window := &QuotaWindow{
		Limit:     parseInt64Ptr(headers.Get("x-ratelimit-limit-" + dimension)),
		Remaining: parseInt64Ptr(headers.Get("x-ratelimit-remaining-" + dimension)),
	}
	if reset := parseResetHeader(headers.Get("x-ratelimit-reset-" + dimension)); reset != nil {
		window.ResetUnix = reset
		window.ResetAt = time.Unix(*reset, 0).UTC().Format(time.RFC3339)
	}
	if window.Limit == nil && window.Remaining == nil && window.ResetUnix == nil {
		return nil
	}
	return window
}

func parseResetHeader(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if value > 1_000_000_000_000 {
			value = value / 1000
		}
		return &value
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		value := t.Unix()
		return &value
	}
	return nil
}

func parseRetryAfter(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return &value
	}
	if t, err := http.ParseTime(raw); err == nil {
		seconds := int(time.Until(t).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		return &seconds
	}
	return nil
}

func parseInt64Ptr(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
