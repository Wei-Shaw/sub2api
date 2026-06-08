package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GeminiModelListingProvider is a narrow interface for the Gemini model
// listing endpoints (GET /v1beta/models, GET /v1beta/models/:model).
// It decouples the GatewayHandler from the concrete GeminiMessagesCompatService,
// allowing future implementations to route through the plugin infrastructure.
type GeminiModelListingProvider interface {
	// SelectAccountForAIStudioEndpoints selects an account suitable for
	// AI Studio GET requests (model listing). Returns the best-ranked
	// account or an error if none is available.
	SelectAccountForAIStudioEndpoints(ctx context.Context, groupID *int64) (*service.Account, error)

	// HasAntigravityAccounts checks whether there are schedulable
	// Antigravity accounts in the group (used for fallback model lists).
	HasAntigravityAccounts(ctx context.Context, groupID *int64) (bool, error)

	// ForwardAIStudioGET proxies a GET request to AI Studio for the
	// given path (e.g. "/v1beta/models" or "/v1beta/models/gemini-pro").
	ForwardAIStudioGET(ctx context.Context, account *service.Account, path string) (*service.UpstreamHTTPResult, error)
}

// geminiModelListingAdapter adapts *service.GeminiMessagesCompatService to
// the GeminiModelListingProvider interface.
type geminiModelListingAdapter struct {
	svc *service.GeminiMessagesCompatService
}

// NewGeminiModelListingProvider wraps a GeminiMessagesCompatService in the
// narrow GeminiModelListingProvider interface.
func NewGeminiModelListingProvider(svc *service.GeminiMessagesCompatService) GeminiModelListingProvider {
	if svc == nil {
		return nil
	}
	return &geminiModelListingAdapter{svc: svc}
}

func (a *geminiModelListingAdapter) SelectAccountForAIStudioEndpoints(ctx context.Context, groupID *int64) (*service.Account, error) {
	return a.svc.SelectAccountForAIStudioEndpoints(ctx, groupID)
}

func (a *geminiModelListingAdapter) HasAntigravityAccounts(ctx context.Context, groupID *int64) (bool, error) {
	return a.svc.HasAntigravityAccounts(ctx, groupID)
}

func (a *geminiModelListingAdapter) ForwardAIStudioGET(ctx context.Context, account *service.Account, path string) (*service.UpstreamHTTPResult, error) {
	return a.svc.ForwardAIStudioGET(ctx, account, path)
}
