package gateway

import (
	"context"
	"net/http"
)

// GatewayProvider is the platform-specific upstream forwarder interface.
// Each platform (anthropic / openai / antigravity) implements this; the
// GatewayPipeline calls it after account selection + billing acquire.
//
// Phase 1: three host-internal adapters wrap the existing god-object
// methods (GatewayService.Forward, OpenAIGatewayService.Forward, etc.).
// Phase 2+: plugins implement this over gRPC via GatewayProviderExtension.
type GatewayProvider interface {
	Platform() string
	Protocols() []string
	Forward(ctx context.Context, w http.ResponseWriter, req *ForwardRequest) (*ForwardResult, error)
	ShouldFailover(ctx context.Context, req *ForwardRequest, err error) bool
}
