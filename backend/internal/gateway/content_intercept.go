package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ContentInterceptor inspects request content before forwarding.
// Implementations call external moderation services (via plugin gRPC)
// and return a decision that may block the request.
type ContentInterceptor interface {
	// Check evaluates the request content. Returns nil when the request
	// is allowed (or when interception is disabled/unavailable).
	// A non-nil result with Blocked=true causes the pipeline to abort.
	Check(ctx context.Context, c *gin.Context, req *ForwardRequest) (*ContentCheckResult, error)
}

// ContentCheckResult is the outcome of a content interception check.
type ContentCheckResult struct {
	Blocked         bool
	StatusCode      int
	Message         string
	HighestCategory string
	HighestScore    float64
}

// ContentBlockedError is returned by the pipeline when content interception
// blocks a request. Handlers use it to format protocol-appropriate errors.
type ContentBlockedError struct {
	StatusCode int
	Message    string
	Category   string
}

func (e *ContentBlockedError) Error() string {
	return fmt.Sprintf("content blocked: %s (category=%s)", e.Message, e.Category)
}

func (p *GatewayPipeline) checkContentIntercept(c *gin.Context, req *ForwardRequest) error {
	if p.contentInterceptor == nil {
		return nil
	}
	result, err := p.contentInterceptor.Check(c.Request.Context(), c, req)
	if err != nil {
		return nil
	}
	return contentCheckResultToError(result)
}

// CheckContentForPayload runs the content interceptor against a specific
// request body (e.g. a subsequent WebSocket turn's payload) rather than
// req.RawBody. Turn-invariant fields (Model, Protocol, auth context) are reused
// from req; only the body differs per turn. It returns a *ContentBlockedError
// when the content is blocked, or nil otherwise. RawBody is restored before
// returning so req remains consistent for the caller.
func (p *GatewayPipeline) CheckContentForPayload(c *gin.Context, req *ForwardRequest, payload []byte) error {
	if p.contentInterceptor == nil || req == nil {
		return nil
	}
	originalBody := req.RawBody
	req.RawBody = payload
	defer func() { req.RawBody = originalBody }()

	result, err := p.contentInterceptor.Check(c.Request.Context(), c, req)
	if err != nil {
		return nil
	}
	return contentCheckResultToError(result)
}

// contentCheckResultToError converts an interceptor result into a
// *ContentBlockedError when the content is blocked, or nil when allowed.
func contentCheckResultToError(result *ContentCheckResult) error {
	if result == nil || !result.Blocked {
		return nil
	}
	status := result.StatusCode
	if status == 0 {
		status = http.StatusForbidden
	}
	return &ContentBlockedError{
		StatusCode: status,
		Message:    result.Message,
		Category:   result.HighestCategory,
	}
}
