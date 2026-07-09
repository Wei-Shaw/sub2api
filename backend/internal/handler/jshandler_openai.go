package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

type jshandlerRunner struct {
	js service.JSHandlerGateway
}

func (r jshandlerRunner) applyJSBeforeRequest(c *gin.Context, body []byte, model, sourceFormat string) []byte {
	if r.js == nil {
		return body
	}
	return runJSRequestHook(c, r.js, body, model, sourceFormat, "", "", "", "on_before_request")
}

func (r jshandlerRunner) applyJSBeforeForward(c *gin.Context, body []byte, model, sourceFormat string, account *service.Account, mappedModel string) []byte {
	toFormat := ""
	accountPlatform := ""
	if account != nil {
		accountPlatform = string(account.Platform)
		toFormat = accountPlatform
	}
	return runJSRequestHook(c, r.js, body, model, sourceFormat, toFormat, accountPlatform, mappedModel, "on_after_auth_request")
}

func runJSRequestHook(c *gin.Context, js service.JSHandlerGateway, body []byte, model, sourceFormat, toFormat, accountPlatform, mappedModel, hookName string) []byte {
	if js == nil {
		return body
	}
	ctx := c.Request.Context()
	if !js.Enabled(ctx) {
		return body
	}
	headers := c.Request.Header.Clone()
	out := js.ApplyRequestHooks(ctx, hookName, jshandler.RequestHookInput{
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