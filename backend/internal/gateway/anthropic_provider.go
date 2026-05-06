package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AnthropicProvider wraps GatewayService.Forward as a GatewayProvider.
// Phase 1 thin adapter: no business logic, only type conversion.
type AnthropicProvider struct {
	gatewayService *service.GatewayService
}

// NewAnthropicProvider creates an AnthropicProvider backed by the
// existing GatewayService god object.
func NewAnthropicProvider(gw *service.GatewayService) *AnthropicProvider {
	return &AnthropicProvider{gatewayService: gw}
}

func (p *AnthropicProvider) Platform() string   { return "anthropic" }
func (p *AnthropicProvider) Protocols() []string { return []string{"anthropic"} }

// Forward converts ForwardRequest to service.ParsedRequest and delegates
// to GatewayService.Forward, then maps the result back.
func (p *AnthropicProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	parsed := toServiceParsedRequest(req)
	result, err := p.gatewayService.Forward(ctx, req.GinContext, req.Account, parsed)
	if err != nil {
		return nil, err
	}
	return anthropicResultToForwardResult(result), nil
}

// ShouldFailover returns true when the error is an UpstreamFailoverError,
// which signals the Pipeline to try the next account.
func (p *AnthropicProvider) ShouldFailover(
	_ context.Context, _ *ForwardRequest, err error,
) bool {
	var failoverErr *service.UpstreamFailoverError
	return errors.As(err, &failoverErr)
}

// toServiceParsedRequest builds the service.ParsedRequest that the legacy
// GatewayService.Forward expects from a gateway.ForwardRequest.
func toServiceParsedRequest(req *ForwardRequest) *service.ParsedRequest {
	return &service.ParsedRequest{
		Body:           req.RawBody,
		Model:          req.Model,
		Stream:         req.Stream,
		MetadataUserID: req.MetadataUserID,
		GroupID:        req.GroupID,
		OnUpstreamAccepted: func() {
			req.UpstreamAccepted = true
		},
	}
}

// anthropicResultToForwardResult maps a service.ForwardResult (Anthropic /
// Claude usage) to the protocol-agnostic gateway.ForwardResult.
func anthropicResultToForwardResult(r *service.ForwardResult) *ForwardResult {
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
