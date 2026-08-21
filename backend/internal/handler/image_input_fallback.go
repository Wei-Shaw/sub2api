package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// applyOpenAIImageInputFallbackBeforeUpstream 在请求调度到上游之前执行多模态模拟
// （图片输入降级主动处理）：命中主动处理模型名单的请求体在此被改写
// （strip=删除图片 / describe=用视觉模型描述替换），避免上游返回
// unknown variant `image_url`。未命中或改写失败时原样返回，不影响转发。
func (h *OpenAIGatewayHandler) applyOpenAIImageInputFallbackBeforeUpstream(c *gin.Context, body []byte) []byte {
	if h == nil || h.gatewayService == nil || len(body) == 0 {
		return body
	}
	newBody, reason, changed, err := h.gatewayService.ApplyImageInputFallbackBeforeUpstream(c.Request.Context(), body)
	if err != nil || !changed {
		return body
	}
	logger.LegacyPrintf("handler.openai_gateway", "[OpenAI] %s for model %s before upstream", reason, gjson.GetBytes(newBody, "model").String())
	return newBody
}

func (h *GatewayHandler) applyOpenAIImageInputFallbackBeforeUpstream(c *gin.Context, body []byte) []byte {
	if h == nil || h.openAIGatewayService == nil || len(body) == 0 {
		return body
	}
	newBody, reason, changed, err := h.openAIGatewayService.ApplyImageInputFallbackBeforeUpstream(c.Request.Context(), body)
	if err != nil || !changed {
		return body
	}
	logger.LegacyPrintf("handler.gateway", "[OpenAI] %s for model %s before upstream", reason, gjson.GetBytes(newBody, "model").String())
	return newBody
}
