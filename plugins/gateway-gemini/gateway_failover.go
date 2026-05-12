package main

import (
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// --- failover decision ---

// failoverStatusCodes are upstream HTTP status codes that indicate the
// request should be retried with a different account. These match the
// host's shouldFailoverUpstreamError logic.
var failoverStatusCodes = map[int]bool{
	401: true, // unauthorized -- credentials invalid/expired
	403: true, // forbidden -- account banned or out of scope
	429: true, // rate limited
	503: true, // service unavailable (MODEL_CAPACITY_EXHAUSTED)
}

// classifyShouldFailover determines whether a failed forward should be
// retried with a different account based on the error context.
//
// Policy:
//   - UpstreamFailoverError from our own Forward: always failover
//   - Network/connection errors: failover
//   - Other errors: no failover (likely client error)
func classifyShouldFailover(req *pb.GatewayFailoverRequest) bool {
	errType := req.GetErrorType()
	errMsg := req.GetErrorMessage()

	// UpstreamFailoverError from our own Forward: always failover.
	if errType == "UpstreamFailoverError" || errType == "*service.UpstreamFailoverError" {
		return true
	}

	// Network/connection errors: failover.
	if isNetworkError(errMsg) {
		return true
	}

	return false
}

// shouldFailoverStatus returns true for status codes that warrant failover.
// Used by handleErrorResponse to attach GatewayUpstreamError.
func shouldFailoverStatus(statusCode int) bool {
	if failoverStatusCodes[statusCode] {
		return true
	}
	// Also failover on all 5xx server errors.
	return statusCode >= 500
}

// classifyErrorType maps an upstream HTTP status code to a structured error
// type string for the host's routing decisions. Matches the host-side
// UpstreamFailoverError classification.
func classifyErrorType(statusCode int) string {
	switch statusCode {
	case 429:
		return "rate_limit"
	case 503:
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
