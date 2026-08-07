package middleware

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const clientRequestIDHeader = "X-Client-Request-ID"

// ClientRequestID ensures every request has a unique client_request_id in request.Context().
//
// This is used by the Ops monitoring module for end-to-end request correlation.
func ClientRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		id, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		id, valid := normalizeCorrelationID(id)
		if !valid {
			id, valid = normalizeCorrelationID(c.GetHeader(clientRequestIDHeader))
		}
		if !valid {
			id = uuid.New().String()
		}
		c.Header(clientRequestIDHeader, id)
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, id)
		requestLogger := logger.FromContext(ctx).With(zap.String("client_request_id", strings.TrimSpace(id)))
		ctx = logger.IntoContext(ctx, requestLogger)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
