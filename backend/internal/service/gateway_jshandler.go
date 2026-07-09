package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

// JSHandlerGateway applies configurable JavaScript hooks on gateway traffic.
type JSHandlerGateway interface {
	Enabled(ctx context.Context) bool
	ApplyRequestHooks(ctx context.Context, hookName string, in jshandler.RequestHookInput) jshandler.RequestHookOutput
	ApplyNonStreamResponseHooks(ctx context.Context, in jshandler.ResponseHookInput) jshandler.ResponseHookOutput
}

func cloneGinRequestHeaders(c *gin.Context) http.Header {
	if c == nil || c.Request == nil {
		return nil
	}
	return c.Request.Header.Clone()
}

func applyJSRequestToBody(ctx context.Context, js JSHandlerGateway, hookName string, body []byte, headers http.Header, model, sourceFormat, toFormat, accountPlatform, mappedModel, requestID string) ([]byte, http.Header) {
	if js == nil || !js.Enabled(ctx) {
		return body, headers
	}
	out := js.ApplyRequestHooks(ctx, hookName, jshandler.RequestHookInput{
		Body:            body,
		Headers:         headers,
		Model:           model,
		SourceFormat:    sourceFormat,
		ToFormat:        toFormat,
		AccountPlatform: accountPlatform,
		MappedModel:     mappedModel,
		RequestID:       requestID,
	})
	return out.Body, out.Headers
}

func (s *GatewayService) applyJSNonStreamResponse(ctx context.Context, c *gin.Context, body, reqBody []byte, model string) []byte {
	if s == nil || s.jsHandler == nil || !s.jsHandler.Enabled(ctx) {
		return body
	}
	reqHeaders := jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c))
	out := s.jsHandler.ApplyNonStreamResponseHooks(ctx, jshandler.ResponseHookInput{
		Body:            body,
		RequestBody:     reqBody,
		RequestHeaders:  reqHeaders,
		ResponseHeaders: nil,
		Model:           model,
		Protocol:        "anthropic_messages",
		RequestID:       clientRequestIDFromGin(c),
	})
	return out.Body
}

func clientRequestIDFromGin(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if id, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(c.GetHeader("X-Request-Id"))
}

func jshandlerHeaderToAnyMap(h http.Header) map[string]any {
	m := make(map[string]any)
	if h == nil {
		return m
	}
	for k, v := range h {
		switch len(v) {
		case 0:
			continue
		case 1:
			m[k] = v[0]
		default:
			m[k] = append([]string(nil), v...)
		}
	}
	return m
}

// ApplyJSHookHeadersToGinRequest merges hook-modified headers into the inbound gin request.
func ApplyJSHookHeadersToGinRequest(c *gin.Context, headers http.Header, clearHeaders []string) {
	if c == nil || c.Request == nil {
		return
	}
	for _, k := range clearHeaders {
		c.Request.Header.Del(k)
	}
	if headers == nil {
		return
	}
	for k, vals := range headers {
		c.Request.Header.Del(k)
		for _, v := range vals {
			c.Request.Header.Add(k, v)
		}
	}
}