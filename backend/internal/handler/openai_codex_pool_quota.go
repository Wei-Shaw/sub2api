package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

const codexPoolQuotaMessage = "The account pool's Codex usage limit has been reached. Try again at the earliest known reset."

func codexPoolQuotaEnabled(key *service.APIKey) bool {
	return key != nil && key.Group != nil && key.Group.CodexPoolQuotaErrorEnabled
}

func codexPoolQuotaBody(reset time.Time) gin.H {
	return gin.H{
		"type":      "usage_limit_reached",
		"message":   codexPoolQuotaMessage,
		"resets_at": reset.Unix(),
	}
}

func (h *OpenAIGatewayHandler) writeCodexPoolQuotaHTTP(c *gin.Context, req service.CodexPoolQuotaRequest, failover *service.UpstreamFailoverError) bool {
	if c.Writer.Written() || service.IsResponseCommitted(c) {
		return false
	}
	reset := h.gatewayService.EarliestCodexPoolQuotaReset(c.Request.Context(), req)
	if reset.IsZero() {
		return false
	}
	if failover != nil {
		copyFailoverRetryAfter(c, failover.ResponseHeaders)
		if h.applyOpenAIFailoverPassthroughRule(c, failover.StatusCode, failover.ResponseBody, false) {
			return true
		}
	}

	markOpsRoutingCapacityLimited(c)
	headers := c.Writer.Header()
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Cache-Control", "no-store")
	headers.Del("Content-Length")
	headers.Del("Retry-After")
	headers.Del("X-Accel-Buffering")
	c.JSON(http.StatusTooManyRequests, gin.H{"error": codexPoolQuotaBody(reset)})
	return true
}

func (h *OpenAIGatewayHandler) writeCodexPoolQuotaWS(ctx context.Context, c *gin.Context, conn *coderws.Conn, req service.CodexPoolQuotaRequest) bool {
	reset := h.gatewayService.EarliestCodexPoolQuotaReset(ctx, req)
	if reset.IsZero() {
		return false
	}
	payload, err := json.Marshal(gin.H{
		"type":   "error",
		"status": http.StatusTooManyRequests,
		"error":  codexPoolQuotaBody(reset),
	})
	if err != nil {
		return false
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := conn.Write(writeCtx, coderws.MessageText, payload); err != nil {
		return false
	}
	markOpsRoutingCapacityLimited(c)
	service.MarkOpsStreamError(c, "usage_limit_reached", codexPoolQuotaMessage, http.StatusTooManyRequests)
	return true
}
