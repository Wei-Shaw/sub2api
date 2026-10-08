package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleOpenAIDeliveryFailure short-circuits a downstream delivery failure:
// no failover, no account health penalty, no schedule success/failure report.
// Observed usage is recorded with its failed outcome (existing pricing);
// unknown consumption is audited and never written as a zero-usage row.
func handleOpenAIDeliveryFailure(
	c *gin.Context,
	reqLog *zap.Logger,
	account *service.Account,
	result *service.OpenAIForwardResult,
	err error,
	submitUsage func(*service.OpenAIForwardResult),
) bool {
	deliveryErr, ok := service.AsOpenAIDeliveryError(err)
	if !ok {
		return false
	}
	consumption := "unknown"
	if result != nil && result.HasObservedConsumption() {
		consumption = "observed"
		if result.ResponseOutcome == "" {
			result.ResponseOutcome = deliveryErr.Outcome
		}
		submitUsage(result)
	}
	message := "downstream delivery " + string(deliveryErr.Outcome) + " during " + deliveryErr.Phase + "; consumption=" + consumption
	fields := []zap.Field{
		zap.String("response_outcome", string(deliveryErr.Outcome)),
		zap.String("delivery_phase", deliveryErr.Phase),
		zap.String("consumption", consumption),
		zap.Error(err),
	}
	if account != nil {
		fields = append(fields, zap.Int64("account_id", account.ID))
	}
	if result != nil {
		fields = append(fields, zap.Int64("forward_duration_ms", result.Duration.Milliseconds()))
		if result.FirstTokenMs != nil {
			fields = append(fields, zap.Int("first_output_token_ms", *result.FirstTokenMs))
		}
	}
	reqLog.Info("openai.downstream_delivery_failed", fields...)

	c.Set(service.OpsDeliveryAuditKey, message)
	if c.Writer.Written() {
		// Wire status is already committed: record one request-scoped error row
		// so the request is counted once as a (client-side) error.
		service.MarkOpsStreamErrorValue(c, service.OpsStreamError{
			ErrType:        "client_closed_request",
			Code:           string(deliveryErr.Outcome),
			Message:        message,
			IntendedStatus: statusClientClosedRequest,
			RequestScoped:  true,
			NonStream:      result != nil && !result.Stream,
		})
	} else if deliveryErr.Outcome == service.ResponseOutcomeClientCancelled {
		c.Status(statusClientClosedRequest)
	} else {
		c.Status(http.StatusBadGateway)
	}
	return true
}

// handleOpenAIUpstreamFailedWithUsage stops retry/failover for an attempt whose
// supplier already reported nonzero usage: the upstream failure is reported
// once, the client gets the failover error response, and the observed
// consumption is recorded once as upstream_failed.
func (h *OpenAIGatewayHandler) handleOpenAIUpstreamFailedWithUsage(
	c *gin.Context,
	reqLog *zap.Logger,
	account *service.Account,
	result *service.OpenAIForwardResult,
	err error,
	streamStarted bool,
	reportFailure func(error),
	submitUsage func(*service.OpenAIForwardResult),
) bool {
	stopErr, ok := service.AsOpenAIUpstreamFailedWithUsage(err)
	if !ok {
		return false
	}
	if stopErr.Failover.ShouldReportAccountScheduleFailure() {
		reportFailure(err)
	}
	reqLog.Warn("openai.upstream_failed_with_usage_failover_stopped",
		zap.Int64("account_id", account.ID),
		zap.Int("upstream_status", stopErr.Failover.StatusCode),
		zap.Error(err),
	)
	h.handleFailoverExhausted(c, stopErr.Failover, streamStarted || c.Writer.Written())
	if result != nil {
		result.ResponseOutcome = service.ResponseOutcomeUpstreamFailed
		submitUsage(result)
	}
	return true
}

// logOpenAIAttemptTiming emits one structured timing record per GPT attempt
// (success and error). It separates local queue/routing, upstream response
// headers, the first semantic output token (never header time; absent when
// unknown), the whole forward attempt and the buffered response write. The
// request logger carries request_id/client_request_id. The existing internal
// request identity continues to be used for billing idempotency.
func logOpenAIAttemptTiming(c *gin.Context, reqLog *zap.Logger, account *service.Account, result *service.OpenAIForwardResult, forwardMs int64, err error) {
	fields := make([]zap.Field, 0, 10)
	if account != nil {
		fields = append(fields, zap.Int64("account_id", account.ID))
	}
	if v, ok := getContextInt64(c, service.OpsRoutingLatencyMsKey); ok {
		fields = append(fields, zap.Int64("routing_ms", v))
	}
	if v, ok := getContextInt64(c, service.OpsUpstreamLatencyMsKey); ok {
		fields = append(fields, zap.Int64("upstream_headers_ms", v))
	}
	fields = append(fields, zap.Int64("forward_ms", forwardMs))
	if v, ok := getContextInt64(c, service.OpsResponseWriteMsKey); ok {
		fields = append(fields, zap.Int64("response_write_ms", v))
	}
	if result != nil {
		if result.FirstTokenMs != nil {
			fields = append(fields, zap.Int("first_output_token_ms", *result.FirstTokenMs))
		}
		if result.ResponseOutcome != "" {
			fields = append(fields, zap.String("response_outcome", string(result.ResponseOutcome)))
		}
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	reqLog.Info("openai.attempt_timing", fields...)
}
