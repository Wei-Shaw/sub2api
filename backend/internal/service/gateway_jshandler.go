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

type jsNonStreamResponseResult struct {
	body         []byte
	headers      http.Header
	clearHeaders []string
}

func (s *GatewayService) applyJSNonStreamResponse(ctx context.Context, c *gin.Context, body, reqBody []byte, model string, upstreamResp http.Header) jsNonStreamResponseResult {
	fallback := jsNonStreamResponseResult{body: body}
	if s == nil || s.jsHandler == nil || !s.jsHandler.Enabled(ctx) {
		return fallback
	}
	respHeaders := http.Header{}
	if upstreamResp != nil {
		respHeaders = upstreamResp.Clone()
	}
	reqHeaders := jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c))
	out := s.jsHandler.ApplyNonStreamResponseHooks(ctx, jshandler.ResponseHookInput{
		Body:            body,
		RequestBody:     reqBody,
		RequestHeaders:  reqHeaders,
		ResponseHeaders: respHeaders,
		Model:           model,
		Protocol:        "anthropic_messages",
		RequestID:       clientRequestIDFromGin(c),
	})
	return jsNonStreamResponseResult{
		body:         out.Body,
		headers:      out.Headers,
		clearHeaders: out.ClearHeaders,
	}
}

func applyJSHookHeadersToWriter(dst http.Header, hookHeaders http.Header, clearHeaders []string) {
	if dst == nil {
		return
	}
	for _, k := range clearHeaders {
		dst.Del(k)
	}
	if hookHeaders == nil {
		return
	}
	for k, vals := range hookHeaders {
		dst.Del(k)
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
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