package upstreamstation

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newRepositoryMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, NewRepository(db)
}

func TestRepositoryCreateStation(t *testing.T) {
	db, mock, repo := newRepositoryMock(t)
	_ = db

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("INSERT INTO upstream_stations").
		WithArgs("Alpha", SiteTypeSub2API, "https://alpha.example", CredentialModePassword, "cipher", 2.0, RechargeSourceManual, true, true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(7), now, now))

	station, err := repo.CreateStation(context.Background(), &Station{
		Name:               "Alpha",
		SiteType:           SiteTypeSub2API,
		BaseURL:            "https://alpha.example",
		CredentialMode:     CredentialModePassword,
		CredentialCipher:   "cipher",
		RechargeMultiplier: 2,
		RechargeSource:     RechargeSourceManual,
		Enabled:            true,
		AutoSync:           true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), station.ID)
	require.Equal(t, now, station.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryListRoutesDecodesModels(t *testing.T) {
	_, mock, repo := newRepositoryMock(t)

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	models, err := json.Marshal([]string{"gpt-5", "o3"})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT (.+) FROM upstream_routes").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "station_id", "remote_group_key", "remote_group_name", "platform", "models",
			"group_rate", "recharge_multiplier", "effective_rate", "fixed_route", "remote_api_key_id",
			"api_key_cipher", "managed_account_id", "schedulable", "health_status", "last_error",
			"last_test_at", "last_sync_at", "created_at", "updated_at",
		}).AddRow(int64(3), int64(7), "11", "cheap", "openai", models, 0.8, 2.0, 0.4, false, "99", "secret", int64(21), true, HealthStatusHealthy, "", nil, now, now, now))

	routes, err := repo.ListRoutes(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Equal(t, []string{"gpt-5", "o3"}, routes[0].Models)
	require.Equal(t, 0.4, routes[0].EffectiveRate)
	require.Equal(t, int64(21), *routes[0].ManagedAccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStationJSONOmitsCredentialCipher(t *testing.T) {
	body, err := json.Marshal(Station{CredentialCipher: "top-secret"})
	require.NoError(t, err)
	require.NotContains(t, string(body), "top-secret")
	require.NotContains(t, string(body), "credential_cipher")
}
