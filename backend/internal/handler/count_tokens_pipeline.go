package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// countTokensPipeline handles the /v1/messages/count_tokens endpoint
// through the GatewayPipeline. CountTokens does not need concurrency
// tracking or usage recording, but still needs billing eligibility
// check and account selection via the standard pipeline lifecycle.
func (h *GatewayHandler) countTokensPipeline(c *gin.Context) {
	if h.pipeline == nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Gateway pipeline not initialized")
		return
	}

	// Pre-pipeline: parse body for Claude Code detection and thinking context
	body, parsedReq, err := h.readAndParseMessagesBody(c)
	if err != nil {
		return // error already written
	}
	SetClaudeCodeClientContext(c, body, parsedReq)
	c.Request = c.Request.WithContext(
		service.WithThinkingEnabled(c.Request.Context(), parsedReq.ThinkingEnabled, h.metadataBridgeEnabled()),
	)

	parse := h.buildCountTokensParseFunc(c, body, parsedReq)
	err = h.pipeline.Execute(c, gateway.ProtocolCountTokens, "", parse, nil)
	if err != nil {
		h.handleCountTokensPipelineError(c, err)
	}
}

// buildCountTokensParseFunc builds the ParseRequestFunc for count_tokens.
// The body and parsed request are pre-computed so the pipeline does not
// re-read or re-parse them.
func (h *GatewayHandler) buildCountTokensParseFunc(
	c *gin.Context,
	body []byte,
	parsed *service.ParsedRequest,
) gateway.ParseRequestFunc {
	return func(_ []byte) (*gateway.ForwardRequest, error) {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)

		parsed.GroupID = nil
		if apiKey != nil {
			parsed.GroupID = apiKey.GroupID
		}
		parsed.SessionContext = &service.SessionContext{
			ClientIP:  ip.GetClientIP(c),
			UserAgent: c.GetHeader("User-Agent"),
		}
		if apiKey != nil {
			parsed.SessionContext.APIKeyID = apiKey.ID
		}
		sessionHash := h.gatewayService.GenerateSessionHash(parsed)

		return &gateway.ForwardRequest{
			Model:       parsed.Model,
			Stream:      false,
			RawBody:     body,
			GinContext:  c,
			SessionHash: sessionHash,
		}, nil
	}
}

// handleCountTokensPipelineError translates pipeline errors into
// Anthropic Messages error responses for count_tokens.
func (h *GatewayHandler) handleCountTokensPipelineError(c *gin.Context, err error) {
	if c.Writer.Size() > 0 {
		return // bytes already written to client, cannot send error
	}

	status, code, message, metadata := classifyPipelineError(err)
	if len(metadata) > 0 {
		h.errorResponseWithMetadata(c, status, code, message, metadata)
	} else {
		h.errorResponse(c, status, code, message)
	}
}
