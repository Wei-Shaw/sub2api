package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
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

// applyJSBeforeAccountSelection runs group-bound on_before_request hooks once
// before account selection. to_format / account_platform / mapped_model are empty.
func applyJSBeforeAccountSelection(c *gin.Context, js service.JSHandlerGateway, apiKey *service.APIKey, body []byte, model, sourceFormat string) []byte {
	if js == nil || apiKey == nil {
		return body
	}
	ctx := c.Request.Context()
	scriptIDs := service.JShandlerScriptIDsFromAPIKeyGroup(apiKey)
	if len(scriptIDs) == 0 || !js.Enabled(ctx) {
		return body
	}
	headers := c.Request.Header.Clone()
	out := js.ApplyRequestHooksChain(ctx, scriptIDs, "on_before_request", jshandler.RequestHookInput{
		Body:         body,
		Headers:      headers,
		Model:        model,
		SourceFormat: sourceFormat,
		RequestID:    clientRequestIDFromContext(c),
	})
	service.ApplyJSHookHeadersToGinRequest(c, out.Headers, out.ClearHeaders)
	return out.Body
}

func (r jshandlerRunner) applyJSBeforeAccountSelection(c *gin.Context, apiKey *service.APIKey, body []byte, model, sourceFormat string) []byte {
	return applyJSBeforeAccountSelection(c, r.js, apiKey, body, model, sourceFormat)
}

func (h *GatewayHandler) applyJSBeforeAccountSelection(c *gin.Context, apiKey *service.APIKey, body []byte, model, sourceFormat string) []byte {
	if h == nil {
		return body
	}
	return applyJSBeforeAccountSelection(c, h.jsHandler, apiKey, body, model, sourceFormat)
}

// modelFromJSONBody extracts model after on_before_request rewrites.
// Falls back to previous model when the field is missing or empty.
func modelFromJSONBody(body []byte, fallback string) string {
	m := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if m == "" {
		return fallback
	}
	return m
}

// streamFromJSONBody extracts stream after on_before_request rewrites.
// Missing field or non-boolean type keeps fallback (fail-open, same spirit as JS errors).
func streamFromJSONBody(body []byte, fallback bool) bool {
	streamResult := gjson.GetBytes(body, "stream")
	if !streamResult.Exists() {
		return fallback
	}
	if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		return fallback
	}
	return streamResult.Bool()
}
