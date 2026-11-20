package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AbortWithRequestBodyReadFailure writes the shared response for a failed body
// read in middleware that inspects the body before the handler. It applies the
// same classification, client message, protocol-shaped error body and
// payload-free diagnostics as the gateway handlers, so a body read in
// middleware fails exactly as it would in the handler.
func AbortWithRequestBodyReadFailure(c *gin.Context, component string, err error) {
	failure := httputil.DescribeRequestBodyReadFailure(c.Request, err)
	if failure.Kind != httputil.RequestBodyReadErrorMaxBytes {
		logRequestBodyReadFailure(c, component, failure)
	}
	if isDeepseekAnthropicFilesUpload(c) {
		// Anthropic-format Files clients expect the Anthropic error envelope,
		// including request_too_large for an oversized upload.
		errType := "invalid_request_error"
		if failure.Status == http.StatusRequestEntityTooLarge {
			errType = "request_too_large"
		}
		c.AbortWithStatusJSON(failure.Status, gin.H{
			"type":  "error",
			"error": gin.H{"type": errType, "message": failure.ClientMessage()},
		})
		return
	}
	writeGatewayInvalidRequest(c, failure.Status, failure.ClientMessage())
	c.Abort()
}

func isDeepseekAnthropicFilesUpload(c *gin.Context) bool {
	return c.Request != nil && c.Request.URL != nil &&
		c.Request.URL.Path == "/v1/files" && service.IsDeepseekFilesAnthropicRequest(c)
}

func logRequestBodyReadFailure(c *gin.Context, component string, failure httputil.RequestBodyReadFailure) {
	fields := append([]zap.Field{zap.String("component", component)}, failure.LogFields()...)
	logger.FromContext(c.Request.Context()).Warn("read request body failed", fields...)
}
