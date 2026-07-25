package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func EvaluationEvidencePersistenceFailureCount() uint64 {
	return service.EvaluationEvidencePersistenceFailureCount()
}

func EvaluationEvidence(repo service.EvaluationEvidenceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		evaluation, ok := service.EvaluationContextFromContext(c.Request.Context())
		if !ok || repo == nil {
			c.Next()
			return
		}

		startedAt := time.Now()
		ctx := service.WithEvaluationEvidenceRepository(c.Request.Context(), repo)
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		trace, _ := service.RouteTraceFromContext(c.Request.Context())
		snapshot := trace.Snapshot()
		finishedAt := time.Now()
		evidence := finalizeEvaluationRouteEvidence(c.Request.Context(), evaluation, snapshot, c.Writer.Status(), startedAt, finishedAt)
		if err := repo.UpsertTransport(c.Request.Context(), evidence); err != nil {
			service.RecordEvaluationEvidencePersistenceFailure()
			logger.FromContext(c.Request.Context()).Warn("evaluation route evidence persistence failed",
				zap.String("route_trace_id", evaluation.RouteTraceID),
				zap.Error(err),
			)
		}
	}
}

func finalizeEvaluationRouteEvidence(
	ctx context.Context,
	evaluation service.EvaluationContext,
	snapshot service.RouteEvidence,
	status int,
	startedAt time.Time,
	finishedAt time.Time,
) service.RouteEvidence {
	requestedModel := evaluation.ExpectedModelAlias
	if model, ok := service.RequestedPublicModelFromContext(ctx); ok {
		requestedModel = model
	}
	resolvedModel, _ := service.ResolvedUpstreamModelFromContext(ctx)
	provider, _ := service.ResolvedTargetPlatformFromContext(ctx)

	var latest service.RouteFallbackEntry
	if count := len(snapshot.FallbackChain); count > 0 {
		latest = snapshot.FallbackChain[count-1]
	}
	if resolvedModel == "" {
		resolvedModel = latest.ResolvedModel
	}
	if provider == "" {
		provider = latest.Provider
	}

	return service.RouteEvidence{
		RouteTraceID:        evaluation.RouteTraceID,
		EvaluationRunID:     evaluation.RunID,
		SampleID:            evaluation.SampleID,
		APIKeyID:            evaluation.APIKeyID,
		RequestID:           evaluationEvidenceRequestID(ctx),
		RequestedModel:      requestedModel,
		ResolvedModel:       resolvedModel,
		RouteProfileVersion: evaluation.ExpectedRouteProfile,
		Provider:            provider,
		ChannelRef:          latest.ChannelRef,
		AccountPoolRef:      latest.AccountPoolRef,
		Region:              latest.Region,
		Attempts:            snapshot.Attempts,
		FallbackChain:       append([]service.RouteFallbackEntry(nil), snapshot.FallbackChain...),
		TransportStatus:     classifyEvaluationTransportStatus(status, ctx.Err(), snapshot),
		ErrorCode:           latest.ErrorCode,
		StartedAt:           startedAt,
		FinishedAt:          &finishedAt,
	}
}

func classifyEvaluationTransportStatus(status int, requestErr error, trace service.RouteEvidence) string {
	if status == 499 || errors.Is(requestErr, context.Canceled) {
		return "client_cancelled"
	}
	if status >= http.StatusOK && status < http.StatusBadRequest {
		return "succeeded"
	}
	if count := len(trace.FallbackChain); count > 0 && strings.TrimSpace(trace.FallbackChain[count-1].ErrorCode) != "" {
		return "upstream_failed"
	}
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		return "protocol_failed"
	}
	if status >= http.StatusInternalServerError {
		return "gateway_failed"
	}
	return "started"
}

func evaluationEvidenceRequestID(ctx context.Context) string {
	if value, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(value) != "" {
		return "client:" + strings.TrimSpace(value)
	}
	if value, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(value) != "" {
		return "local:" + strings.TrimSpace(value)
	}
	return ""
}
