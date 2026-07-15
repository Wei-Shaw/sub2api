package upstreamstation

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateStationValidatesCredentialsForSelectedMode(t *testing.T) {
	tests := []struct {
		name           string
		siteType       string
		credentialMode string
		credentials    *Credentials
		wantError      string
	}{
		{
			name:           "password requires username",
			siteType:       SiteTypeSub2API,
			credentialMode: CredentialModePassword,
			credentials:    &Credentials{Password: "secret"},
			wantError:      "username is required for password credentials",
		},
		{
			name:           "password requires password",
			siteType:       SiteTypeNewAPI,
			credentialMode: CredentialModePassword,
			credentials:    &Credentials{Username: "boss@example.com"},
			wantError:      "password is required for password credentials",
		},
		{
			name:           "token requires access token or cookie",
			siteType:       SiteTypeSub2API,
			credentialMode: CredentialModeToken,
			credentials:    &Credentials{},
			wantError:      "access token or cookie is required for token credentials",
		},
		{
			name:           "newapi token requires user id",
			siteType:       SiteTypeNewAPI,
			credentialMode: CredentialModeToken,
			credentials:    &Credentials{AccessToken: "token"},
			wantError:      "user_id is required for newapi token credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, repository := newRepositoryMock(t)
			service := &Service{
				repository: repository,
				codec:      NewCredentialCodec(testEncryptor{}),
			}

			_, err := service.CreateStation(context.Background(), StationInput{
				Name:               "Alpha",
				SiteType:           tt.siteType,
				BaseURL:            "https://alpha.example",
				CredentialMode:     tt.credentialMode,
				Credentials:        tt.credentials,
				RechargeMultiplier: 1,
			})

			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func TestUpdateStationMergesPartialCredentials(t *testing.T) {
	_, mock, repository := newRepositoryMock(t)
	codec := NewCredentialCodec(testEncryptor{})
	original := Credentials{
		Username:     "boss@example.com",
		Password:     "old-password",
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		Cookie:       "session=old",
		UserID:       "17",
		Extra:        map[string]any{"tenant": "alpha", "region": "cn"},
	}
	originalCipher, err := codec.Encrypt(original)
	require.NoError(t, err)
	want := original
	want.AccessToken = "new-access-token"
	want.Extra = map[string]any{"tenant": "alpha", "region": "us"}
	wantCipher, err := codec.Encrypt(want)
	require.NoError(t, err)

	mock.ExpectQuery("SELECT (.+) FROM upstream_stations").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestStationRows(7, CredentialModeToken, originalCipher))
	mock.ExpectExec("UPDATE upstream_stations SET").
		WithArgs(
			int64(7), "Alpha", SiteTypeSub2API, "https://alpha.example", CredentialModeToken,
			wantCipher, 1.0, RechargeSourceManual, true, true,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT (.+) FROM upstream_stations").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestStationRows(7, CredentialModeToken, wantCipher))

	service := &Service{repository: repository, codec: codec}
	_, err = service.UpdateStation(context.Background(), 7, StationUpdateInput{
		Credentials: &Credentials{
			AccessToken: "new-access-token",
			Extra:       map[string]any{"region": "us"},
		},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateStationValidatesCredentialsWhenModeChanges(t *testing.T) {
	_, mock, repository := newRepositoryMock(t)
	codec := NewCredentialCodec(testEncryptor{})
	originalCipher, err := codec.Encrypt(Credentials{Username: "boss@example.com", Password: "secret"})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT (.+) FROM upstream_stations").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestStationRows(7, CredentialModePassword, originalCipher))

	mode := CredentialModeToken
	service := &Service{repository: repository, codec: codec}
	_, err = service.UpdateStation(context.Background(), 7, StationUpdateInput{CredentialMode: &mode})

	require.ErrorContains(t, err, "access token or cookie is required for token credentials")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetRouteSchedulableKeepsUnhealthyManagedAccountDisabled(t *testing.T) {
	_, mock, repository := newRepositoryMock(t)
	admin := &schedulableTestAdmin{}
	mock.ExpectQuery("SELECT (.+) FROM upstream_routes").
		WithArgs(int64(9)).
		WillReturnRows(serviceTestRouteRows(9, HealthStatusError, int64(42)))
	mock.ExpectExec("UPDATE upstream_routes SET schedulable").
		WithArgs(int64(9), true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	service := &Service{repository: repository, admin: admin}
	err := service.SetRouteSchedulable(context.Background(), 9, true)

	require.NoError(t, err)
	require.Equal(t, []bool{false}, admin.values)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateRouteAppendsSnapshotWhenRechargeMultiplierChanges(t *testing.T) {
	_, mock, repository := newRepositoryMock(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	routeRows := func(multiplier, effective float64) *sqlmock.Rows {
		return sqlmock.NewRows([]string{
			"id", "station_id", "remote_group_key", "remote_group_name", "platform", "models",
			"group_rate", "recharge_multiplier", "effective_rate", "fixed_route", "remote_api_key_id",
			"api_key_cipher", "managed_account_id", "schedulable", "health_status", "last_error",
			"last_test_at", "last_sync_at", "created_at", "updated_at",
		}).AddRow(
			int64(9), int64(7), "cheap", "Cheap", coreservice.PlatformOpenAI, []byte(`["gpt-5"]`),
			0.5, multiplier, effective, false, "", "cipher", nil, true, HealthStatusHealthy, "",
			nil, now, now, now,
		)
	}

	mock.ExpectQuery("SELECT (.+) FROM upstream_routes").
		WithArgs(int64(9)).
		WillReturnRows(routeRows(1, 0.5))
	mock.ExpectQuery("INSERT INTO upstream_routes").
		WithArgs(
			int64(7), "cheap", "Cheap", coreservice.PlatformOpenAI, []byte(`["gpt-5"]`),
			1.0, 2.0, 0.5, false, "", "cipher", nil, true, HealthStatusHealthy, "",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "last_sync_at"}).AddRow(int64(9), now, now, now))
	mock.ExpectExec("INSERT INTO upstream_rate_snapshots").
		WithArgs(int64(9), 1.0, 2.0, 0.5, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	service := &Service{repository: repository}
	multiplier := 2.0
	groupRate := 1.0
	_, err := service.UpdateRoute(context.Background(), 9, RouteUpdateInput{
		GroupRate: &groupRate, RechargeMultiplier: &multiplier,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateStationPropagatesRechargeMultiplierToExistingRoutes(t *testing.T) {
	_, mock, repository := newRepositoryMock(t)
	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{AccessToken: "token"})
	require.NoError(t, err)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT (.+) FROM upstream_stations").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestStationRowsWithMultiplier(7, CredentialModeToken, cipher, 1))
	mock.ExpectExec("UPDATE upstream_stations SET").
		WithArgs(
			int64(7), "Alpha", SiteTypeSub2API, "https://alpha.example", CredentialModeToken,
			cipher, 2.0, RechargeSourceManual, true, true,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT (.+) FROM upstream_routes").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestRouteRows(9, HealthStatusHealthy, nil))
	mock.ExpectQuery("SELECT (.+) FROM upstream_routes").
		WithArgs(int64(9)).
		WillReturnRows(serviceTestRouteRows(9, HealthStatusHealthy, nil))
	mock.ExpectQuery("INSERT INTO upstream_routes").
		WithArgs(
			int64(7), "cheap", "Cheap", coreservice.PlatformOpenAI, []byte(`["gpt-5"]`),
			0.5, 2.0, 0.25, false, "", "cipher", nil, false, HealthStatusHealthy, "unhealthy",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "last_sync_at"}).AddRow(int64(9), now, now, now))
	mock.ExpectExec("INSERT INTO upstream_rate_snapshots").
		WithArgs(int64(9), 0.5, 2.0, 0.25, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT (.+) FROM upstream_stations").
		WithArgs(int64(7)).
		WillReturnRows(serviceTestStationRowsWithMultiplier(7, CredentialModeToken, cipher, 2))

	service := &Service{repository: repository, codec: codec}
	multiplier := 2.0
	station, err := service.UpdateStation(context.Background(), 7, StationUpdateInput{RechargeMultiplier: &multiplier})

	require.NoError(t, err)
	require.Equal(t, 2.0, station.RechargeMultiplier)
	require.NoError(t, mock.ExpectationsWereMet())
}

func serviceTestStationRows(id int64, credentialMode, credentialCipher string) *sqlmock.Rows {
	return serviceTestStationRowsWithMultiplier(id, credentialMode, credentialCipher, 1)
}

func serviceTestStationRowsWithMultiplier(id int64, credentialMode, credentialCipher string, multiplier float64) *sqlmock.Rows {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "name", "site_type", "base_url", "credential_mode", "credential_cipher",
		"recharge_multiplier", "recharge_source", "balance", "enabled", "auto_sync", "health_status",
		"last_error", "last_sync_at", "last_test_at", "created_at", "updated_at",
	}).AddRow(
		id, "Alpha", SiteTypeSub2API, "https://alpha.example", credentialMode, credentialCipher,
		multiplier, RechargeSourceManual, nil, true, true, HealthStatusUnknown,
		"", nil, nil, now, now,
	)
}

func serviceTestRouteRows(id int64, healthStatus string, managedAccountID any) *sqlmock.Rows {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "station_id", "remote_group_key", "remote_group_name", "platform", "models",
		"group_rate", "recharge_multiplier", "effective_rate", "fixed_route", "remote_api_key_id",
		"api_key_cipher", "managed_account_id", "schedulable", "health_status", "last_error",
		"last_test_at", "last_sync_at", "created_at", "updated_at",
	}).AddRow(
		id, int64(7), "cheap", "Cheap", coreservice.PlatformOpenAI, []byte(`["gpt-5"]`),
		0.5, 1.0, 0.5, false, "", "cipher", managedAccountID, false, healthStatus, "unhealthy",
		nil, now, now, now,
	)
}

type schedulableTestAdmin struct {
	values []bool
}

func (*schedulableTestAdmin) GetAllGroups(context.Context) ([]coreservice.Group, error) {
	return nil, nil
}

func (*schedulableTestAdmin) CreateGroup(context.Context, *coreservice.CreateGroupInput) (*coreservice.Group, error) {
	return nil, nil
}

func (*schedulableTestAdmin) GetAccount(context.Context, int64) (*coreservice.Account, error) {
	return nil, nil
}

func (*schedulableTestAdmin) CreateAccount(context.Context, *coreservice.CreateAccountInput) (*coreservice.Account, error) {
	return nil, nil
}

func (*schedulableTestAdmin) UpdateAccount(context.Context, int64, *coreservice.UpdateAccountInput) (*coreservice.Account, error) {
	return nil, nil
}

func (a *schedulableTestAdmin) SetAccountSchedulable(_ context.Context, id int64, schedulable bool) (*coreservice.Account, error) {
	a.values = append(a.values, schedulable)
	return &coreservice.Account{ID: id, Schedulable: schedulable}, nil
}
