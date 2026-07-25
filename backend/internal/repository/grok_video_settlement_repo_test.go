//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetGrokVideoSettlementForOwnerScansBillingSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	rows := sqlmock.NewRows([]string{
		"id", "request_id", "request_fingerprint", "user_id", "api_key_id", "group_id",
		"account_id", "account_type", "subscription_id", "requested_model", "billing_model",
		"upstream_model", "input_tokens", "output_tokens", "cache_creation_tokens",
		"cache_read_tokens", "image_input_tokens", "image_output_tokens", "video_resolution",
		"video_duration_seconds", "request_duration_ms", "request_payload_hash",
		"inbound_endpoint", "upstream_endpoint", "user_agent", "ip_address", "quota_platform",
		"channel_id", "channel_mapped_model", "billing_model_source", "model_mapping_chain",
		"pricing_snapshot_version", "pricing_basis", "billing_mode", "billing_type",
		"input_cost", "image_input_cost", "output_cost", "image_output_cost",
		"cache_creation_cost", "cache_read_cost", "total_cost", "actual_cost",
		"rate_multiplier", "account_rate_multiplier", "long_context_billing_applied", "account_stats_cost",
		"status", "terminal_at", "settled_at", "created_at", "updated_at",
	}).AddRow(
		int64(1), "video-request-1", "fingerprint", int64(11), int64(12), int64(7),
		int64(13), service.AccountTypeOAuth, nil, "grok-imagine-video", "grok-imagine-video",
		"vendor-video", 1, 2, 3, 4, 5, 6, service.VideoBillingResolution720P,
		10, int64(1250), "payload-hash", "/v1/videos/generations", "/v1/videos/generations",
		"test-agent", "192.0.2.1", service.PlatformGrok, int64(9), "mapped-video",
		service.BillingModelSourceChannelMapped, "requested->mapped",
		service.GrokVideoPricingSnapshotVersion, service.GrokVideoPricingBasisVideoSecond,
		string(service.BillingModeVideo), service.BillingTypeBalance,
		0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 1.25, 0.9, false, nil,
		service.GrokVideoSettlementStatusPending,
		nil, nil, now, now,
	)
	mock.ExpectQuery(`(?s)SELECT .* FROM grok_video_settlements.*WHERE request_id = \$1 AND user_id = \$2 AND api_key_id = \$3`).
		WithArgs("video-request-1", int64(11), int64(12)).
		WillReturnRows(rows)

	repo := &usageBillingRepository{db: db}
	got, err := repo.GetGrokVideoSettlementForOwner(context.Background(), "video-request-1", 11, 12)

	require.NoError(t, err)
	require.Equal(t, int64(13), got.AccountID)
	require.Equal(t, service.AccountTypeOAuth, got.AccountType)
	require.Equal(t, service.VideoBillingResolution720P, got.VideoResolution)
	require.Equal(t, 10, got.VideoDurationSeconds)
	require.Equal(t, 1250*time.Millisecond, got.RequestDuration)
	require.Equal(t, int64(9), got.ChannelID)
	require.Equal(t, "mapped-video", got.ChannelMappedModel)
	require.Equal(t, service.BillingModelSourceChannelMapped, got.BillingModelSource)
	require.Equal(t, service.GrokVideoPricingBasisVideoSecond, got.PricingBasis)
	require.InDelta(t, 0.8, got.ActualCost, 1e-12)
	require.InDelta(t, 1.25, got.RateMultiplier, 1e-12)
	require.Nil(t, got.SubscriptionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkGrokVideoSettlementTerminalOnlyUpdatesPendingRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(`(?s)UPDATE grok_video_settlements.*WHERE id = \$1 AND status = 'pending'`).
		WithArgs(int64(1), service.GrokVideoSettlementStatusFailed).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := &usageBillingRepository{db: db}

	err = repo.MarkGrokVideoSettlementTerminal(context.Background(), 1, "failure")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingRepositoryAlsoProvidesGrokVideoSettlementRepository(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUsageBillingRepository(nil, db)
	_, ok := repo.(service.GrokVideoSettlementRepository)
	require.True(t, ok)
}

func TestMarkGrokVideoSettlementSettledTxRequiresMatchingPendingRow(t *testing.T) {
	subscriptionID := int64(14)
	command := func() *service.UsageBillingCommand {
		return &service.UsageBillingCommand{
			GrokVideoSettlementID: 1,
			UserID:                11,
			APIKeyID:              12,
			AccountID:             13,
			SubscriptionID:        &subscriptionID,
		}
	}
	t.Run("pending row is settled", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		mock.ExpectExec(`(?s)UPDATE grok_video_settlements.*WHERE id = \$1 AND user_id = \$2 AND api_key_id = \$3.*account_id = \$4 AND subscription_id IS NOT DISTINCT FROM \$5.*status = 'pending'`).
			WithArgs(int64(1), int64(11), int64(12), int64(13), subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()

		require.NoError(t, markGrokVideoSettlementSettledTx(context.Background(), tx, command()))
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("subscription mismatch rejects billing transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		mock.ExpectExec(`(?s)UPDATE grok_video_settlements.*WHERE id = \$1 AND user_id = \$2 AND api_key_id = \$3.*account_id = \$4 AND subscription_id IS NOT DISTINCT FROM \$5.*status = 'pending'`).
			WithArgs(int64(1), int64(11), int64(12), int64(13), subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		err = markGrokVideoSettlementSettledTx(context.Background(), tx, command())
		require.ErrorIs(t, err, service.ErrGrokVideoSettlementNotPending)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
