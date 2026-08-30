package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
		WithArgs("deepseek", "DeepSeek", "Peak hour note", "CNY", rate, "deepseek", "/logos/deepseek.svg", 20).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
	provider := &service.DisplayPricingProvider{
		Provider: "deepseek", DisplayName: "DeepSeek", ProviderNote: "Peak hour note", Currency: "CNY", Multiplier: &rate,
		LogoKey: "deepseek", LogoURL: "/logos/deepseek.svg", SortOrder: 20,
	}
	require.NoError(t, repo.CreateProvider(ctx, provider))
	require.Equal(t, now, provider.UpdatedAt)

	mock.ExpectQuery(`UPDATE display_pricing_providers`).
		WithArgs("deepseek", "DeepSeek AI", "Updated note", "CNY", rate, "deepseek", "https://cdn.example.com/deepseek.svg", 21).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now.Add(time.Second)))
	provider.DisplayName = "DeepSeek AI"
	provider.ProviderNote = "Updated note"
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
		WithArgs("custom", "Custom", "", "USD", nil, "", "", 0).
		WillReturnError(sql.ErrNoRows)
	require.ErrorIs(t, repo.CreateProvider(ctx, provider), service.ErrDisplayProviderExists)

	mock.ExpectQuery(`UPDATE display_pricing_providers`).
		WithArgs("custom", "Custom", "", "USD", nil, "", "", 0).
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

func TestDisplayPricingRepositoryListProvidersIncludesLogoAndNote(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now()
	mock.ExpectQuery(`SELECT provider, display_name, provider_note, currency, multiplier, logo_key, logo_url`).
		WillReturnRows(sqlmock.NewRows([]string{"provider", "display_name", "provider_note", "currency", "multiplier", "logo_key", "logo_url", "sort_order", "updated_at"}).
			AddRow("moonshot", "Kimi", "Provider note", "CNY", 0.125, "kimi", "/logos/kimi.svg", 40, now))

	providers, err := NewDisplayPricingRepository(db).ListProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, "kimi", providers[0].LogoKey)
	require.Equal(t, "/logos/kimi.svg", providers[0].LogoURL)
	require.Equal(t, "Provider note", providers[0].ProviderNote)
	require.NoError(t, mock.ExpectationsWereMet())
}
