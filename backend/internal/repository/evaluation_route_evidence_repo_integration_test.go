//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestEvaluationRouteEvidenceRepository_OutOfOrderWritesConvergeOnPostgres(t *testing.T) {
	ctx := context.Background()
	suffix := uuid.NewString()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: "route-evidence-" + suffix + "@example.com"})
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{
		UserID: user.ID, Key: "sk-route-evidence-" + suffix, Name: "route evidence",
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM evaluation_route_evidence WHERE api_key_id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	repo := &evaluationRouteEvidenceRepository{sql: integrationDB}
	for _, transportFirst := range []bool{true, false} {
		name := "billing_then_transport"
		if transportFirst {
			name = "transport_then_billing"
		}
		t.Run(name, func(t *testing.T) {
			traceID := "trace-" + uuid.NewString()
			runID := uuid.NewString()
			sampleID := uuid.NewString()
			evalCtx := service.WithEvaluationContext(ctx, service.EvaluationContext{
				RunID: runID, SampleID: sampleID, APIKeyID: apiKey.ID, RouteTraceID: traceID,
				ExpectedModelAlias: "expected-placeholder", ExpectedRouteProfile: "route-placeholder",
				IssuedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			})
			startedAt := time.Date(2026, 7, 25, 12, 1, 0, 0, time.UTC)
			finishedAt := startedAt.Add(1250 * time.Millisecond)
			transport := service.RouteEvidence{
				RouteTraceID: traceID, EvaluationRunID: runID, SampleID: sampleID, APIKeyID: apiKey.ID,
				RequestID: "request-real", RequestedModel: "public-coder-alias", ResolvedModel: "qwen3-coder-2026-07",
				RouteProfileVersion: "route-v43-real", Provider: "qwen", ChannelRef: "channel_redacted",
				AccountPoolRef: "account_redacted", Region: "cn-east-real", Attempts: 2,
				FallbackChain: []service.RouteFallbackEntry{
					{Ordinal: 1, Provider: "qwen", AccountPoolRef: "account_first", ChannelRef: "channel_redacted", ResolvedModel: "qwen3-coder-2026-07", Region: "cn-east-real", ErrorCode: "429"},
					{Ordinal: 2, Provider: "qwen", AccountPoolRef: "account_redacted", ChannelRef: "channel_redacted", ResolvedModel: "qwen3-coder-2026-07", Region: "cn-east-real"},
				},
				TransportStatus: "succeeded", StartedAt: startedAt, FinishedAt: &finishedAt,
			}
			ttft := 123
			latency := 1250
			usage := service.RouteUsageEvidence{
				InputTokens: 101, OutputTokens: 37, TTFT: &ttft, Latency: &latency,
				BilledAmount: decimal.RequireFromString("0.00012345"), FinishReason: "stop",
			}

			writeTransport := func() { require.NoError(t, repo.UpsertTransport(evalCtx, transport)) }
			writeBilling := func() { require.NoError(t, repo.AttachBilling(evalCtx, traceID, usage)) }
			if transportFirst {
				writeTransport()
				writeTransport()
				writeBilling()
				writeBilling()
			} else {
				writeBilling()
				writeBilling()
				writeTransport()
				writeTransport()
			}

			conflict := transport
			conflict.EvaluationRunID = uuid.NewString()
			require.ErrorIs(t, repo.UpsertTransport(evalCtx, conflict), service.ErrRouteEvidenceIdentityConflict)

			assertEvaluationRouteEvidenceRow(t, ctx, traceID, transport, usage)
		})
	}
}

func assertEvaluationRouteEvidenceRow(
	t *testing.T,
	ctx context.Context,
	traceID string,
	wantTransport service.RouteEvidence,
	wantUsage service.RouteUsageEvidence,
) {
	t.Helper()

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM evaluation_route_evidence WHERE route_trace_id = $1", traceID).Scan(&count))
	require.Equal(t, 1, count)

	var (
		runID, sampleID, requestID, requestedModel, resolvedModel, routeProfile string
		provider, channelRef, accountPoolRef, region, status, errorCode         string
		finishReason, billedAmount                                              string
		apiKeyID                                                                int64
		attempts, inputTokens, outputTokens, ttft, latency                      int
		fallbackJSON                                                            []byte
		startedAt, finishedAt                                                   time.Time
	)
	err := integrationDB.QueryRowContext(ctx, `
		SELECT evaluation_run_id::text, sample_id::text, api_key_id,
			request_id, requested_model, resolved_model, route_profile_version,
			provider, channel_ref, account_pool_ref, region, attempts, fallback_chain,
			finish_reason, input_tokens, output_tokens, ttft_ms, latency_ms,
			billed_amount::text, transport_status, COALESCE(error_code, ''), started_at, finished_at
		FROM evaluation_route_evidence
		WHERE route_trace_id = $1`, traceID).Scan(
		&runID, &sampleID, &apiKeyID,
		&requestID, &requestedModel, &resolvedModel, &routeProfile,
		&provider, &channelRef, &accountPoolRef, &region, &attempts, &fallbackJSON,
		&finishReason, &inputTokens, &outputTokens, &ttft, &latency,
		&billedAmount, &status, &errorCode, &startedAt, &finishedAt,
	)
	require.NoError(t, err)

	var fallback []service.RouteFallbackEntry
	require.NoError(t, json.Unmarshal(fallbackJSON, &fallback))
	require.Equal(t, wantTransport.EvaluationRunID, runID)
	require.Equal(t, wantTransport.SampleID, sampleID)
	require.Equal(t, wantTransport.APIKeyID, apiKeyID)
	require.Equal(t, wantTransport.RequestID, requestID)
	require.Equal(t, wantTransport.RequestedModel, requestedModel)
	require.Equal(t, wantTransport.ResolvedModel, resolvedModel)
	require.Equal(t, wantTransport.RouteProfileVersion, routeProfile)
	require.Equal(t, wantTransport.Provider, provider)
	require.Equal(t, wantTransport.ChannelRef, channelRef)
	require.Equal(t, wantTransport.AccountPoolRef, accountPoolRef)
	require.Equal(t, wantTransport.Region, region)
	require.Equal(t, wantTransport.Attempts, attempts)
	require.Equal(t, wantTransport.FallbackChain, fallback)
	require.Equal(t, wantUsage.FinishReason, finishReason)
	require.Equal(t, wantUsage.InputTokens, inputTokens)
	require.Equal(t, wantUsage.OutputTokens, outputTokens)
	require.Equal(t, *wantUsage.TTFT, ttft)
	require.Equal(t, *wantUsage.Latency, latency)
	require.Equal(t, wantUsage.BilledAmount.StringFixed(8), billedAmount, fmt.Sprintf("stored amount %s", billedAmount))
	require.Equal(t, wantTransport.TransportStatus, status)
	require.Equal(t, wantTransport.ErrorCode, errorCode)
	require.Equal(t, wantTransport.StartedAt, startedAt)
	require.Equal(t, *wantTransport.FinishedAt, finishedAt)
}
