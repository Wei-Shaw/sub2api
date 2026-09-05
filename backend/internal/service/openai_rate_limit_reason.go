package service

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// OpenAIRateLimitReason classifies evidence, not assumptions about the scope of
// a provider limit. A bare 429 must never be treated as device-slot saturation.
type OpenAIRateLimitReason string

const (
	OpenAIRateLimitUnknown     OpenAIRateLimitReason = "unknown"
	OpenAIRateLimitQuota       OpenAIRateLimitReason = "quota_exhausted"
	OpenAIRateLimitConcurrency OpenAIRateLimitReason = "concurrency_limited"
	OpenAIRateLimitRate        OpenAIRateLimitReason = "rate_limited"
)

// OpenAIFailoverRateLimitReason preserves classification based on the actual
// failure headers; a successful SSE/WS handshake may have unrelated quota data.
func OpenAIFailoverRateLimitReason(failure *UpstreamFailoverError) OpenAIRateLimitReason {
	if failure == nil {
		return OpenAIRateLimitUnknown
	}
	for _, reason := range []OpenAIRateLimitReason{OpenAIRateLimitUnknown, OpenAIRateLimitQuota, OpenAIRateLimitConcurrency, OpenAIRateLimitRate} {
		if failure.Reason == GatewayFailureReason("openai_429_"+string(reason)) {
			return reason
		}
	}
	return ClassifyOpenAIRateLimitReason(nil, failure.ResponseBody)
}

func ClassifyOpenAIRateLimitReason(headers http.Header, body []byte) OpenAIRateLimitReason {
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		if normalized := snapshot.Normalize(); normalized != nil &&
			((normalized.Used5hPercent != nil && *normalized.Used5hPercent >= 100) ||
				(normalized.Used7dPercent != nil && *normalized.Used7dPercent >= 100)) {
			return OpenAIRateLimitQuota
		}
	}
	reason := OpenAIRateLimitUnknown
	if !gjson.ValidBytes(body) {
		return reason
	}
	for _, path := range []string{"error.code", "error.type", "response.error.code", "response.error.type", "code", "type"} {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())) {
		case "insufficient_quota", "usage_limit_reached", "usage_limit_exceeded", "quota_exceeded", "billing_hard_limit_reached":
			return OpenAIRateLimitQuota
		case "concurrency_limit_exceeded", "concurrent_request_limit_exceeded", "too_many_concurrent_requests":
			reason = OpenAIRateLimitConcurrency
		case "rate_limit_exceeded", "rate_limit_error", "rate_limit_reached":
			if reason == OpenAIRateLimitUnknown {
				reason = OpenAIRateLimitRate
			}
		}
	}
	return reason
}
