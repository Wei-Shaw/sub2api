package setup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestDecideAdminBootstrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		should     bool
		reason     string
	}{
		{
			name:       "empty database should create admin",
			totalUsers: 0,
			adminUsers: 0,
			should:     true,
			reason:     adminBootstrapReasonEmptyDatabase,
		},
		{
			name:       "admin exists should skip",
			totalUsers: 10,
			adminUsers: 1,
			should:     false,
			reason:     adminBootstrapReasonAdminExists,
		},
		{
			name:       "users exist without admin should skip",
			totalUsers: 5,
			adminUsers: 0,
			should:     false,
			reason:     adminBootstrapReasonUsersExistWithoutAdmin,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decideAdminBootstrap(tc.totalUsers, tc.adminUsers)
			if got.shouldCreate != tc.should {
				t.Fatalf("shouldCreate=%v, want %v", got.shouldCreate, tc.should)
			}
			if got.reason != tc.reason {
				t.Fatalf("reason=%q, want %q", got.reason, tc.reason)
			}
		})
	}
}

func TestSetupDefaultAdminConcurrency(t *testing.T) {
	t.Run("simple mode admin uses higher concurrency", func(t *testing.T) {
		t.Setenv("RUN_MODE", "simple")
		if got := setupDefaultAdminConcurrency(); got != simpleModeAdminConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, simpleModeAdminConcurrency)
		}
	})

	t.Run("standard mode keeps existing default", func(t *testing.T) {
		t.Setenv("RUN_MODE", "standard")
		if got := setupDefaultAdminConcurrency(); got != defaultUserConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, defaultUserConcurrency)
		}
	})
}

func TestNeedsSetupSkipsWhenSkipSetupIsEnabled(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "true", value: "true"},
		{name: "one", value: "1"},
		{name: "yes", value: "yes"},
		{name: "trimmed mixed case true", value: "  TrUe  "},
		{name: "trimmed mixed case yes", value: "  YeS  "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATA_DIR", t.TempDir())
			t.Setenv("SKIP_SETUP", tc.value)

			if NeedsSetup() {
				t.Fatalf("NeedsSetup() = true, want false when SKIP_SETUP is enabled")
			}
		})
	}
}

func TestNeedsSetupFallsBackToFileDetectionWhenSkipSetupIsDisabled(t *testing.T) {
	tests := []struct {
		name         string
		skipSetupSet bool
		skipSetup    string
		markerFile   string
		want         bool
	}{
		{
			name: "unset without installation files",
			want: true,
		},
		{
			name:         "false without installation files",
			skipSetupSet: true,
			skipSetup:    " false ",
			want:         true,
		},
		{
			name:         "invalid value without installation files",
			skipSetupSet: true,
			skipSetup:    "enabled",
			want:         true,
		},
		{
			name:         "config file exists",
			skipSetupSet: true,
			skipSetup:    "false",
			markerFile:   ConfigFileName,
			want:         false,
		},
		{
			name:         "install lock file exists",
			skipSetupSet: true,
			skipSetup:    "invalid",
			markerFile:   InstallLockFile,
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv("DATA_DIR", dataDir)
			if tc.skipSetupSet {
				t.Setenv("SKIP_SETUP", tc.skipSetup)
			} else {
				originalValue, wasSet := os.LookupEnv("SKIP_SETUP")
				if err := os.Unsetenv("SKIP_SETUP"); err != nil {
					t.Fatalf("Unsetenv(SKIP_SETUP) error = %v", err)
				}
				t.Cleanup(func() {
					if wasSet {
						_ = os.Setenv("SKIP_SETUP", originalValue)
						return
					}
					_ = os.Unsetenv("SKIP_SETUP")
				})
			}

			if tc.markerFile != "" {
				if err := os.WriteFile(filepath.Join(dataDir, tc.markerFile), nil, 0o600); err != nil {
					t.Fatalf("WriteFile(%s) error = %v", tc.markerFile, err)
				}
			}

			if got := NeedsSetup(); got != tc.want {
				t.Fatalf("NeedsSetup() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetupMigrationTimeout(t *testing.T) {
	t.Run("uses default timeout when unset", func(t *testing.T) {
		cfg := &SetupConfig{}
		if got := cfg.migrationTimeout(); got != 60*time.Second {
			t.Fatalf("migrationTimeout()=%s, want 60s", got)
		}
	})

	t.Run("uses configured timeout", func(t *testing.T) {
		cfg := &SetupConfig{MigrationTimeoutSeconds: 300}
		if got := cfg.migrationTimeout(); got != 300*time.Second {
			t.Fatalf("migrationTimeout()=%s, want 300s", got)
		}
	})
}

func TestWriteConfigFileKeepsDefaultUserConcurrency(t *testing.T) {
	t.Setenv("RUN_MODE", "simple")
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "user_concurrency: 5") {
		t.Fatalf("config missing default user concurrency, got:\n%s", string(data))
	}
}

func TestWriteConfigFileIncludesRedisUsername(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{
		Redis: RedisConfig{
			Host:     "redis",
			Port:     6379,
			Username: "app-user",
		},
	}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "username: app-user") {
		t.Fatalf("config missing Redis username, got:\n%s", string(data))
	}
}

func TestDatabaseConnectionDSNsUseConfiguredTargetAndLegacyBootstrapDatabase(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "db",
		Port:     5432,
		User:     "sub2api",
		Password: "secret",
		DBName:   "sub2api",
		SSLMode:  "disable",
	}

	targetDSN := buildPostgresDSN(cfg, cfg.DBName)
	bootstrapDSN := buildPostgresDSN(cfg, postgresBootstrapDatabase)

	if !strings.Contains(targetDSN, "dbname=sub2api") {
		t.Fatalf("target DSN = %q, want configured database", targetDSN)
	}
	if !strings.Contains(bootstrapDSN, "dbname=postgres") {
		t.Fatalf("bootstrap DSN = %q, want legacy postgres bootstrap database", bootstrapDSN)
	}
}

func TestIsDatabaseNotFoundError(t *testing.T) {
	if !isDatabaseNotFoundError(&pq.Error{Code: "3D000"}) {
		t.Fatal("isDatabaseNotFoundError() = false, want true for PostgreSQL invalid_catalog_name")
	}
	if isDatabaseNotFoundError(&pq.Error{Code: "28P01"}) {
		t.Fatal("isDatabaseNotFoundError() = true, want false for invalid_password")
	}
	if !isDatabaseNotFoundError(fmt.Errorf("wrapped: %w", &pq.Error{Code: "3D000"})) {
		t.Fatal("isDatabaseNotFoundError() = false, want true for wrapped PostgreSQL error")
	}
}

func TestDatabaseConnectionUsesConfiguredTargetBeforeBootstrapDatabase(t *testing.T) {
	cfg := &DatabaseConfig{DBName: "customdb"}
	targetDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer targetDB.Close()

	var opened []string
	openDatabase := func(_ *DatabaseConfig, dbName string) (*sql.DB, error) {
		opened = append(opened, dbName)
		return targetDB, nil
	}

	if err := testDatabaseConnection(cfg, openDatabase); err != nil {
		t.Fatalf("testDatabaseConnection() error = %v", err)
	}
	if len(opened) != 1 || opened[0] != cfg.DBName {
		t.Fatalf("opened databases = %v, want only configured target %q", opened, cfg.DBName)
	}
}

func TestDatabaseConnectionUsesLegacyBootstrapOnlyForMissingTarget(t *testing.T) {
	cfg := &DatabaseConfig{DBName: "customdb"}
	bootstrapDB, bootstrapMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() bootstrap error = %v", err)
	}
	defer bootstrapDB.Close()
	targetDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() target error = %v", err)
	}
	defer targetDB.Close()

	bootstrapMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
		WithArgs(cfg.DBName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	bootstrapMock.ExpectExec(`CREATE DATABASE customdb`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var opened []string
	targetAttempts := 0
	openDatabase := func(_ *DatabaseConfig, dbName string) (*sql.DB, error) {
		opened = append(opened, dbName)
		switch dbName {
		case cfg.DBName:
			targetAttempts++
			if targetAttempts == 1 {
				return nil, &pq.Error{Code: "3D000"}
			}
			return targetDB, nil
		case postgresBootstrapDatabase:
			return bootstrapDB, nil
		default:
			return nil, fmt.Errorf("unexpected database %q", dbName)
		}
	}

	if err := testDatabaseConnection(cfg, openDatabase); err != nil {
		t.Fatalf("testDatabaseConnection() error = %v", err)
	}
	if err := bootstrapMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("bootstrap database expectations: %v", err)
	}
	wantOpened := []string{cfg.DBName, postgresBootstrapDatabase, cfg.DBName}
	if len(opened) != len(wantOpened) {
		t.Fatalf("opened databases = %v, want %v", opened, wantOpened)
	}
	for i := range wantOpened {
		if opened[i] != wantOpened[i] {
			t.Fatalf("opened databases = %v, want %v", opened, wantOpened)
		}
	}
}

func TestDatabaseConnectionDoesNotFallbackForTargetAuthenticationError(t *testing.T) {
	cfg := &DatabaseConfig{DBName: "customdb"}
	var opened []string
	openDatabase := func(_ *DatabaseConfig, dbName string) (*sql.DB, error) {
		opened = append(opened, dbName)
		return nil, &pq.Error{Code: "28P01"}
	}

	if err := testDatabaseConnection(cfg, openDatabase); err == nil {
		t.Fatal("testDatabaseConnection() error = nil, want authentication error")
	}
	if len(opened) != 1 || opened[0] != cfg.DBName {
		t.Fatalf("opened databases = %v, want only configured target %q", opened, cfg.DBName)
	}
}
