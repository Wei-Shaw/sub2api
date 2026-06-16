// Package gatewayutil provides shared utility functions for gateway plugins.
// These functions are common across all gateway plugin implementations
// (anthropic, openai, gemini, antigravity) and extracted here to avoid
// duplication. Platform-specific differences are expressed through parameters.
package gatewayutil

import (
	"strings"
)

// Network error detection patterns. These match connection-level failures
// that warrant automatic failover to a different account.
var networkErrorPatterns = []string{
	"connection refused",
	"connection reset",
	"no such host",
	"dial tcp",
	"tls handshake",
	"i/o timeout",
	"eof",
}

// IsNetworkError detects connection-level failures from error messages.
// Returns true if the message contains any known network error pattern.
func IsNetworkError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, p := range networkErrorPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// ShouldFailoverStatus returns true for upstream HTTP status codes that
// warrant automatic failover. The extraCodes parameter allows plugins to
// add platform-specific status codes (e.g., 529 for Anthropic overloaded).
// All 5xx status codes are always considered failover-eligible.
func ShouldFailoverStatus(statusCode int, extraCodes map[int]bool) bool {
	// Common failover codes across all platforms.
	switch statusCode {
	case 401, 403, 429:
		return true
	}
	if extraCodes[statusCode] {
		return true
	}
	return statusCode >= 500
}

// ClassifyErrorType maps an upstream HTTP status code to a structured
// error type string for the host's routing decisions. The extraOverloaded
// parameter specifies additional status codes to classify as "overloaded"
// (e.g., 529 for Anthropic).
func ClassifyErrorType(statusCode int, extraOverloaded []int) string {
	switch statusCode {
	case 429:
		return "rate_limit"
	case 503:
		return "overloaded"
	case 401, 403:
		return "auth_error"
	case 500, 502, 504:
		return "server_error"
	}
	for _, code := range extraOverloaded {
		if statusCode == code {
			return "overloaded"
		}
	}
	return ""
}
