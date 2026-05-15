package main

import (
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
)

// extraFailoverCodes lists platform-specific status codes that trigger
// failover beyond the common set (401, 403, 429, 5xx).
var extraFailoverCodes = map[int]bool{
	529: true, // Anthropic overloaded (antigravity routes to Anthropic)
}

// extraOverloadedCodes lists status codes classified as "overloaded"
// beyond the common 503.
var extraOverloadedCodes = []int{529}

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

// maxErrorBodyForProto caps the upstream error body embedded in the proto
// message to avoid excessive gRPC message sizes.
const maxErrorBodyForProto = gatewayutil.MaxErrorBodyForProto
