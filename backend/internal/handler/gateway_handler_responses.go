package handler

import (
	"github.com/gin-gonic/gin"
)

// Responses handles OpenAI Responses API endpoint for Anthropic platform groups.
// POST /v1/responses
// This converts Responses API requests to Anthropic format, forwards to Anthropic
// upstream, and converts responses back to Responses format.
func (h *GatewayHandler) Responses(c *gin.Context) {
	h.responsesPipeline(c)
}

// responsesErrorResponse writes an error in OpenAI Responses API format.
func (h *GatewayHandler) responsesErrorResponse(c *gin.Context, status int, code, message string) {
	h.responsesErrorResponseWithMetadata(c, status, code, message, nil)
}

// responsesErrorResponseWithMetadata 带 metadata + reason 的 Responses 格式错误响应
// （配额超限等场景要求前端能取到 metadata 和 reason 做 i18n 渲染）
func (h *GatewayHandler) responsesErrorResponseWithMetadata(c *gin.Context, status int, code, message string, metadata map[string]string) {
	body := gin.H{
		"reason": code,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}
	c.JSON(status, body)
}
