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

// gatewayTransportErrorTempUnschedDuration is how long an account is temporarily
// unscheduled after a durable transport failure (matches the OpenAI-side
// openAITransportErrorTempUnschedDuration).
const gatewayTransportErrorTempUnschedDuration = 10 * time.Minute

// gatewayTransportFailoverBody is the Anthropic-format error body attached to
// the failover error for a transport-level failure. Kept identical to the
// legacy inline 502 body so the client-visible payload is unchanged if
// failover is ultimately exhausted.
var gatewayTransportFailoverBody = []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`)

// handleUpstreamTransportError handles a transport-level upstream failure on
// the Anthropic/Bedrock forward paths (Do/DoWithTLS returned a non-HTTP error:
// proxy / DNS / TCP / TLS). It:
//  1. records the failure in Ops error logs (status 0, kind=request_error) —
//     the caller passes path-specific fields (UpstreamURL, Passthrough) via
//     event; identity and classification fields are filled here;
//  2. for durable faults (expired/rejected proxy creds, dead proxy,
//     DNS/routing) temporarily unschedules the account and logs a stable warn
//     event that alert rules can key on;
//  3. returns an error that is *UpstreamFailoverError (so the handler fails
//     over to a healthy account) for all non-canceled errors, or the original
//     error for context.Canceled (client gone — no failover, no eviction).
//
// It deliberately does NOT write to the response: the handler owns the
// response (failover, or a protocol-correct error once failover is exhausted).
func (s *GatewayService) handleUpstreamTransportError(ctx context.Context, c *gin.Context, account *Account, err error, event OpsUpstreamErrorEvent) error {
	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	event.Platform = account.Platform
	event.AccountID = account.ID
	event.AccountName = account.Name
	event.UpstreamStatusCode = 0
	event.Kind = "request_error"
	event.Message = safeErr
	appendOpsUpstreamError(c, event)

	// Client disconnected: do NOT fail over to another account and do NOT
	// evict this one — the upstream never had a chance to exhibit a fault.
	if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return err
	}

	// Transport attempt left local validation; count Ollama Cloud activity.
	scheduleOllamaCloudUsageActivity(s.deferredService, account)

	if classifyUpstreamTransportError(err).Persistent {
		s.tempUnscheduleTransportError(ctx, account, safeErr)
	}

	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: gatewayTransportFailoverBody,
	}
}

// tempUnscheduleTransportError marks an account temporarily unschedulable
// after a durable transport failure. Unlike the OpenAI side there is no
// in-memory scheduler block on this path: the Anthropic/Bedrock scheduler
// reads the persisted temp-unschedulable state, so the DB write is the single
// source of truth (same as tempUnscheduleGoogleConfigError /
// tempUnscheduleEmptyResponse).
//
// Log semantics:
//   - "gateway.account_temp_unscheduled_transport" — DB write succeeded.
//   - "gateway.account_temp_unschedule_transport_failed" — DB write attempted
//     but returned an error (the account remains schedulable).
func (s *GatewayService) tempUnscheduleTransportError(ctx context.Context, account *Account, safeErr string) {
	if s == nil || account == nil || s.accountRepo == nil {
		return
	}
	until := time.Now().Add(gatewayTransportErrorTempUnschedDuration)
	reason := "upstream transport error (proxy/network): " + safeErr

	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"gateway.account_temp_unschedule_transport_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return
	}
	logger.L().With(zap.String("component", "service.gateway")).Warn(
		"gateway.account_temp_unscheduled_transport",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("platform", account.Platform),
		zap.Time("until", until),
		zap.String("reason", reason),
	)
}
