package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) applyJSBeforeRequest(c *gin.Context, body []byte, model, sourceFormat string) []byte {
	if h == nil {
		return body
	}
	return jshandlerRunner{js: h.jsHandler}.applyJSBeforeRequest(c, body, model, sourceFormat)
}

func (h *GatewayHandler) applyJSBeforeForward(c *gin.Context, body []byte, model, sourceFormat string, account *service.Account, mappedModel string) []byte {
	if h == nil {
		return body
	}
	return jshandlerRunner{js: h.jsHandler}.applyJSBeforeForward(c, body, model, sourceFormat, account, mappedModel)
}

func clientRequestIDFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if id, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(c.GetHeader("X-Request-Id"))
}