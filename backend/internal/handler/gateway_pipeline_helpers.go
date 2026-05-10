package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// classifyPipelineError inspects a pipeline error and returns the
// appropriate HTTP status code, error type, message, and optional metadata
// for the error response. This is shared by CC and Responses pipeline
// handlers to avoid duplicating error classification logic.
func classifyPipelineError(err error) (status int, code, message string, metadata map[string]string) {
	if err == nil {
		return http.StatusInternalServerError, "api_error", "Unknown error", nil
	}

	msg := err.Error()

	// Pipeline wraps billing errors as "gateway: billing check: <inner>"
	// or "gateway: billing consume: <inner>". Unwrap to get the original
	// billing error and delegate to the existing billingErrorDetails.
	if strings.Contains(msg, "billing check:") || strings.Contains(msg, "billing consume:") {
		inner := errors.Unwrap(err)
		if inner != nil {
			return billingErrorDetails(inner)
		}
	}

	// Check for upstream failover errors (may be wrapped)
	var failoverErr *service.UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		statusCode := http.StatusBadGateway
		if failoverErr.StatusCode > 0 {
			statusCode = failoverErr.StatusCode
		}
		return statusCode, "server_error", "All available accounts exhausted", nil
	}

	// Pattern match on pipeline error prefixes
	switch {
	case strings.Contains(msg, "user wait queue full"):
		return http.StatusTooManyRequests, "rate_limit_error",
			"Too many pending requests, please retry later", nil
	case strings.Contains(msg, "acquire user slot"):
		return http.StatusTooManyRequests, "rate_limit_error",
			"Concurrency limit exceeded for user, please retry later", nil
	case strings.Contains(msg, "account slot"):
		return http.StatusTooManyRequests, "rate_limit_error",
			"Concurrency limit exceeded for account, please retry later", nil
	case strings.Contains(msg, "empty request body"):
		return http.StatusBadRequest, "invalid_request_error",
			"Request body is empty", nil
	case strings.Contains(msg, "read body"):
		return http.StatusBadRequest, "invalid_request_error",
			"Failed to read request body", nil
	case strings.Contains(msg, "parse request"):
		return http.StatusBadRequest, "invalid_request_error",
			"Failed to parse request body", nil
	case strings.Contains(msg, "select account"):
		return http.StatusServiceUnavailable, "api_error",
			"No available accounts: " + msg, nil
	case strings.Contains(msg, "failover limit reached"):
		return http.StatusBadGateway, "server_error",
			"All available accounts exhausted", nil
	case strings.Contains(msg, "forward failed after data written"),
		strings.Contains(msg, "forward failed after upstream accepted"):
		return http.StatusBadGateway, "server_error",
			"Upstream error after streaming started", nil
	case strings.Contains(msg, "no provider for platform"):
		return http.StatusServiceUnavailable, "api_error", msg, nil
	default:
		return http.StatusInternalServerError, "api_error", msg, nil
	}
}

// toServiceForwardResult converts a gateway.ForwardResult to
// service.ForwardResult for the RecordUsage call.
func toServiceForwardResult(r *gateway.ForwardResult) *service.ForwardResult {
	if r == nil {
		return nil
	}
	return &service.ForwardResult{
		RequestID:     r.RequestID,
		Model:         r.Model,
		UpstreamModel: r.UpstreamModel,
		Stream:        r.Stream,
		Duration:      r.Duration,
		FirstTokenMs:  r.FirstTokenMs,
		Usage: service.ClaudeUsage{
			InputTokens:              int(r.InputTokens),
			OutputTokens:             int(r.OutputTokens),
			CacheCreationInputTokens: int(r.CacheCreationTokens),
			CacheReadInputTokens:     int(r.CacheReadTokens),
			ImageOutputTokens:        int(r.ImageOutputTokens),
		},
		ClientDisconnect: r.ClientDisconnect,
		ImageCount:       r.ImageCount,
		ImageSize:        r.ImageSize,
	}
}

// toOpenAIForwardResult converts a gateway.ForwardResult to
// service.OpenAIForwardResult for the OpenAI RecordUsage call.
func toOpenAIForwardResult(r *gateway.ForwardResult) *service.OpenAIForwardResult {
	if r == nil {
		return nil
	}
	return &service.OpenAIForwardResult{
		RequestID:     r.RequestID,
		Model:         r.Model,
		UpstreamModel: r.UpstreamModel,
		Stream:        r.Stream,
		Duration:      r.Duration,
		FirstTokenMs:  r.FirstTokenMs,
		Usage: service.OpenAIUsage{
			InputTokens:              int(r.InputTokens),
			OutputTokens:             int(r.OutputTokens),
			CacheCreationInputTokens: int(r.CacheCreationTokens),
			CacheReadInputTokens:     int(r.CacheReadTokens),
			ImageOutputTokens:        int(r.ImageOutputTokens),
		},
		ImageCount: r.ImageCount,
		ImageSize:  r.ImageSize,
	}
}

// resolveChannelMappingFromContext resolves the channel mapping for usage
// recording. This is extracted as a shared helper because both CC and
// Responses pipeline record functions need the same logic.
func (h *GatewayHandler) resolveChannelMappingFromContext(
	c *gin.Context,
	apiKey *service.APIKey,
	reqModel string,
) service.ChannelMappingResult {
	if apiKey == nil {
		return service.ChannelMappingResult{}
	}
	mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(
		c.Request.Context(), apiKey.GroupID, apiKey.GroupPlatform(), reqModel,
	)
	return mapping
}
