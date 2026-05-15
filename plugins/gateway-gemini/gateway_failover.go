package main

import (
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
)

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
	if gatewayutil.IsNetworkError(errMsg) {
		return true
	}

	return false
}

// shouldFailoverStatus returns true for status codes that warrant failover.
// Gemini does not use 529, so no extra codes are needed.
func shouldFailoverStatus(statusCode int) bool {
	return gatewayutil.ShouldFailoverStatus(statusCode, nil)
}

// classifyErrorType maps an upstream HTTP status code to a structured error
// type string for the host's routing decisions.
func classifyErrorType(statusCode int) string {
	return gatewayutil.ClassifyErrorType(statusCode, nil)
}
