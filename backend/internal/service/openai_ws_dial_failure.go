package service

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
)

// handleOpenAIWSDialTransportFailure 处理 WS 拨号阶段的传输层失败（未收到 HTTP 状态码）：
// 当次请求返回可换号的错误交给 handler 切账号，账号冷却及 Ops 记录复用 HTTP 传输失败入口。
// 收到 HTTP 状态码或客户端已取消时返回 nil，由调用方沿用既有分支。
func (s *OpenAIGatewayService) handleOpenAIWSDialTransportFailure(ctx context.Context, c *gin.Context, account *Account, turn int, dialErr *openAIWSDialError, passthrough bool) *UpstreamFailoverError {
	if s == nil || account == nil || dialErr == nil || dialErr.StatusCode != 0 || ctx.Err() != nil {
		return nil
	}
	err := s.handleOpenAIUpstreamTransportError(ctx, c, account, dialErr, passthrough)
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		return nil
	}
	safeErr := sanitizeUpstreamErrorMessage(dialErr.Error())
	logOpenAIWSModeInfo(
		"ingress_ws_dial_transport_failover account_id=%d turn=%d cause=%s",
		account.ID,
		turn,
		truncateOpenAIWSLogValue(safeErr, openAIWSLogValueMaxLen),
	)
	return failoverErr
}
