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
	ApplyStreamChunkHooks(ctx context.Context, in jshandler.StreamChunkHookInput) jshandler.StreamChunkHookOutput
}

type gatewayStreamJSState struct {
	history      []string
	reqBody      []byte
	reqHdr       map[string]any
	respHdr      http.Header
	model        string
	protocol     string
	requestID    string
	writerHeader http.Header
}

func (s *GatewayService) newGatewayStreamJSState(ctx context.Context, c *gin.Context, mappedModel string, upstreamResp http.Header) *gatewayStreamJSState {
	if s == nil || s.jsHandler == nil || !s.jsHandler.Enabled(ctx) {
		return nil
	}
	reqBody := []byte(nil)
	if parsed, ok := c.Get("parsed_request"); ok {
		if pr, okParsed := parsed.(*ParsedRequest); okParsed && pr != nil {
			reqBody = pr.Body.Bytes()
		}
	}
	respHdr := http.Header{}
	if upstreamResp != nil {
		respHdr = upstreamResp.Clone()
	}
	return &gatewayStreamJSState{
		reqBody:      reqBody,
		reqHdr:       jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c)),
		respHdr:      respHdr,
		model:        mappedModel,
		protocol:     "anthropic_messages",
		requestID:    clientRequestIDFromGin(c),
		writerHeader: c.Writer.Header(),
	}
}

func (st *gatewayStreamJSState) transformSSEBlocks(ctx context.Context, js JSHandlerGateway, eventName string, blocks []string) []string {
	if st == nil || js == nil || len(blocks) == 0 {
		return blocks
	}
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		chunk, evName, ok := anthropicSSEBlockDataPayload(block)
		if !ok {
			out = append(out, block)
			continue
		}
		if evName == "" {
			evName = eventName
		}
		hooked := js.ApplyStreamChunkHooks(ctx, jshandler.StreamChunkHookInput{
			Chunk:           chunk,
			HistoryChunks:   append([]string(nil), st.history...),
			RequestBody:     st.reqBody,
			RequestHeaders:  st.reqHdr,
			ResponseHeaders: st.respHdr.Clone(),
			Model:           st.model,
			Protocol:        st.protocol,
			RequestID:       st.requestID,
		})
		if len(hooked.ClearHeaders) > 0 || hooked.Headers != nil {
			applyJSHookHeadersToWriter(st.writerHeader, hooked.Headers, hooked.ClearHeaders)
		}
		if hooked.DropChunk {
			continue
		}
		newBlock := rebuildAnthropicSSEBlock(evName, hooked.Chunk)
		out = append(out, newBlock)
		st.history = append(st.history, hooked.Chunk)
	}
	return out
}

func anthropicSSEBlockDataPayload(block string) (data string, eventName string, ok bool) {
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			ok = true
		}
	}
	return data, eventName, ok
}

func rebuildAnthropicSSEBlock(eventName, data string) string {
	var b strings.Builder
	if eventName != "" {
		b.WriteString("event: ")
		b.WriteString(eventName)
		b.WriteString("\n")
	}
	b.WriteString("data: ")
	b.WriteString(data)
	b.WriteString("\n\n")
	return b.String()
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