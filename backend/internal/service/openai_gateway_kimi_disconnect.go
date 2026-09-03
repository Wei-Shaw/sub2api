package service

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// shouldCancelKimiUpstreamOnClientDisconnect intentionally scopes the runtime
// switch to Kimi streaming traffic. Other providers retain the historical
// usage-draining behavior even when the switch is enabled.
func (s *OpenAIGatewayService) shouldCancelKimiUpstreamOnClientDisconnect(
	ctx context.Context,
	account *Account,
	stream bool,
) bool {
	if !stream || account == nil || account.Platform != PlatformKimi || s == nil || s.settingService == nil {
		return false
	}
	protocol := account.GetAPIProtocol()
	if protocol != APIProtocolChatCompletions && protocol != APIProtocolAnthropic {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.settingService.IsCancelKimiUpstreamOnClientDisconnectEnabled(ctx)
}

// kimiCCUpstreamContext preserves the Chat Completions pipeline's historical
// behavior of detaching every upstream request, including non-streaming ones.
// The only exception is an opted-in Kimi streaming request using one of the
// two explicitly supported account protocols.
func (s *OpenAIGatewayService) kimiCCUpstreamContext(
	ctx context.Context,
	account *Account,
	stream bool,
) (context.Context, context.CancelFunc) {
	if s.shouldCancelKimiUpstreamOnClientDisconnect(ctx, account, stream) {
		if ctx == nil {
			return context.Background(), func() {}
		}
		return ctx, func() {}
	}
	return detachUpstreamContext(ctx)
}

// kimiStreamUpstreamContext preserves the native Anthropic pipeline's existing
// context behavior: streaming requests are detached while non-streaming
// requests keep the client context. When enabled for Kimi streaming traffic,
// the original client context reaches the HTTP transport.
func (s *OpenAIGatewayService) kimiStreamUpstreamContext(
	ctx context.Context,
	account *Account,
	stream bool,
) (context.Context, context.CancelFunc) {
	if s.shouldCancelKimiUpstreamOnClientDisconnect(ctx, account, stream) {
		if ctx == nil {
			return context.Background(), func() {}
		}
		return ctx, func() {}
	}
	return detachStreamUpstreamContext(ctx, stream)
}

// closeKimiUpstreamAfterClientDisconnect handles the narrow window in which a
// downstream write fails before net/http has propagated cancellation to the
// request context. Closing the response body aborts the upstream read
// immediately on a best-effort basis.
func (s *OpenAIGatewayService) closeKimiUpstreamAfterClientDisconnect(
	c *gin.Context,
	resp *http.Response,
	account *Account,
) bool {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	if !s.shouldCancelKimiUpstreamOnClientDisconnect(ctx, account, true) {
		return false
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return true
}
