package middleware

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/gin-gonic/gin"
)

// ResponsesCompactionBodyLimit applies an explicitly configured upload threshold
// before model routing reads the body. It excludes WebSocket upgrades and compact
// endpoints so clients can send their larger history to an explicit compaction API.
func ResponsesCompactionBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		switch c.FullPath() {
		case "/v1/responses", "/responses", "/backend-api/codex/responses":
		default:
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			abortResponsesBodyLimit(c)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			var limitErr *http.MaxBytesError
			if errors.As(err, &limitErr) && limitErr.Limit == maxBytes {
				abortResponsesBodyLimit(c)
			} else {
				status := http.StatusBadRequest
				if errors.As(err, &limitErr) {
					status = http.StatusRequestEntityTooLarge
				}
				c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
					"type": "invalid_request_error", "message": "Failed to read request body",
				}})
			}
			return
		}
		requestmodel.ResetRequestBody(c.Request, body)
		c.Next()
	}
}

func abortResponsesBodyLimit(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{
		"type":    "invalid_request_error",
		"code":    "context_length_exceeded",
		"message": "Request body exceeds the configured gateway limit. Compact your session or reduce the input size.",
	}})
}
