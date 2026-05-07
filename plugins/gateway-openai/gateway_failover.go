package main

import (
	"strconv"
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// failoverStatusCodes are HTTP status codes that indicate transient
// upstream issues where retrying with a different account may succeed.
var failoverStatusCodes = []int{429, 500, 502, 503, 504}

// noFailoverStatusCodes indicate credential or request problems that
// won't be fixed by switching accounts.
var noFailoverStatusCodes = []int{401, 403}

// classifyShouldFailover determines whether a failed forward should be
// retried with a different account based on the error context.
//
// Policy:
//   - 429 (rate limit), 5xx (server errors) -> failover
//   - 401, 403 (auth/permission) -> do NOT failover
//   - Unknown/network errors -> failover (benefit of the doubt)
func classifyShouldFailover(req *pb.GatewayFailoverRequest) bool {
	errMsg := req.GetErrorMessage()
	errType := req.GetErrorType()

	// If the error type explicitly indicates a failover error, honor it
	if errType == "UpstreamFailoverError" || errType == "*service.UpstreamFailoverError" {
		return true
	}

	// Check non-failover codes first (more specific)
	for _, code := range noFailoverStatusCodes {
		if strings.Contains(errMsg, strconv.Itoa(code)) {
			return false
		}
	}

	// Check failover-eligible codes
	for _, code := range failoverStatusCodes {
		if strings.Contains(errMsg, strconv.Itoa(code)) {
			return true
		}
	}

	// Default: failover on unknown errors (network timeouts, etc.)
	return true
}
