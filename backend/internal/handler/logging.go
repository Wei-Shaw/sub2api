package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestLogger(c *gin.Context, component string, fields ...zap.Field) *zap.Logger {
	base := logger.L()
	if c != nil && c.Request != nil {
		base = logger.FromContext(c.Request.Context())
REDACTED

	if component != "" {
		fields = append([]zap.Field{zap.String("component", component)REDACTED, fields...)
REDACTED
	return base.With(fields...)
REDACTED
