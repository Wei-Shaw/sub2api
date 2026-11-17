package service

import "strings"

type ChannelMonitorV2ErrorInput struct {
	ErrorType          string
	ErrorOwner         string
	ErrorSource        string
	StatusCode         int
	UpstreamStatusCode int
	Message            string
}

// ClassifyChannelMonitorV2Error is deliberately ordered. Once rows are stored
// with a taxonomy version, changing this order requires a new version.
func ClassifyChannelMonitorV2Error(input ChannelMonitorV2ErrorInput) string {
	errorType := strings.ToLower(strings.TrimSpace(input.ErrorType))
	owner := strings.ToLower(strings.TrimSpace(input.ErrorOwner))
	text := strings.ToLower(strings.Join([]string{input.ErrorType, input.ErrorSource, input.Message}, " "))

	if errorType == "cyber_policy" || channelMonitorV2ContainsAny(text, "content policy", "content_policy", "safety policy", "moderation", "blocked keyword") {
		return "content_policy"
	}
	if input.StatusCode == 401 || input.UpstreamStatusCode == 401 || channelMonitorV2ContainsAny(text, "unauthorized", "invalid api key", "invalid_api_key", "authentication", "api_key_disabled") {
		return "authentication"
	}
	if channelMonitorV2ContainsAny(text, "context window", "context length", "maximum prompt length", "too many tokens", "max_tokens") {
		return "context_limit"
	}
	if channelMonitorV2ContainsAny(text, "failed to deserialize", "missing required parameter", "invalid request", "invalid_request", "tool_choice") {
		return "invalid_request"
	}
	if channelMonitorV2ContainsAny(text, "does not support the requested model", "not supported by any configured account", "model not supported", "unsupported model") {
		return "model_unsupported"
	}
	if channelMonitorV2ContainsAny(text, "group not allowed", "group_not_allowed", "group access") {
		return "group_access"
	}
	if channelMonitorV2ContainsAny(text, "run out of credits", "insufficient balance", "insufficient quota", "subscription", "quota exceeded", "billing hard limit") {
		return "quota_or_balance"
	}
	if channelMonitorV2ContainsAny(text, "no available accounts", "no healthy account", "no healthy upstream account", "failover budget exhausted", "account pool") {
		return "account_pool_unavailable"
	}
	if input.StatusCode == 429 || input.UpstreamStatusCode == 429 || channelMonitorV2ContainsAny(text, "rate limit", "rate_limit", "high demand", "overloaded", "concurrency limit", "capacity") {
		return "rate_or_capacity"
	}
	if input.StatusCode == 408 || input.StatusCode == 504 || channelMonitorV2ContainsAny(text, "timeout", "deadline exceeded", "error code: 524", "gateway time-out", "gateway timeout") {
		return "timeout"
	}
	if channelMonitorV2ContainsAny(text, "transport", "stream_read_error", "connection reset", "connection refused", "tls", "http2", "missing terminal event", "unexpected eof") {
		return "transport_or_stream"
	}
	if input.StatusCode == 403 || input.UpstreamStatusCode == 403 {
		return "upstream_forbidden"
	}
	if input.StatusCode == 404 || input.UpstreamStatusCode == 404 {
		return "not_found"
	}
	if input.StatusCode == 499 || channelMonitorV2ContainsAny(text, "client cancelled", "client canceled", "context canceled") {
		return "client_cancelled"
	}
	if input.UpstreamStatusCode >= 500 || (owner == "provider" && input.StatusCode >= 500) {
		return "upstream_5xx"
	}
	if input.StatusCode >= 500 || errorType == "internal" || owner == "system" {
		return "internal"
	}
	return "other"
}

func channelMonitorV2ContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
