//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelServiceTierConfigJSON(t *testing.T) {
	defaults := service.DefaultChannelServiceTierConfig()
	data, err := marshalServiceTierConfig(defaults)
	require.NoError(t, err)

	decoded, configError := unmarshalServiceTierConfig(data)
	require.Empty(t, configError)
	require.Equal(t, defaults, decoded)

	decoded, configError = unmarshalServiceTierConfig(nil)
	require.Empty(t, configError)
	require.Equal(t, defaults, decoded)

	decoded, configError = unmarshalServiceTierConfig([]byte(`{"standard":{"enabled":true,"multiplier":1},"priority":{"enabled":true,"multiplier":2},"flex":{"enabled":true,"multiplier":0.5}}`))
	require.Empty(t, configError)
	require.True(t, decoded.UseOutboundTierForBilling, "legacy channel JSON must preserve the safe outbound-authoritative default")

	decoded, configError = unmarshalServiceTierConfig([]byte(`{"standard":{"enabled":true,"multiplier":1},"priority":{"enabled":true,"multiplier":2},"flex":{"enabled":true,"multiplier":0.5},"use_outbound_tier_for_billing":false}`))
	require.Empty(t, configError)
	require.False(t, decoded.UseOutboundTierForBilling, "an explicit false must remain distinguishable from an omitted legacy field")

	_, configError = unmarshalServiceTierConfig([]byte(`{"standard":`))
	require.Contains(t, configError, "decode service_tier_config")

	_, configError = unmarshalServiceTierConfig([]byte(`{"standard":{"enabled":false,"multiplier":1},"priority":{"enabled":false,"multiplier":2},"flex":{"enabled":false,"multiplier":0.5}}`))
	require.Contains(t, configError, "at least one")
}

func TestChannelUpdateReturnsTransactionLockedPreviousServiceTierConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	previous := service.DefaultChannelServiceTierConfig()
	previous.Priority.Multiplier = 2.5
	previousJSON, err := marshalServiceTierConfig(previous)
	require.NoError(t, err)
	after := service.DefaultChannelServiceTierConfig()
	after.Priority.Multiplier = 3
	revision := time.Date(2026, 7, 31, 12, 0, 0, 123456000, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT service_tier_config, updated_at FROM channels WHERE id = $1 FOR UPDATE`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"service_tier_config", "updated_at"}).AddRow(previousJSON, revision))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE channels SET name = $1, description = $2, status = $3, model_mapping = $4, billing_model_source = $5, restrict_models = $6, features = $7, features_config = $8, service_tier_config = $9, apply_pricing_to_account_stats = $10, updated_at = NOW()
			 WHERE id = $11`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := &channelRepository{db: db}
	auditSnapshot, err := repo.Update(context.Background(), &service.Channel{
		ID:                42,
		Name:              "channel",
		Status:            service.StatusActive,
		ServiceTierConfig: after,
		UpdatedAt:         revision,
	})
	require.NoError(t, err)
	require.Equal(t, previous, auditSnapshot.Before)
	require.Equal(t, after, auditSnapshot.After)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelUpdateRejectsStaleRevisionBeforeWriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	current := service.DefaultChannelServiceTierConfig()
	currentJSON, err := marshalServiceTierConfig(current)
	require.NoError(t, err)
	staleRevision := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	currentRevision := staleRevision.Add(time.Second)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT service_tier_config, updated_at FROM channels WHERE id = $1 FOR UPDATE`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"service_tier_config", "updated_at"}).AddRow(currentJSON, currentRevision))
	mock.ExpectRollback()

	repo := &channelRepository{db: db}
	auditSnapshot, err := repo.Update(context.Background(), &service.Channel{
		ID:                42,
		Name:              "stale-channel",
		Status:            service.StatusActive,
		ServiceTierConfig: current,
		UpdatedAt:         staleRevision,
	})
	require.ErrorIs(t, err, service.ErrChannelUpdateConflict)
	require.Equal(t, service.ChannelServiceTierAuditSnapshot{}, auditSnapshot)
	require.NoError(t, mock.ExpectationsWereMet())
}
