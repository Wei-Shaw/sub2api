package main

import (
	"context"
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
	529: true, // Anthropic overloaded
}

// ShouldFailover returns true when the error from Forward is transient
// and the Pipeline should try a different account.
//
// Classification:
//   - HTTP 401/403/429/529 and 5xx: failover
//   - Connection/network errors: failover
//   - HTTP 400 (bad request): do NOT failover (client error)
//   - Other 4xx: do NOT failover
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	errMsg := req.GetErrorMessage()
	errType := req.GetErrorType()

	// UpstreamFailoverError from our own Forward: always failover.
	if errType == "UpstreamFailoverError" {
		return &pb.GatewayFailoverResponse{ShouldFailover: true}, nil
	}

	// Network/connection errors: failover.
	if isNetworkError(errMsg) {
		return &pb.GatewayFailoverResponse{ShouldFailover: true}, nil
	}

	return &pb.GatewayFailoverResponse{ShouldFailover: false}, nil
}

// shouldFailoverStatus returns true for status codes that warrant failover.
func shouldFailoverStatus(statusCode int) bool {
	if failoverStatusCodes[statusCode] {
		return true
	}
	return statusCode >= 500
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
