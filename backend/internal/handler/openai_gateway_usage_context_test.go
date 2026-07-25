package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type usageContextEvidenceRepoStub struct{}

func (*usageContextEvidenceRepoStub) UpsertTransport(context.Context, service.RouteEvidence) error {
	return nil
}

func (*usageContextEvidenceRepoStub) AttachBilling(context.Context, string, service.RouteUsageEvidence) error {
	return nil
}

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")
	evaluation := service.EvaluationContext{RunID: "run-123", RouteTraceID: "trace-123"}
	repo := &usageContextEvidenceRepoStub{}
	parent = service.WithEvaluationContext(parent, evaluation)
	parent = service.WithEvaluationEvidenceRepository(parent, repo)

	var gotClientRequestID string
	var gotRequestID string
	var gotEvaluation service.EvaluationContext
	var gotRepo service.EvaluationEvidenceRepository
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotEvaluation, _ = service.EvaluationContextFromContext(ctx)
		gotRepo, _ = service.EvaluationEvidenceRepositoryFromContext(ctx)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
	require.Equal(t, evaluation, gotEvaluation)
	require.Same(t, repo, gotRepo)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")
	evaluation := service.EvaluationContext{RunID: "openai-run-123", RouteTraceID: "openai-trace-123"}
	repo := &usageContextEvidenceRepoStub{}
	parent = service.WithEvaluationContext(parent, evaluation)
	parent = service.WithEvaluationEvidenceRepository(parent, repo)

	var gotClientRequestID string
	var gotRequestID string
	var gotEvaluation service.EvaluationContext
	var gotRepo service.EvaluationEvidenceRepository
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotEvaluation, _ = service.EvaluationContextFromContext(ctx)
		gotRepo, _ = service.EvaluationEvidenceRepositoryFromContext(ctx)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
	require.Equal(t, evaluation, gotEvaluation)
	require.Same(t, repo, gotRepo)
}
