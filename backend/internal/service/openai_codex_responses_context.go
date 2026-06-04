package service

import (
	"context"

	"github.com/gin-gonic/gin"
)

type codexResponsesStreamCompatContextKey struct{}

const codexResponsesStreamCompatGinKey = "codex_responses_stream_compat"

// WithCodexResponsesStreamCompat enables Codex CLI stream compatibility for a request.
func WithCodexResponsesStreamCompat(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, codexResponsesStreamCompatContextKey{}, true)
}

func codexResponsesStreamCompatEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(codexResponsesStreamCompatContextKey{}).(bool)
	return enabled
}

// MarkCodexResponsesStreamCompat enables Codex stream compatibility on Gin context.
func MarkCodexResponsesStreamCompat(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(codexResponsesStreamCompatGinKey, true)
}

func codexResponsesStreamCompatEnabledForGin(c *gin.Context) bool {
	if c == nil {
		return false
	}
	enabled, _ := c.Get(codexResponsesStreamCompatGinKey)
	v, _ := enabled.(bool)
	return v
}
