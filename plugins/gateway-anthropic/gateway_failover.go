package main

import (
	"context"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
)

// extraFailoverCodes lists platform-specific status codes that trigger
// failover beyond the common set (401, 403, 429, 5xx).
var extraFailoverCodes = map[int]bool{
	529: true, // Anthropic overloaded
}

// extraOverloadedCodes lists status codes classified as "overloaded"
// beyond the common 503.
var extraOverloadedCodes = []int{529}

// ShouldFailover returns true when the error from Forward is transient
// and the Pipeline should try a different account.
//
// Classification:
//   - HTTP 401/403/429/529 and 5xx: failover
//   - Connection/network errors: failover
//   - HTTP 400 (bad request): do NOT failover
//   - Other 4xx: do NOT failover
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	errMsg := req.GetErrorMessage()
	errType := req.GetErrorType()

	// UpstreamFailoverError from our own Forward: always failover.
	if errType == "UpstreamFailoverError" || errType == "*service.UpstreamFailoverError" {
		return &pb.GatewayFailoverResponse{ShouldFailover: true}, nil
	}

	// Network/connection errors: failover.
	if gatewayutil.IsNetworkError(errMsg) {
		return &pb.GatewayFailoverResponse{ShouldFailover: true}, nil
	}

	return &pb.GatewayFailoverResponse{ShouldFailover: false}, nil
}
