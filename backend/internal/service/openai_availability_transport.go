package service

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func openAIAvailabilityCallerGone(ctx context.Context, c *gin.Context, err error) bool {
	return errors.Is(err, context.Canceled) || (ctx != nil && ctx.Err() != nil) ||
		(c != nil && c.Request != nil && c.Request.Context().Err() != nil)
}

func openAIAvailabilityModel(account *Account, originalModel, mappedModel string) string {
	if mappedModel != "" {
		return mappedModel
	}
	return canonicalOpenAIAccountSchedulingModel(account, originalModel)
}

func (s *OpenAIGatewayService) openAIAvailabilityReadFailure(ctx context.Context, c *gin.Context, account *Account, err error) bool {
	return s.openAIAPIKeyAvailabilityEnabled(account) && err != nil &&
		!openAIAvailabilityCallerGone(ctx, c, err) && !errors.Is(err, bufio.ErrTooLong) &&
		!errors.Is(err, ErrUpstreamResponseBodyTooLarge)
}

// Called only for observed upstream I/O failures, never for local staging,
// response size limits, parameter validation, or downstream cancellation.
func (s *OpenAIGatewayService) recordOpenAIAvailabilityTransportFailure(ctx context.Context, c *gin.Context, account *Account, model string, err error) {
	if !s.openAIAvailabilityReadFailure(ctx, c, account, err) {
		return
	}
	decision := s.recordOpenAIAccountModelTransientFailureWithInitialCooldown(account, model, time.Now(), openAIModelTransientShortCooldown)
	if decision.FailureStreak > 0 {
		slog.Warn("openai_model_transient_state", "account_id", account.ID,
			"model", openAIAccountModelTransientModel(model), "failure_streak", decision.FailureStreak,
			"cooldown_ms", decision.Cooldown.Milliseconds(), "block_scope", "account_model", "cause", "upstream_transport")
	}
}

func (s *OpenAIGatewayService) newOpenAIAvailabilityStreamTransportError(ctx context.Context, c *gin.Context, account *Account, passthrough bool, requestID, model, message string, cause error) error {
	if s.openAIAPIKeyAvailabilityEnabled(account) && openAIAvailabilityCallerGone(ctx, c, cause) {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return cause
	}
	s.recordOpenAIAvailabilityTransportFailure(ctx, c, account, model, cause)
	return s.newOpenAIStreamFailoverErrorWithModel(c, account, passthrough, requestID, nil, message, model)
}
