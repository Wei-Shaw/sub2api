package gateway

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// OpenAIProvider wraps OpenAIGatewayService.Forward as a GatewayProvider.
// Phase 1 thin adapter: no business logic, only type conversion.
//
// Supported protocols:
//   - "openai"            -> OpenAIGatewayService.Forward              (/openai/v1/responses)
//   - "chat_completions"  -> OpenAIGatewayService.ForwardAsChatCompletions (/v1/chat/completions)
//   - "responses"         -> OpenAIGatewayService.Forward              (alias for openai)
type OpenAIProvider struct {
	openaiService *service.OpenAIGatewayService
}

// NewOpenAIProvider creates an OpenAIProvider backed by the existing
// OpenAIGatewayService god object.
func NewOpenAIProvider(svc *service.OpenAIGatewayService) *OpenAIProvider {
	return &OpenAIProvider{openaiService: svc}
}

func (p *OpenAIProvider) Platform() string { return domain.PlatformOpenAI }
func (p *OpenAIProvider) Protocols() []string {
	return []string{ProtocolOpenAI, ProtocolChatCompletions, ProtocolResponses}
}

// Forward dispatches to the appropriate OpenAIGatewayService method based on
// req.Protocol, then maps the result to gateway.ForwardResult.
func (p *OpenAIProvider) Forward(
	ctx context.Context,
	_ http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	switch req.Protocol {
	case ProtocolChatCompletions:
		return p.forwardChatCompletions(ctx, req)
	default:
		return p.forwardResponses(ctx, req)
	}
}

func (p *OpenAIProvider) forwardChatCompletions(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	result, err := p.openaiService.ForwardAsChatCompletions(
		ctx, req.GinContext, req.Account, req.RawBody,
		req.PromptCacheKey, req.DefaultMappedModel,
	)
	if err != nil {
		return nil, err
	}
	return openaiResultToForwardResult(result), nil
}

func (p *OpenAIProvider) forwardResponses(
	ctx context.Context, req *ForwardRequest,
) (*ForwardResult, error) {
	result, err := p.openaiService.Forward(
		ctx, req.GinContext, req.Account, req.RawBody,
	)
	if err != nil {
		return nil, err
	}
	return openaiResultToForwardResult(result), nil
}

// ShouldFailover returns true when the error is an UpstreamFailoverError,
// which signals the Pipeline to try the next account.
func (p *OpenAIProvider) ShouldFailover(
	_ context.Context, _ *ForwardRequest, err error,
) bool {
	return DefaultShouldFailover(err)
}

// openaiResultToForwardResult maps a service.OpenAIForwardResult to the
// protocol-agnostic gateway.ForwardResult.
func openaiResultToForwardResult(r *service.OpenAIForwardResult) *ForwardResult {
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
		ImageCount:          r.ImageCount,
		ImageSize:           r.ImageSize,
	}
}
