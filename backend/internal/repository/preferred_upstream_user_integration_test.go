//go:build integration

package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestPreferredUpstreamUserPersistence(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	accountRepo := newAccountRepositoryWithSQL(client, tx, nil)
	userRepo := newUserRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "preferred-upstream", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
	})
	stale, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, stale.PreferredUserID)

	require.NoError(t, accountRepo.UpdateWithAccountBillingSettings(ctx, account, nil, nil, nil, &user.ID))
	require.Equal(t, &user.ID, account.PreferredUserID)

	stale.Name = "name-only-update"
	require.NoError(t, accountRepo.Update(ctx, stale))
	require.Equal(t, &user.ID, stale.PreferredUserID, "an update from an old snapshot must not erase the preference")

	require.NoError(t, userRepo.Delete(ctx, user.ID))
	current, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, current.PreferredUserID)
	require.ErrorIs(t, accountRepo.UpdateWithAccountBillingSettings(ctx, current, nil, nil, nil, &user.ID), service.ErrUserNotFound)
}

func TestPreferredUpstreamUserMigrationUpgrade(t *testing.T) {
	ctx := context.Background()
	const dbName = "sub2api_preferred_user_upgrade"
	_, err := integrationDB.ExecContext(ctx, "CREATE DATABASE "+dbName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, "DROP DATABASE "+dbName)
		require.NoError(t, err)
	})

	dsn, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	dsn.Path = "/" + dbName
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Apply the actual preceding migration set, then write using only old columns.
	files, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	prior := fstest.MapFS{}
	for _, name := range files {
		if strings.Compare(name, "251_preferred_upstream_user.sql") >= 0 {
			continue
		}
		data, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		prior[name] = &fstest.MapFile{Data: data}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, prior))
	var userID, groupID, accountID int64
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO users (email, password_hash) VALUES ('upgrade@example.test', 'unused') RETURNING id").Scan(&userID))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO groups (name, platform) VALUES ('upgrade-group', 'openai') RETURNING id").Scan(&groupID))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO accounts (name, platform, type) VALUES ('upgrade-account', 'openai', 'oauth') RETURNING id").Scan(&accountID))
	_, err = db.ExecContext(ctx, "INSERT INTO account_groups (account_id, group_id) VALUES ($1, $2)", accountID, groupID)
	require.NoError(t, err)

	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, ApplyMigrations(ctx, db), "restarting with the new schema should skip applied migrations")
	require.NoError(t, applyMigrationsFS(ctx, db, prior), "the preceding migration set should accept the upgraded schema")

	var name, email string
	var preferredUser sql.NullInt64
	var enabled bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT name, preferred_user_id FROM accounts WHERE id = $1", accountID).Scan(&name, &preferredUser))
	require.Equal(t, "upgrade-account", name)
	require.False(t, preferredUser.Valid)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT prefer_user_accounts FROM groups WHERE id = $1", groupID).Scan(&enabled))
	require.False(t, enabled)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT email FROM users WHERE id = $1", userID).Scan(&email))
	require.Equal(t, "upgrade@example.test", email)
	var boundGroupID int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT group_id FROM account_groups WHERE account_id = $1", accountID).Scan(&boundGroupID))
	require.Equal(t, groupID, boundGroupID)

	// An older writer still uses the same columns after upgrade.
	_, err = db.ExecContext(ctx, "UPDATE accounts SET name = 'legacy-edit' WHERE id = $1", accountID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO accounts (name, platform, type) VALUES ('legacy-insert', 'openai', 'oauth')")
	require.NoError(t, err)

	var valid bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT convalidated FROM pg_constraint WHERE conname = 'fk_accounts_preferred_user_id' AND conrelid = 'accounts'::regclass").Scan(&valid))
	require.True(t, valid)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT indisvalid FROM pg_index WHERE indexrelid = 'idx_accounts_preferred_user_id'::regclass").Scan(&valid))
	require.True(t, valid)
}
