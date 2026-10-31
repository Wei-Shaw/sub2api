package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AbortWithRequestBodyReadFailure writes the shared response for a failed body
// read in middleware that inspects the body before the handler. It applies the
// same classification and payload-free diagnostics as the gateway handlers.
func AbortWithRequestBodyReadFailure(c *gin.Context, component string, err error) {
	failure := httputil.DescribeRequestBodyReadFailure(c.Request, err)
	if failure.Kind != httputil.RequestBodyReadErrorMaxBytes {
		fields := append([]zap.Field{zap.String("component", component)}, failure.LogFields()...)
		logger.FromContext(c.Request.Context()).Warn("read request body failed", fields...)
	}
	c.AbortWithStatusJSON(failure.Status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": failure.Message}})
}
