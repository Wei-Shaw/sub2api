package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

func (p *AntigravityProvider) Platform() string   { return "antigravity" }
func (p *AntigravityProvider) Protocols() []string { return []string{"anthropic", "gemini"} }

// Forward dispatches to the appropriate AntigravityGatewayService method
// based on req.Protocol.
func (p *AntigravityProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	switch req.Protocol {
	case "anthropic":
		return p.forwardAnthropic(ctx, req)
	case "gemini":
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
	var failoverErr *service.UpstreamFailoverError
	return errors.As(err, &failoverErr)
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
	return antigravityResultToForwardResult(result), nil
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
	return antigravityResultToForwardResult(result), nil
}

// antigravityResultToForwardResult maps a service.ForwardResult (shared
// with Anthropic) to the protocol-agnostic gateway.ForwardResult.
// Antigravity returns the same service.ForwardResult type as Anthropic.
func antigravityResultToForwardResult(r *service.ForwardResult) *ForwardResult {
	return &ForwardResult{
		RequestID:           r.RequestID,
		Model:               r.Model,
		UpstreamModel:       r.UpstreamModel,
		Stream:              r.Stream,
		Duration:            r.Duration,
		FirstTokenMs:        r.FirstTokenMs,
		InputTokens:         int64(r.Usage.InputTokens),
		OutputTokens:        int64(r.Usage.OutputTokens),
		CacheCreationTokens: int64(r.Usage.CacheCreationInputTokens),
		CacheReadTokens:     int64(r.Usage.CacheReadInputTokens),
		ImageOutputTokens:   int64(r.Usage.ImageOutputTokens),
		ClientDisconnect:    r.ClientDisconnect,
		ImageCount:          r.ImageCount,
		ImageSize:           r.ImageSize,
	}
}
