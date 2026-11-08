package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// geminiTransportFailoverBody 是传输层失败时挂在 failover 错误上的 Google 格式错误体，
// 供 handler 在换号耗尽后做透传规则匹配与错误信息提取。
var geminiTransportFailoverBody = []byte(`{"error":{"code":502,"message":"Upstream request failed","status":"INTERNAL"}}`)

// handleUpstreamTransportError 处理 Gemini 三条转发路径（原生 / Claude 兼容 /
// Chat Completions 兼容）上的传输层失败：Do 返回非 HTTP 错误（代理 / DNS / TCP / TLS /
// 响应头超时），没有拿到任何上游状态码。
//
//   - 记录 Ops 错误事件（status 0，kind=request_error）；客户端断开不记录
//     （见 isClientCanceledTransportError）；
//   - 持久性故障（代理凭据失效、端点拒绝连接、DNS/路由不可达）临时摘除账号；
//     瞬时故障（EOF / reset / 超时）账号保持可调度；
//   - 客户端已断开（context.Canceled）原样返回：不换号、不摘号；
//   - 其余一律返回 *UpstreamFailoverError，由 handler 的 failover 循环换号。
//
// 本函数不写响应：响应归 handler 所有（换号，或耗尽后按端点格式渲染错误）。
func (s *GeminiMessagesCompatService) handleUpstreamTransportError(ctx context.Context, c *gin.Context, account *Account, err error) error {
	if isClientCanceledTransportError(ctx, err) {
		return err
	}
	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	event := OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: 0,
		Kind:               "request_error",
		Message:            safeErr,
	}
	event.ProxyID, event.ProxyName = opsUpstreamProxyAttribution(account)
	appendOpsUpstreamError(c, event)

	if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return err
	}

	if classifyUpstreamTransportError(err).Persistent {
		tempUnscheduleAccountForTransportError(ctx, s.accountRepo, account, safeErr)
	}

	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: geminiTransportFailoverBody,
	}
}
