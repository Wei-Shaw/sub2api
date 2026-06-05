package handler

import (
	"github.com/gin-gonic/gin"
)

// ChatCompletions handles OpenAI Chat Completions API endpoint for Anthropic platform groups.
// POST /v1/chat/completions
// This converts Chat Completions requests to Anthropic format (via Responses format chain),
// forwards to Anthropic upstream, and converts responses back to Chat Completions format.
func (h *GatewayHandler) ChatCompletions(c *gin.Context) {
	h.chatCompletionsPipeline(c)
}

// chatCompletionsErrorResponse writes an error in OpenAI Chat Completions format.
func (h *GatewayHandler) chatCompletionsErrorResponse(c *gin.Context, status int, errType, message string) {
	h.chatCompletionsErrorResponseWithMetadata(c, status, errType, message, nil)
}

// chatCompletionsErrorResponseWithMetadata 带 metadata + reason 的 CC 格式错误响应
// （配额超限等场景要求前端能取到 metadata 和 reason 做 i18n 渲染）
func (h *GatewayHandler) chatCompletionsErrorResponseWithMetadata(c *gin.Context, status int, errType, message string, metadata map[string]string) {
	body := gin.H{
		"reason": errType,
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}
	c.JSON(status, body)
}
