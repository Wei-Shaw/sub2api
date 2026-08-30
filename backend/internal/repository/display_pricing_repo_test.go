package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestDisplayPricingRepositoryProviderCRUD(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewDisplayPricingRepository(db)
	ctx := context.Background()
	now := time.Now()
	rate := 0.125

	mock.ExpectQuery(`INSERT INTO display_pricing_providers`).
		WithArgs("deepseek", "DeepSeek", "Peak hour note", "Request note", "Image note", "CNY", rate, "deepseek", "/logos/deepseek.svg", 20).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
	provider := &service.DisplayPricingProvider{
		Provider: "deepseek", DisplayName: "DeepSeek", ProviderNote: "Peak hour note",
		PerRequestNote: "Request note", ImageNote: "Image note", Currency: "CNY", Multiplier: &rate,
		LogoKey: "deepseek", LogoURL: "/logos/deepseek.svg", SortOrder: 20,
	}
	require.NoError(t, repo.CreateProvider(ctx, provider))
	require.Equal(t, now, provider.UpdatedAt)

	mock.ExpectQuery(`UPDATE display_pricing_providers`).
		WithArgs("deepseek", "DeepSeek AI", "Updated note", "Updated request note", "Updated image note", "CNY", rate, "deepseek", "https://cdn.example.com/deepseek.svg", 21).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now.Add(time.Second)))
	provider.DisplayName = "DeepSeek AI"
	provider.ProviderNote = "Updated note"
	provider.PerRequestNote = "Updated request note"
	provider.ImageNote = "Updated image note"
	provider.LogoURL = "https://cdn.example.com/deepseek.svg"
	provider.SortOrder = 21
	require.NoError(t, repo.UpdateProvider(ctx, provider))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT provider FROM display_pricing_providers`).
		WithArgs("deepseek").
		WillReturnRows(sqlmock.NewRows([]string{"provider"}).AddRow("deepseek"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM display_model_prices`).
		WithArgs("deepseek").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	mock.ExpectExec(`DELETE FROM display_pricing_providers`).
		WithArgs("deepseek").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	deleted, err := repo.DeleteProvider(ctx, "deepseek")
	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisplayPricingRepositoryProviderConflictAndNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewDisplayPricingRepository(db)
	ctx := context.Background()
	provider := &service.DisplayPricingProvider{Provider: "custom", DisplayName: "Custom", Currency: "USD"}

	mock.ExpectQuery(`INSERT INTO display_pricing_providers`).
		WithArgs("custom", "Custom", "", "", "", "USD", nil, "", "", 0).
		WillReturnError(sql.ErrNoRows)
	require.ErrorIs(t, repo.CreateProvider(ctx, provider), service.ErrDisplayProviderExists)

	mock.ExpectQuery(`UPDATE display_pricing_providers`).
		WithArgs("custom", "Custom", "", "", "", "USD", nil, "", "", 0).
		WillReturnError(sql.ErrNoRows)
	require.ErrorIs(t, repo.UpdateProvider(ctx, provider), service.ErrDisplayProviderNotFound)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT provider FROM display_pricing_providers`).
		WithArgs("custom").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err = repo.DeleteProvider(ctx, "custom")
	require.ErrorIs(t, err, service.ErrDisplayProviderNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisplayPricingRepositoryListProvidersIncludesLogoAndModeNotes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now()
	mock.ExpectQuery(`SELECT provider, display_name, provider_note, per_request_note, image_note, currency, multiplier, logo_key, logo_url`).
		WillReturnRows(sqlmock.NewRows([]string{"provider", "display_name", "provider_note", "per_request_note", "image_note", "currency", "multiplier", "logo_key", "logo_url", "sort_order", "updated_at"}).
			AddRow("moonshot", "Kimi", "Token note", "Request note", "Image note", "CNY", 0.125, "kimi", "/logos/kimi.svg", 40, now))

	providers, err := NewDisplayPricingRepository(db).ListProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, "kimi", providers[0].LogoKey)
	require.Equal(t, "/logos/kimi.svg", providers[0].LogoURL)
	require.Equal(t, "Token note", providers[0].ProviderNote)
	require.Equal(t, "Request note", providers[0].PerRequestNote)
	require.Equal(t, "Image note", providers[0].ImageNote)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisplayPricingRepositoryApplyOfficialPricesIsAtomicAndScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := decimal.RequireFromString("1.6000")
	output := decimal.RequireFromString("4.7000")
	updates := []service.OfficialPriceUpdate{{
		ModelID: 9, ExpectedUpdatedAt: now, InputPerMillion: &input, OutputPerMillion: &output,
		OfficialPriceSource:    service.OfficialPriceSourceHerohaoAggregate,
		OfficialPriceSourceURL: service.HerohaoOfficialPriceCandidateURL, OfficialPriceSyncedAt: now.Add(time.Minute),
	}}

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE display_model_prices SET\s+official_input_per_million=\$1,\s+official_output_per_million=\$2,\s+official_cache_write_per_million=\$3,\s+official_cache_read_per_million=\$4,\s+official_price_source=\$5,\s+official_price_source_url=\$6,\s+official_price_synced_at=\$7,\s+updated_at=NOW\(\)\s+WHERE id=\$8 AND billing_mode='token' AND currency='CNY' AND updated_at=\$9`).
		WithArgs("1.60000000", "4.70000000", nil, nil, service.OfficialPriceSourceHerohaoAggregate,
			service.HerohaoOfficialPriceCandidateURL, now.Add(time.Minute), int64(9), now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectCommit()
	require.NoError(t, NewDisplayPricingRepository(db).(*displayPricingRepository).ApplyOfficialPriceUpdates(context.Background(), updates))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisplayPricingRepositoryOfficialPriceConflictRollsBackWholeBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := decimal.RequireFromString("1")
	updates := []service.OfficialPriceUpdate{
		{ModelID: 1, ExpectedUpdatedAt: now, InputPerMillion: &input, OfficialPriceSource: service.OfficialPriceSourceHerohaoAggregate, OfficialPriceSourceURL: service.HerohaoOfficialPriceCandidateURL, OfficialPriceSyncedAt: now},
		{ModelID: 2, ExpectedUpdatedAt: now, InputPerMillion: &input, OfficialPriceSource: service.OfficialPriceSourceHerohaoAggregate, OfficialPriceSourceURL: service.HerohaoOfficialPriceCandidateURL, OfficialPriceSyncedAt: now},
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE display_model_prices`).
		WithArgs("1.00000000", nil, nil, nil, service.OfficialPriceSourceHerohaoAggregate, service.HerohaoOfficialPriceCandidateURL, now, int64(1), now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectQuery(`UPDATE display_model_prices`).
		WithArgs("1.00000000", nil, nil, nil, service.OfficialPriceSourceHerohaoAggregate, service.HerohaoOfficialPriceCandidateURL, now, int64(2), now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	err = NewDisplayPricingRepository(db).(*displayPricingRepository).ApplyOfficialPriceUpdates(context.Background(), updates)
	require.ErrorIs(t, err, service.ErrOfficialPriceApplyConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
