package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// The caller must not have committed a downstream response. Reuse the bounded
// failover loop, but do not cool down an account for a generation/schema failure.
func antigravityMalformedFunctionCallError() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":{"type":"api_error","message":"upstream returned MALFORMED_FUNCTION_CALL"}}`),
		RetryableOnSameAccount: true,
		SameAccountRetryMax:    1,
		RequestScopedTransient: true,
	}
}

// Log metadata with the request context, never the upstream content or signature.
func recordAntigravityMalformedFunctionCall(c *gin.Context, model string) {
	setOpsUpstreamError(c, http.StatusOK, antigravity.ErrMalformedFunctionCall.Error(), "")
	if c.Writer.Written() {
		MarkOpsStreamError(c, "upstream_error", antigravity.ErrMalformedFunctionCall.Error(), http.StatusBadGateway)
	}
	logger.FromContext(c.Request.Context()).Warn("antigravity.malformed_function_call",
		zap.String("finish_reason", "MALFORMED_FUNCTION_CALL"),
		zap.String("model", model),
		zap.Int("upstream_status", http.StatusOK),
		zap.Bool("response_committed", c.Writer.Written()),
	)
}
