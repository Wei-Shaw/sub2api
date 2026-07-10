package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

type jshandlerRunner struct {
	js service.JSHandlerGateway
}

func (r jshandlerRunner) applyJSBeforeForward(c *gin.Context, body []byte, model, sourceFormat string, account *service.Account, mappedModel string) []byte {
	toFormat := inferJSToFormat(c, sourceFormat, account)
	accountPlatform := ""
	if account != nil {
		accountPlatform = string(account.Platform)
	}
	return runJSRequestHook(c, r.js, account, body, model, sourceFormat, toFormat, accountPlatform, mappedModel, "on_after_auth_request")
}

func runJSRequestHook(c *gin.Context, js service.JSHandlerGateway, account *service.Account, body []byte, model, sourceFormat, toFormat, accountPlatform, mappedModel, hookName string) []byte {
	if js == nil {
		return body
	}
	ctx := c.Request.Context()
	scriptIDs := service.JShandlerScriptIDsFromAccount(account)
	if len(scriptIDs) == 0 || !js.Enabled(ctx) {
		return body
	}
	headers := c.Request.Header.Clone()
	out := js.ApplyRequestHooksChain(ctx, scriptIDs, hookName, jshandler.RequestHookInput{
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