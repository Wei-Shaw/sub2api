package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type evaluationRouteEvidenceRepository struct {
	sql sqlExecutor
}

func NewEvaluationRouteEvidenceRepository(db *sql.DB) service.EvaluationEvidenceRepository {
	return &evaluationRouteEvidenceRepository{sql: db}
}

func (r *evaluationRouteEvidenceRepository) UpsertTransport(ctx context.Context, evidence service.RouteEvidence) error {
	fallbackChain, err := json.Marshal(evidence.FallbackChain)
	if err != nil {
		return fmt.Errorf("marshal route fallback chain: %w", err)
	}

	result, err := r.sql.ExecContext(ctx, `
		INSERT INTO evaluation_route_evidence (
			route_trace_id, evaluation_run_id, sample_id, api_key_id, request_id,
			requested_model, resolved_model, route_profile_version, provider,
			channel_ref, account_pool_ref, region, attempts, fallback_chain,
			transport_status, error_code, started_at, finished_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''),
			$6, NULLIF($7, ''), $8, NULLIF($9, ''),
			NULLIF($10, ''), NULLIF($11, ''), $12, $13, $14,
			$15, NULLIF($16, ''), $17, $18
		)
		ON CONFLICT (route_trace_id) DO UPDATE SET
			request_id = EXCLUDED.request_id,
			requested_model = EXCLUDED.requested_model,
			resolved_model = EXCLUDED.resolved_model,
			route_profile_version = EXCLUDED.route_profile_version,
			provider = EXCLUDED.provider,
			channel_ref = EXCLUDED.channel_ref,
			account_pool_ref = EXCLUDED.account_pool_ref,
			region = EXCLUDED.region,
			attempts = EXCLUDED.attempts,
			fallback_chain = EXCLUDED.fallback_chain,
			transport_status = EXCLUDED.transport_status,
			error_code = EXCLUDED.error_code,
			started_at = EXCLUDED.started_at,
			finished_at = EXCLUDED.finished_at,
			updated_at = NOW()
		WHERE evaluation_route_evidence.evaluation_run_id = EXCLUDED.evaluation_run_id
			AND evaluation_route_evidence.sample_id = EXCLUDED.sample_id
			AND evaluation_route_evidence.api_key_id = EXCLUDED.api_key_id`,
		evidence.RouteTraceID,
		evidence.EvaluationRunID,
		evidence.SampleID,
		evidence.APIKeyID,
		evidence.RequestID,
		evidence.RequestedModel,
		evidence.ResolvedModel,
		evidence.RouteProfileVersion,
		evidence.Provider,
		evidence.ChannelRef,
		evidence.AccountPoolRef,
		evidence.Region,
		evidence.Attempts,
		fallbackChain,
		evidence.TransportStatus,
		evidence.ErrorCode,
		evidence.StartedAt,
		evidence.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert route transport evidence: %w", err)
	}
	return r.checkIdentityConflict(ctx, result, evidence.RouteTraceID, evidence.EvaluationRunID, evidence.SampleID, evidence.APIKeyID)
}

func (r *evaluationRouteEvidenceRepository) AttachBilling(ctx context.Context, traceID string, usage service.RouteUsageEvidence) error {
	evaluation, ok := service.EvaluationContextFromContext(ctx)
	if !ok {
		return fmt.Errorf("attach route billing evidence: evaluation context missing")
	}

	result, err := r.sql.ExecContext(ctx, `
		INSERT INTO evaluation_route_evidence (
			route_trace_id, evaluation_run_id, sample_id, api_key_id,
			requested_model, route_profile_version, region, started_at,
			input_tokens, output_tokens, ttft_ms, latency_ms, billed_amount, finish_reason,
			transport_status
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12, $13, NULLIF($14, ''),
			'started'
		)
		ON CONFLICT (route_trace_id) DO UPDATE SET
			input_tokens = COALESCE(EXCLUDED.input_tokens, evaluation_route_evidence.input_tokens),
			output_tokens = COALESCE(EXCLUDED.output_tokens, evaluation_route_evidence.output_tokens),
			ttft_ms = COALESCE(EXCLUDED.ttft_ms, evaluation_route_evidence.ttft_ms),
			latency_ms = COALESCE(EXCLUDED.latency_ms, evaluation_route_evidence.latency_ms),
			billed_amount = COALESCE(EXCLUDED.billed_amount, evaluation_route_evidence.billed_amount),
			finish_reason = COALESCE(EXCLUDED.finish_reason, evaluation_route_evidence.finish_reason),
			updated_at = NOW()
		WHERE evaluation_route_evidence.evaluation_run_id = EXCLUDED.evaluation_run_id
			AND evaluation_route_evidence.sample_id = EXCLUDED.sample_id
			AND evaluation_route_evidence.api_key_id = EXCLUDED.api_key_id`,
		traceID,
		evaluation.RunID,
		evaluation.SampleID,
		evaluation.APIKeyID,
		evaluation.ExpectedModelAlias,
		evaluation.ExpectedRouteProfile,
		"",
		evaluation.IssuedAt,
		usage.InputTokens,
		usage.OutputTokens,
		usage.TTFT,
		usage.Latency,
		usage.BilledAmount,
		usage.FinishReason,
	)
	if err != nil {
		return fmt.Errorf("attach route billing evidence: %w", err)
	}
	return r.checkIdentityConflict(ctx, result, traceID, evaluation.RunID, evaluation.SampleID, evaluation.APIKeyID)
}

func (r *evaluationRouteEvidenceRepository) checkIdentityConflict(
	ctx context.Context,
	result sql.Result,
	traceID string,
	runID string,
	sampleID string,
	apiKeyID int64,
) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read route evidence upsert result: %w", err)
	}
	if affected != 0 {
		return nil
	}

	var existingRunID string
	var existingSampleID string
	var existingAPIKeyID int64
	rows, err := r.sql.QueryContext(ctx, `
		SELECT evaluation_run_id::text, sample_id::text, api_key_id
		FROM evaluation_route_evidence
		WHERE route_trace_id = $1`, traceID)
	if err != nil {
		return fmt.Errorf("%w: load route evidence identity: %w", service.ErrRouteEvidenceIdentityConflict, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("%w: load route evidence identity: %w", service.ErrRouteEvidenceIdentityConflict, err)
		}
		return fmt.Errorf("%w: route evidence disappeared after guarded upsert: %w", service.ErrRouteEvidenceIdentityConflict, sql.ErrNoRows)
	}
	if err := rows.Scan(&existingRunID, &existingSampleID, &existingAPIKeyID); err != nil {
		return fmt.Errorf("%w: scan route evidence identity: %w", service.ErrRouteEvidenceIdentityConflict, err)
	}
	if existingRunID != runID || existingSampleID != sampleID || existingAPIKeyID != apiKeyID {
		return fmt.Errorf("%w: route_trace_id %q", service.ErrRouteEvidenceIdentityConflict, traceID)
	}
	return fmt.Errorf("%w: guarded upsert affected no rows for matching identity %q", service.ErrRouteEvidenceIdentityConflict, traceID)
}
