//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type evaluationEvidenceRepoStub struct {
	transport service.RouteEvidence
	err       error
	calls     int
}

func (s *evaluationEvidenceRepoStub) UpsertTransport(_ context.Context, evidence service.RouteEvidence) error {
	s.calls++
	s.transport = evidence
	return s.err
}

func (s *evaluationEvidenceRepoStub) AttachBilling(context.Context, string, service.RouteUsageEvidence) error {
	return nil
}

func TestClassifyEvaluationTransportStatus(t *testing.T) {
	failedTrace := service.RouteEvidence{FallbackChain: []service.RouteFallbackEntry{{ErrorCode: "503"}}}

	tests := []struct {
		name       string
		status     int
		requestErr error
		trace      service.RouteEvidence
		want       string
	}{
		{name: "started", status: http.StatusContinue, want: "started"},
		{name: "succeeded", status: http.StatusOK, trace: failedTrace, want: "succeeded"},
		{name: "upstream failure", status: http.StatusBadGateway, trace: failedTrace, want: "upstream_failed"},
		{name: "protocol failure", status: http.StatusBadRequest, want: "protocol_failed"},
		{name: "client cancellation status", status: 499, want: "client_cancelled"},
		{name: "client cancellation context", status: http.StatusBadGateway, requestErr: context.Canceled, trace: failedTrace, want: "client_cancelled"},
		{name: "gateway failure", status: http.StatusInternalServerError, want: "gateway_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyEvaluationTransportStatus(tt.status, tt.requestErr, tt.trace))
		})
	}
}

func TestEvaluationEvidencePersistsFinalizedRouteAfterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &evaluationEvidenceRepoStub{}
	evaluation := service.EvaluationContext{
		RunID:                "018f4f20-3d12-7e50-9000-000000000001",
		SampleID:             "018f4f20-3d12-7e50-9000-000000000002",
		ExpectedModelAlias:   "expected-model",
		ExpectedRouteProfile: "route-v42",
		APIKeyID:             41,
		RouteTraceID:         "trace-server-generated",
	}
	trace := service.NewRouteTrace(evaluation, service.RouteTraceConfig{HashKey: []byte("test-route-hash-key")})
	trace.RecordAttempt(service.RouteAttempt{
		Provider:      "qwen",
		AccountID:     91,
		ChannelID:     17,
		ResolvedModel: "qwen3-coder-2026-07",
		Region:        "cn-east",
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := service.WithEvaluationContext(c.Request.Context(), evaluation)
		ctx = service.WithRouteTrace(ctx, trace)
		ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-request-123")
		ctx = service.WithCompositeRouteDecision(ctx, service.CompositeRouteDecision{
			Matched:        true,
			PublicModel:    "public-coder-alias",
			UpstreamModel:  "qwen3-coder-2026-07",
			TargetPlatform: "qwen",
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(EvaluationEvidence(repo))
	router.GET("/test", func(c *gin.Context) {
		injected, ok := service.EvaluationEvidenceRepositoryFromContext(c.Request.Context())
		require.True(t, ok)
		require.Same(t, repo, injected)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, 1, repo.calls)
	got := repo.transport
	require.Equal(t, evaluation.RouteTraceID, got.RouteTraceID)
	require.Equal(t, evaluation.RunID, got.EvaluationRunID)
	require.Equal(t, evaluation.SampleID, got.SampleID)
	require.Equal(t, evaluation.APIKeyID, got.APIKeyID)
	require.Equal(t, "client:client-request-123", got.RequestID)
	require.Equal(t, "public-coder-alias", got.RequestedModel)
	require.Equal(t, "qwen3-coder-2026-07", got.ResolvedModel)
	require.Equal(t, evaluation.ExpectedRouteProfile, got.RouteProfileVersion)
	require.Equal(t, "qwen", got.Provider)
	require.Equal(t, "cn-east", got.Region)
	require.Equal(t, 1, got.Attempts)
	require.Len(t, got.FallbackChain, 1)
	require.Equal(t, "succeeded", got.TransportStatus)
	require.WithinDuration(t, time.Now(), got.StartedAt, 2*time.Second)
	require.NotNil(t, got.FinishedAt)
}

func TestEvaluationEvidencePersistenceFailureDoesNotChangeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &evaluationEvidenceRepoStub{err: errors.New("database unavailable")}
	before := EvaluationEvidencePersistenceFailureCount()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		evaluation := service.EvaluationContext{
			RunID: "018f4f20-3d12-7e50-9000-000000000001", SampleID: "018f4f20-3d12-7e50-9000-000000000002",
			ExpectedModelAlias: "model", ExpectedRouteProfile: "route-v42", APIKeyID: 41, RouteTraceID: "trace-server-generated",
		}
		ctx := service.WithEvaluationContext(c.Request.Context(), evaluation)
		ctx = service.WithRouteTrace(ctx, service.NewRouteTrace(evaluation, service.RouteTraceConfig{HashKey: []byte("test-route-hash-key")}))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(EvaluationEvidence(repo))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"ok":true}`, recorder.Body.String())
	require.Equal(t, before+1, EvaluationEvidencePersistenceFailureCount())
}
