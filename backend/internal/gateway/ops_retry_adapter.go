package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// OpsRetryForwarderAdapter implements service.OpsRetryForwarder by looking
// up a GatewayProvider from the ProviderRegistry and building a ForwardRequest.
// This adapter lives in the gateway package to access ForwardRequest and
// ProviderRegistry without creating a circular import.
type OpsRetryForwarderAdapter struct {
	registry *ProviderRegistry
}

// NewOpsRetryForwarderAdapter creates an adapter backed by the given registry.
func NewOpsRetryForwarderAdapter(registry *ProviderRegistry) *OpsRetryForwarderAdapter {
	return &OpsRetryForwarderAdapter{registry: registry}
}

// Forward looks up the provider for the account's platform and dispatches
// the request. Builds a minimal ForwardRequest from the retry parameters.
func (a *OpsRetryForwarderAdapter) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	params *service.OpsRetryForwardParams,
) error {
	if params == nil || params.Account == nil {
		return fmt.Errorf("ops retry: missing account")
	}

	provider, ok := a.registry.Get(params.Account.Platform)
	if !ok {
		return fmt.Errorf("ops retry: no provider for platform %q", params.Account.Platform)
	}

	fwdReq := buildOpsRetryForwardRequest(params)
	_, err := provider.Forward(ctx, w, fwdReq)
	return err
}

// HasProvider returns true if a provider is registered for the given platform.
func (a *OpsRetryForwarderAdapter) HasProvider(platform string) bool {
	_, ok := a.registry.Get(platform)
	return ok
}

// buildOpsRetryForwardRequest converts OpsRetryForwardParams into a
// ForwardRequest suitable for provider.Forward. Only fields relevant
// to retry forwarding are populated; billing, auth, and slot fields
// are left at zero values since retry bypasses those pipeline stages.
func buildOpsRetryForwardRequest(params *service.OpsRetryForwardParams) *ForwardRequest {
	return &ForwardRequest{
		RawBody:      params.RawBody,
		Model:        params.Model,
		Stream:       params.Stream,
		Protocol:     params.Protocol,
		GeminiAction: params.GeminiAction,
		GroupID:      params.GroupID,
		Account:      params.Account,
		GinContext:   params.GinContext,
		RequestID:    uuid.NewString(),
	}
}
