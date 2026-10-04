package service

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type SystemOneForwardResult struct {
	ForwardResult
	StatusCode  int
	Body        []byte
	ContentType string
}

type SystemOneUpstreamError struct {
	StatusCode int
}

const TypeSafeCredentialRejectedReason GatewayFailureReason = "typesafe_api_key_rejected"

func (e *SystemOneUpstreamError) Error() string {
	return fmt.Sprintf("typesafe upstream rejected request with status %d", e.StatusCode)
}

func (s *GatewayService) ForwardSystemOne(ctx context.Context, c *gin.Context, account *Account, body []byte) (*SystemOneForwardResult, error) {
	started := time.Now()
	if account == nil || !account.IsTypeSafe() || account.Type != AccountTypeAPIKey {
		return nil, errors.New("invalid typesafe account")
	}
	key := account.GetTypeSafeAPIKey()
	if key == "" {
		return nil, errors.New("typesafe api key is missing")
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetTypeSafeBaseURL())
	if err != nil {
		return nil, err
	}
	// SystemOne 路径此前把客户端 body 原样转发，账号的 model_mapping 完全不生效。
	// 这会让「typesafe 平台对接非 TypeSafe 上游」的场景无法工作：入站校验强制
	// model=jev-latest，而目标上游（如 OpenCode Zen 的 systemone 端点）只认自己
	// 的模型名。此处按账号映射改写出站 model，客户端仍发送规范模型名。
	body = applySystemOneModelMapping(account, body)
	req, err := typesafe.NewSystemOneRequest(ctx, baseURL, key, body)
	if err != nil {
		return nil, err
	}
	upstreamURL := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleUpstreamTransportError(ctx, c, account, err, OpsUpstreamErrorEvent{
			Passthrough: true,
			UpstreamURL: upstreamURL,
		})
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleSystemOneErrorResponse(ctx, c, account, resp, upstreamURL)
	}

	decoded, err := typesafe.DecodeSystemOneResponse(resp.Body)
	if err != nil {
		// The upstream accepted (and may have charged) this request but the
		// gateway cannot relay it; keep an ops trail for reconciliation.
		setOpsUpstreamError(c, resp.StatusCode, err.Error(), "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Passthrough:        true,
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			UpstreamURL:        upstreamURL,
			Kind:               "response_error",
			Message:            err.Error(),
		})
		return nil, err
	}
	return &SystemOneForwardResult{
		ForwardResult: ForwardResult{
			RequestID:             resp.Header.Get("x-request-id"),
			UpstreamHeaders:       resp.Header.Clone(),
			Usage:                 ClaudeUsage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens},
			Model:                 typesafe.JevLatestModel,
			UpstreamResponseModel: decoded.Model,
			Duration:              time.Since(started),
		},
		StatusCode:  resp.StatusCode,
		Body:        decoded.Body,
		ContentType: systemOneResponseContentType(resp.Header.Get("Content-Type")),
	}, nil
}

// IsSystemOneRequestErrorStatus reports upstream statuses that describe the
// caller's own payload (malformed, unprocessable, or too large).
func IsSystemOneRequestErrorStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

// handleSystemOneErrorResponse applies the shared account error policy to a
// non-2xx System One response. 400/413/422 describe the caller's own payload, so
// they never touch account state (a tenant must not be able to disable an
// account with bad input) and are not retried elsewhere. Every other status
// goes through the account error policy (custom error codes, temporary
// unschedulable rules, pool mode) and fails over when the status is retryable
// or the policy took the account out of rotation.
func (s *GatewayService) handleSystemOneErrorResponse(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, upstreamURL string) error {
	respBody, _ := s.readUpstreamErrorBody(resp)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
	event := OpsUpstreamErrorEvent{
		Passthrough:        true,
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		UpstreamURL:        upstreamURL,
		Kind:               "http_error",
		Message:            upstreamMsg,
	}

	if IsSystemOneRequestErrorStatus(resp.StatusCode) {
		appendOpsUpstreamError(c, event)
		return &SystemOneUpstreamError{StatusCode: resp.StatusCode}
	}

	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, typesafe.JevLatestModel)
	}
	if !shouldDisable && !s.shouldFailoverUpstreamError(resp.StatusCode) {
		appendOpsUpstreamError(c, event)
		return &SystemOneUpstreamError{StatusCode: resp.StatusCode}
	}

	event.Kind = "failover"
	appendOpsUpstreamError(c, event)
	failoverErr := &UpstreamFailoverError{
		StatusCode:             resp.StatusCode,
		ResponseBody:           respBody,
		ResponseHeaders:        resp.Header.Clone(),
		RetryableOnSameAccount: !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	}
	if resp.StatusCode == http.StatusUnauthorized {
		failoverErr.Stage = GatewayFailureStageAccountAuth
		failoverErr.Scope = GatewayFailureScopeAccount
		failoverErr.Reason = TypeSafeCredentialRejectedReason
		failoverErr.NextAccountAction = NextAccountRetry
	}
	return failoverErr
}

// applySystemOneModelMapping 按账号 model_mapping 改写 SystemOne body 的 model
// 字段，使出站模型名与目标上游一致。
//
// 未命中映射、映射结果与原值相同或 body 形状异常时原样返回：SystemOne 请求的
// 形状由 ValidateSystemOneRequest 在上游侧把守，这里只做尽力而为的改名，绝不
// 因为改名失败而中断请求。
func applySystemOneModelMapping(account *Account, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
	}
	requested := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if requested == "" {
		return body
	}
	mapped, matched := account.ResolveMappedModel(requested)
	if !matched || mapped == "" || mapped == requested {
		return body
	}
	rewritten, err := sjson.SetBytes(body, "model", mapped)
	if err != nil {
		return body
	}
	return rewritten
}

// systemOneResponseContentType keeps the upstream JSON media type (and its
// charset) but never relays a non-JSON type for a body already validated as
// JSON, so the gateway origin cannot be made to serve it as HTML.
func systemOneResponseContentType(raw string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return "application/json"
	}
	if mediaType == "application/json" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")) {
		return strings.TrimSpace(raw)
	}
	return "application/json"
}
