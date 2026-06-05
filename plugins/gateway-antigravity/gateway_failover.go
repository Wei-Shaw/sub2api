package main

import (
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// extraFailoverCodes lists platform-specific status codes that trigger
// failover beyond the common set (401, 403, 429, 5xx).
var extraFailoverCodes = map[int]bool{
	529: true, // Anthropic overloaded (antigravity routes to Anthropic)
}

// extraOverloadedCodes lists status codes classified as "overloaded"
// beyond the common 503.
var extraOverloadedCodes = []int{529}

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

	if errType == "UpstreamFailoverError" || errType == "*service.UpstreamFailoverError" {
		return true
	}

	if gatewayutil.IsNetworkError(errMsg) {
		return true
	}

	return false
}

// shouldFailoverStatus returns true for status codes that warrant failover.
// Antigravity includes 529 (Anthropic overloaded) as an extra code.
func shouldFailoverStatus(statusCode int) bool {
	return gatewayutil.ShouldFailoverStatus(statusCode, extraFailoverCodes)
}

// classifyErrorType maps an upstream HTTP status code to a structured error
// type string for the host's routing decisions.
func classifyErrorType(statusCode int) string {
	return gatewayutil.ClassifyErrorType(statusCode, extraOverloadedCodes)
}
