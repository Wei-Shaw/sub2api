package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

func newGatewayCompatStreamJSState(js JSHandlerGateway, ctx context.Context, c *gin.Context, account *Account, mappedModel, protocol string, upstreamResp http.Header) *openaiStreamJSState {
	scriptID := jshandlerScriptActive(ctx, js, account)
	if scriptID == "" {
		return nil
	}
	respHdr := http.Header{}
	if upstreamResp != nil {
		respHdr = upstreamResp.Clone()
	}
	var session *jshandler.StreamSession
	if js != nil {
		session = js.OpenStreamSession(ctx, scriptID)
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

func (s *GatewayService) newGatewayCompatStreamJSState(ctx context.Context, c *gin.Context, account *Account, mappedModel, protocol string, upstreamResp http.Header) *openaiStreamJSState {
	if s == nil {
		return nil
	}
	return newGatewayCompatStreamJSState(s.jsHandler, ctx, c, account, mappedModel, protocol, upstreamResp)
}

func applyJSNonStreamOpenAICompat(js JSHandlerGateway, ctx context.Context, c *gin.Context, account *Account, body []byte, model, protocol string, upstreamResp http.Header) jsNonStreamResponseResult {
	fallback := jsNonStreamResponseResult{body: body}
	scriptID := jshandlerScriptActive(ctx, js, account)
	if scriptID == "" {
		return fallback
	}
	respHeaders := http.Header{}
	if upstreamResp != nil {
		respHeaders = upstreamResp.Clone()
	}
	out := js.ApplyNonStreamResponseHooks(ctx, scriptID, jshandler.ResponseHookInput{
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

func (s *GatewayService) applyJSNonStreamOpenAICompat(ctx context.Context, c *gin.Context, account *Account, body []byte, model, protocol string, upstreamResp http.Header) jsNonStreamResponseResult {
	if s == nil {
		return jsNonStreamResponseResult{body: body}
	}
	return applyJSNonStreamOpenAICompat(s.jsHandler, ctx, c, account, body, model, protocol, upstreamResp)
}

func applyOpenAICompatSSEDataHooks(ctx context.Context, js JSHandlerGateway, st *openaiStreamJSState, sse string) string {
	if st == nil || js == nil || sse == "" {
		return sse
	}
	blocks := strings.Split(sse, "\n\n")
	for i, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		for j, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			hooked, keep := st.applyDataLine(ctx, js, data)
			if !keep {
				blocks[i] = ""
				break
			}
			lines[j] = "data: " + hooked
		}
		if blocks[i] != "" {
			blocks[i] = strings.Join(lines, "\n") + "\n\n"
		}
	}
	var b strings.Builder
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		b.WriteString(block)
		if !strings.HasSuffix(block, "\n\n") {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func emitOpenAIChatCompletionJSON(js JSHandlerGateway, c *gin.Context, account *Account, chatResp *apicompat.ChatCompletionsResponse, mappedModel string, upstream http.Header) {
	if chatResp == nil {
		return
	}
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	respBytes, err := json.Marshal(chatResp)
	if err != nil {
		c.JSON(http.StatusOK, chatResp)
		return
	}
	respBytes = reverseToolNamesIfPresent(c, respBytes)
	jsResult := applyJSNonStreamOpenAICompat(js, c.Request.Context(), c, account, respBytes, mappedModel, "openai_chat", upstream)
	applyJSHookHeadersToWriter(c.Writer.Header(), jsResult.headers, jsResult.clearHeaders)
	c.Data(http.StatusOK, "application/json; charset=utf-8", jsResult.body)
}

func (s *GatewayService) emitOpenAIChatCompletionJSON(c *gin.Context, account *Account, chatResp *apicompat.ChatCompletionsResponse, mappedModel string, upstream http.Header) {
	emitOpenAIChatCompletionJSON(s.jsHandler, c, account, chatResp, mappedModel, upstream)
}

func emitOpenAIResponsesJSON(js JSHandlerGateway, c *gin.Context, account *Account, responsesResp *apicompat.ResponsesResponse, mappedModel string, upstream http.Header) {
	if responsesResp == nil {
		return
	}
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	respBytes, err := json.Marshal(responsesResp)
	if err != nil {
		c.JSON(http.StatusOK, responsesResp)
		return
	}
	respBytes = reverseToolNamesIfPresent(c, respBytes)
	jsResult := applyJSNonStreamOpenAICompat(js, c.Request.Context(), c, account, respBytes, mappedModel, "openai_responses", upstream)
	applyJSHookHeadersToWriter(c.Writer.Header(), jsResult.headers, jsResult.clearHeaders)
	c.Data(http.StatusOK, "application/json; charset=utf-8", jsResult.body)
}

func (s *GatewayService) emitOpenAIResponsesJSON(c *gin.Context, account *Account, responsesResp *apicompat.ResponsesResponse, mappedModel string, upstream http.Header) {
	emitOpenAIResponsesJSON(s.jsHandler, c, account, responsesResp, mappedModel, upstream)
}