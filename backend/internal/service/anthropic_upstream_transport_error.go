package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// anthropicTransportErrorTempUnschedDuration is how long an account is temporarily
// unscheduled after a durable transport failure (matches openAITransportErrorTempUnschedDuration).
const anthropicTransportErrorTempUnschedDuration = 10 * time.Minute

// anthropicTransportFailoverBody is the Anthropic-format error body attached to the
// failover error for a transport-level failure. Kept identical to the legacy inline
// 502 body so the client-visible payload is unchanged if failover is ultimately exhausted.
var anthropicTransportFailoverBody = []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`)

// handleAnthropicUpstreamTransportError handles a transport-level upstream failure
// on the Anthropic forwarding paths (Do/DoWithTLS returned a non-HTTP error:
// proxy/DNS/TCP/TLS). It mirrors handleOpenAIUpstreamTransportError:
//  1. records the failure in Ops error logs (status 0, kind=request_error);
//  2. for durable faults (expired/rejected proxy creds, dead proxy, DNS/routing)
//     temporarily unschedules the account so it stops being scheduled into hard
//     failures;
//  3. returns an error that is *UpstreamFailoverError (so the handler fails over
//     to a healthy account) for all non-canceled errors, or a plain error for
//     context.Canceled (client gone — no failover, no eviction).
//
// It deliberately does NOT write to the response: the handler owns the response
// (failover, or a protocol-correct error once failover is exhausted).
//
// upstreamURL must already be sanitized (safeUpstreamURL); passthrough tags the
// Ops error event for the Anthropic API-Key passthrough forward path.
func (s *GatewayService) handleAnthropicUpstreamTransportError(ctx context.Context, c *gin.Context, account *Account, upstreamURL string, err error, passthrough bool) error {
	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: 0,
		UpstreamURL:        upstreamURL,
		Passthrough:        passthrough,
		Kind:               "request_error",
		Message:            safeErr,
	})

	// Client disconnected: do NOT fail over to another account and do NOT evict
	// this one — the upstream never had a chance to exhibit a fault.
	if errors.Is(err, context.Canceled) {
		return err
	}

	if classifyOpenAITransportError(err).Persistent {
		s.tempUnscheduleAnthropicTransportError(ctx, account, safeErr)
	}

	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: anthropicTransportFailoverBody,
	}
}

// tempUnscheduleAnthropicTransportError marks an account temporarily unschedulable
// after a durable transport failure. Unlike the OpenAI variant there is no
// in-memory runtime block on the Anthropic scheduler; persistence via the account
// repository is the established mechanism (see tempUnscheduleGoogleConfigError).
func (s *GatewayService) tempUnscheduleAnthropicTransportError(ctx context.Context, account *Account, safeErr string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(anthropicTransportErrorTempUnschedDuration)
	reason := "upstream transport error (proxy/network): " + safeErr

	if s.accountRepo == nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"anthropic.account_temp_unscheduled_transport_skipped_no_repo",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("platform", account.Platform),
			zap.String("reason", reason),
		)
		return
	}

	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"anthropic.account_temp_unscheduled_transport_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return
	}

	logger.L().With(zap.String("component", "service.gateway")).Warn(
		"anthropic.account_temp_unscheduled_transport",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("platform", account.Platform),
		zap.Time("until", until),
		zap.String("reason", reason),
	)
}
