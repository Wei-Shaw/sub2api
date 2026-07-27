//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

const (
	testRouteTraceID = "trace-server-generated"
	testRunID        = "018f4f20-3d12-7e50-9000-000000000001"
	testSampleID     = "018f4f20-3d12-7e50-9000-000000000002"
)

type capturedEvidenceExec struct {
	db           *sql.DB
	rowsAffected []int64
	queries      []string
	args         [][]any
}

func (c *capturedEvidenceExec) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	c.queries = append(c.queries, query)
	c.args = append(c.args, append([]any(nil), args...))
	if len(c.rowsAffected) > 0 {
		affected := c.rowsAffected[0]
		c.rowsAffected = c.rowsAffected[1:]
		return sqlmock.NewResult(0, affected), nil
	}
	return sqlmock.NewResult(0, 1), nil
}

func (c *capturedEvidenceExec) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

func TestEvaluationRouteEvidenceRepository_TransportThenBillingIsIdempotent(t *testing.T) {
	repo, exec, mock := newCapturedEvaluationEvidenceRepository(t)
	ctx := evaluationEvidenceContext()
	transport := completeTransportEvidence()
	usage := completeUsageEvidence()

	require.NoError(t, repo.UpsertTransport(ctx, transport))
	require.NoError(t, repo.UpsertTransport(ctx, transport))
	require.NoError(t, repo.AttachBilling(ctx, testRouteTraceID, usage))
	require.NoError(t, repo.AttachBilling(ctx, testRouteTraceID, usage))

	require.Len(t, exec.queries, 4, "retries must update the same route_trace_id row")
	for _, query := range exec.queries {
		requireRouteEvidenceConflictGuard(t, query)
	}
	requireTransportArguments(t, exec.args[0], transport)
	requireUsageArguments(t, exec.args[2], usage)
	require.Contains(t, normalizedEvidenceSQL(exec.queries[2]), "input_tokens = COALESCE(EXCLUDED.input_tokens, evaluation_route_evidence.input_tokens)")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEvaluationRouteEvidenceRepository_BillingThenTransportReplacesPlaceholdersAndPreservesBilling(t *testing.T) {
	repo, exec, mock := newCapturedEvaluationEvidenceRepository(t)
	ctx := evaluationEvidenceContext()
	transport := completeTransportEvidence()
	usage := completeUsageEvidence()

	require.NoError(t, repo.AttachBilling(ctx, testRouteTraceID, usage))
	require.NoError(t, repo.AttachBilling(ctx, testRouteTraceID, usage))
	require.NoError(t, repo.UpsertTransport(ctx, transport))
	require.NoError(t, repo.UpsertTransport(ctx, transport))

	require.Len(t, exec.queries, 4, "both phases must converge on one route_trace_id row")
	attachSQL := normalizedEvidenceSQL(exec.queries[0])
	transportSQL := normalizedEvidenceSQL(exec.queries[2])
	require.Contains(t, attachSQL, "transport_status")
	require.Contains(t, transportSQL, "requested_model = EXCLUDED.requested_model")
	require.Contains(t, transportSQL, "resolved_model = EXCLUDED.resolved_model")
	require.Contains(t, transportSQL, "route_profile_version = EXCLUDED.route_profile_version")
	require.Contains(t, transportSQL, "region = EXCLUDED.region")
	require.Contains(t, transportSQL, "started_at = EXCLUDED.started_at")
	require.NotContains(t, transportSQL, "billed_amount =", "transport finalization must preserve attached billing")
	requireTransportArguments(t, exec.args[2], transport)
	requireUsageArguments(t, exec.args[0], usage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEvaluationRouteEvidenceRepository_NilFallbackChainPersistsEmptyJSONArray(t *testing.T) {
	repo, exec, mock := newCapturedEvaluationEvidenceRepository(t)
	transport := completeTransportEvidence()
	transport.FallbackChain = nil

	require.NoError(t, repo.UpsertTransport(evaluationEvidenceContext(), transport))

	require.Len(t, exec.args, 1)
	require.Equal(t, []byte("[]"), exec.args[0][13])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEvaluationRouteEvidenceRepository_ZeroRowsAlwaysReturnsIdentityConflict(t *testing.T) {
	tests := []struct {
		name string
		rows *sqlmock.Rows
	}{
		{
			name: "matching row",
			rows: sqlmock.NewRows([]string{"evaluation_run_id", "sample_id", "api_key_id"}).
				AddRow(testRunID, testSampleID, int64(41)),
		},
		{
			name: "conflicting row",
			rows: sqlmock.NewRows([]string{"evaluation_run_id", "sample_id", "api_key_id"}).
				AddRow("018f4f20-3d12-7e50-9000-000000000099", testSampleID, int64(41)),
		},
		{
			name: "row disappeared",
			rows: sqlmock.NewRows([]string{"evaluation_run_id", "sample_id", "api_key_id"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			exec := &capturedEvidenceExec{db: db, rowsAffected: []int64{0}}
			repo := &evaluationRouteEvidenceRepository{sql: exec}

			mock.ExpectQuery("SELECT evaluation_run_id::text, sample_id::text, api_key_id").
				WithArgs(testRouteTraceID).
				WillReturnRows(tt.rows)

			err = repo.UpsertTransport(evaluationEvidenceContext(), completeTransportEvidence())
			require.ErrorIs(t, err, service.ErrRouteEvidenceIdentityConflict)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func newCapturedEvaluationEvidenceRepository(t *testing.T) (*evaluationRouteEvidenceRepository, *capturedEvidenceExec, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	exec := &capturedEvidenceExec{db: db}
	return &evaluationRouteEvidenceRepository{sql: exec}, exec, mock
}

func evaluationEvidenceContext() context.Context {
	return service.WithEvaluationContext(context.Background(), service.EvaluationContext{
		RunID:                testRunID,
		SampleID:             testSampleID,
		ExpectedModelAlias:   "qwen3-coder",
		ExpectedRouteProfile: "route-v42",
		APIKeyID:             41,
		RouteTraceID:         testRouteTraceID,
		IssuedAt:             time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	})
}

func completeTransportEvidence() service.RouteEvidence {
	started := time.Date(2026, 7, 25, 12, 1, 0, 0, time.UTC)
	finished := started.Add(1250 * time.Millisecond)
	return service.RouteEvidence{
		RouteTraceID:        testRouteTraceID,
		EvaluationRunID:     testRunID,
		SampleID:            testSampleID,
		APIKeyID:            41,
		RequestID:           "request-real",
		RequestedModel:      "public-coder-alias",
		ResolvedModel:       "qwen3-coder-2026-07",
		RouteProfileVersion: "route-v43-real",
		Provider:            "qwen",
		ChannelRef:          "channel_redacted",
		AccountPoolRef:      "account_redacted",
		Region:              "cn-east-real",
		Attempts:            2,
		FallbackChain: []service.RouteFallbackEntry{
			{Ordinal: 1, Provider: "qwen", AccountPoolRef: "account_first", ChannelRef: "channel_redacted", ResolvedModel: "qwen3-coder-2026-07", Region: "cn-east-real", ErrorCode: "429"},
			{Ordinal: 2, Provider: "qwen", AccountPoolRef: "account_redacted", ChannelRef: "channel_redacted", ResolvedModel: "qwen3-coder-2026-07", Region: "cn-east-real"},
		},
		TransportStatus: "succeeded",
		StartedAt:       started,
		FinishedAt:      &finished,
	}
}

func completeUsageEvidence() service.RouteUsageEvidence {
	ttft := 123
	latency := 1250
	return service.RouteUsageEvidence{
		InputTokens:  101,
		OutputTokens: 37,
		TTFT:         &ttft,
		Latency:      &latency,
		BilledAmount: decimal.RequireFromString("0.00012345"),
		FinishReason: "stop",
	}
}

func requireRouteEvidenceConflictGuard(t *testing.T, query string) {
	t.Helper()
	normalized := normalizedEvidenceSQL(query)
	require.Contains(t, normalized, "ON CONFLICT (route_trace_id) DO UPDATE")
	require.Contains(t, normalized, "evaluation_route_evidence.evaluation_run_id = EXCLUDED.evaluation_run_id")
	require.Contains(t, normalized, "evaluation_route_evidence.sample_id = EXCLUDED.sample_id")
	require.Contains(t, normalized, "evaluation_route_evidence.api_key_id = EXCLUDED.api_key_id")
}

func requireTransportArguments(t *testing.T, args []any, want service.RouteEvidence) {
	t.Helper()
	require.GreaterOrEqual(t, len(args), 18)
	require.Equal(t, want.RouteTraceID, args[0])
	require.Equal(t, want.EvaluationRunID, args[1])
	require.Equal(t, want.SampleID, args[2])
	require.Equal(t, want.APIKeyID, args[3])
	require.Equal(t, want.RequestedModel, args[5])
	require.Equal(t, want.ResolvedModel, args[6])
	require.Equal(t, want.RouteProfileVersion, args[7])
	require.Equal(t, want.Region, args[11])
	require.Equal(t, want.StartedAt, args[16])
	require.Equal(t, want.FinishedAt, args[17])

	encoded, ok := args[13].([]byte)
	require.True(t, ok)
	var fallback []service.RouteFallbackEntry
	require.NoError(t, json.Unmarshal(encoded, &fallback))
	require.Equal(t, want.FallbackChain, fallback)
}

func requireUsageArguments(t *testing.T, args []any, want service.RouteUsageEvidence) {
	t.Helper()
	require.GreaterOrEqual(t, len(args), 14)
	require.Equal(t, testRouteTraceID, args[0])
	require.Equal(t, testRunID, args[1])
	require.Equal(t, testSampleID, args[2])
	require.Equal(t, int64(41), args[3])
	require.Equal(t, want.InputTokens, args[8])
	require.Equal(t, want.OutputTokens, args[9])
	require.Equal(t, want.TTFT, args[10])
	require.Equal(t, want.Latency, args[11])
	require.Equal(t, want.BilledAmount, args[12])
	require.Equal(t, want.FinishReason, args[13])
}

func normalizedEvidenceSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}
