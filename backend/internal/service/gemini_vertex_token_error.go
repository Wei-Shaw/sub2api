package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// vertexServiceAccountNotFoundReason 标识换取 SA access token 时 SA 已不存在。
const vertexServiceAccountNotFoundReason GatewayFailureReason = "vertex_service_account_not_found"

// geminiVertexCredentialFailoverBody 是 SA 凭据失效时挂在 failover 错误上的 Google 格式错误体，
// 供 handler 在换号耗尽后做透传规则匹配与错误信息提取。
var geminiVertexCredentialFailoverBody = []byte(`{"error":{"code":401,"message":"Upstream authentication failed","status":"UNAUTHENTICATED"}}`)

// handleVertexServiceAccountNotFound 处理 Gemini 三条转发路径（原生 / Claude 兼容 /
// Chat Completions 兼容）构造上游请求时换取 SA access token 失败、且 SA 已不存在的情况：
//
//   - 账号置为 error 并停止调度，错误信息带上 token 端点的 error_description；
//   - 记录凭据获取阶段（stage=account_auth）的 Ops 事件，token 端点的状态码只出现在 Message 中；
//   - 返回 *UpstreamFailoverError(401)，由 handler 的 failover 循环换号。
//
// 不是该类失败时返回 nil，由调用方继续处理。本函数不写响应。
func (s *GeminiMessagesCompatService) handleVertexServiceAccountNotFound(ctx context.Context, c *gin.Context, account *Account, err error) *UpstreamFailoverError {
	tokenErr, ok := vertexServiceAccountNotFoundError(err)
	if !ok {
		return nil
	}

	msg := "Service account token rejected (400): " + tokenErr.Description
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	if s.rateLimitService != nil {
		s.rateLimitService.handleAuthError(stateCtx, account, msg)
	} else if s.accountRepo != nil {
		if setErr := s.accountRepo.SetError(stateCtx, account.ID, msg); setErr != nil {
			slog.Warn("account_set_error_failed", "account_id", account.ID, "error", setErr)
		}
	}

	event := OpsUpstreamErrorEvent{
		Platform:    account.Platform,
		AccountID:   account.ID,
		AccountName: account.Name,
		Kind:        "failover",
		Stage:       string(GatewayFailureStageAccountAuth),
		Scope:       string(GatewayFailureScopeAccount),
		Reason:      string(vertexServiceAccountNotFoundReason),
		Message:     sanitizeUpstreamErrorMessage(err.Error()),
	}
	event.ProxyID, event.ProxyName = opsUpstreamProxyAttribution(account)
	appendOpsUpstreamError(c, event)

	return &UpstreamFailoverError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: geminiVertexCredentialFailoverBody,
	}
}
