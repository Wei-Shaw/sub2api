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

const gatewayTransportErrorStateUpdateTimeout = 5 * time.Second

// Keep the legacy Anthropic-shaped 502 payload until the failover loop is
// exhausted. The handler owns the actual client response during failover.
var gatewayTransportFailoverBody = []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`)

// handleGatewayUpstreamTransportError converts a failed HTTP round-trip into
// the failover error understood by the gateway handlers. A transport error has
// no upstream HTTP status, so writing 502 here would bypass account failover.
func (s *GatewayService) handleGatewayUpstreamTransportError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamURL string,
	err error,
	passthrough bool,
) error {
	if err == nil {
		return nil
	}

	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: 0,
		UpstreamURL:        safeUpstreamURL(upstreamURL),
		Passthrough:        passthrough,
		Kind:               "request_error",
		Message:            safeErr,
	})

	// The client is gone. Retrying another account would only create work and
	// could incorrectly quarantine a healthy account.
	if errors.Is(err, context.Canceled) {
		return err
	}

	if s != nil {
		scheduleOllamaCloudUsageActivity(s.deferredService, account)
	}

	if classifyUpstreamTransportError(err).Persistent {
		s.tempUnscheduleGatewayTransportError(ctx, account, safeErr)
	}

	return &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		ResponseBody:      gatewayTransportFailoverBody,
		NextAccountAction: NextAccountRetry,
	}
}

// tempUnscheduleGatewayTransportError persists a durable network/proxy fault
// and updates the selected account object immediately for the current process.
// The repository also refreshes the scheduler snapshot after the DB write.
func (s *GatewayService) tempUnscheduleGatewayTransportError(ctx context.Context, account *Account, safeErr string) {
	if s == nil || account == nil {
		return
	}

	until := time.Now().Add(upstreamTransportErrorTempUnschedDuration)
	reason := "upstream transport error (proxy/network): " + safeErr
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason

	if s.accountRepo == nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"gateway.account_temp_unscheduled_transport_memory_only",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("platform", account.Platform),
			zap.Time("until", until),
			zap.String("reason", reason),
		)
		return
	}

	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	stateCtx, cancel := context.WithTimeout(baseCtx, gatewayTransportErrorStateUpdateTimeout)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"gateway.account_temp_unscheduled_transport_failed",
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
