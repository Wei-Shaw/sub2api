package main

import (
	"strings"
)

// --- failover decision ---

// failoverStatusCodes are upstream HTTP status codes that indicate the
// request should be retried with a different account. Matches the host's
// AntigravityGatewayService.shouldFailoverUpstreamError logic.
var failoverStatusCodes = map[int]bool{
	401: true, // unauthorized -- credentials invalid/expired
	403: true, // forbidden -- account banned or out of scope
	429: true, // rate limited
	529: true, // Anthropic overloaded
}

// shouldFailoverStatus returns true for status codes that warrant failover.
func shouldFailoverStatus(statusCode int) bool {
	if failoverStatusCodes[statusCode] {
		return true
	}
	return statusCode >= 500
}

// classifyErrorType maps an upstream HTTP status code to a structured error
// type string for the host's routing decisions.
func classifyErrorType(statusCode int) string {
	switch statusCode {
	case 429:
		return "rate_limit"
	case 529, 503:
		return "overloaded"
	case 401, 403:
		return "auth_error"
	case 500, 502, 504:
		return "server_error"
	default:
		return ""
	}
}

// isNetworkError detects connection-level failures from error messages.
func isNetworkError(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"dial tcp",
		"tls handshake",
		"i/o timeout",
		"eof",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// maxErrorBodyForProto caps the upstream error body embedded in the proto
// message to avoid excessive gRPC message sizes.
const maxErrorBodyForProto = 8 << 10 // 8 KB

// truncateBytes returns b truncated to at most maxLen bytes.
func truncateBytes(b []byte, maxLen int) []byte {
	if len(b) <= maxLen {
		return b
	}
	return b[:maxLen]
}
