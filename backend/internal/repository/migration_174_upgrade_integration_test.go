//go:build integration

package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestMigration174_ExistingUsersReceiveZeroExtraConcurrencyWithoutLosingData(t *testing.T) {
	ctx := context.Background()
	db := newMigrationUpgradeTestDB(t)

	require.NoError(t, applyMigrationsFS(ctx, db, migrationFilesThrough(t, "173_allow_cyber_blocked_usage_request_type.sql")))
	require.True(t, migrationIsApplied(t, db, "173_allow_cyber_blocked_usage_request_type.sql"))
	require.False(t, migrationIsApplied(t, db, "174_add_user_extra_concurrency.sql"))

	var extraConcurrencyExists bool
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = 'public'
	  AND table_name = 'users'
	  AND column_name = 'extra_concurrency'
)
`).Scan(&extraConcurrencyExists))
	require.False(t, extraConcurrencyExists)

	const (
		email        = "existing-before-174@example.com"
		passwordHash = "existing-password-hash"
		role         = "admin"
		status       = "disabled"
		balance      = "42.12500000"
		concurrency  = 17
		notes        = "preserve this existing user"
	)

	var userID int64
	require.NoError(t, db.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id
`, email, passwordHash, role, status, balance, concurrency, notes).Scan(&userID))

	require.NoError(t, applyMigrationsFS(ctx, db, migrationFileOnly(t, "174_add_user_extra_concurrency.sql")))

	var actual struct {
		Email            string
		PasswordHash     string
		Role             string
		Status           string
		Balance          string
		Concurrency      int
		Notes            string
		ExtraConcurrency int
	}
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT email, password_hash, role, status, balance::text, concurrency, notes, extra_concurrency
FROM users
WHERE id = $1
`, userID).Scan(
		&actual.Email,
		&actual.PasswordHash,
		&actual.Role,
		&actual.Status,
		&actual.Balance,
		&actual.Concurrency,
		&actual.Notes,
		&actual.ExtraConcurrency,
	))

	require.Equal(t, email, actual.Email)
	require.Equal(t, passwordHash, actual.PasswordHash)
	require.Equal(t, role, actual.Role)
	require.Equal(t, status, actual.Status)
	require.Equal(t, balance, actual.Balance)
	require.Equal(t, concurrency, actual.Concurrency)
	require.Equal(t, notes, actual.Notes)
	require.Zero(t, actual.ExtraConcurrency)
}

func newMigrationUpgradeTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	options := []testcontainers.ContainerCustomizer{
		tcpostgres.WithDatabase("sub2api_migration_174_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	}
	options = append(options, integrationContainerOptions(postgresTestDataPath)...)
	pgContainer, err := tcpostgres.Run(ctx, selectDockerImage(ctx, postgresImageTag), options...)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(context.Background()))
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)

	db, err := openSQLWithRetry(ctx, dsn, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}

func migrationFilesThrough(t *testing.T, target string) fs.FS {
	t.Helper()

	names, err := fs.Glob(migrationfiles.FS, "*.sql")
	require.NoError(t, err)
	sort.Strings(names)

	selected := fstest.MapFS{}
	found := false
	for _, name := range names {
		content, err := fs.ReadFile(migrationfiles.FS, name)
		require.NoError(t, err)
		selected[name] = &fstest.MapFile{Data: content}
		if name == target {
			found = true
			break
		}
	}

	require.Truef(t, found, "migration %s was not found", target)
	return selected
}

func migrationFileOnly(t *testing.T, name string) fs.FS {
	t.Helper()

	content, err := fs.ReadFile(migrationfiles.FS, name)
	require.NoError(t, err)
	return fstest.MapFS{name: &fstest.MapFile{Data: content}}
}

func migrationIsApplied(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var applied bool
	require.NoError(t, db.QueryRowContext(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM schema_migrations
	WHERE filename = $1
)
`, name).Scan(&applied))
	return applied
}
