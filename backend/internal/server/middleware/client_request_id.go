package middleware

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const clientRequestIDHeader = "X-Client-Request-ID"

// ClientRequestID ensures every request has a unique client_request_id in request.Context().
//
// This is used by the Ops monitoring module for end-to-end request correlation.
// It also gives every gateway request the PromptAuditCharge that carries
// prompt-audit fees to the request's usage billing, and bills the fees left
// over when the request ends.
func ClientRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
			var valid bool
			v, valid = normalizeCorrelationID(v)
			if !valid {
				v = uuid.New().String()
			}
			c.Header(clientRequestIDHeader, v)
			ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, v)
			charge := service.NewPromptAuditCharge()
			ctx = service.WithPromptAuditCharge(ctx, charge)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			finishPromptAuditCharge(ctx, charge)
			return
		}

		id := uuid.New().String()
		c.Header(clientRequestIDHeader, id)
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, id)
		requestLogger := logger.FromContext(ctx).With(zap.String("client_request_id", strings.TrimSpace(id)))
		ctx = logger.IntoContext(ctx, requestLogger)
		charge := service.NewPromptAuditCharge()
		ctx = service.WithPromptAuditCharge(ctx, charge)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		finishPromptAuditCharge(ctx, charge)
	}
}

// finishPromptAuditCharge bills the audit fees that no usage record took, so an
// audited request is billed even when its upstream call failed.
func finishPromptAuditCharge(ctx context.Context, charge *service.PromptAuditCharge) {
	if cost, err := charge.Finish(ctx); err != nil {
		logger.FromContext(ctx).Error("security_audit.unbilled_fee_charge_failed", zap.Float64("cost", cost), zap.Error(err))
	}
}
