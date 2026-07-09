package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) applyJSBeforeRequest(c *gin.Context, body []byte, model, sourceFormat string) []byte {
	return h.runJSRequestHook(c, body, model, sourceFormat, "", "", "", "on_before_request")
}

func (h *GatewayHandler) applyJSBeforeForward(c *gin.Context, body []byte, model, sourceFormat string, account *service.Account, mappedModel string) []byte {
	toFormat := ""
	accountPlatform := ""
	if account != nil {
		accountPlatform = string(account.Platform)
		toFormat = accountPlatform
	}
	return h.runJSRequestHook(c, body, model, sourceFormat, toFormat, accountPlatform, mappedModel, "on_after_auth_request")
}

func (h *GatewayHandler) runJSRequestHook(c *gin.Context, body []byte, model, sourceFormat, toFormat, accountPlatform, mappedModel, hookName string) []byte {
	if h == nil || h.jsHandler == nil {
		return body
	}
	ctx := c.Request.Context()
	if !h.jsHandler.Enabled(ctx) {
		return body
	}
	headers := c.Request.Header.Clone()
	out := h.jsHandler.ApplyRequestHooks(ctx, hookName, jshandler.RequestHookInput{
		Body:            body,
		Headers:         headers,
		Model:           model,
		SourceFormat:    sourceFormat,
		ToFormat:        toFormat,
		AccountPlatform: accountPlatform,
		MappedModel:     mappedModel,
		RequestID:       clientRequestIDFromContext(c),
	})
	service.ApplyJSHookHeadersToGinRequest(c, out.Headers, out.ClearHeaders)
	return out.Body
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