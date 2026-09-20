package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type openAIWSHandshakeFailoverError struct {
	*UpstreamFailoverError
}

func (e *openAIWSHandshakeFailoverError) Unwrap() error {
	return e.UpstreamFailoverError
}

func IsOpenAIWSHandshakeFailover(err error) bool {
	var handshakeErr *openAIWSHandshakeFailoverError
	return errors.As(err, &handshakeErr)
}

// 仅由首轮无续链依赖的握手调用；后续轮与语义错误保留各模式原有恢复边界。
func (s *OpenAIGatewayService) handleOpenAIWSHandshakeFailure(ctx context.Context, c *gin.Context, account *Account, canonicalModel string, dialErr *openAIWSDialError, passthrough bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(dialErr, context.Canceled) {
		return dialErr
	}
	body := s.redactAgentIdentitySensitiveBody(ctx, account, dialErr.ResponseBody)
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	shouldFailover := s.shouldFailoverOpenAIUpstreamResponse(account, dialErr.StatusCode, message, body)
	if !shouldFailover {
		if hit, code, cyberMessage := detectOpenAICyberPolicy(body); hit {
			MarkOpsCyberPolicy(c, CyberPolicyMark{Code: code, Message: cyberMessage, UpstreamStatus: dialErr.StatusCode})
			return dialErr
		}
		if isOpenAIContextWindowError(message, body) {
			return dialErr
		}
		if _, _, _, matched := applyErrorPassthroughRule(c, PlatformOpenAI, dialErr.StatusCode, body, http.StatusBadGateway, "upstream_error", "Upstream request failed"); matched {
			return dialErr
		}
		if !account.ShouldHandleErrorCode(dialErr.StatusCode) {
			return dialErr
		}
	}

	shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, dialErr.StatusCode, dialErr.ResponseHeaders, body, canonicalModel)
	if !shouldFailover && !shouldDisable {
		return dialErr
	}
	retryable := !shouldDisable && account.IsPoolMode() && (account.IsPoolModeRetryableStatus(dialErr.StatusCode) ||
		isOpenAITransientProcessingError(dialErr.StatusCode, message, body))
	failoverErr := s.newOpenAIAccountFailoverError(account, dialErr.StatusCode, dialErr.ResponseHeaders, body, message, shouldDisable, retryable)
	detail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, dialErr.StatusCode, message, detail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: dialErr.StatusCode, UpstreamRequestID: dialErr.ResponseHeaders.Get("x-request-id"),
		Passthrough: passthrough, Kind: "failover", Message: message, Detail: detail, UpstreamResponseBody: detail,
	})
	return &openAIWSHandshakeFailoverError{UpstreamFailoverError: failoverErr}
}

func openAIWSHandshakeHTTPPolicyApplies(account *Account, turn int, previousResponseID string, dialErr *openAIWSDialError) bool {
	return account != nil && account.Platform == PlatformOpenAI && turn == 1 &&
		strings.TrimSpace(previousResponseID) == "" && dialErr != nil && dialErr.StatusCode >= http.StatusBadRequest
}

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
