package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AntigravityProvider wraps AntigravityGatewayService as a GatewayProvider.
// It supports both "anthropic" and "gemini" input protocols, dispatching
// to Forward or ForwardGemini respectively.
type AntigravityProvider struct {
	antigravityService *service.AntigravityGatewayService
}

// NewAntigravityProvider creates an AntigravityProvider backed by the
// existing AntigravityGatewayService god object.
func NewAntigravityProvider(svc *service.AntigravityGatewayService) *AntigravityProvider {
	return &AntigravityProvider{antigravityService: svc}
}

func (p *AntigravityProvider) Platform() string { return domain.PlatformAntigravity }
func (p *AntigravityProvider) Protocols() []string {
	return []string{domain.PlatformAnthropic, domain.PlatformGemini}
}

// Forward dispatches to the appropriate AntigravityGatewayService method
// based on req.Protocol.
func (p *AntigravityProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	switch req.Protocol {
	case domain.PlatformAnthropic:
		return p.forwardAnthropic(ctx, req)
	case domain.PlatformGemini:
		return p.forwardGemini(ctx, req)
	default:
		return nil, fmt.Errorf("antigravity: unsupported protocol %q", req.Protocol)
	}
}

// ShouldFailover returns true when the error is an UpstreamFailoverError,
// which signals the Pipeline to try the next account.
func (p *AntigravityProvider) ShouldFailover(
	_ context.Context, _ *ForwardRequest, err error,
) bool {
	return DefaultShouldFailover(err)
}

func (p *AntigravityProvider) forwardAnthropic(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	result, err := p.antigravityService.Forward(
		ctx, req.GinContext, req.Account,
		req.RawBody, req.IsStickySession,
	)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}

func (p *AntigravityProvider) forwardGemini(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	result, err := p.antigravityService.ForwardGemini(
		ctx, req.GinContext, req.Account,
		req.Model, req.GeminiAction, req.Stream,
		req.RawBody, req.IsStickySession,
	)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}
