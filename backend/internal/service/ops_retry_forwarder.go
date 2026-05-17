package service

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpsRetryForwardParams carries the parameters needed for an ops retry forward call.
// This struct decouples the service layer from the gateway.ForwardRequest type
// to avoid circular imports between service and gateway packages.
type OpsRetryForwardParams struct {
	Account      *Account
	RawBody      []byte
	Model        string
	Stream       bool
	Protocol     string
	GeminiAction string
	GroupID      *int64
	GinContext   *gin.Context
}

// OpsRetryForwarder abstracts platform-specific upstream forwarding for the
// ops retry subsystem. The gateway.OpsRetryForwarderAdapter implements this
// by looking up a GatewayProvider from the ProviderRegistry and building a
// ForwardRequest. This interface lives in the service package to avoid a
// circular import (service -> gateway -> service).
type OpsRetryForwarder interface {
	// Forward dispatches the request body to the upstream provider for the
	// given account's platform. Returns nil on success; any error means
	// the forward failed.
	Forward(ctx context.Context, w http.ResponseWriter, params *OpsRetryForwardParams) error

	// HasProvider returns true if a provider is registered for the given platform.
	HasProvider(platform string) bool
}
