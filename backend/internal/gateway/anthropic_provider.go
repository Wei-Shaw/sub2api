package gateway

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AnthropicProvider wraps GatewayService.Forward as a GatewayProvider.
// Phase 1 thin adapter: no business logic, only type conversion.
//
// Supported protocols:
//   - "anthropic"        -> GatewayService.Forward                  (/v1/messages)
//   - "chat_completions" -> GatewayService.ForwardAsChatCompletions (/v1/chat/completions)
//   - "responses"        -> GatewayService.ForwardAsResponses       (/v1/responses)
type AnthropicProvider struct {
	gatewayService *service.GatewayService
}

// NewAnthropicProvider creates an AnthropicProvider backed by the
// existing GatewayService god object.
func NewAnthropicProvider(gw *service.GatewayService) *AnthropicProvider {
	return &AnthropicProvider{gatewayService: gw}
}

func (p *AnthropicProvider) Platform() string { return domain.PlatformAnthropic }
func (p *AnthropicProvider) Protocols() []string {
	return []string{ProtocolAnthropic, ProtocolChatCompletions, ProtocolResponses}
}

// Forward dispatches to the appropriate GatewayService method based on
// req.Protocol, then maps the result to gateway.ForwardResult.
func (p *AnthropicProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	switch req.Protocol {
	case ProtocolChatCompletions:
		return p.forwardChatCompletions(ctx, req)
	case ProtocolResponses:
		return p.forwardResponses(ctx, req)
	default:
		return p.forwardMessages(ctx, req)
	}
}

func (p *AnthropicProvider) forwardMessages(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	parsed := toServiceParsedRequest(req)
	result, err := p.gatewayService.Forward(ctx, req.GinContext, req.Account, parsed)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}

func (p *AnthropicProvider) forwardChatCompletions(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	parsed := toServiceParsedRequest(req)
	result, err := p.gatewayService.ForwardAsChatCompletions(
		ctx, req.GinContext, req.Account, req.RawBody, parsed,
	)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}

func (p *AnthropicProvider) forwardResponses(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	parsed := toServiceParsedRequest(req)
	result, err := p.gatewayService.ForwardAsResponses(
		ctx, req.GinContext, req.Account, req.RawBody, parsed,
	)
	if err != nil {
		return nil, err
	}
	return ServiceResultToForwardResult(result), nil
}

// ShouldFailover returns true when the error is an UpstreamFailoverError,
// which signals the Pipeline to try the next account.
func (p *AnthropicProvider) ShouldFailover(
	_ context.Context, _ *ForwardRequest, err error,
) bool {
	return DefaultShouldFailover(err)
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
