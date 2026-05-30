package handler

import (
	"errors"
	"fmt"
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
	return classifyPipelineErrorWithContext(nil, "", err)
}

// classifyPipelineErrorWithContext is the context-aware variant that also
// consults error passthrough rules and applies upstream status code mapping
// (e.g. 401/403 → 502) matching the legacy handleFailoverExhausted behaviour.
func classifyPipelineErrorWithContext(c *gin.Context, platform string, err error) (status int, code, message string, metadata map[string]string) {
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

	// Content moderation blocked
	var contentBlocked *gateway.ContentBlockedError
	if errors.As(err, &contentBlocked) {
		return contentBlocked.StatusCode, "content_policy_violation", contentBlocked.Message, nil
	}

	// Check for upstream failover errors (may be wrapped)
	var failoverErr *service.UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		return classifyFailoverError(c, platform, failoverErr)
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

// classifyFailoverError handles UpstreamFailoverError with error passthrough
// rule consultation and upstream status code mapping (401/403 → 502).
// This matches the legacy handleFailoverExhausted + mapUpstreamError behaviour.
func classifyFailoverError(c *gin.Context, platform string, failoverErr *service.UpstreamFailoverError) (status int, code, message string, metadata map[string]string) {
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// Consult error passthrough rules (Bug 9)
	if c != nil && len(responseBody) > 0 {
		svc := service.GetBoundErrorPassthroughService(c)
		if svc != nil {
			if rule := svc.MatchRule(platform, statusCode, responseBody); rule != nil {
				respCode := statusCode
				if !rule.PassthroughCode && rule.ResponseCode != nil {
					respCode = *rule.ResponseCode
				}
				msg := service.ExtractUpstreamErrorMessage(responseBody)
				if !rule.PassthroughBody && rule.CustomMessage != nil {
					msg = *rule.CustomMessage
				}
				if rule.SkipMonitoring {
					c.Set(service.OpsSkipPassthroughKey, true)
				}
				return respCode, "upstream_error", msg, nil
			}
		}
	}

	// Record original upstream error for ops logging
	if c != nil && statusCode > 0 {
		upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
		service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")
	}

	// Map upstream status codes to client-facing codes (Bug 8)
	return mapPipelineUpstreamError(statusCode)
}

// mapPipelineUpstreamError maps upstream HTTP status codes to client-facing
// error responses, matching the legacy mapUpstreamError behaviour.
func mapPipelineUpstreamError(statusCode int) (status int, code, message string, metadata map[string]string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator", nil
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator", nil
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later", nil
	case 529:
		return http.StatusServiceUnavailable, "overloaded_error", "Upstream service overloaded, please retry later", nil
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable", nil
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed", nil
	}
}

// writeSSEErrorEvent writes an SSE error event to the client when streaming
// has already started (c.Writer.Size() > 0). This ensures the client receives
// a structured error instead of a silent stream termination.
// Reuses the existing streamingErrorEvent helper from gateway_handler.go.
func writeSSEErrorEvent(c *gin.Context, errType, message string, metadata map[string]string) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	errorEvent := streamingErrorEvent(errType, message, metadata)
	if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
		_ = c.Error(err)
	}
	flusher.Flush()
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


// fromOpenAIForwardResult converts a service.OpenAIForwardResult back to
// gateway.ForwardResult. Used by WS handlers that receive per-turn results
// in OpenAI format but need to pass them to the pipeline hooks.
func fromOpenAIForwardResult(r *service.OpenAIForwardResult) *gateway.ForwardResult {
	if r == nil {
		return nil
	}
	return &gateway.ForwardResult{
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
		ResponseHeaders:     r.ResponseHeaders,
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
