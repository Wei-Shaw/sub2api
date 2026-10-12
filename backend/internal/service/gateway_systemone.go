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
