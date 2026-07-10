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
	scriptID     string
	session      *jshandler.StreamSession
	history      []string
	reqBody      []byte
	reqHdr       map[string]any
	respHdr      http.Header
	model        string
	protocol     string
	requestID    string
	writerHeader http.Header
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

func (s *OpenAIGatewayService) newOpenAIStreamJSState(ctx context.Context, c *gin.Context, account *Account, mappedModel, protocol string, upstreamResp http.Header) *openaiStreamJSState {
	scriptID := jshandlerScriptActive(ctx, s.jsHandler, account)
	if scriptID == "" {
		return nil
	}
	respHdr := http.Header{}
	if upstreamResp != nil {
		respHdr = upstreamResp.Clone()
	}
	var session *jshandler.StreamSession
	if s.jsHandler != nil {
		session = s.jsHandler.OpenStreamSession(ctx, scriptID)
	}
	return &openaiStreamJSState{
		scriptID:     scriptID,
		session:      session,
		reqBody:      openAIInboundRequestBody(c),
		reqHdr:       jshandlerHeaderToAnyMap(cloneGinRequestHeaders(c)),
		respHdr:      respHdr,
		model:        mappedModel,
		protocol:     protocol,
		requestID:    clientRequestIDFromGin(c),
		writerHeader: c.Writer.Header(),
	}
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
			hooked = js.ApplyStreamChunkHooks(ctx, st.scriptID, in)
		}
	} else {
		hooked = js.ApplyStreamChunkHooks(ctx, st.scriptID, in)
	}
	if len(hooked.ClearHeaders) > 0 || hooked.Headers != nil {
		applyJSHookHeadersToWriter(st.writerHeader, hooked.Headers, hooked.ClearHeaders)
	}
	if hooked.DropChunk {
		return "", false
	}
	st.history = appendJSStreamHistory(st.history, hooked.Chunk)
	return hooked.Chunk, true
}

func (s *OpenAIGatewayService) applyJSNonStreamOpenAI(ctx context.Context, c *gin.Context, account *Account, body []byte, model, protocol string, upstreamResp http.Header) jsNonStreamResponseResult {
	fallback := jsNonStreamResponseResult{body: body}
	scriptID := jshandlerScriptActive(ctx, s.jsHandler, account)
	if scriptID == "" {
		return fallback
	}
	respHeaders := http.Header{}
	if upstreamResp != nil {
		respHeaders = upstreamResp.Clone()
	}
	out := s.jsHandler.ApplyNonStreamResponseHooks(ctx, scriptID, jshandler.ResponseHookInput{
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