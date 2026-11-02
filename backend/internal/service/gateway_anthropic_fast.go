package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const anthropicFastCreditsRequiredMessage = "Usage credits are required for fast mode."
const anthropicFastCreditsRequiredReason GatewayFailureReason = "anthropic_fast_credits_required"

// This is an entitlement rejection of the requested speed, not evidence that
// ordinary requests have exhausted the account. Match the provider, status and
// structured error exactly; other 429s retain normal rate-limit handling.
func isAnthropicFastCreditsRequired(account *Account, status int, body []byte) bool {
	return account != nil && account.Platform == PlatformAnthropic &&
		!account.IsBedrock() && account.Type != AccountTypeServiceAccount &&
		status == http.StatusTooManyRequests && json.Valid(body) &&
		gjson.GetBytes(body, "type").String() == "error" &&
		gjson.GetBytes(body, "error.type").String() == "rate_limit_error" &&
		strings.TrimSpace(gjson.GetBytes(body, "error.message").String()) == anthropicFastCreditsRequiredMessage
}

// IsAnthropicFastCreditsRequired identifies the request-scoped terminal outcome
// so each client protocol can surface the same actionable message.
func (e *UpstreamFailoverError) IsAnthropicFastCreditsRequired() bool {
	return e != nil && e.Reason == anthropicFastCreditsRequiredReason
}

func (s *GatewayService) anthropicFastRejection(ctx context.Context, c *gin.Context, account *Account, status int, headers http.Header, body []byte) *UpstreamFailoverError {
	if !isAnthropicFastCreditsRequired(account, status, body) {
		return nil
	}
	if s.rateLimitService != nil {
		// Preserve independently proven 5h/7d exhaustion, never the Fast reset.
		s.rateLimitService.HandleUpstreamError(ctx, account, status, headers, body)
	}
	setOpsUpstreamError(c, status, anthropicFastCreditsRequiredMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID,
		UpstreamStatusCode: status, UpstreamRequestID: headers.Get("x-request-id"),
		Kind: "request_rejected", Message: anthropicFastCreditsRequiredMessage,
	})
	return &UpstreamFailoverError{
		StatusCode: status, ResponseBody: body,
		Stage: GatewayFailureStageInference, Scope: GatewayFailureScopeRequest,
		Reason: anthropicFastCreditsRequiredReason, NextAccountAction: NextAccountStop,
		ClientStatusCode: http.StatusTooManyRequests, ClientMessage: anthropicFastCreditsRequiredMessage,
	}
}

// Inspect before same-account retries as well as before failover, restoring the
// bounded error body for ordinary error handling.
func (s *GatewayService) anthropicFastResponseRejection(ctx context.Context, c *gin.Context, account *Account, resp *http.Response) *UpstreamFailoverError {
	if resp == nil || resp.Body == nil || resp.StatusCode != http.StatusTooManyRequests || account == nil || account.Platform != PlatformAnthropic {
		return nil
	}
	body, err := s.readUpstreamErrorBody(resp)
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return s.anthropicFastRejection(ctx, c, account, resp.StatusCode, resp.Header, body)
}
