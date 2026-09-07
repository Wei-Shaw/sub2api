package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/enginetrace"
	"github.com/gin-gonic/gin"
)

func newOpenAIEngineTrace(c *gin.Context, account *Account, requestedModel, upstreamModel, transport string) *enginetrace.Trace {
	if c == nil || account == nil || account.Platform != PlatformOpenAI {
		return nil
	}
	requestID := ""
	if c.Request != nil {
		requestID, _ = c.Request.Context().Value(ctxkey.RequestID).(string)
	}
	return enginetrace.New(requestID, account.ID, requestedModel, upstreamModel, transport)
}
