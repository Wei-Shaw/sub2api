package gateway

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GeminiProvider wraps GeminiMessagesCompatService as a GatewayProvider.
// Phase 1 thin adapter: no business logic, only type conversion.
//
// Supported protocols:
//   - "gemini" -> GeminiMessagesCompatService.ForwardNative (/v1beta/models/{model}:*)
//
// Note: Antigravity accounts with Gemini protocol are handled by
// AntigravityProvider, which also supports ProtocolGemini. The pipeline
// selects the provider based on account.Platform after account selection.
type GeminiProvider struct {
	geminiService *service.GeminiMessagesCompatService
}

// NewGeminiProvider creates a GeminiProvider backed by the existing
// GeminiMessagesCompatService.
func NewGeminiProvider(svc *service.GeminiMessagesCompatService) *GeminiProvider {
	return &GeminiProvider{geminiService: svc}
}

func (p *GeminiProvider) Platform() string      { return domain.PlatformGemini }
func (p *GeminiProvider) Protocols() []string    { return []string{ProtocolGemini} }

// Forward dispatches to GeminiMessagesCompatService.ForwardNative,
// then maps the result to gateway.ForwardResult.
func (p *GeminiProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	result, err := p.geminiService.ForwardNative(
		ctx, req.GinContext, req.Account,
		req.Model, req.GeminiAction, req.Stream, req.RawBody,
	)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}

// ShouldFailover returns true when the error is an UpstreamFailoverError.
func (p *GeminiProvider) ShouldFailover(
	_ context.Context, _ *ForwardRequest, err error,
) bool {
	return DefaultShouldFailover(err)
}
