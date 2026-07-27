package service

import "context"

type recordUsageEvidenceRepoStub struct {
	attachCalls int
	traceID     string
	usage       RouteUsageEvidence
	err         error
	lastCtxErr  error
}

func (s *recordUsageEvidenceRepoStub) UpsertTransport(context.Context, RouteEvidence) error {
	return nil
}

func (s *recordUsageEvidenceRepoStub) AttachBilling(ctx context.Context, traceID string, usage RouteUsageEvidence) error {
	s.attachCalls++
	s.traceID = traceID
	s.usage = usage
	s.lastCtxErr = ctx.Err()
	return s.err
}

func evaluationRecordUsageContext(repo EvaluationEvidenceRepository, traceID string) context.Context {
	ctx := WithEvaluationContext(context.Background(), EvaluationContext{
		RunID: "018f4f20-3d12-7e50-9000-000000000001", SampleID: "018f4f20-3d12-7e50-9000-000000000002",
		APIKeyID: 501, RouteTraceID: traceID,
	})
	return WithEvaluationEvidenceRepository(ctx, repo)
}
