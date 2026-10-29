package setup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	"gopkg.in/yaml.v3"
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

func TestWriteConfigFileOmitsObsoleteRateLimit(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{
		Redis:    RedisConfig{Host: "redis", Port: 6379, Username: "app-user"},
		Timezone: "UTC",
	}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("generated config is invalid YAML: %v", err)
	}
	if rateLimit, exists := config["rate_limit"]; exists {
		fields, ok := rateLimit.(map[string]any)
		if !ok {
			t.Fatalf("generated rate_limit must be a mapping: %#v", rateLimit)
		}
		for _, key := range []string{"requests_per_minute", "burst_size"} {
			if _, present := fields[key]; present {
				t.Fatalf("generated config contains obsolete rate_limit.%s: %s", key, data)
			}
		}
	}
	redis, ok := config["redis"].(map[string]any)
	if !ok || redis["host"] != "redis" || redis["port"] != 6379 || redis["username"] != "app-user" {
		t.Fatalf("generated config lost Redis settings: %#v", config["redis"])
	}
	if config["timezone"] != "UTC" {
		t.Fatalf("generated config timezone = %#v, want UTC", config["timezone"])
	}
	defaults, ok := config["default"].(map[string]any)
	if !ok || defaults["user_concurrency"] != defaultUserConcurrency || defaults["api_key_prefix"] != "sk-" {
		t.Fatalf("generated config lost default settings: %#v", config["default"])
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
	defer func() { _ = targetDB.Close() }()

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
	defer func() { _ = bootstrapDB.Close() }()
	targetDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() target error = %v", err)
	}
	defer func() { _ = targetDB.Close() }()

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

func TestPrepareAdminCredentialsGeneratesMissingValues(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "  ", Password: ""}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if !emailGenerated || !passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (true, true)", emailGenerated, passwordGenerated)
	}
	if !regexp.MustCompile(`^admin-[0-9a-f]{12}@sub2api\.local$`).MatchString(admin.Email) {
		t.Fatalf("generated email = %q, want admin-<12 hex>@sub2api.local", admin.Email)
	}
	// 生成的邮箱必须能通过登录接口的 binding:"required,email" 校验。
	loginReq := struct {
		Email string `binding:"required,email"`
	}{Email: admin.Email}
	if err := binding.Validator.ValidateStruct(&loginReq); err != nil {
		t.Fatalf("generated email %q rejected by login validator: %v", admin.Email, err)
	}
	if len(admin.Password) != 32 {
		t.Fatalf("generated password length = %d, want 32", len(admin.Password))
	}

	other := AdminConfig{}
	if _, _, err := prepareAdminCredentials(&other); err != nil {
		t.Fatalf("prepareAdminCredentials() second call error = %v", err)
	}
	if other.Email == admin.Email {
		t.Fatalf("generated emails should be random, got %q twice", admin.Email)
	}
}

func TestPrepareAdminCredentialsKeepsProvidedValues(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "owner@example.com", Password: "a-strong-password"}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if emailGenerated || passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (false, false)", emailGenerated, passwordGenerated)
	}
	if admin.Email != "owner@example.com" || admin.Password != "a-strong-password" {
		t.Fatalf("provided credentials were modified: %+v", admin)
	}
}

func TestPrepareAdminCredentialsRejectsWeakPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
	}{
		{name: "too short", password: "123456"},
		{name: "seven chars", password: "1234567"},
		{name: "exceeds bcrypt limit", password: strings.Repeat("a", 73)},
		{name: "former 128 limit", password: strings.Repeat("a", 128)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			admin := AdminConfig{Email: "owner@example.com", Password: tt.password}
			_, _, err := prepareAdminCredentials(&admin)
			if err == nil || !strings.Contains(err.Error(), "invalid admin password") {
				t.Fatalf("prepareAdminCredentials(%q) error = %v, want invalid admin password error", tt.password, err)
			}
		})
	}
}

func TestPrepareAdminCredentialsRejectsUnloginableEmail(t *testing.T) {
	t.Parallel()

	for _, email := range []string{"admin", "a@b", "Owner <owner@example.com>", "<owner@example.com>"} {
		t.Run(email, func(t *testing.T) {
			t.Parallel()

			admin := AdminConfig{Email: email, Password: "a-strong-password"}
			_, _, err := prepareAdminCredentials(&admin)
			if err == nil || !strings.Contains(err.Error(), "invalid admin email") {
				t.Fatalf("prepareAdminCredentials(%q) error = %v, want invalid admin email error", email, err)
			}
		})
	}
}

func TestPrepareAdminCredentialsTrimsProvidedEmail(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "  owner@example.com\n", Password: "a-strong-password"}
	if _, _, err := prepareAdminCredentials(&admin); err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if admin.Email != "owner@example.com" {
		t.Fatalf("email = %q, want trimmed owner@example.com", admin.Email)
	}
}

func TestPrepareAdminCredentialsAcceptsBcryptMaxLengthPassword(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "owner@example.com", Password: strings.Repeat("a", 72)}
	if _, _, err := prepareAdminCredentials(&admin); err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	// 校验上限必须与 bcrypt 实际可哈希的上限一致，否则通过校验后仍会在建号时失败。
	user := service.User{}
	if err := user.SetPassword(admin.Password); err != nil {
		t.Fatalf("SetPassword() with max-length password error = %v", err)
	}
}

func expectAdminBootstrapCounts(mock sqlmock.Sqlmock, totalUsers, adminUsers int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1) FROM users")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(totalUsers))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1) FROM users WHERE role = $1")).
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(adminUsers))
}

func TestBootstrapAdminUserSkipsValidationWhenNotCreating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		reason     string
	}{
		{name: "admin exists", totalUsers: 3, adminUsers: 1, reason: adminBootstrapReasonAdminExists},
		{name: "users exist without admin", totalUsers: 3, adminUsers: 0, reason: adminBootstrapReasonUsersExistWithoutAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New() error = %v", err)
			}
			defer func() { _ = db.Close() }()
			expectAdminBootstrapCounts(mock, tt.totalUsers, tt.adminUsers)

			// 已有部署里遗留的弱密码/非法邮箱不能阻断启动。
			cfg := &SetupConfig{Admin: AdminConfig{Email: "admin", Password: "123456"}}
			created, reason, err := bootstrapAdminUser(context.Background(), db, cfg)
			if err != nil || created || reason != tt.reason {
				t.Fatalf("bootstrapAdminUser() = (%v, %q, %v), want (false, %q, nil)", created, reason, err, tt.reason)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected database interaction: %v", err)
			}
		})
	}
}

func TestBootstrapAdminUserRejectsWeakPasswordWithoutInsert(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	expectAdminBootstrapCounts(mock, 0, 0)

	cfg := &SetupConfig{Admin: AdminConfig{Password: "123456"}}
	created, _, err := bootstrapAdminUser(context.Background(), db, cfg)
	if err == nil || created || !strings.Contains(err.Error(), "invalid admin password") {
		t.Fatalf("bootstrapAdminUser() = (%v, %v), want invalid admin password error", created, err)
	}
	// 未设置 INSERT 期望：若发生插入，sqlmock 会返回非预期调用错误，上面的错误断言即失败。
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database interaction: %v", err)
	}
}

func TestBootstrapAdminUserCreatesAdminWithGeneratedCredentials(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	expectAdminBootstrapCounts(mock, 0, 0)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), service.RoleAdmin, sqlmock.AnyArg(),
			sqlmock.AnyArg(), service.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &SetupConfig{}
	created, reason, err := bootstrapAdminUser(context.Background(), db, cfg)
	if err != nil || !created || reason != adminBootstrapReasonEmptyDatabase {
		t.Fatalf("bootstrapAdminUser() = (%v, %q, %v), want (true, %q, nil)", created, reason, err, adminBootstrapReasonEmptyDatabase)
	}
	if !regexp.MustCompile(`^admin-[0-9a-f]{12}@sub2api\.local$`).MatchString(cfg.Admin.Email) {
		t.Fatalf("admin email = %q, want generated admin-<12 hex>@sub2api.local", cfg.Admin.Email)
	}
	if len(cfg.Admin.Password) != 32 {
		t.Fatalf("admin password length = %d, want generated 32", len(cfg.Admin.Password))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations not met: %v", err)
	}
}

func TestPrepareAdminCredentialsAcceptsMinimumLengthPassword(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Password: "12345678"}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if !emailGenerated || passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (true, false)", emailGenerated, passwordGenerated)
	}
	if admin.Password != "12345678" {
		t.Fatalf("password = %q, want unchanged", admin.Password)
	}
}
