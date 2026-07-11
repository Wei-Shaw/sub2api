package service

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

// maxJSStreamHistoryChunks bounds history passed into stream hooks to avoid
// unbounded memory growth and O(n²) copies on long SSE responses.
const maxJSStreamHistoryChunks = 64

type openaiStreamJSState struct {
	scriptIDs    []string
	session      *jshandler.StreamSessionChain
	history      []string
	reqBody      []byte
	reqHdr       map[string]any
	respHdr      http.Header
	model        string
	protocol     string
	requestID    string
	writerHeader http.Header
	headerInited bool
}

func jsStreamHistorySnapshot(history []string) []string {
	if len(history) == 0 {
		return nil
	}
	return append([]string(nil), history...)
}

func appendJSStreamHistory(history []string, chunk string) []string {
	history = append(history, chunk)
	if len(history) <= maxJSStreamHistoryChunks {
		return history
	}
	// Drop oldest chunks; keep a contiguous recent window for the next hook call.
	overflow := len(history) - maxJSStreamHistoryChunks
	return append([]string(nil), history[overflow:]...)
}

func newOpenAIStreamJSState(js JSHandlerGateway, ctx context.Context, c *gin.Context, account *Account, mappedModel, protocol string, upstreamResp http.Header) *openaiStreamJSState {
	scriptIDs := jshandlerScriptsActive(ctx, js, account)
	if len(scriptIDs) == 0 {
		return nil
	}
	respHdr := http.Header{}
	if upstreamResp != nil {
		respHdr = upstreamResp.Clone()
	}
	var session *jshandler.StreamSessionChain
	if js != nil {
		session = js.OpenStreamSessionChain(ctx, scriptIDs)
	}
	st := &openaiStreamJSState{
		scriptIDs:    scriptIDs,
		session:      session,
		reqBody:      openAIInboundRequestBody(c),
		reqHdr:       jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c)),
		respHdr:      respHdr,
		model:        mappedModel,
		protocol:     protocol,
		requestID:    clientRequestIDFromGin(c),
		writerHeader: c.Writer.Header(),
	}
	st.runHeaderInit(ctx, js)
	return st
}

func (s *OpenAIGatewayService) newOpenAIStreamJSState(ctx context.Context, c *gin.Context, account *Account, mappedModel, protocol string, upstreamResp http.Header) *openaiStreamJSState {
	if s == nil {
		return nil
	}
	return newOpenAIStreamJSState(s.jsHandler, ctx, c, account, mappedModel, protocol, upstreamResp)
}

func openAIInboundRequestBody(c *gin.Context) []byte {
	if c == nil {
		return nil
	}
	if v, ok := c.Get("openai_forward_body"); ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	if parsed, ok := c.Get("parsed_request"); ok {
		if pr, ok := parsed.(*ParsedRequest); ok && pr != nil {
			return pr.Body.Bytes()
		}
	}
	return nil
}

func (st *openaiStreamJSState) runHeaderInit(ctx context.Context, js JSHandlerGateway) {
	if st == nil || st.headerInited || js == nil {
		return
	}
	st.headerInited = true
	in := jshandler.StreamChunkHookInput{
		Chunk:           "",
		HistoryChunks:   nil,
		RequestBody:     st.reqBody,
		RequestHeaders:  st.reqHdr,
		ResponseHeaders: st.respHdr.Clone(),
		Model:           st.model,
		Protocol:        st.protocol,
		RequestID:       st.requestID,
		HeaderInit:      true,
	}
	var hooked jshandler.StreamChunkHookOutput
	if st.session != nil {
		var err error
		hooked, err = st.session.ApplyChunk(in)
		if err != nil {
			hooked = js.ApplyStreamChunkHooksChain(ctx, st.scriptIDs, in)
		}
	} else {
		hooked = js.ApplyStreamChunkHooksChain(ctx, st.scriptIDs, in)
	}
	if len(hooked.ClearHeaders) > 0 || hooked.Headers != nil {
		applyJSHookHeadersToWriter(st.writerHeader, hooked.Headers, hooked.ClearHeaders)
		if hooked.Headers != nil {
			st.respHdr = hooked.Headers.Clone()
		}
	}
}

func (st *openaiStreamJSState) applyDataLine(ctx context.Context, js JSHandlerGateway, data string) (string, bool) {
	if st == nil || js == nil {
		return data, true
	}
	in := jshandler.StreamChunkHookInput{
		Chunk:           data,
		HistoryChunks:   jsStreamHistorySnapshot(st.history),
		RequestBody:     st.reqBody,
		RequestHeaders:  st.reqHdr,
		ResponseHeaders: st.respHdr.Clone(),
		Model:           st.model,
		Protocol:        st.protocol,
		RequestID:       st.requestID,
	}
	var hooked jshandler.StreamChunkHookOutput
	if st.session != nil {
		var err error
		hooked, err = st.session.ApplyChunk(in)
		if err != nil {
			hooked = js.ApplyStreamChunkHooksChain(ctx, st.scriptIDs, in)
		}
	} else {
		hooked = js.ApplyStreamChunkHooksChain(ctx, st.scriptIDs, in)
	}
	if len(hooked.ClearHeaders) > 0 || hooked.Headers != nil {
		applyJSHookHeadersToWriter(st.writerHeader, hooked.Headers, hooked.ClearHeaders)
		if hooked.Headers != nil {
			st.respHdr = hooked.Headers.Clone()
		}
	}
	if hooked.DropChunk {
		return "", false
	}
	st.history = appendJSStreamHistory(st.history, hooked.Chunk)
	return hooked.Chunk, true
}

func (s *OpenAIGatewayService) applyJSNonStreamOpenAI(ctx context.Context, c *gin.Context, account *Account, body []byte, model, protocol string, upstreamResp http.Header) jsNonStreamResponseResult {
	fallback := jsNonStreamResponseResult{body: body}
	scriptIDs := jshandlerScriptsActive(ctx, s.jsHandler, account)
	if len(scriptIDs) == 0 {
		return fallback
	}
	respHeaders := http.Header{}
	if upstreamResp != nil {
		respHeaders = upstreamResp.Clone()
	}
	out := s.jsHandler.ApplyNonStreamResponseHooksChain(ctx, scriptIDs, jshandler.ResponseHookInput{
		Body:            body,
		RequestBody:     openAIInboundRequestBody(c),
		RequestHeaders:  jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c)),
		ResponseHeaders: respHeaders,
		Model:           model,
		Protocol:        protocol,
		RequestID:       clientRequestIDFromGin(c),
	})
	return jsNonStreamResponseResult{
		body:         out.Body,
		headers:      out.Headers,
		clearHeaders: out.ClearHeaders,
	}
}

func SetOpenAIForwardBody(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	c.Set("openai_forward_body", append([]byte(nil), body...))
}

// applyOpenAIWSAccountAfterAuth runs account-bound on_after_auth_request on a WS
// response.create payload (after group on_before_request when both apply).
func (s *OpenAIGatewayService) applyOpenAIWSAccountAfterAuth(ctx context.Context, c *gin.Context, account *Account, body []byte, originalModel, mappedModel string) []byte {
	if s == nil || len(body) == 0 {
		return body
	}
	scriptIDs := jshandlerScriptsActive(ctx, s.jsHandler, account)
	if len(scriptIDs) == 0 {
		return body
	}
	out := s.jsHandler.ApplyRequestHooksChain(ctx, scriptIDs, "on_after_auth_request", jshandler.RequestHookInput{
		Body:            body,
		Headers:         cloneGinRequestHeaders(c),
		Model:           originalModel,
		SourceFormat:    "openai_responses",
		ToFormat:        "openai_responses",
		AccountPlatform: string(account.Platform),
		MappedModel:     mappedModel,
		RequestID:       clientRequestIDFromGin(c),
	})
	if len(out.Body) > 0 {
		body = out.Body
	}
	ApplyJSHookHeadersToGinRequest(c, out.Headers, out.ClearHeaders)
	return body
}
