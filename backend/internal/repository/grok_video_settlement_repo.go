package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const grokVideoSettlementColumns = `
	id, request_id, request_fingerprint, user_id, api_key_id, group_id,
	account_id, account_type, subscription_id, requested_model, billing_model,
	upstream_model, input_tokens, output_tokens, cache_creation_tokens,
	cache_read_tokens, image_input_tokens, image_output_tokens, video_resolution,
	video_duration_seconds, request_duration_ms, request_payload_hash,
	inbound_endpoint, upstream_endpoint, user_agent, ip_address, session_id, quota_platform,
	channel_id, channel_mapped_model, billing_model_source, model_mapping_chain,
	pricing_snapshot_version, pricing_basis, billing_mode, billing_type,
	input_cost, image_input_cost, output_cost, image_output_cost,
	cache_creation_cost, cache_read_cost, total_cost, actual_cost,
	rate_multiplier, account_rate_multiplier, long_context_billing_applied,
	account_stats_cost,
	status, terminal_at, settled_at, created_at, updated_at`

type grokVideoSettlementScanner interface {
	Scan(dest ...any) error
}

func (r *usageBillingRepository) CreateGrokVideoSettlement(ctx context.Context, settlement *service.GrokVideoSettlement) (*service.GrokVideoSettlement, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("grok video settlement repository db is nil")
	}
	if settlement == nil {
		return nil, errors.New("grok video settlement is nil")
	}
	settlement.Normalize()

	created, err := scanGrokVideoSettlement(r.db.QueryRowContext(ctx, `
		INSERT INTO grok_video_settlements (
			request_id, request_fingerprint, user_id, api_key_id, group_id,
			account_id, account_type, subscription_id, requested_model, billing_model,
			upstream_model, input_tokens, output_tokens, cache_creation_tokens,
			cache_read_tokens, image_input_tokens, image_output_tokens, video_resolution,
			video_duration_seconds, request_duration_ms, request_payload_hash,
			inbound_endpoint, upstream_endpoint, user_agent, ip_address, session_id, quota_platform,
			channel_id, channel_mapped_model, billing_model_source, model_mapping_chain,
			pricing_snapshot_version, pricing_basis, billing_mode, billing_type,
			input_cost, image_input_cost, output_cost, image_output_cost,
			cache_creation_cost, cache_read_cost, total_cost, actual_cost,
			rate_multiplier, account_rate_multiplier, long_context_billing_applied,
			account_stats_cost,
			status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
			$31, $32, $33, $34, $35, $36, $37, $38, $39, $40,
			$41, $42, $43, $44, $45, $46, $47, $48
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING `+grokVideoSettlementColumns,
		settlement.RequestID,
		settlement.RequestFingerprint,
		settlement.UserID,
		settlement.APIKeyID,
		settlement.GroupID,
		settlement.AccountID,
		settlement.AccountType,
		settlement.SubscriptionID,
		settlement.RequestedModel,
		settlement.BillingModel,
		settlement.UpstreamModel,
		settlement.Usage.InputTokens,
		settlement.Usage.OutputTokens,
		settlement.Usage.CacheCreationInputTokens,
		settlement.Usage.CacheReadInputTokens,
		settlement.Usage.ImageInputTokens,
		settlement.Usage.ImageOutputTokens,
		settlement.VideoResolution,
		settlement.VideoDurationSeconds,
		settlement.RequestDuration.Milliseconds(),
		settlement.RequestPayloadHash,
		settlement.InboundEndpoint,
		settlement.UpstreamEndpoint,
		settlement.UserAgent,
		settlement.IPAddress,
		settlement.SessionID,
		settlement.QuotaPlatform,
		settlement.ChannelID,
		settlement.ChannelMappedModel,
		settlement.BillingModelSource,
		settlement.ModelMappingChain,
		settlement.PricingSnapshotVersion,
		settlement.PricingBasis,
		settlement.BillingMode,
		settlement.BillingType,
		settlement.InputCost,
		settlement.ImageInputCost,
		settlement.OutputCost,
		settlement.ImageOutputCost,
		settlement.CacheCreationCost,
		settlement.CacheReadCost,
		settlement.TotalCost,
		settlement.ActualCost,
		settlement.RateMultiplier,
		settlement.AccountRateMultiplier,
		settlement.LongContextBillingApplied,
		settlement.AccountStatsCost,
		service.GrokVideoSettlementStatusPending,
	))
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	existing, err := r.GetGrokVideoSettlementForOwner(ctx, settlement.RequestID, settlement.UserID, settlement.APIKeyID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(existing.RequestFingerprint, settlement.RequestFingerprint) {
		return nil, service.ErrGrokVideoSettlementConflict
	}
	return existing, nil
}

func (r *usageBillingRepository) GetGrokVideoSettlementForOwner(ctx context.Context, requestID string, userID, apiKeyID int64) (*service.GrokVideoSettlement, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("grok video settlement repository db is nil")
	}
	settlement, err := scanGrokVideoSettlement(r.db.QueryRowContext(ctx, `
		SELECT `+grokVideoSettlementColumns+`
		FROM grok_video_settlements
		WHERE request_id = $1 AND user_id = $2 AND api_key_id = $3
	`, strings.TrimSpace(requestID), userID, apiKeyID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGrokVideoSettlementNotFound
	}
	return settlement, err
}

func (r *usageBillingRepository) MarkGrokVideoSettlementTerminal(ctx context.Context, id int64, status string) error {
	if r == nil || r.db == nil {
		return errors.New("grok video settlement repository db is nil")
	}
	status = service.NormalizeGrokVideoUpstreamStatus(status)
	switch status {
	case service.GrokVideoUpstreamStatusFailed, service.GrokVideoUpstreamStatusExpired, service.GrokVideoSettlementStatusCancelled:
	default:
		return errors.New("invalid grok video terminal settlement status")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE grok_video_settlements
		SET status = $2, terminal_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id, status)
	if err != nil {
		return err
	}
	return r.validateGrokVideoSettlementUpdate(ctx, id, status, result)
}

func (r *usageBillingRepository) MarkGrokVideoSettlementSettled(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("grok video settlement repository db is nil")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE grok_video_settlements
		SET status = 'settled', terminal_at = COALESCE(terminal_at, NOW()),
			settled_at = COALESCE(settled_at, NOW()), updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return err
	}
	return r.validateGrokVideoSettlementUpdate(ctx, id, service.GrokVideoSettlementStatusSettled, result)
}

func (r *usageBillingRepository) validateGrokVideoSettlementUpdate(ctx context.Context, id int64, expectedStatus string, result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var currentStatus string
	err = r.db.QueryRowContext(ctx, `SELECT status FROM grok_video_settlements WHERE id = $1`, id).Scan(&currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGrokVideoSettlementNotFound
	}
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(currentStatus), strings.TrimSpace(expectedStatus)) {
		return nil
	}
	return service.ErrGrokVideoSettlementNotPending
}

func scanGrokVideoSettlement(scanner grokVideoSettlementScanner) (*service.GrokVideoSettlement, error) {
	settlement := &service.GrokVideoSettlement{}
	var groupID sql.NullInt64
	var subscriptionID sql.NullInt64
	var requestDurationMS int64
	var terminalAt sql.NullTime
	var settledAt sql.NullTime
	var accountStatsCost sql.NullFloat64
	err := scanner.Scan(
		&settlement.ID,
		&settlement.RequestID,
		&settlement.RequestFingerprint,
		&settlement.UserID,
		&settlement.APIKeyID,
		&groupID,
		&settlement.AccountID,
		&settlement.AccountType,
		&subscriptionID,
		&settlement.RequestedModel,
		&settlement.BillingModel,
		&settlement.UpstreamModel,
		&settlement.Usage.InputTokens,
		&settlement.Usage.OutputTokens,
		&settlement.Usage.CacheCreationInputTokens,
		&settlement.Usage.CacheReadInputTokens,
		&settlement.Usage.ImageInputTokens,
		&settlement.Usage.ImageOutputTokens,
		&settlement.VideoResolution,
		&settlement.VideoDurationSeconds,
		&requestDurationMS,
		&settlement.RequestPayloadHash,
		&settlement.InboundEndpoint,
		&settlement.UpstreamEndpoint,
		&settlement.UserAgent,
		&settlement.IPAddress,
		&settlement.SessionID,
		&settlement.QuotaPlatform,
		&settlement.ChannelID,
		&settlement.ChannelMappedModel,
		&settlement.BillingModelSource,
		&settlement.ModelMappingChain,
		&settlement.PricingSnapshotVersion,
		&settlement.PricingBasis,
		&settlement.BillingMode,
		&settlement.BillingType,
		&settlement.InputCost,
		&settlement.ImageInputCost,
		&settlement.OutputCost,
		&settlement.ImageOutputCost,
		&settlement.CacheCreationCost,
		&settlement.CacheReadCost,
		&settlement.TotalCost,
		&settlement.ActualCost,
		&settlement.RateMultiplier,
		&settlement.AccountRateMultiplier,
		&settlement.LongContextBillingApplied,
		&accountStatsCost,
		&settlement.Status,
		&terminalAt,
		&settledAt,
		&settlement.CreatedAt,
		&settlement.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if groupID.Valid {
		settlement.GroupID = &groupID.Int64
	}
	if subscriptionID.Valid {
		settlement.SubscriptionID = &subscriptionID.Int64
	}
	if terminalAt.Valid {
		value := terminalAt.Time
		settlement.TerminalAt = &value
	}
	if settledAt.Valid {
		value := settledAt.Time
		settlement.SettledAt = &value
	}
	if accountStatsCost.Valid {
		value := accountStatsCost.Float64
		settlement.AccountStatsCost = &value
	}
	settlement.RequestDuration = time.Duration(requestDurationMS) * time.Millisecond
	return settlement, nil
}

var _ service.GrokVideoSettlementRepository = (*usageBillingRepository)(nil)
